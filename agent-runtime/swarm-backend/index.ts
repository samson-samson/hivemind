/**
 * OpsHive Swarm Backend —— Agent Swarms 的可插拔后端之一。
 *
 * 作用：teammates（会议室里的 agents）真实执行（in-process/tmux），
 * 同时把 SendMessage + Mailbox 消息**镜像**到 OpsHive 控制平面：
 *   - structured 消息 → IOM 节点（evidence/hypothesis/fact/guidance）
 *   - 纯文本消息 → 会议纪要（不进 IOM，聊天不进事实层）
 *
 * 对应 fork 的 backends registry（src/utils/swarm/backends/）：
 *   tmux / iterm2 / in-process / opshive ← 本实现
 *
 * 部署形态：agent-runtime 与 claude-code fork 同机（或同网），
 * 订阅 mailbox 目录 + 透传 SendMessage 载荷。
 */
import { watch } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { homedir } from 'node:os';
import {
  type OacpEnvelope,
  isFactLayer,
  isIcOnly,
} from '../protocol/schema.ts';

export interface MirrorTarget {
  /** 控制平面 base URL（如 http://localhost:8081） */
  baseUrl: string;
  /** 事故 ID（会议室）；未指定时从信封 incident_id 取 */
  incidentId?: string;
}

/** 把 OACP 信封映射为控制平面 IOM 写入 */
export async function mirrorToControlPlane(
  env: OacpEnvelope,
  target: MirrorTarget,
): Promise<{ ok: boolean; node?: string; error?: string }> {
  const incidentId = target.incidentId ?? env.incident_id;
  if (!incidentId) return { ok: false, error: 'no incident_id' };

  // 事实层过滤：非 structured 或非事实类型 → 只记录，不写 IOM
  if (!isFactLayer(env)) {
    return { ok: true, node: 'ledger-only' };
  }
  // decision/guidance 仅限 IC
  if (isIcOnly(env) && env.from !== 'ic' && !env.from.startsWith('ic:')) {
    return { ok: false, error: `type=${env.type} requires IC sender, got ${env.from}` };
  }

  const base = target.baseUrl.replace(/\/$/, '');
  let path = '';
  let body: Record<string, unknown> = {};

  switch (env.type) {
    case 'evidence':
      path = `/api/v1/incidents/${incidentId}/evidence`;
      body = {
        operation_id: (env.evidence_ref as string) ?? 'op_swarm',
        data_source: 'agent-swarms',
        result: env.content.slice(0, 2000),
        source: env.from,
      };
      break;
    case 'hypothesis':
      path = `/api/v1/incidents/${incidentId}/hypotheses`;
      body = {
        topic: env.content.slice(0, 300),
        independence_weight: 0.5,
        confidence: parseConfidence(env),
        status: 'proposed',
        source: env.from,
      };
      break;
    case 'fact':
      path = `/api/v1/incidents/${incidentId}/facts`;
      body = {
        statement: env.content.slice(0, 500),
        evidence_chain: [],
        confirmed_by: env.from,
        is_confirmed: false,
        source: env.from,
      };
      break;
    case 'guidance':
      path = `/api/v1/incidents/${incidentId}/guidance`;
      body = { from_ic: env.from, text: env.content.slice(0, 1000), priority: 2 };
      break;
    case 'decision':
      return { ok: false, error: 'decision must be issued via IC workflow' };
    default:
      return { ok: true, node: 'ledger-only' };
  }

  const res = await fetch(`${base}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) return { ok: false, error: `POST ${path} → ${res.status}` };
  const created = (await res.json()) as { id?: string };
  return { ok: true, node: created.id };
}

function parseConfidence(env: OacpEnvelope): number {
  try {
    const p = JSON.parse(env.content) as { confidence?: number };
    const c = p.confidence;
    return typeof c === 'number' && c >= 0 && c <= 1 ? c : 0.5;
  } catch {
    return 0.5;
  }
}

/** 订阅 teammate mailbox 目录，把新消息镜像到控制平面 */
export function watchMailbox(target: MirrorTarget, teamDir?: string): () => void {
  const teamsRoot = teamDir ?? join(homedir(), '.claude', 'teams');
  const watcher = watch(teamsRoot, { recursive: true }, async (event, filename) => {
    if (!filename || !String(filename).endsWith('.json')) return;
    const inboxPath = join(teamsRoot, String(filename));
    try {
      const raw = await readFile(inboxPath, 'utf8');
      const envelope = JSON.parse(raw) as OacpEnvelope;
      await mirrorToControlPlane(envelope, target);
    } catch (err) {
      console.error('[opshive-backend] mirror failed:', err instanceof Error ? err.message : err);
    }
  });
  return () => watcher.close();
}
