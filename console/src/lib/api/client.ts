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
} from './types';

/**
 * API client 接口 —— 路径严格对齐后端简报 §5：
 *
 *   POST   /api/v1/incidents
 *   GET    /api/v1/incidents
 *   GET    /api/v1/incidents/{id}
 *   GET    /api/v1/incidents/{id}/context@vN
 *   POST   /api/v1/incidents/{id}/work-nodes
 *   GET    /api/v1/incidents/{id}/work-nodes
 *   POST   /api/v1/incidents/{id}/leases
 *   POST   /api/v1/incidents/{id}/leases/{lid}/heartbeat
 *   DELETE /api/v1/incidents/{id}/leases/{lid}
 *   POST   /api/v1/incidents/{id}/operations
 *   POST   /api/v1/incidents/{id}/evidence
 *   GET    /api/v1/incidents/{id}/evidence
 *   POST   /api/v1/incidents/{id}/guidance
 *   GET    /api/v1/incidents/{id}/stats
 *   GET    /api/v1/incidents/{id}/events          (SSE)
 *   GET    /api/v1/incidents/{id}/knowledge       (P1 提供，P0 留接口)
 *
 * P0 由 mock 适配器实现；后端就绪后以 openapi-typescript 生成的 client
 * 实现同一接口，一处替换即可。
 */
export interface HivemindApi {
  // 事故
  listIncidents(): Promise<Incident[]>;
  getIncident(id: ID): Promise<Incident>;
  createIncident(input: CreateIncidentInput): Promise<Incident>;
  getContext(id: ID, version?: number): Promise<ContextPack>;

  // 工作图
  listWorkNodes(id: ID): Promise<WorkNode[]>;
  createWorkNode(id: ID, input: CreateWorkNodeInput): Promise<WorkNode>;
  updateWorkNode(id: ID, nodeId: ID, patch: Partial<WorkNode>): Promise<WorkNode>;
  /** IC 拖拽调整布局（前端辅助，落库为工作图元数据） */
  saveWorkGraphLayout(id: ID, positions: Record<ID, { x: number; y: number }>): Promise<void>;

  // 租约（咨询性）
  listLeases(id: ID): Promise<Lease[]>;
  heartbeatLease(id: ID, leaseId: ID): Promise<Lease>;
  releaseLease(id: ID, leaseId: ID): Promise<void>;

  // 操作 / 证据
  listOperations(id: ID): Promise<Operation[]>;
  listEvidence(id: ID): Promise<Evidence[]>;
  listFacts(id: ID): Promise<Fact[]>;
  listHypotheses(id: ID): Promise<Hypothesis[]>;

  // IC 发言 / 决策
  listGuidance(id: ID): Promise<Guidance[]>;
  createGuidance(id: ID, input: CreateGuidanceInput): Promise<Guidance>;

  // 指标
  getStats(id: ID): Promise<IncidentStats>;

  /** 触发一次 AI 诊断：headless-diagnoser 进场提交假设（P0 联调通道） */
  runDiagnose(id: ID): Promise<{ ok: boolean; tail?: string }>;

  // 事件流：一次性快照 + SSE 增量（seq 单调递增，前端去重）
  listEvents(id: ID): Promise<IncidentEvent[]>;
  /**
   * SSE 预留：订阅 GET /api/v1/incidents/{id}/events。
   * 返回取消订阅函数。mock 适配器在本地写操作时同步推送对应事件，
   * 真实适配器将改为 EventSource 增量合并。
   */
  subscribeEvents(
    id: ID,
    handler: (event: IncidentEvent) => void,
    onError?: (err: unknown) => void,
  ): () => void;

  // 知识面板（P1）
  listKnowledge(id: ID): Promise<KnowledgeHit[]>;
}
