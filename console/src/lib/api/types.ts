/**
 * IOM 数据模型（P0 子集）—— 类型签名严格对齐
 * docs/implementation/P0-backend-brief.md §3（数据模型）/ §5（API 契约）。
 * 后端就绪后以 control-plane/api/openapi.yaml 生成的 client 替换 mock 适配器。
 */

export type ID = string;

export type IncidentStatus =
  | 'detected'
  | 'triaging'
  | 'investigating'
  | 'candidate'
  | 'mitigated'
  | 'resolved';

export type Severity = 'P1' | 'P2' | 'P3';

export interface TimeRange {
  start: string; // ISO 8601
  end: string | null;
}

export interface Incident {
  id: ID;
  fingerprint: string;
  title: string;
  status: IncidentStatus;
  severity: Severity;
  ic_id: string;
  ic_name: string;
  time_range: TimeRange;
  alert_ids: string[];
  symptom_set: string[];
  source: string;
  proof_trace: string;
  created_at: string;
  updated_at: string;
}

export type WorkNodeStatus =
  | 'pending'
  | 'claimed'
  | 'in_progress'
  | 'done'
  | 'stale'
  | 'cancelled';

export type AgentRole = 'explorer' | 'verifier' | 'skeptic' | 'executor' | 'ic';

export type CostLevel = 'low' | 'medium' | 'high';

export interface WorkNode {
  id: ID;
  incident_id: ID;
  question: string;
  scope: string;
  expected_evidence: string[];
  cost: CostLevel;
  deadline: string | null;
  assignee: string | null;
  role: AgentRole;
  status: WorkNodeStatus;
  lease_id: ID | null;
  depends_on: ID[];
  proof_trace: string;
  created_at: string;
  updated_at: string;
}

export type LeaseStatus = 'active' | 'expired' | 'released';

export interface Lease {
  id: ID;
  incident_id: ID;
  work_node_id: ID;
  holder: string;
  acquired_at: string;
  expires_at: string;
  last_heartbeat_at: string;
  status: LeaseStatus;
}

/** 去重状态：完全相同查询被 single-flight 压掉；兼容查询复用；需新鲜度才重查 */
export type DedupStatus = 'single_flight' | 'reused' | 'fresh';

export type DataSource = 'prometheus' | 'sls' | 'k8s' | 'cmdb' | 'argocd';

export interface Operation {
  id: ID;
  incident_id: ID;
  fingerprint: string;
  registered_by: string;
  data_source: DataSource;
  target_entity: string;
  summary: string;
  registered_at: string;
  dedup_status: DedupStatus;
  /** dedup_status 为 single_flight/reused 时指向被复用的 Operation */
  dedup_of: ID | null;
  result_ref: string | null;
  proof_trace: string;
}

export interface Evidence {
  id: ID;
  incident_id: ID;
  operation_id: ID;
  /** 血缘 DAG：父证据列表（derived_from 边） */
  parent_ids: ID[];
  source: string;
  summary: string;
  result: string;
  conclusion: string;
  /** 独立性评分 0-1：按采集链路/权限域/底层数据源计算，不按 agent 数 */
  independence_score: number;
  timestamp: string;
  proof_trace: string;
}

export interface Fact {
  id: ID;
  incident_id: ID;
  statement: string;
  evidence_ids: ID[];
  confirmed_by: string;
  is_confirmed: boolean;
  confirmed_at: string;
  proof_trace: string;
}

export type HypothesisStatus =
  | 'open'
  | 'strengthening'
  | 'weakening'
  | 'refuted'
  | 'confirmed';

export interface Hypothesis {
  id: ID;
  incident_id: ID;
  topic: string;
  /** supports 边：支撑证据 */
  supporting: ID[];
  /** refutes 边：反证证据 */
  refuting: ID[];
  independence_weight: number;
  confidence: number; // 0-1
  status: HypothesisStatus;
  proposed_by: string;
  updated_at: string;
  proof_trace: string;
}

export type GuidancePriority = 'info' | 'directive' | 'urgent';

export interface Guidance {
  id: ID;
  incident_id: ID;
  from_ic: string;
  text: string;
  priority: GuidancePriority;
  created_at: string;
}

export interface IncidentStats {
  incident_id: ID;
  facts_confirmed: number;
  open_questions: number;
  hypotheses_open: number;
  operations_total: number;
  duplicates_avoided: number;
  /** 无意重复率（被去重的重复查询占比） */
  dedup_rate: number;
  /** 决策延迟（分钟）：首告警 → 当前最新确认事实/决策 */
  decision_latency_min: number;
  /** 单位工具成本信息增益 */
  info_gain_per_cost: number;
  updated_at: string;
}

/** SSE 事件类型（GET /incidents/{id}/events） */
export type EventType =
  | 'incident.updated'
  | 'work_node.updated'
  | 'lease.changed'
  | 'evidence.added'
  | 'fact.confirmed'
  | 'guidance.added';

export interface IncidentEvent {
  /** 后端保证单调递增，前端用于去重 */
  seq: number;
  incident_id: ID;
  type: EventType;
  at: string;
  actor: string;
  summary: string;
  /** 关联对象（工作单元 / 证据 / 事实…），时间线点击定位用 */
  ref_id: ID | null;
}

/** 知识面板（P1 提供检索，P0 留接口返回空集占位） */
export interface KnowledgeHit {
  id: ID;
  title: string;
  kind: 'runbook' | 'postmortem' | 'candidate';
  score: number;
  certified: boolean;
  summary: string;
}

// ---------- 写操作输入（IC 三项操作） ----------

export interface CreateIncidentInput {
  title: string;
  severity: Severity;
  ic_name: string;
  alert_ids?: string[];
  symptom_set?: string[];
}

export interface CreateWorkNodeInput {
  question: string;
  scope: string;
  expected_evidence: string[];
  cost: CostLevel;
  role: AgentRole;
  assignee?: string;
  depends_on?: ID[];
  deadline?: string | null;
}

export interface CreateGuidanceInput {
  from_ic: string;
  text: string;
  priority: GuidancePriority;
}

export interface ContextPack {
  incident: Incident;
  work_nodes: WorkNode[];
  evidence: Evidence[];
  facts: Fact[];
  hypotheses: Hypothesis[];
  refutation_checklist: string[];
  version: number;
}
