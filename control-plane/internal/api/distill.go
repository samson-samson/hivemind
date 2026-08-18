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
	"sort"
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

// tokenize 症状文本为中英混合词条：英文/数字按小写整词，中文按连续汉字段
// 整段切分；中英文交界（如"GPU利用率"）也切开（不做分词器依赖，够召回匹配用）。
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	curKind := -1
	kindOf := func(r rune) int {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return 0
		case r >= 0x4e00 && r <= 0x9fff:
			return 1
		default:
			return -1
		}
	}
	for _, r := range strings.ToLower(s) {
		k := kindOf(r)
		if k < 0 {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		if cur.Len() > 0 && curKind != k {
			out = append(out, cur.String())
			cur.Reset()
		}
		curKind = k
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// expand 词条集：英文/数字整词直接用；中文段展开为字符 bigram。
// 修复（第二轮）：整段中文一个 token 仍脆弱——"请求超时取消" vs
// "请求超时被取消"只差一个"被"字就完全失配；bigram 展开后二者
// 4/7≈0.57 强重叠，召回对"多一字少一字"鲁棒。
func expand(s string) []string {
	runes := []rune(s)
	hasCJK := false
	for _, r := range runes {
		if r >= 0x4e00 && r <= 0x9fff {
			hasCJK = true
			break
		}
	}
	if !hasCJK || len(runes) < 2 {
		return []string{s}
	}
	out := make([]string, 0, len(runes)-1)
	for i := 0; i+1 < len(runes); i++ {
		out = append(out, string(runes[i:i+2]))
	}
	return out
}

// symptomJaccard 两段症状文本的展开词集 Jaccard 相似度（0~1）。
// 修复：LLM 蒸馏产物是浓缩措辞（"GPU利用率异常"），告警是原始描述
// （"GPU 利用率低于阈值"），字符串包含匹配召回率=0；bigram 展开后
// 词级重叠才是"症状指纹"召回的正确语义。
func symptomJaccard(a, b string) float64 {
	var ea, eb []string
	for _, t := range tokenize(a) {
		ea = append(ea, expand(t)...)
	}
	for _, t := range tokenize(b) {
		eb = append(eb, expand(t)...)
	}
	if len(ea) == 0 || len(eb) == 0 {
		return 0
	}
	sa := map[string]bool{}
	for _, t := range ea {
		sa[t] = true
	}
	inter := 0
	union := map[string]bool{}
	for _, t := range ea {
		union[t] = true
	}
	for _, t := range eb {
		if sa[t] {
			inter++
		}
		union[t] = true
	}
	return float64(inter) / float64(len(union))
}

func (s *Service) handleListKnowledge(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	inc, err := s.store.GetIncident(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	all, _ := s.store.ListAllRunbooks(r.Context())
	// 召回：症状词级 Jaccard 相似度（命中越多分越高）；certified 优先。
	var hits []knowledgeHit
	for _, rb := range all {
		if rb.Status != iam.RunbookCertified && rb.Status != iam.RunbookVerified {
			continue // 只召回验证过的（候选不浮出）
		}
		score := 0.0
		for _, sy := range inc.SymptomSet {
			for _, rs := range rb.Symptoms {
				score += symptomJaccard(sy, rs)
			}
		}
		if score >= 0.15 { // 至少一段症状有词级重叠才召回（防噪声）
			hits = append(hits, knowledgeHit{
				ID: rb.ID, Title: rb.Title, Kind: "runbook",
				Score: score, Certified: rb.Status == iam.RunbookCertified,
				Summary: rb.RootCause,
			})
		}
	}
	// certified 优先，同档按分数降序（认证链信任度排序）。
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Certified != hits[j].Certified {
			return hits[i].Certified
		}
		return hits[i].Score > hits[j].Score
	})
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
