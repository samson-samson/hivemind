package api

// 智能诊断端点：触发 headless-agent 对某事故做 AI 诊断，结果以
// Hypothesis 节点写回（source=headless-diagnoser），会议室自动可见。
//
// 实现：Go 子进程调用 Python 诊断脚本（P0 务实做法；生产形态为
// headless-agent 独立部署走「发言人」协议，此处为开发期联调通道）。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// triggerDiagnose 触发一次 LLM 智能诊断（后台，最长 3 分钟）。
// 只读分析 + 写假设节点。供 handleDiagnose 与 ingest auto_diagnose 复用。
//
// 关键修复：诊断目标 = 被触发的事故本身（按该事故的 title/symptom
// 过滤采集），而不是 SLS 最新事故（避免"串房"误导决策）。
func (s *Service) triggerDiagnose(ctx context.Context, incidentID string) (string, error) {
	inc, err := s.store.GetIncident(ctx, incidentID)
	if err != nil {
		return "", err
	}

	script := os.Getenv("OPSHIVE_DIAGNOSE_SCRIPT")
	if script == "" {
		script = "../headless-agent/diagnose.py"
	}
	// cp-base 用可信配置（拒绝用不可信 Host 头构造，防回连）。
	cpBase := os.Getenv("OPSHIVE_PUBLIC_BASE")
	if cpBase == "" {
		cpBase = "http://localhost:8081"
	}

	// 把被触发事故的语义传给 Python：诊断按其 title/症状过滤采集。
	incidentJSON, _ := json.Marshal(map[string]any{
		"title":       inc.Title,
		"symptom_set": inc.SymptomSet,
		"id":          inc.ID,
		"fingerprint": inc.Fingerprint,
	})

	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", script,
		"--push", incidentID, "--cp-base", cpBase,
		"--incident-json", string(incidentJSON))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("diagnose failed: %w (hint: set OPSHIVE_DIAGNOSE_SCRIPT, ensure headless-agent deps)", err)
	}
	return string(out)[:min(len(out), 1500)], nil
}

// handleDiagnose POST /api/v1/incidents/{id}/diagnose
func (s *Service) handleDiagnose(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	tail, err := s.triggerDiagnose(r.Context(), incidentID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"script":   os.Getenv("OPSHIVE_DIAGNOSE_SCRIPT"),
		"incident": incidentID,
		"tail":     tail,
	})
}
