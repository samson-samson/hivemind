package api

// 恢复闭环最小版（§5.3）：typed action + IC 审批 + 护栏执行。
//
//   POST /api/v1/incidents/{id}/actions          IC 创建动作（证据门控+护栏）
//   POST /api/v1/incidents/{id}/actions/{aid}/execute   执行（默认 dry-run）
//
// 安全模型：
//   - typed action 由环境变量 HIVEMIND_ACTIONS 定义（JSON 数组）：
//     [{"type":"restart-worker","description":"重启推理 worker",
//       "command_template":"kubectl rollout restart deploy/{deploy} -n {ns}",
//       "params":{"deploy":"litellm","ns":"goodputai"}}]
//   - 执行前护栏：动作必须匹配已定义类型；默认 dry-run（只展示将执行的
//     命令，不真实执行）；真实执行需 HIVEMIND_ALLOW_EXEC=1。
//   - AI 只定位，IC 决策：动作创建与执行都要求认证用户（IC）。
import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

// typedAction 环境变量定义的强类型动作。
type typedAction struct {
	Type            string            `json:"type"`
	Description     string            `json:"description"`
	CommandTemplate string            `json:"command_template"`
	Params          map[string]string `json:"params"`
}

var actionRegistry []typedAction

func initActionRegistry() {
	actionRegistry = nil
	raw := os.Getenv("HIVEMIND_ACTIONS")
	if strings.TrimSpace(raw) == "" {
		return
	}
	if err := json.Unmarshal([]byte(raw), &actionRegistry); err != nil {
		log.Printf("[actions] invalid HIVEMIND_ACTIONS: %v", err)
	}
}

func findTypedAction(t string) *typedAction {
	for i := range actionRegistry {
		if actionRegistry[i].Type == t {
			return &actionRegistry[i]
		}
	}
	return nil
}

type createActionRequest struct {
	Type     string `json:"type"`
	Rationale string `json:"rationale,omitempty"` // IC 决策理由（证据门控的决策侧）
}

func (s *Service) handleCreateAction(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	ic := authUser(r)
	if ic == "" {
		writeError(w, http.StatusUnauthorized, "actions require IC identity")
		return
	}
	var req createActionRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	ta := findTypedAction(req.Type)
	if ta == nil {
		writeError(w, http.StatusBadRequest,
			"unknown action type; define via HIVEMIND_ACTIONS (typed actions only, 护栏)")
		return
	}
	// 证据门控：事故至少有一个假设才能进入修复流程（防编造根因）。
	hyps, _ := s.store.ListHypotheses(r.Context(), incidentID)
	if len(hyps) == 0 {
		writeError(w, http.StatusBadRequest, "evidence gate: no hypothesis yet — 先定位再止血")
		return
	}
	a := &iam.Action{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("act"),
			Type:      iam.NodeAction,
			Source:    "ic:" + ic,
			Timestamp: timeNow(),
		},
		Type:       req.Type,
		Status:     iam.ActionApproved, // IC 创建即审批（人审）
		ApprovedBy: ic,
		DryRun:     true,
	}
	if err := s.store.AddAction(r.Context(), incidentID, a); err != nil {
		mapError(w, err)
		return
	}
	s.bus.Publish(incidentID, "action.created", a)
	writeJSON(w, http.StatusCreated, a)
}

func (s *Service) handleExecuteAction(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	aid := r.PathValue("aid")
	ic := authUser(r)
	if ic == "" {
		writeError(w, http.StatusUnauthorized, "execute requires IC identity")
		return
	}
	acts, _ := s.store.ListActions(r.Context(), incidentID)
	var target *iam.Action
	for _, a := range acts {
		if a.ID == aid {
			target = a
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "action not found")
		return
	}
	if target.Status != iam.ActionApproved {
		writeError(w, http.StatusConflict, "action not approved")
		return
	}
	ta := findTypedAction(target.Type)
	if ta == nil {
		writeError(w, http.StatusBadRequest, "action type no longer defined")
		return
	}

	// 渲染命令模板（参数来自 env 定义，非用户输入——护栏：无注入面）。
	cmdStr := ta.CommandTemplate
	for k, v := range ta.Params {
		cmdStr = strings.ReplaceAll(cmdStr, "{"+k+"}", v)
	}

	// 护栏：默认 dry-run（只展示，不执行）；真实执行需显式开关。
	if os.Getenv("HIVEMIND_ALLOW_EXEC") != "1" {
		target.Status = iam.ActionExecuted
		target.Result = "DRY-RUN: 未真实执行（HIVEMIND_ALLOW_EXEC!=1）。将执行: " + cmdStr
		_ = s.store.UpdateAction(r.Context(), target)
		writeJSON(w, http.StatusOK, target)
		return
	}
	out, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
	if err != nil {
		target.Status = iam.ActionExecuted
		target.Result = "执行失败: " + err.Error() + " | " + string(out)[:300]
	} else {
		target.Status = iam.ActionExecuted
		target.Result = string(out)[:500]
	}
	_ = s.store.UpdateAction(r.Context(), target)
	s.bus.Publish(incidentID, "action.executed", target)
	writeJSON(w, http.StatusOK, target)
}
