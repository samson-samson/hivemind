/**
 * 真实后端 HTTP 适配器 —— 实现 OpsHiveApi 接口（契约见 client.ts）。
 *
 * 后端 Go control-plane 返回 snake_case 扁平字段，与前端视图类型存在
 * 少量命名/形态差异，本文件内的 normalize 函数负责确定性翻译
 * （映射规则以 control-plane/internal/iam/model.go 为准，逐字段注释）。
 *
 * 后端就绪后的运行方式：`VITE_API_MODE=http VITE_API_BASE=http://localhost:8081 npm run dev`
 */
import type {
  ContextPack,
  CreateGuidanceInput,
  CreateIncidentInput,
  CreateWorkNodeInput,
  Evidence,
  Fact,
  Guidance,
  Hypothesis,
  ID,
  Incident,
  IncidentEvent,
  IncidentStats,
  KnowledgeHit,
  Lease,
  Operation,
  WorkNode,
} from '../types';
import type { OpsHiveApi } from '../client';

const BASE = import.meta.env.VITE_API_BASE ?? 'http://localhost:8081';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(`${res.status} ${res.statusText}: ${body.slice(0, 200)}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

// ---------- normalize：后端字段 → 前端视图形状 ----------

type BackendIncident = {
  id: string; fingerprint: string; title?: string; severity?: string;
  status?: string; ic_id?: string; time_range?: { start: string; end?: string | null };
  alert_ids?: string[]; symptom_set?: string[]; source?: string; timestamp?: string;
};

function normIncident(b: BackendIncident): Incident {
  const ts = b.timestamp ?? new Date().toISOString();
  const statusMap: Record<string, Incident['status']> = {
    open: 'detected', investigating: 'investigating', mitigated: 'mitigated', closed: 'resolved',
  };
  return {
    id: b.id,
    fingerprint: b.fingerprint ?? '',
    title: b.title ?? b.symptom_set?.[0] ?? b.id,
    status: (b.status ? statusMap[b.status] : undefined) ?? 'detected',
    severity: (b.severity as SeverityT) ?? 'P2',
    ic_id: b.ic_id ?? '',
    ic_name: b.ic_id ?? '—',
    time_range: {
      start: b.time_range?.start ?? ts,
      end: b.time_range?.end ?? null,
    },
    alert_ids: b.alert_ids ?? [],
    symptom_set: b.symptom_set ?? [],
    source: b.source ?? 'backend',
    proof_trace: b.source ?? 'backend',
    created_at: ts,
    updated_at: ts,
  };
}
type SeverityT = Incident['severity'];

type BackendWorkNode = {
  id: string; question: string; scope?: string; expected_evidence?: string[];
  cost?: number; deadline?: string | null; assignee?: string; role?: string;
  status?: string; lease_id?: string; depends_on?: string[];
  source?: string; timestamp?: string;
};

function normWorkNode(b: BackendWorkNode, incidentId: ID): WorkNode {
  const ts = b.timestamp ?? new Date().toISOString();
  const costMap = (c: number): WorkNode['cost'] => (c <= 5 ? 'low' : c <= 15 ? 'medium' : 'high');
  const statusMap: Record<string, WorkNode['status']> = {
    open: 'pending', active: 'in_progress', done: 'done', stale: 'stale',
  };
  return {
    id: b.id,
    incident_id: incidentId,
    question: b.question,
    scope: b.scope ?? '',
    expected_evidence: b.expected_evidence ?? [],
    cost: costMap(b.cost ?? 1),
    deadline: b.deadline ?? null,
    assignee: b.assignee ?? null,
    role: (b.role as WorkNode['role']) ?? 'explorer',
    status: (b.status ? statusMap[b.status] : undefined) ?? 'pending',
    lease_id: b.lease_id ?? null,
    depends_on: b.depends_on ?? [],
    proof_trace: b.source ?? 'backend',
    created_at: ts,
    updated_at: ts,
  };
}

type BackendLease = {
  id: string; incident_id?: string; work_node_id?: string; assignee?: string;
  claimed_at?: string; heartbeat_at?: string; expires_at?: string; status?: string;
};

function normLease(b: BackendLease): Lease {
  return {
    id: b.id,
    incident_id: b.incident_id ?? '',
    work_node_id: b.work_node_id ?? '',
    holder: b.assignee ?? 'unknown',
    acquired_at: b.claimed_at ?? new Date().toISOString(),
    expires_at: b.expires_at ?? new Date().toISOString(),
    last_heartbeat_at: b.heartbeat_at ?? b.claimed_at ?? new Date().toISOString(),
    status: (b.status as Lease['status']) ?? 'active',
  };
}

type BackendOperation = {
  id: string; fingerprint?: string; query?: { target?: string; data_source?: string; query_ast?: string };
  registered_at?: string; dedup_status?: string; merged_into?: string;
  result_ref?: string; source?: string; timestamp?: string;
};

function normOperation(b: BackendOperation, incidentId: ID): Operation {
  const ts = b.registered_at ?? b.timestamp ?? new Date().toISOString();
  const q = b.query ?? {};
  return {
    id: b.id,
    incident_id: incidentId,
    fingerprint: b.fingerprint ?? '',
    registered_by: b.source ?? 'unknown',
    data_source: (q.data_source as Operation['data_source']) ?? 'k8s',
    target_entity: q.target ?? '',
    summary: (q.query_ast ?? q.target ?? b.id).slice(0, 120),
    registered_at: ts,
    dedup_status: (b.dedup_status as Operation['dedup_status']) ?? 'fresh',
    dedup_of: b.merged_into ?? null,
    result_ref: b.result_ref ?? null,
    proof_trace: b.source ?? 'backend',
  };
}

type BackendEvidence = {
  id: string; operation_id?: string; lineage_dag?: string[]; data_source?: string;
  result?: string; conclusion?: string; independence_score?: number;
  is_stale?: boolean; source?: string; timestamp?: string;
};

function normEvidence(b: BackendEvidence, incidentId: ID): Evidence {
  const ts = b.timestamp ?? new Date().toISOString();
  return {
    id: b.id,
    incident_id: incidentId,
    operation_id: b.operation_id ?? '',
    parent_ids: b.lineage_dag ?? [],
    source: b.data_source ?? b.source ?? 'backend',
    summary: (b.result ?? b.conclusion ?? b.id).slice(0, 120),
    result: b.result ?? '',
    conclusion: b.conclusion ?? '',
    independence_score: b.independence_score ?? 0,
    timestamp: ts,
    proof_trace: b.source ?? 'backend',
  };
}

type BackendFact = {
  id: string; statement?: string; evidence_chain?: string[]; confirmed_by?: string;
  is_confirmed?: boolean; source?: string; timestamp?: string;
};

function normFact(b: BackendFact, incidentId: ID): Fact {
  const ts = b.timestamp ?? new Date().toISOString();
  return {
    id: b.id,
    incident_id: incidentId,
    statement: b.statement ?? '',
    evidence_ids: b.evidence_chain ?? [],
    confirmed_by: b.confirmed_by ?? '',
    is_confirmed: b.is_confirmed ?? false,
    confirmed_at: ts,
    proof_trace: b.source ?? 'backend',
  };
}

type BackendHypothesis = {
  id: string; topic?: string; supporting?: string[]; refuting?: string[];
  independence_weight?: number; confidence?: number; status?: string;
  source?: string; timestamp?: string;
};

function normHypothesis(b: BackendHypothesis, incidentId: ID): Hypothesis {
  const statusMap: Record<string, Hypothesis['status']> = {
    proposed: 'open', supported: 'strengthening', refuted: 'refuted', confirmed: 'confirmed',
  };
  return {
    id: b.id,
    incident_id: incidentId,
    topic: b.topic ?? '',
    supporting: b.supporting ?? [],
    refuting: b.refuting ?? [],
    independence_weight: b.independence_weight ?? 0,
    confidence: b.confidence ?? 0,
    status: (b.status ? statusMap[b.status] : undefined) ?? 'open',
    proposed_by: b.source ?? 'unknown',
    updated_at: b.timestamp ?? new Date().toISOString(),
    proof_trace: b.source ?? 'backend',
  };
}

type BackendGuidance = {
  id: string; from_ic?: string; text?: string; priority?: number; source?: string; timestamp?: string;
};

function normGuidance(b: BackendGuidance): Guidance {
  const prioMap: Record<number, Guidance['priority']> = { 1: 'urgent', 2: 'directive' };
  return {
    id: b.id,
    incident_id: '',
    from_ic: b.from_ic ?? b.source ?? 'ic',
    text: b.text ?? '',
    priority: b.priority ? (prioMap[b.priority] ?? 'info') : 'info',
    created_at: b.timestamp ?? new Date().toISOString(),
  };
}

type BackendStats = {
  incident_id?: string; total_operations?: number; deduped_operations?: number;
  dedup_rate?: number; confirmed_facts?: number; total_facts?: number;
  info_gain_per_operation?: number; total_hypotheses?: number;
  work_nodes_open?: number; work_nodes_active?: number;
  decision_latency_ms?: number | null;
};

function normStats(b: BackendStats): IncidentStats {
  const openQuestions = (b.work_nodes_open ?? 0) + (b.work_nodes_active ?? 0);
  const latencyMs = b.decision_latency_ms;
  return {
    incident_id: b.incident_id ?? '',
    facts_confirmed: b.confirmed_facts ?? 0,
    open_questions: openQuestions,
    hypotheses_open: b.total_hypotheses ?? 0,
    operations_total: b.total_operations ?? 0,
    duplicates_avoided: b.deduped_operations ?? 0,
    dedup_rate: b.dedup_rate ?? 0,
    decision_latency_min: latencyMs != null ? Math.round(latencyMs / 60000) : 0,
    info_gain_per_cost: b.info_gain_per_operation ?? 0,
    updated_at: new Date().toISOString(),
  };
}

// ---------- HTTP 适配器 ----------

export class HttpOpsHiveApi implements OpsHiveApi {
  private sseSeq = 0;
  private eventSources = new Map<ID, EventSource>();

  listIncidents(): Promise<Incident[]> {
    return request<BackendIncident[]>('/api/v1/incidents').then((xs) => xs.map(normIncident));
  }
  getIncident(id: ID): Promise<Incident> {
    return request<BackendIncident>(`/api/v1/incidents/${id}`).then(normIncident);
  }
  createIncident(input: CreateIncidentInput): Promise<Incident> {
    return request<BackendIncident>('/api/v1/incidents', {
      method: 'POST',
      body: JSON.stringify({
        title: input.title,
        severity: input.severity,
        ic_id: input.ic_name,
        alert_ids: input.alert_ids,
        symptom_set: input.symptom_set,
      }),
    }).then(normIncident);
  }
  async getContext(id: ID, _version?: number): Promise<ContextPack> {
    const [incident, workNodes, evidence, facts, hypotheses] = await Promise.all([
      this.getIncident(id),
      this.listWorkNodes(id),
      this.listEvidence(id),
      this.listFacts(id),
      this.listHypotheses(id),
    ]);
    return {
      incident,
      work_nodes: workNodes,
      evidence,
      facts,
      hypotheses,
      refutation_checklist: hypotheses.filter((h) => h.status === 'refuted').map((h) => h.topic),
      version: _version ?? 1,
    };
  }

  listWorkNodes(id: ID): Promise<WorkNode[]> {
    return request<BackendWorkNode[]>(`/api/v1/incidents/${id}/work-nodes`).then((xs) => xs.map((x) => normWorkNode(x, id)));
  }
  createWorkNode(id: ID, input: CreateWorkNodeInput): Promise<WorkNode> {
    return request<BackendWorkNode>(`/api/v1/incidents/${id}/work-nodes`, {
      method: 'POST',
      body: JSON.stringify({
        question: input.question,
        scope: input.scope,
        expected_evidence: input.expected_evidence,
        role: input.role,
        assignee: input.assignee,
        deadline: input.deadline ?? null,
      }),
    }).then((x) => normWorkNode(x, id));
  }
  async updateWorkNode(id: ID, nodeId: ID, patch: Partial<WorkNode>): Promise<WorkNode> {
    const body: Record<string, unknown> = {};
    if (patch.question !== undefined) body.question = patch.question;
    if (patch.scope !== undefined) body.scope = patch.scope;
    if (patch.assignee !== undefined) body.assignee = patch.assignee;
    if (patch.role !== undefined) body.role = patch.role;
    if (patch.status !== undefined) body.status = patch.status;
    return request<BackendWorkNode>(`/api/v1/incidents/${id}/work-nodes/${nodeId}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }).then((x) => normWorkNode(x, id));
  }
  // IC 拖拽布局：前端辅助，客户端本地化（localStorage），不占后端。
  async saveWorkGraphLayout(id: ID, positions: Record<ID, { x: number; y: number }>): Promise<void> {
    try {
      localStorage.setItem(`opshive.layout.${id}`, JSON.stringify(positions));
    } catch {
      /* 隐私模式等场景静默失败 */
    }
  }

  listLeases(id: ID): Promise<Lease[]> {
    return request<BackendLease[]>(`/api/v1/incidents/${id}/leases`).then((xs) => xs.map(normLease));
  }
  heartbeatLease(id: ID, leaseId: ID): Promise<Lease> {
    return request<BackendLease>(`/api/v1/incidents/${id}/leases/${leaseId}/heartbeat`, { method: 'POST' }).then(normLease);
  }
  async releaseLease(id: ID, leaseId: ID): Promise<void> {
    await request<unknown>(`/api/v1/incidents/${id}/leases/${leaseId}`, { method: 'DELETE' });
  }

  listOperations(id: ID): Promise<Operation[]> {
    return request<BackendOperation[]>(`/api/v1/incidents/${id}/operations`).then((xs) => xs.map((x) => normOperation(x, id)));
  }
  async listEvidence(id: ID): Promise<Evidence[]> {
    // 后端返回富响应 {incident_id, evidence, indep, total_indep, ledger}。
    const res = await request<{ incident_id: string; evidence: BackendEvidence[] }>(
      `/api/v1/incidents/${id}/evidence`,
    );
    return (res.evidence ?? []).map((x) => normEvidence(x, id));
  }
  listFacts(id: ID): Promise<Fact[]> {
    return request<BackendFact[]>(`/api/v1/incidents/${id}/facts`).then((xs) => xs.map((x) => normFact(x, id)));
  }
  listHypotheses(id: ID): Promise<Hypothesis[]> {
    return request<BackendHypothesis[]>(`/api/v1/incidents/${id}/hypotheses`).then((xs) => xs.map((x) => normHypothesis(x, id)));
  }

  listGuidance(id: ID): Promise<Guidance[]> {
    return request<BackendGuidance[]>(`/api/v1/incidents/${id}/guidance`).then((xs) => xs.map(normGuidance));
  }
  createGuidance(id: ID, input: CreateGuidanceInput): Promise<Guidance> {
    const prioNum: Record<Guidance['priority'], number> = { info: 3, directive: 2, urgent: 1 };
    return request<BackendGuidance>(`/api/v1/incidents/${id}/guidance`, {
      method: 'POST',
      body: JSON.stringify({ from_ic: input.from_ic, text: input.text, priority: prioNum[input.priority] }),
    }).then(normGuidance);
  }

  getStats(id: ID): Promise<IncidentStats> {
    return request<BackendStats>(`/api/v1/incidents/${id}/stats`).then(normStats);
  }

  /**
   * 事件快照：后端无独立事件表，由各实体列表按时间合成（供时间线初始渲染；
   * 增量由 subscribeEvents 的 SSE 提供）。
   */
  async listEvents(id: ID): Promise<IncidentEvent[]> {
    const [workNodes, ops, evs, facts, hyps, guides] = await Promise.all([
      this.listWorkNodes(id),
      this.listOperations(id),
      this.listEvidence(id),
      this.listFacts(id),
      this.listHypotheses(id),
      this.listGuidance(id),
    ]);
    const events: IncidentEvent[] = [];
    let seq = 0;
    const push = (type: IncidentEvent['type'], at: string, actor: string, summary: string, ref_id: ID | null) => {
      events.push({ seq: ++seq, incident_id: id, type, at, actor, summary, ref_id });
    };
    for (const w of workNodes) {
      push('work_node.created', w.created_at, w.assignee ?? 'ic', `工作单元：${w.question}`, w.id);
    }
    for (const o of ops) {
      push('operation.registered', o.registered_at, o.registered_by, o.summary, o.id);
    }
    for (const e of evs) {
      push('evidence.appended', e.timestamp, e.source, e.summary, e.id);
    }
    for (const f of facts) {
      push('fact.posted', f.confirmed_at, f.confirmed_by, f.statement, f.id);
    }
    for (const h of hyps) {
      push('hypothesis.posted', h.updated_at, h.proposed_by, h.topic, h.id);
    }
    for (const g of guides) {
      push('guidance.posted', g.created_at, g.from_ic, g.text, g.id);
    }
    return events.sort((a, b) => a.at.localeCompare(b.at));
  }

  subscribeEvents(
    id: ID,
    handler: (event: IncidentEvent) => void,
    onError?: (err: unknown) => void,
  ): () => void {
    const es = new EventSource(`${BASE}/api/v1/incidents/${id}/events`);
    this.eventSources.set(id, es);
    const listen = (ev: MessageEvent<string>) => {
      const payload = JSON.parse(ev.data) as { id?: string; type?: string; source?: string; timestamp?: string };
      const event: IncidentEvent = {
        seq: ++this.sseSeq,
        incident_id: id,
        type: (ev.type as IncidentEvent['type']) ?? 'incident.updated',
        at: payload.timestamp ?? new Date().toISOString(),
        actor: payload.source ?? 'backend',
        summary: ev.type ?? 'incident.updated',
        ref_id: payload.id ?? null,
      };
      handler(event);
    };
    // 事件名与后端 bus.Publish 一一对应（见 control-plane/internal/api/handlers.go）。
    es.addEventListener('incident.created', listen);
    es.addEventListener('work_node.created', listen);
    es.addEventListener('work_node.updated', listen);
    es.addEventListener('lease.claimed', listen);
    es.addEventListener('lease.heartbeat', listen);
    es.addEventListener('lease.released', listen);
    es.addEventListener('operation.registered', listen);
    es.addEventListener('evidence.appended', listen);
    es.addEventListener('fact.posted', listen);
    es.addEventListener('hypothesis.posted', listen);
    es.addEventListener('guidance.posted', listen);
    es.addEventListener('error', () => onError?.(new Error('sse disconnected')));
    return () => {
      es.close();
      this.eventSources.delete(id);
    };
  }

  async listKnowledge(_id: ID): Promise<KnowledgeHit[]> {
    return []; // P1 提供检索，P0 空集占位
  }
}
