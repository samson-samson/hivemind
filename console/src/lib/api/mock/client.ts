import type { HivemindApi } from '../client';
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
import * as seed from './data';

/**
 * MOCK 适配器：内存态 + 模拟延迟。类型签名与真实适配器一致，
 * 后端就绪后在 lib/api/index.ts 一处替换为 OpenAPI client 即可。
 *
 * 写操作（建事故/建工作单元/发 Guidance）会同步向 subscribeEvents
 * 的订阅者推送对应事件，模拟 SSE 增量更新。
 */

const LATENCY_MS = 120;

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T;

interface State {
  incidents: Incident[];
  workNodes: WorkNode[];
  leases: Lease[];
  operations: Operation[];
  evidence: Evidence[];
  facts: Fact[];
  hypotheses: Hypothesis[];
  guidance: Guidance[];
  stats: IncidentStats;
  events: IncidentEvent[];
  layouts: Record<ID, Record<ID, { x: number; y: number }>>;
  nextSeq: number;
}

const state: State = {
  incidents: [clone(seed.incident)],
  workNodes: clone(seed.workNodes),
  leases: clone(seed.leases),
  operations: clone(seed.operations),
  evidence: clone(seed.evidence),
  facts: clone(seed.facts),
  hypotheses: clone(seed.hypotheses),
  guidance: clone(seed.guidance),
  stats: clone(seed.stats),
  events: clone(seed.events),
  layouts: {},
  nextSeq: Math.max(...seed.events.map((e) => e.seq)) + 1,
};

type Listener = (e: IncidentEvent) => void;
const listeners = new Map<ID, Set<Listener>>();

const nowIso = () =>
  new Date().toLocaleString('sv-SE', { timeZone: 'Asia/Shanghai' }).replace(' ', 'T') + '+08:00';

function emit(event: Omit<IncidentEvent, 'seq' | 'at'> & { at?: string }) {
  const full: IncidentEvent = {
    seq: state.nextSeq++,
    at: event.at ?? nowIso(),
    ...event,
  };
  state.events.push(full);
  const subs = listeners.get(full.incident_id);
  subs?.forEach((fn) => fn(full));
  return full;
}

const delay = <T>(value: T): Promise<T> =>
  new Promise((resolve) => setTimeout(() => resolve(clone(value)), LATENCY_MS));

const mustIncident = (id: ID): Incident => {
  const inc = state.incidents.find((i) => i.id === id);
  if (!inc) throw new Error(`incident not found: ${id}`);
  return inc;
};

let idCounter = 1000;
const nextId = (prefix: string) => `${prefix}-local-${idCounter++}`;

export const mockApi: HivemindApi = {
  listIncidents: () => delay(state.incidents),
  getIncident: (id) => delay(mustIncident(id)),

  createIncident: (input: CreateIncidentInput) => {
    const inc: Incident = {
      id: nextId('inc'),
      fingerprint: `fp:${Math.random().toString(16).slice(2, 18)}`,
      title: input.title,
      status: 'detected',
      severity: input.severity,
      ic_id: 'u-local',
      ic_name: input.ic_name,
      time_range: { start: nowIso(), end: null },
      alert_ids: input.alert_ids ?? [],
      symptom_set: input.symptom_set ?? [],
      source: 'manual',
      proof_trace: 'ledger://local',
      created_at: nowIso(),
      updated_at: nowIso(),
    };
    state.incidents.unshift(inc);
    emit({ incident_id: inc.id, type: 'incident.created', actor: input.ic_name, summary: `手动创建事故：${inc.title}`, ref_id: inc.id });
    return delay(inc);
  },

  getContext: (id, version) => {
    mustIncident(id);
    const pack: ContextPack = {
      incident: mustIncident(id),
      work_nodes: state.workNodes.filter((n) => n.incident_id === id),
      evidence: state.evidence.filter((e) => e.incident_id === id),
      facts: state.facts.filter((f) => f.incident_id === id),
      hypotheses: state.hypotheses.filter((h) => h.incident_id === id),
      refutation_checklist: state.hypotheses
        .filter((h) => h.incident_id === id && h.refuting.length > 0)
        .map((h) => `${h.topic}（反证 ${h.refuting.length} 条）`),
      version: version ?? 1,
    };
    return delay(pack);
  },

  listWorkNodes: (id) => delay(state.workNodes.filter((n) => n.incident_id === id)),

  createWorkNode: (id, input: CreateWorkNodeInput) => {
    mustIncident(id);
    const node: WorkNode = {
      id: nextId('wn'),
      incident_id: id,
      question: input.question,
      scope: input.scope,
      expected_evidence: input.expected_evidence,
      cost: input.cost,
      deadline: input.deadline ?? null,
      assignee: input.assignee ?? null,
      role: input.role,
      status: 'pending',
      lease_id: null,
      depends_on: input.depends_on ?? [],
      proof_trace: 'ledger://local',
      created_at: nowIso(),
      updated_at: nowIso(),
    };
    state.workNodes.push(node);
    emit({ incident_id: id, type: 'work_node.created', actor: 'IC', summary: `新增工作单元：${node.question}`, ref_id: node.id });
    return delay(node);
  },

  updateWorkNode: (id, nodeId, patch) => {
    const node = state.workNodes.find((n) => n.incident_id === id && n.id === nodeId);
    if (!node) throw new Error(`work node not found: ${nodeId}`);
    Object.assign(node, patch, { id: node.id, updated_at: nowIso() });
    return delay(node);
  },

  saveWorkGraphLayout: (id, positions) => {
    state.layouts[id] = { ...state.layouts[id], ...positions };
    return delay(undefined);
  },

  listLeases: (id) => delay(state.leases.filter((l) => l.incident_id === id)),

  heartbeatLease: (id, leaseId) => {
    const lease = state.leases.find((l) => l.incident_id === id && l.id === leaseId);
    if (!lease) throw new Error(`lease not found: ${leaseId}`);
    lease.last_heartbeat_at = nowIso();
    return delay(lease);
  },

  releaseLease: (id, leaseId) => {
    const lease = state.leases.find((l) => l.incident_id === id && l.id === leaseId);
    if (lease) lease.status = 'released';
    return delay(undefined);
  },

  listOperations: (id) => delay(state.operations.filter((o) => o.incident_id === id)),
  listEvidence: (id) => delay(state.evidence.filter((e) => e.incident_id === id)),
  listFacts: (id) => delay(state.facts.filter((f) => f.incident_id === id)),
  listHypotheses: (id) => delay(state.hypotheses.filter((h) => h.incident_id === id)),
  listGuidance: (id) => delay(state.guidance.filter((g) => g.incident_id === id)),

  createGuidance: (id, input: CreateGuidanceInput) => {
    mustIncident(id);
    const g: Guidance = {
      id: nextId('g'),
      incident_id: id,
      from_ic: input.from_ic,
      text: input.text,
      priority: input.priority,
      created_at: nowIso(),
    };
    state.guidance.push(g);
    emit({ incident_id: id, type: 'guidance.posted', actor: input.from_ic, summary: `Guidance：${input.text.slice(0, 60)}`, ref_id: g.id });
    return delay(g);
  },

  getStats: (id) => {
    mustIncident(id);
    return delay(state.stats);
  },

  // mock 模式的 AI 诊断：直接往事故里塞一条 headless-diagnoser 假设（演示交互）。
  runDiagnose: async (id) => {
    mustIncident(id);
    const h = {
      id: `hyp-mock-diag-${Date.now()}`,
      incident_id: id,
      topic: '[AI诊断] 模拟假设：示例环境资源水位接近上限（mock 诊断）',
      supporting: [],
      refuting: [],
      independence_weight: 0.6,
      confidence: 0.6,
      status: 'strengthening' as const,
      proposed_by: 'headless-diagnoser',
      updated_at: new Date().toISOString(),
      proof_trace: 'mock',
    };
    state.hypotheses.push(h);
    state.events.push({
      seq: state.events.length + 1,
      incident_id: id,
      type: 'hypothesis.posted',
      at: new Date().toISOString(),
      actor: 'headless-diagnoser',
      summary: h.topic,
      ref_id: h.id,
    });
    emit({
      incident_id: id,
      type: 'hypothesis.posted',
      actor: 'headless-diagnoser',
      summary: h.topic,
      ref_id: h.id,
    });
    return { ok: true };
  },

  listEvents: (id) =>
    delay(state.events.filter((e) => e.incident_id === id).sort((a, b) => a.seq - b.seq)),

  subscribeEvents: (id, handler) => {
    let subs = listeners.get(id);
    if (!subs) {
      subs = new Set();
      listeners.set(id, subs);
    }
    subs.add(handler);
    return () => {
      subs.delete(handler);
    };
  },

  listKnowledge: (_id) => delay([] as KnowledgeHit[]),
};
