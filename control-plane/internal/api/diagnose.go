package api

// 智能诊断端点：触发 headless-agent 对某事故做 AI 诊断，结果以
// Hypothesis 节点写回（source=headless-diagnoser），会议室自动可见。
//
// 实现：Go 子进程调用 Python 诊断脚本（P0 务实做法；生产形态为
// headless-agent 独立部署走「发言人」协议，此处为开发期联调通道）。

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// handleDiagnose POST /api/v1/incidents/{id}/diagnose
// 触发一次 LLM 智能诊断（最长 3 分钟）。只读分析 + 写假设节点。
func (s *Service) handleDiagnose(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if _, err := s.store.GetIncident(r.Context(), incidentID); err != nil {
		mapError(w, err)
		return
	}

	script := os.Getenv("OPSHIVE_DIAGNOSE_SCRIPT")
	if script == "" {
		script = "../headless-agent/diagnose.py"
	}
	cpBase := "http://" + r.Host

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", script,
		"--index", "0", "--push", incidentID, "--cp-base", cpBase)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeError(w, http.StatusBadGateway,
			fmt.Sprintf("diagnose failed: %v (hint: set OPSHIVE_DIAGNOSE_SCRIPT, ensure headless-agent deps)", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"script": script,
		"tail":   string(out)[:min(len(out), 1500)],
	})
}
