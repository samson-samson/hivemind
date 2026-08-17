package api

// 知识蒸馏（§5.2/5.6 两流水线的"事故后认证"最小版）：
//
//   POST /api/v1/incidents/{id}/runbooks   创建候选 runbook（蒸馏产物）
//   PATCH /api/v1/runbooks/{id}            IC 确认状态（candidate→certified）
//   GET  /api/v1/incidents/{id}/knowledge  按症状指纹召回已有 runbook（经验命中）
//
// 流水线：事故关闭/手动触发 → distiller 收集（症状/确认假设/IC 决策）
// → LLM 生成类型化候选 → IC 确认 → certified（下次同类事故秒级召回）。
// 一次事故成功不能自动 certified（发生过≠可复用）。

import (
	"net/http"
	"strings"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

type createRunbookRequest struct {
	Title               string   `json:"title"`
	Symptoms            []string `json:"symptoms,omitempty"`
	RootCause           string   `json:"root_cause"`
	DiagnosticSteps     []string `json:"diagnostic_steps,omitempty"`
	VerificationActions []string `json:"verification_actions,omitempty"`
	Rollback            string   `json:"rollback,omitempty"`
	SuccessCriteria     string   `json:"success_criteria,omitempty"`
	Source              string   `json:"source,omitempty"`
}

func (s *Service) handleCreateRunbook(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	var req createRunbookRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	if req.Title == "" || req.RootCause == "" {
		writeError(w, http.StatusBadRequest, "title and root_cause are required")
		return
	}
	rb := &iam.Runbook{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("rb"),
			Type:      iam.NodeRunbook,
			Source:    defaultSource(req.Source),
			Timestamp: timeNow(),
		},
		Title:               req.Title,
		Symptoms:            req.Symptoms,
		RootCause:           req.RootCause,
		DiagnosticSteps:     req.DiagnosticSteps,
		VerificationActions: req.VerificationActions,
		Rollback:            req.Rollback,
		SuccessCriteria:     req.SuccessCriteria,
		Status:              iam.RunbookCandidate,
	}
	if err := s.store.AddRunbook(r.Context(), incidentID, rb); err != nil {
		mapError(w, err)
		return
	}
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "runbook.created", rb)
	writeJSON(w, http.StatusCreated, rb)
}

type updateRunbookRequest struct {
	Status string `json:"status"` // candidate|verified|certified|revoked
}

func (s *Service) handleUpdateRunbook(w http.ResponseWriter, r *http.Request) {
	rid := r.PathValue("rid")
	// 全局查找 runbook（P0 内存实现；按事故查询的接口在 ListRunbooks）
	all, _ := s.store.ListAllRunbooks(r.Context())
	var target *iam.Runbook
	for _, rb := range all {
		if rb.ID == rid {
			target = rb
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "runbook not found")
		return
	}
	var req updateRunbookRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	switch iam.RunbookStatus(req.Status) {
	case iam.RunbookVerified, iam.RunbookCertified, iam.RunbookRevoked:
		// 认证流水线：仅 IC（认证用户）可确认 certified。
		if req.Status == string(iam.RunbookCertified) && authUser(r) == "" {
			writeError(w, http.StatusUnauthorized, "certify requires IC identity")
			return
		}
		target.Status = iam.RunbookStatus(req.Status)
	default:
		writeError(w, http.StatusBadRequest, "invalid status: "+req.Status)
		return
	}
	if err := s.store.UpdateRunbook(r.Context(), target); err != nil {
		mapError(w, err)
		return
	}
	s.bus.Publish(target.IncidentID, "runbook.updated", target)
	writeJSON(w, http.StatusOK, target)
}

// knowledgeHit 前端 KnowledgeHit 形状（P0 最小：症状关键词匹配）。
type knowledgeHit struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Score     float64 `json:"score"`
	Certified bool   `json:"certified"`
	Summary   string `json:"summary"`
}

func (s *Service) handleListKnowledge(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	inc, err := s.store.GetIncident(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	all, _ := s.store.ListAllRunbooks(r.Context())
	// 召回：症状关键词重叠（命中越多分越高）；certified 优先。
	var hits []knowledgeHit
	for _, rb := range all {
		if rb.Status != iam.RunbookCertified && rb.Status != iam.RunbookVerified {
			continue // 只召回验证过的（候选不浮出）
		}
		score := 0.0
		for _, sy := range inc.SymptomSet {
			for _, rs := range rb.Symptoms {
				if sy != "" && rs != "" && (strings.Contains(sy, rs) || strings.Contains(rs, sy)) {
					score += 1
				}
			}
		}
		for _, rs := range rb.Symptoms {
			for _, sy := range inc.SymptomSet {
				if sy != "" && rs != "" && (strings.Contains(rs, sy)) {
					score += 0.5
				}
			}
		}
		if score > 0 {
			hits = append(hits, knowledgeHit{
				ID: rb.ID, Title: rb.Title, Kind: "runbook",
				Score: score, Certified: rb.Status == iam.RunbookCertified,
				Summary: rb.RootCause,
			})
		}
	}
	if hits == nil {
		hits = []knowledgeHit{}
	}
	writeJSON(w, http.StatusOK, hits)
}

func (s *Service) handleListRunbooks(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	rbs, err := s.store.ListRunbooks(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	if rbs == nil {
		rbs = []*iam.Runbook{}
	}
	writeJSON(w, http.StatusOK, rbs)
}
