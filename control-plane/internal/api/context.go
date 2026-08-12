package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ops-hive/control-plane/internal/iam"
)

// handleGetContext 返回 context 上下文包（版本号取 query 参数 v，缺省最新）。
// 也兼容路径形式 /context@v{version}（见 handleGetContextVersioned）。
func (s *Service) handleGetContext(w http.ResponseWriter, r *http.Request) {
	version := 0
	if v := r.URL.Query().Get("v"); v != "" {
		version, _ = strconv.Atoi(v)
	}
	s.serveContext(w, r, version)
}

// handleContextPathAlias 处理 /incidents/{id}/context@vN 路径形式，
// 其余未知子路径返回 404。
func (s *Service) handleContextPathAlias(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	if strings.HasPrefix(rest, "context@v") {
		verStr := strings.TrimPrefix(rest, "context@v")
		version, err := strconv.Atoi(verStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid context version")
			return
		}
		s.serveContext(w, r, version)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Service) serveContext(w http.ResponseWriter, r *http.Request, version int) {
	incidentID := r.PathValue("id")
	inc, err := s.store.GetIncident(r.Context(), incidentID)
	if err != nil {
		mapError(w, err)
		return
	}
	cur := s.contextVersion(incidentID)
	if version > cur {
		version = cur
	}

	wns, _ := s.store.ListWorkNodes(r.Context(), incidentID)
	ops, _ := s.store.ListOperations(r.Context(), incidentID)
	evs, _ := s.store.ListEvidence(r.Context(), incidentID)
	facts, _ := s.store.ListFacts(r.Context(), incidentID)
	hyps, _ := s.store.ListHypotheses(r.Context(), incidentID)
	gds, _ := s.store.ListGuidance(r.Context(), incidentID)

	pkg := &ContextPackage{
		IncidentID:  incidentID,
		Version:     cur,
		Incident:    inc,
		WorkNodes:   wns,
		Operations:  ops,
		Evidence:    evs,
		Facts:       facts,
		Hypotheses:  hyps,
		Guidance:    gds,
		RefutedList: make([]string, 0),
	}
	for _, h := range hyps {
		if h.Status == "refuted" {
			pkg.RefutedList = append(pkg.RefutedList, h.Topic)
		}
	}
	if pkg.WorkNodes == nil {
		pkg.WorkNodes = []*iam.WorkNode{}
	}
	if pkg.Operations == nil {
		pkg.Operations = []*iam.Operation{}
	}
	if pkg.Evidence == nil {
		pkg.Evidence = []*iam.Evidence{}
	}
	writeJSON(w, http.StatusOK, pkg)
}
