package api

import (
	"time"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

// ---- 请求体 ----

type createIncidentRequest struct {
	Fingerprint string             `json:"fingerprint,omitempty"` // 缺省按 symptom_set 生成
	Title       string             `json:"title,omitempty"`       // 缺省由 symptom_set 合成
	Severity    iam.Severity       `json:"severity,omitempty"`    // P1/P2/P3，缺省 P2
	Status      iam.IncidentStatus `json:"status,omitempty"`
	ICID        string             `json:"ic_id"`
	TimeRange   iam.TimeRange      `json:"time_range"`
	AlertIDs    []string           `json:"alert_ids,omitempty"`
	SymptomSet  []string           `json:"symptom_set"`
	Source      string             `json:"source,omitempty"`
}

type createWorkNodeRequest struct {
	Question         string       `json:"question"`
	Scope            string       `json:"scope"`
	ExpectedEvidence []string     `json:"expected_evidence,omitempty"`
	Cost             int          `json:"cost"`
	Deadline         *time.Time   `json:"deadline,omitempty"`
	Assignee         string       `json:"assignee,omitempty"`
	Role             iam.WorkRole `json:"role"`
	DependsOn        []string     `json:"depends_on,omitempty"` // 依赖的工作单元（工作图 DAG 边）
	Source           string       `json:"source,omitempty"`
}

type createLeaseRequest struct {
	WorkNodeID string       `json:"work_node_id"`
	Assignee   string       `json:"assignee"`
	Role       iam.WorkRole `json:"role"`
}

type registerOperationRequest struct {
	Query      iam.QuerySpec `json:"query"`
	Source     string        `json:"source,omitempty"`
	WorkNodeID string        `json:"work_node_id,omitempty"`
}

type pushEvidenceRequest struct {
	OperationID      string   `json:"operation_id"`
	LineageDAG       []string `json:"lineage_dag,omitempty"` // 父证据 ID 列表
	DataSource       string   `json:"data_source"`
	PermissionDomain string   `json:"permission_domain,omitempty"`
	CollectionChain  string   `json:"collection_chain,omitempty"`
	Result           string   `json:"result"`
	Conclusion       string   `json:"conclusion,omitempty"`
	Source           string   `json:"source,omitempty"`
}

type postGuidanceRequest struct {
	FromIC   string `json:"from_ic"`
	Text     string `json:"text"`
	Priority int    `json:"priority,omitempty"`
	Source   string `json:"source,omitempty"`
}

type postFactRequest struct {
	Statement     string   `json:"statement"`
	EvidenceChain []string `json:"evidence_chain,omitempty"`
	ConfirmedBy   string   `json:"confirmed_by,omitempty"`
	Source        string   `json:"source,omitempty"`
}

type postHypothesisRequest struct {
	Topic            string   `json:"topic"`
	Supporting       []string `json:"supporting,omitempty"`
	Refuting         []string `json:"refuting,omitempty"`
	Confidence       float64  `json:"confidence,omitempty"`
	Subsystem        string   `json:"subsystem,omitempty"`         // 分叉签名
	CausalMechanism  string   `json:"causal_mechanism,omitempty"`  // 分叉签名
	Falsifier        string   `json:"falsifier,omitempty"`         // 可证伪预测
	Source           string   `json:"source,omitempty"`
}

// ---- 响应体 ----

type registerOperationResponse struct {
	Operation         *iam.Operation  `json:"operation"`
	DedupStatus       iam.DedupStatus `json:"dedup_status"`
	ReusedOperationID string          `json:"reused_operation_id,omitempty"`
	ReusedEvidenceID  string          `json:"reused_evidence_id,omitempty"`
}

// ContextPackage context@vN 上下文包：工作项 + 证据 + 反证清单 + 操作 + 发言。
type ContextPackage struct {
	IncidentID string            `json:"incident_id"`
	Version    int               `json:"version"`
	Incident   *iam.Incident     `json:"incident"`
	WorkNodes  []*iam.WorkNode   `json:"work_nodes"`
	Operations []*iam.Operation  `json:"operations"`
	Evidence   []*iam.Evidence   `json:"evidence"`
	Facts      []*iam.Fact       `json:"facts"`
	Hypotheses []*iam.Hypothesis `json:"hypotheses"`
	Guidance   []*iam.Guidance   `json:"guidance,omitempty"`
	// RefutedList 反证清单（被反驳假设的 topic）。
	RefutedList []string `json:"refuted_list,omitempty"`
}

type pushEvidenceResponse struct {
	Seq      int64         `json:"seq"`
	Hash     string        `json:"hash"`
	Evidence *iam.Evidence `json:"evidence"` // 已入库证据（含 proof_trace）
}

type evidenceListResponse struct {
	IncidentID string                 `json:"incident_id"`
	Evidence   []*iam.Evidence        `json:"evidence"`
	Indep      map[string]float64     `json:"independence_per_source"`
	TotalIndep float64                `json:"total_independence"`
	Ledger     []*evidenceLedgerEntry `json:"ledger,omitempty"`
}

type evidenceLedgerEntry struct {
	Seq      int64         `json:"seq"`
	Evidence *iam.Evidence `json:"evidence"`
	Hash     string        `json:"hash"`
}
