package api

import (
	"errors"
	"net/http"

	"github.com/ops-hive/control-plane/internal/evidence"
	"github.com/ops-hive/control-plane/internal/iam"
	"github.com/ops-hive/control-plane/internal/lease"
)

// mapError 将存储层错误映射为 HTTP 状态码。
func mapError(w http.ResponseWriter, err error) {
	var nf *iam.NotFoundError
	if errors.As(err, &nf) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	var cf *iam.ConflictError
	if errors.As(err, &cf) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, lease.ErrNotActive) {
		writeError(w, http.StatusGone, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// ---- Incident ----

func (s *Service) handleCreateIncident(w http.ResponseWriter, r *http.Request) {
	var req createIncidentRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	now := timeNow()
	fp := req.Fingerprint
	if fp == "" {
		fp = iam.IncidentFingerprint(req.SymptomSet)
	}
	status := req.Status
	if status == "" {
		status = iam.IncidentOpen
	}
	src := req.Source
	if src == "" {
		src = "api"
	}
	title := req.Title
	if title == "" && len(req.SymptomSet) > 0 {
		title = req.SymptomSet[0]
	}
	sev := req.Severity
	if sev == "" {
		sev = iam.SeverityP2
	}
	// 未显式给时间窗时默认从当前时刻起（避免零值 0001-01-01 污染前端展示）。
	tr := req.TimeRange
	if tr.Start.IsZero() {
		tr.Start = now
	}

	inc := &iam.Incident{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("inc"),
			Type:      iam.NodeIncident,
			Source:    src,
			Timestamp: now,
		},
		Fingerprint: fp,
		Title:       title,
		Severity:    sev,
		Status:      status,
		ICID:        req.ICID,
		TimeRange:   tr,
		AlertIDs:    req.AlertIDs,
		SymptomSet:  req.SymptomSet,
	}
	if err := s.store.CreateIncident(r.Context(), inc); err != nil {
		mapError(w, err)
		return
	}
	s.bumpVersion(inc.ID)
	s.bus.Publish(inc.ID, "incident.created", inc)
	writeJSON(w, http.StatusCreated, inc)
}

func (s *Service) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	incs, err := s.store.ListIncidents(r.Context())
	if err != nil {
		mapError(w, err)
		return
	}
	if incs == nil {
		incs = []*iam.Incident{}
	}
	writeJSON(w, http.StatusOK, incs)
}

func (s *Service) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inc, err := s.store.GetIncident(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inc)
}

// ---- Work graph ----

func (s *Service) handleCreateWorkNode(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if _, err := s.store.GetIncident(r.Context(), incidentID); err != nil {
		mapError(w, err)
		return
	}
	var req createWorkNodeRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	if req.Role == "" {
		req.Role = iam.RoleExplorer
	}
	wn := &iam.WorkNode{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("wn"),
			Type:      iam.NodeWorkNode,
			Source:    defaultSource(req.Source),
			Timestamp: timeNow(),
		},
		Question:         req.Question,
		Scope:            req.Scope,
		ExpectedEvidence: req.ExpectedEvidence,
		Cost:             req.Cost,
		Deadline:         req.Deadline,
		Assignee:         req.Assignee,
		Role:             req.Role,
		DependsOn:        req.DependsOn,
		Status:           iam.WorkNodeOpen,
	}
	if err := s.store.AddWorkNode(r.Context(), incidentID, wn); err != nil {
		mapError(w, err)
		return
	}
	_ = s.addEdge(r.Context(), incidentID, iam.EdgeBelongsTo, wn.ID, incidentID)
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "work_node.created", wn)
	writeJSON(w, http.StatusCreated, wn)
}

func (s *Service) handleListWorkNodes(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	wns, err := s.store.ListWorkNodes(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	if wns == nil {
		wns = []*iam.WorkNode{}
	}
	writeJSON(w, http.StatusOK, wns)
}

// ---- Advisory lease ----

func (s *Service) handleCreateLease(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	var req createLeaseRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	if req.Assignee == "" {
		writeError(w, http.StatusBadRequest, "assignee is required")
		return
	}
	// 若关联工作单元，先确认存在。
	if req.WorkNodeID != "" {
		if _, err := s.store.GetWorkNode(r.Context(), incidentID, req.WorkNodeID); err != nil {
			mapError(w, err)
			return
		}
	}
	l, err := s.leases.Claim(r.Context(), incidentID, req.WorkNodeID, req.Assignee, req.Role)
	if err != nil {
		mapError(w, err)
		return
	}
	// 把租约挂到工作单元上。
	if req.WorkNodeID != "" {
		if wn, err := s.store.GetWorkNode(r.Context(), incidentID, req.WorkNodeID); err == nil {
			wn.LeaseID = l.ID
			wn.Status = iam.WorkNodeActive
			wn.Assignee = req.Assignee
			_ = s.store.UpdateWorkNode(r.Context(), incidentID, wn)
		}
	}
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "lease.claimed", l)
	writeJSON(w, http.StatusCreated, l)
}

func (s *Service) handleLeaseHeartbeat(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	lid := r.PathValue("lid")
	if err := s.leases.Heartbeat(r.Context(), incidentID, lid); err != nil {
		mapError(w, err)
		return
	}
	l, _ := s.leases.Get(r.Context(), lid)
	s.bus.Publish(incidentID, "lease.heartbeat", l)
	writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func (s *Service) handleReleaseLease(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	lid := r.PathValue("lid")
	if err := s.leases.Release(r.Context(), incidentID, lid); err != nil {
		mapError(w, err)
		return
	}
	if l, err := s.leases.Get(r.Context(), lid); err == nil && l.WorkNodeID != "" {
		if wn, err := s.store.GetWorkNode(r.Context(), incidentID, l.WorkNodeID); err == nil {
			wn.LeaseID = ""
			wn.Status = iam.WorkNodeOpen
			_ = s.store.UpdateWorkNode(r.Context(), incidentID, wn)
		}
	}
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "lease.released", map[string]string{"lease_id": lid})
	w.WriteHeader(http.StatusNoContent)
}

// ---- Operations (single-flight) ----

func (s *Service) handleRegisterOperation(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if _, err := s.store.GetIncident(r.Context(), incidentID); err != nil {
		mapError(w, err)
		return
	}
	var req registerOperationRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	res, err := s.coord.Register(r.Context(), incidentID, req.Query, defaultSource(req.Source), req.WorkNodeID)
	if err != nil {
		mapError(w, err)
		return
	}
	_ = s.addEdge(r.Context(), incidentID, iam.EdgeBelongsTo, res.Operation.ID, incidentID)
	if res.Operation.WorkNodeID != "" {
		_ = s.addEdge(r.Context(), incidentID, iam.EdgeExecutes, res.Operation.WorkNodeID, res.Operation.ID)
	}
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "operation.registered", res)

	resp := registerOperationResponse{
		Operation:         res.Operation,
		DedupStatus:       res.Status,
		ReusedOperationID: res.ReusedOperationID,
		ReusedEvidenceID:  res.ReusedEvidenceID,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- Evidence ----

func (s *Service) handlePushEvidence(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if _, err := s.store.GetIncident(r.Context(), incidentID); err != nil {
		mapError(w, err)
		return
	}
	var req pushEvidenceRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	if req.OperationID == "" {
		writeError(w, http.StatusBadRequest, "operation_id is required")
		return
	}
	if req.DataSource == "" {
		writeError(w, http.StatusBadRequest, "data_source is required")
		return
	}
	op, err := s.store.GetOperation(r.Context(), incidentID, req.OperationID)
	if err != nil {
		mapError(w, err)
		return
	}

	existing, err := s.store.ListEvidence(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}

	ev := &iam.Evidence{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("ev"),
			Type:      iam.NodeEvidence,
			Source:    defaultSource(req.Source),
			Timestamp: timeNow(),
		},
		OperationID:      req.OperationID,
		LineageDAG:       req.LineageDAG,
		DataSource:       req.DataSource,
		PermissionDomain: req.PermissionDomain,
		CollectionChain:  req.CollectionChain,
		Result:           req.Result,
		Conclusion:       req.Conclusion,
	}

	// 血缘 DAG 校验（父证据必须存在且无环）。
	if err := evidence.ValidateDAG(existing, ev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 独立性评分（按数据源/权限域，不按 agent 数）。
	ev.IndependenceScore = evidence.IndependenceForNew(existing, ev)

	// 晚到结果标记 stale：关联工作单元租约已过期/释放。
	if op.WorkNodeID != "" {
		if wn, err := s.store.GetWorkNode(r.Context(), incidentID, op.WorkNodeID); err == nil && wn.LeaseID != "" {
			ev.IsStale = !s.leases.IsActive(wn.LeaseID)
		}
	}

	// 入账本（append-only，哈希链）。
	entry, err := s.ledger.Append(r.Context(), incidentID, ev)
	if err != nil {
		mapError(w, err)
		return
	}
	// 挂 proof_trace（指向账本原始记录，可回放）。
	ev.ProofTrace = []iam.ProofTraceEntry{{
		LedgerSeq:  entry.Seq,
		EvidenceID: ev.ID,
		CapturedAt: entry.CapturedAt,
	}}

	if err := s.store.AddEvidence(r.Context(), incidentID, ev); err != nil {
		mapError(w, err)
		return
	}
	// 标记操作完成，供后续同指纹查询复用。
	_ = s.coord.Complete(r.Context(), incidentID, req.OperationID, ev.ID)

	// 血缘边 derived_from。
	for _, parent := range req.LineageDAG {
		_ = s.addEdge(r.Context(), incidentID, iam.EdgeDerivedFrom, ev.ID, parent)
	}
	_ = s.addEdge(r.Context(), incidentID, iam.EdgeBelongsTo, ev.ID, incidentID)

	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "evidence.appended", ev)
	// 返回账本元数据 + 已入库证据（含 proof_trace）。
	writeJSON(w, http.StatusCreated, pushEvidenceResponse{
		Seq:      entry.Seq,
		Hash:     entry.Hash,
		Evidence: ev,
	})
}

func (s *Service) handleListEvidence(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	evs, err := s.store.ListEvidence(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	entries, _ := s.ledger.List(r.Context(), incidentID)
	perKey, total := evidence.ScoreGroup(evs)

	ledgerView := make([]*evidenceLedgerEntry, 0, len(entries))
	for _, e := range entries {
		ledgerView = append(ledgerView, &evidenceLedgerEntry{Seq: e.Seq, Evidence: e.Evidence, Hash: e.Hash})
	}
	if evs == nil {
		evs = []*iam.Evidence{}
	}
	resp := evidenceListResponse{
		IncidentID: incidentID,
		Evidence:   evs,
		Indep:      perKey,
		TotalIndep: total,
		Ledger:     ledgerView,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- Facts / Hypotheses ----

func (s *Service) handlePostFact(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	var req postFactRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	if req.Statement == "" {
		writeError(w, http.StatusBadRequest, "statement is required")
		return
	}
	confirmedBy := req.ConfirmedBy
	if confirmedBy == "" {
		confirmedBy = req.Source
	}
	f := &iam.Fact{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("fact"),
			Type:      iam.NodeFact,
			Source:    defaultSource(req.Source),
			Timestamp: timeNow(),
		},
		Statement:     req.Statement,
		EvidenceChain: req.EvidenceChain,
		ConfirmedBy:   confirmedBy,
		IsConfirmed:   true,
	}
	if err := s.store.AddFact(r.Context(), incidentID, f); err != nil {
		mapError(w, err)
		return
	}
	_ = s.addEdge(r.Context(), incidentID, iam.EdgeBelongsTo, f.ID, incidentID)
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "fact.posted", f)
	writeJSON(w, http.StatusCreated, f)
}

func (s *Service) handlePostHypothesis(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	var req postHypothesisRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	if req.Topic == "" {
		writeError(w, http.StatusBadRequest, "topic is required")
		return
	}
	// 独立性权重：支持证据链的总独立信号（按数据源去重）。
	all, _ := s.store.ListEvidence(r.Context(), incidentID)
	supportEvs := make([]*iam.Evidence, 0, len(req.Supporting))
	for _, eid := range req.Supporting {
		for _, ev := range all {
			if ev.ID == eid {
				supportEvs = append(supportEvs, ev)
				break
			}
		}
	}
	h := &iam.Hypothesis{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("hyp"),
			Type:      iam.NodeHypothesis,
			Source:    defaultSource(req.Source),
			Timestamp: timeNow(),
		},
		Topic:              req.Topic,
		Supporting:         req.Supporting,
		Refuting:           req.Refuting,
		IndependenceWeight: evidence.ChainIndependence(supportEvs),
		Confidence:         req.Confidence,
		Status:             iam.HypothesisProposed,
	}
	if len(req.Refuting) > 0 {
		h.Status = iam.HypothesisRefuted
	} else if len(req.Supporting) > 0 {
		h.Status = iam.HypothesisSupported
	}
	if err := s.store.AddHypothesis(r.Context(), incidentID, h); err != nil {
		mapError(w, err)
		return
	}
	_ = s.addEdge(r.Context(), incidentID, iam.EdgeBelongsTo, h.ID, incidentID)
	for _, eid := range req.Supporting {
		_ = s.addEdge(r.Context(), incidentID, iam.EdgeSupports, eid, h.ID)
	}
	for _, eid := range req.Refuting {
		_ = s.addEdge(r.Context(), incidentID, iam.EdgeRefutes, eid, h.ID)
	}
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "hypothesis.posted", h)
	writeJSON(w, http.StatusCreated, h)
}

// ---- Guidance ----

func (s *Service) handlePostGuidance(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	var req postGuidanceRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	g := &iam.Guidance{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("guide"),
			Type:      iam.NodeGuidance,
			Source:    defaultSource(req.Source),
			Timestamp: timeNow(),
		},
		FromIC:   req.FromIC,
		Text:     req.Text,
		Priority: req.Priority,
	}
	if err := s.store.AddGuidance(r.Context(), incidentID, g); err != nil {
		mapError(w, err)
		return
	}
	_ = s.addEdge(r.Context(), incidentID, iam.EdgeBelongsTo, g.ID, incidentID)
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "guidance.posted", g)
	writeJSON(w, http.StatusCreated, g)
}

// ---- Stats ----

func (s *Service) handleGetStats(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	snap, err := s.stats.Collect(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// ---- List endpoints (契约补齐) ----

// handleListLeases 列出事故下的全部租约。
func (s *Service) handleListLeases(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	leases, err := s.leases.List(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	if leases == nil {
		leases = []*lease.Lease{}
	}
	writeJSON(w, http.StatusOK, leases)
}

// handleListOperations 列出事故下登记的查询操作。
func (s *Service) handleListOperations(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	ops, err := s.store.ListOperations(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	if ops == nil {
		ops = []*iam.Operation{}
	}
	writeJSON(w, http.StatusOK, ops)
}

// handleListFacts 列出事故下已确认事实。
func (s *Service) handleListFacts(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	facts, err := s.store.ListFacts(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	if facts == nil {
		facts = []*iam.Fact{}
	}
	writeJSON(w, http.StatusOK, facts)
}

// handleListHypotheses 列出事故下的根因假设。
func (s *Service) handleListHypotheses(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	hyps, err := s.store.ListHypotheses(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	if hyps == nil {
		hyps = []*iam.Hypothesis{}
	}
	writeJSON(w, http.StatusOK, hyps)
}

// handleListGuidance 列出事故下 IC 的发言/决策。
func (s *Service) handleListGuidance(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	guides, err := s.store.ListGuidance(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	if guides == nil {
		guides = []*iam.Guidance{}
	}
	writeJSON(w, http.StatusOK, guides)
}

// handleUpdateWorkNode 调整一个工作单元（PATCH：IC 改 question/scope/成本/负责人/状态）。
func (s *Service) handleUpdateWorkNode(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	nodeID := r.PathValue("nid")
	wn, err := s.store.GetWorkNode(r.Context(), incidentID, nodeID)
	if err != nil {
		mapError(w, err)
		return
	}
	var patch map[string]any
	if err := decodeBody(w, r, &patch); err != nil {
		return
	}
	if v, ok := patch["question"].(string); ok {
		wn.Question = v
	}
	if v, ok := patch["scope"].(string); ok {
		wn.Scope = v
	}
	if v, ok := patch["cost"].(float64); ok {
		wn.Cost = int(v)
	}
	if v, ok := patch["assignee"].(string); ok {
		wn.Assignee = v
	}
	if v, ok := patch["role"].(string); ok {
		wn.Role = iam.WorkRole(v)
	}
	if v, ok := patch["status"].(string); ok {
		wn.Status = iam.WorkNodeStatus(v)
	}
	if err := s.store.UpdateWorkNode(r.Context(), incidentID, wn); err != nil {
		mapError(w, err)
		return
	}
	s.bumpVersion(incidentID)
	s.bus.Publish(incidentID, "work_node.updated", wn)
	writeJSON(w, http.StatusOK, wn)
}
