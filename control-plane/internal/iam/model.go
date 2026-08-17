// Package iam 实现 IOM（Incident Object Model）数据模型与存储。
//
// 图是第一公民：所有节点携带公共字段（id/source/timestamp/proof_trace），
// 通过边（Edge）表达 has_symptom / involves_entity / executes / derived_from /
// supports / refutes / decides 等语义。P0 只读：任何节点都不得触发修复/执行路径。
//
// 存储层通过 Store 接口抽象：P0 使用 MemoryStore（纯内存，任何机器可跑），
// 后续接入 PostgreSQL 时实现同一接口即可替换（见 store.go）。
package iam

import "time"

// ---- 节点类型 ----

// NodeType 节点类型枚举（P0 子集，见 P0-brief §3）。
type NodeType string

const (
	NodeIncident   NodeType = "incident"
	NodeWorkNode   NodeType = "work_node"
	NodeOperation  NodeType = "operation"
	NodeEvidence   NodeType = "evidence"
	NodeFact       NodeType = "fact"
	NodeHypothesis NodeType = "hypothesis"
	NodeGuidance   NodeType = "guidance"
	NodeRunbook    NodeType = "runbook"
	NodeAction     NodeType = "action"
)

// ---- 边类型 ----

// EdgeType 边类型（P0 子集）。
type EdgeType string

const (
	// EdgeHasSymptom     Incident -> Symptom 关联症状集。
	EdgeHasSymptom EdgeType = "has_symptom"
	// EdgeInvolvesEntity Incident/WorkNode -> Entity 涉及实体。
	EdgeInvolvesEntity EdgeType = "involves_entity"
	// EdgeExecutes       WorkNode -> Operation 工作单元登记查询。
	EdgeExecutes EdgeType = "executes"
	// EdgeDerivedFrom    Evidence -> Evidence 证据血缘（DAG 入边）。
	EdgeDerivedFrom EdgeType = "derived_from"
	// EdgeSupports       Evidence -> Hypothesis 支持假设。
	EdgeSupports EdgeType = "supports"
	// EdgeRefutes        Evidence -> Hypothesis 反驳假设（反证清单）。
	EdgeRefutes EdgeType = "refutes"
	// EdgeDecides        Guidance -> WorkNode IC 决策指向工作单元。
	EdgeDecides EdgeType = "decides"
	// EdgeBelongsTo      任意节点 -> Incident 归属事故（作用域边）。
	EdgeBelongsTo EdgeType = "belongs_to"
)

// ---- 时间 ----

// TimeRange 时间窗（事故时间窗、查询时间窗通用）。
type TimeRange struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end,omitempty"`
}

// ---- 状态枚举 ----

// IncidentStatus 事故生命周期状态。
type IncidentStatus string

const (
	IncidentOpen          IncidentStatus = "open"
	IncidentInvestigating IncidentStatus = "investigating"
	IncidentMitigated     IncidentStatus = "mitigated"
	IncidentClosed        IncidentStatus = "closed"
)

// WorkRole 工作单元角色：探索 / 验证 / 怀疑 / 执行。
type WorkRole string

const (
	RoleExplorer WorkRole = "explorer"
	RoleVerifier WorkRole = "verifier"
	RoleSkeptic  WorkRole = "skeptic"
	RoleExecutor WorkRole = "executor"
)

// WorkNodeStatus 工作单元状态。
type WorkNodeStatus string

const (
	WorkNodeOpen   WorkNodeStatus = "open"
	WorkNodeActive WorkNodeStatus = "active"
	WorkNodeDone   WorkNodeStatus = "done"
	// WorkNodeStale 租约过期/晚到结果标记，避免污染当前事故。
	WorkNodeStale WorkNodeStatus = "stale"
)

// DedupStatus 操作去重状态：fresh 首次 / single_flight 并发合并 / reused 兼容复用。
type DedupStatus string

const (
	DedupFresh        DedupStatus = "fresh"
	DedupSingleFlight DedupStatus = "single_flight"
	DedupReused       DedupStatus = "reused"
)

// HypothesisStatus 假设生命周期。
type HypothesisStatus string

const (
	HypothesisProposed  HypothesisStatus = "proposed"
	HypothesisSupported HypothesisStatus = "supported"
	HypothesisRefuted   HypothesisStatus = "refuted"
	HypothesisConfirmed HypothesisStatus = "confirmed"
	// HypothesisFrozen 话题冻结（§5.9）：无新证据即冻结；重开需
	// reopen_reason + new_evidence_ref + context_version。
	HypothesisFrozen HypothesisStatus = "frozen"
)

// ---- 公共字段 ----

// ProofTraceEntry 指向证据账本（Ledger）中的一条原始记录，保证可回放。
// P0 审计底线：任何派生节点的 proof_trace 必须可回放到账本原始记录。
type ProofTraceEntry struct {
	LedgerSeq  int64     `json:"ledger_seq"`  // 账本序号
	EvidenceID string    `json:"evidence_id"` // 证据 ID
	CapturedAt time.Time `json:"captured_at"` // 捕获时间
}

// NodeBase 所有节点的公共字段。
type NodeBase struct {
	ID         string            `json:"id"`                    // 全局唯一 ID
	Type       NodeType          `json:"type"`                  // 节点类型
	Source     string            `json:"source"`                // 数据来源 / agent 标识
	Timestamp  time.Time         `json:"timestamp"`             // 节点创建时间
	ProofTrace []ProofTraceEntry `json:"proof_trace,omitempty"` // 可回放证明轨迹
}

// ---- 节点定义（P0 子集）----

// Incident 一次事故。
type Incident struct {
	NodeBase
	Fingerprint string         `json:"fingerprint"`         // 事故指纹（跨事故候选召回）
	Title       string         `json:"title"`               // 事故标题（缺省由症状合成，供前端展示）
	Severity    Severity       `json:"severity"`            // P1/P2/P3
	Status      IncidentStatus `json:"status"`              // 状态
	ICID        string         `json:"ic_id"`               // 人类 Incident Commander
	TimeRange   TimeRange      `json:"time_range"`          // 时间窗
	AlertIDs    []string       `json:"alert_ids,omitempty"` // 关联告警 ID
	SymptomSet  []string       `json:"symptom_set"`         // 症状签名集合
}

// Severity 事故严重度。
type Severity string

const (
	SeverityP1 Severity = "P1"
	SeverityP2 Severity = "P2"
	SeverityP3 Severity = "P3"
)

// WorkNode 一个调查工作单元：question + scope + expected_evidence + cost + deadline。
type WorkNode struct {
	NodeBase
	IncidentID string `json:"incident_id,omitempty"` // 归属事故（跨事故隔离）
	Question         string         `json:"question"`                    // 要回答的问题
	Scope            string         `json:"scope"`                       // 调查范围
	ExpectedEvidence []string       `json:"expected_evidence,omitempty"` // 预期证据
	Cost             int            `json:"cost"`                        // 预估成本（token/step/时长）
	Deadline         *time.Time     `json:"deadline,omitempty"`          // 截止时间
	Assignee         string         `json:"assignee,omitempty"`          // 负责人
	Role             WorkRole       `json:"role"`                        // 角色
	Status           WorkNodeStatus `json:"status"`                      // 状态
	LeaseID          string         `json:"lease_id,omitempty"`          // 咨询性租约
	DependsOn        []string       `json:"depends_on,omitempty"`        // 依赖的工作单元（工作图 DAG 边）
}

// Operation 一次查询/操作登记。去重状态见 DedupStatus。
type Operation struct {
	NodeBase
	IncidentID string `json:"incident_id,omitempty"` // 归属事故（跨事故隔离）
	Fingerprint  string      `json:"fingerprint"`            // 操作指纹（querycoord 生成）
	Query        QuerySpec   `json:"query"`                  // 规范化查询（供指纹生成与复用判断）
	RegisteredAt time.Time   `json:"registered_at"`          // 登记时间
	DedupStatus  DedupStatus `json:"dedup_status"`           // 去重状态
	MergedIntoID string      `json:"merged_into,omitempty"`  // single-flight 合并目标操作 ID
	ResultRef    string      `json:"result_ref,omitempty"`   // 指向结果/证据 ID
	WorkNodeID   string      `json:"work_node_id,omitempty"` // 关联工作单元（可选）
	Tenant       string      `json:"tenant,omitempty"`       // 租户
	ToolVersion  string      `json:"tool_version,omitempty"` // 工具版本
}

// QuerySpec 规范化查询描述，是操作指纹的输入。
type QuerySpec struct {
	Target       string    `json:"target"`                  // 目标，如 ack-xxx-cluster/k8s/pod/foo-7b9c
	DataSource   string    `json:"data_source"`             // prometheus | sls | k8s | cmdb
	TimeWindow   TimeRange `json:"time_window"`             // 时间窗
	QueryAST     string    `json:"query_ast"`               // 规范化查询 AST（含过滤条件）
	Filters      []string  `json:"filters,omitempty"`       // 过滤条件（规范化为有序列表）
	Tenant       string    `json:"tenant,omitempty"`        // 租户
	ToolVersion  string    `json:"tool_version,omitempty"`  // 工具版本，如 kubectl-v1.31
	DataSnapshot string    `json:"data_snapshot,omitempty"` // 数据快照标识
}

// Evidence 一条排查证据，带血缘 DAG 与独立性评分。
type Evidence struct {
	NodeBase
	OperationID       string   `json:"operation_id"`                // 来源 Operation
	LineageDAG        []string `json:"lineage_dag,omitempty"`       // 血缘 DAG 入边（父证据 ID 列表）
	DataSource        string   `json:"data_source"`                 // 底层数据源
	PermissionDomain  string   `json:"permission_domain,omitempty"` // 权限域（独立性维度之一）
	CollectionChain   string   `json:"collection_chain,omitempty"`  // 采集链路（独立性维度之一）
	Result            string   `json:"result"`                      // 结果摘要
	Conclusion        string   `json:"conclusion,omitempty"`        // 结论（可选）
	IndependenceScore float64  `json:"independence_score"`          // 独立性评分（0..1）
	IsStale           bool     `json:"is_stale,omitempty"`          // 晚到结果，不污染当前事故
}

// Fact 已确认事实（非假设）。证据链闭合、多源确认、可引用。
type Fact struct {
	NodeBase
	Statement     string   `json:"statement"`                // 事实陈述
	EvidenceChain []string `json:"evidence_chain,omitempty"` // 证据链（证据 ID）
	ConfirmedBy   string   `json:"confirmed_by"`             // 谁确认（IC / 多源）
	IsConfirmed   bool     `json:"is_confirmed"`             // 是否已确认
}

// Hypothesis 根因假设。
type Hypothesis struct {
	NodeBase
	Topic              string           `json:"topic"`                // 假设主题
	Supporting         []string         `json:"supporting,omitempty"` // 支持证据 ID
	Refuting           []string         `json:"refuting,omitempty"`   // 反驳证据 ID（反证清单）
	IndependenceWeight float64          `json:"independence_weight"`  // 血缘独立性权重
	Confidence         float64          `json:"confidence"`           // 置信度 0..1
	Status             HypothesisStatus `json:"status"`               // 状态
	// ---- 分叉签名（§5.8）：决定"新假设 vs 验证副本" ----
	Subsystem       string `json:"subsystem,omitempty"`        // 受影响子系统
	CausalMechanism string `json:"causal_mechanism,omitempty"` // 因果机制
	Falsifier       string `json:"falsifier,omitempty"`        // 可证伪预测（必须能单点推翻）
}

// Guidance 人的指示（IC 发言），不要求 agent 回复。
type Guidance struct {
	NodeBase
	FromIC   string `json:"from_ic"`  // 发布者（IC）
	Text     string `json:"text"`     // 指示内容
	Priority int    `json:"priority"` // 优先级（越小越高）
}

// ---- 边 ----

// Edge 一条有向边，语义见 EdgeType。
type Edge struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	Type       EdgeType  `json:"type"`
	From       string    `json:"from"` // 源节点 ID
	To         string    `json:"to"`   // 目标节点 ID
	Timestamp  time.Time `json:"timestamp"`
}

// Runbook 认证后的可复用排障方案（§5.2/5.6 类型化字段）。
type Runbook struct {
	NodeBase
	IncidentID          string       `json:"incident_id"`                    // 来源事故
	Title               string       `json:"title"`                          // 标题
	Symptoms            []string     `json:"symptoms,omitempty"`             // 适用症状（指纹召回）
	RootCause           string       `json:"root_cause"`                     // 命名根因
	DiagnosticSteps     []string     `json:"diagnostic_steps,omitempty"`     // 诊断步骤
	VerificationActions []string     `json:"verification_actions,omitempty"` // 验证动作
	Rollback            string       `json:"rollback,omitempty"`             // 回滚/补偿
	SuccessCriteria     string       `json:"success_criteria,omitempty"`     // 成功判定
	Status              RunbookStatus `json:"status"`                        // candidate→verified→certified
}

// RunbookStatus runbook 生命周期。
type RunbookStatus string

const (
	RunbookCandidate RunbookStatus = "candidate"
	RunbookVerified  RunbookStatus = "verified"
	RunbookCertified RunbookStatus = "certified"
	RunbookRevoked   RunbookStatus = "revoked"
)

// Action 修复/止血动作（§5.3 强类型动作最小版）。
type Action struct {
	NodeBase
	IncidentID  string       `json:"incident_id"`           // 归属事故
	Type        string       `json:"type"`                  // typed action 类型（env 定义）
	Status      ActionStatus `json:"status"`                // proposed→approved→executed
	ApprovedBy  string       `json:"approved_by,omitempty"` // IC（认证用户）
	Result      string       `json:"result,omitempty"`      // 执行结果摘要
	DryRun      bool         `json:"dry_run"`               // 护栏：默认 dry-run
}

// ActionStatus 动作生命周期。
type ActionStatus string

const (
	ActionProposed ActionStatus = "proposed"
	ActionApproved ActionStatus = "approved"
	ActionExecuted ActionStatus = "executed"
	ActionRejected ActionStatus = "rejected"
)
