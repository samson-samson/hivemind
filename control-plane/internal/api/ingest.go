package api

// 告警接入端点：每个告警自动开"独立会议室"（事故）。
//
// 语义：按告警指纹（alarmGroup.fp / 规范化签名）聚合——
//   同指纹告警 → 归入已有事故（不重复开房，追加症状）
//   新指纹告警 → 新建独立事故（新会议室）
// 可选 auto_diagnose=true 时触发 headless-diagnoser 进场诊断。

import (
	"net/http"
	"strings"

	"github.com/ops-hive/control-plane/internal/iam"
)

type ingestAlertRequest struct {
	Fingerprint string   `json:"fp,omitempty"`            // 告警指纹（同指纹 = 同一故障）
	AlarmName   string   `json:"alarm_name"`              // 告警名
	Message     string   `json:"message,omitempty"`       // 告警描述
	Severity    string   `json:"severity,omitempty"`      // P1/P2/P3，缺省 P2
	Cluster     string   `json:"cluster,omitempty"`       // 集群
	EntityIDs   []string `json:"entities,omitempty"`      // 受影响实体
	AutoDiagnose bool   `json:"auto_diagnose,omitempty"`  // 是否触发 AI 诊断
	Source      string   `json:"source,omitempty"`        // 来源（alertmanager / cms / …）
}

type ingestAlertResponse struct {
	IncidentID string `json:"incident_id"`
	Created    bool   `json:"created"`   // true=新建会议室 / false=归入已有
	Reused     bool   `json:"reused"`    // 兼容字段：created 的反义
	Diagnosed  bool   `json:"diagnosed"` // auto_diagnose 是否已触发
}

// handleIngestAlert POST /api/v1/ingest/alert
func (s *Service) handleIngestAlert(w http.ResponseWriter, r *http.Request) {
	var req ingestAlertRequest
	if err := decodeBody(w, r, &req); err != nil {
		return
	}
	if req.AlarmName == "" {
		writeError(w, http.StatusBadRequest, "alarm_name is required")
		return
	}

	// 指纹：优先用告警自带 fp；缺省用 alarm_name+cluster 规范化签名。
	fp := req.Fingerprint
	if fp == "" {
		parts := []string{req.AlarmName}
		if req.Cluster != "" {
			parts = append(parts, req.Cluster)
		}
		fp = iam.IncidentFingerprint(parts)
	}

	now := timeNow()
	sev := iam.Severity(req.Severity)
	if sev == "" {
		sev = iam.SeverityP2
	}
	src := req.Source
	if src == "" {
		src = "alertmanager"
	}

	title := req.AlarmName
	symptomSet := []string{title}
	if req.Message != "" {
		symptomSet = append(symptomSet, req.Message)
	}

	// 同指纹 → 归入已有事故（同一故障同一间会议室）。
	existing, err := s.store.GetIncidentByFingerprint(r.Context(), fp)
	if err == nil {
		s.bus.Publish(existing.ID, "incident.updated", map[string]string{"fingerprint": fp})
		writeJSON(w, http.StatusOK, ingestAlertResponse{
			IncidentID: existing.ID,
			Created:    false,
			Reused:     true,
			Diagnosed:  false,
		})
		return
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
		Status:      iam.IncidentInvestigating,
		ICID:        "",
		TimeRange:   iam.TimeRange{Start: now},
		AlertIDs:    req.EntityIDs,
		SymptomSet:  symptomSet,
	}
	if err := s.store.CreateIncident(r.Context(), inc); err != nil {
		mapError(w, err)
		return
	}
	s.bumpVersion(inc.ID)
	s.bus.Publish(inc.ID, "incident.created", inc)

	resp := ingestAlertResponse{IncidentID: inc.ID, Created: true, Reused: false}

	// 可选：自动触发 AI 诊断（headless-diagnoser 进场）。
	if req.AutoDiagnose {
		dr := r.Clone(r.Context())
		dr.SetPathValue("id", inc.ID)
		s.handleDiagnose(w, dr)
		resp.Diagnosed = true
		// handleDiagnose 已写响应；这里仅标记（实际写发生在上面调用中）。
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// normalizeCluster 提取集群名（用于指纹与展示）。
func normalizeCluster(cluster string) string {
	return strings.TrimSpace(cluster)
}
