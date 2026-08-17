/**
 * OACP v0.1 消息信封类型定义。
 *
 * OpsHive Agent Communication Protocol —— 事实层协议：
 * 只有 structured=true 的消息才会写入 IOM（证据总线），
 * 纯文本消息仅进入会议纪要，不改变事故状态。
 *
 * 运行时：Agent Swarms（samson-samson/claude-code fork）。
 * 传输层：SendMessageTool（teammate / '*' / uds: / bridge:）+ Teammate Mailbox。
 */

/** 消息类型：structured 类别进 IOM，message/broadcast 仅会议纪要 */
export type OacpType =
  | 'message'
  | 'broadcast'
  | 'evidence'
  | 'hypothesis'
  | 'fact'
  | 'guidance'
  | 'decision'
  | 'shutdown_request'
  | 'shutdown_response'
  | 'plan_approval_response';

/** 消息发送者（agent 名 / lead / ic） */
export type OacpSender = string;

/** 收件人：teammate 名 / 广播 / uds 本地 peer / bridge 跨会话 */
export type OacpRecipient = string;

export interface OacpEnvelope {
  protocol: 'oacp/v0.1';
  /** 必填：控制平面按此路由到对应会议室 */
  incident_id: string;
  from: OacpSender;
  to: OacpRecipient;
  type: OacpType;
  request_id: string;
  timestamp: string; // ISO 8601
  /** true 时消息才进 IOM；false 仅进会议纪要 */
  structured: boolean;
  /** 消息内容（结构化消息内嵌 JSON 载荷） */
  content: string;
  /** 关联证据 ID（证据链溯源，structured 消息建议携带） */
  evidence_ref?: string | null;
}

/** 事实层消息载荷（structured=true 时 content 应为该 JSON 的序列化） */
export interface StructuredPayload {
  /** evidence：查询结果摘要；hypothesis：根因假设 */
  text: string;
  /** hypothesis 置信度 0-1 */
  confidence?: number;
  /** fact：证据链（证据 ID 列表） */
  evidence_chain?: string[];
  /** decision/guidance：动作或指示 */
  action?: string;
  /** 任意附加元数据（如 independence_weight） */
  meta?: Record<string, unknown>;
}

/** 生成信封（结构化消息的快捷构造） */
export function envelope(
  incidentId: string,
  from: string,
  to: string,
  type: OacpType,
  content: string,
  opts: { structured?: boolean; evidenceRef?: string } = {},
): OacpEnvelope {
  return {
    protocol: 'oacp/v0.1',
    incident_id: incidentId,
    from,
    to,
    type,
    request_id: `r_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
    timestamp: new Date().toISOString(),
    structured: opts.structured ?? false,
    content,
    evidence_ref: opts.evidenceRef ?? null,
  };
}

/**
 * 事实层过滤：判断一条消息是否可写入 IOM。
 * 规则：structured=true 且 type ∈ 事实层集合；decision 仅限 IC。
 */
export function isFactLayer(e: OacpEnvelope): boolean {
  if (!e.structured) return false;
  const factTypes: OacpType[] = ['evidence', 'hypothesis', 'fact', 'guidance', 'decision'];
  return factTypes.includes(e.type);
}

/** decision 只能由 IC 发出（AI 只定位不决策） */
export function isIcOnly(e: OacpEnvelope): boolean {
  return e.type === 'decision' || e.type === 'guidance';
}
