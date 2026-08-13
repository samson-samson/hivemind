import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { api } from '../../lib/api';
import type { IncidentEvent } from '../../lib/api';
import { GUIDANCE_STYLE, HYP_STATUS, fmtTime } from '../../lib/format';

/**
 * 会议室 —— 一个事故一间会议室，所有 agent 的协同诊断汇聚一屏：
 *   顶部事故条（状态 + 上下文飞轮）
 *   左栏 agent 状态（谁在线、什么角色、在查什么、租约状态）
 *   中间协同诊断流（证据/事实/假设/IC 发言按时间合并，像会议纪要）
 *   右栏假设面板（带置信度与冲突标记）
 * 设计原则：聊天不进事实层——流里每一条都是结构化产出，不是闲聊。
 */
function eventActor(e: IncidentEvent): { name: string; kind: string } {
  if (e.type === 'guidance.posted') return { name: e.actor, kind: 'ic' };
  if (e.type === 'fact.posted') return { name: e.actor, kind: 'fact' };
  if (e.type === 'work_node.created') return { name: e.actor, kind: 'agent' };
  return { name: e.actor, kind: 'agent' };
}

function EventCard({ e }: { e: IncidentEvent }) {
  const { name, kind } = eventActor(e);
  const style: Record<string, { chip: string; dot: string }> = {
    ic: { chip: 'bg-violet-500/20 text-violet-300', dot: 'bg-violet-400' },
    fact: { chip: 'bg-emerald-500/20 text-emerald-300', dot: 'bg-emerald-400' },
    agent: { chip: 'bg-sky-500/20 text-sky-300', dot: 'bg-sky-400' },
  };
  const st = style[kind] ?? style.agent;
  const label: Record<string, string> = {
    'guidance.posted': 'IC 指示',
    'fact.posted': '事实确认',
    'evidence.appended': '提交证据',
    'hypothesis.posted': '提出假设',
    'work_node.created': '领取任务',
    'work_node.updated': '更新任务',
    'operation.registered': '发起查询',
    'lease.claimed': '认领租约',
    'lease.released': '释放租约',
    'incident.created': '事故创建',
  };

  return (
    <li className="flex gap-3">
      <div className="flex flex-col items-center">
        <span className={`mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full ${st.dot}`} />
        <span className="mt-1 w-px flex-1 bg-zinc-800" />
      </div>
      <div className="mb-4 min-w-0 flex-1 rounded-lg border border-zinc-800 bg-zinc-900/50 p-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${st.chip}`}>{label[e.type] ?? e.type}</span>
          <span className="text-[11px] font-medium text-zinc-200">{name}</span>
          <span className="ml-auto font-mono text-[10px] text-zinc-500">{fmtTime(e.at)}</span>
        </div>
        <p className="mt-1.5 text-xs leading-relaxed text-zinc-300">{e.summary}</p>
      </div>
    </li>
  );
}

export function MeetingRoom() {
  const { incidentId = '' } = useParams();

  // 10s 轮询：让 headless-diagnoser 等 agent 提交的假设/证据实时进会议室。
  // （SSE 就绪后此处可移除，EventSource 已预留。）
  const REFRESH = { refetchInterval: 10_000 };
  const { data: incident } = useQuery({
    queryKey: ['incident', incidentId],
    queryFn: () => api.getIncident(incidentId),
    ...REFRESH,
  });
  const { data: stats } = useQuery({
    queryKey: ['stats', incidentId],
    queryFn: () => api.getStats(incidentId),
    ...REFRESH,
  });
  const { data: events } = useQuery({
    queryKey: ['events', incidentId],
    queryFn: () => api.listEvents(incidentId),
    ...REFRESH,
  });
  const { data: leases } = useQuery({
    queryKey: ['leases', incidentId],
    queryFn: () => api.listLeases(incidentId),
    ...REFRESH,
  });
  const { data: hypotheses } = useQuery({
    queryKey: ['hypotheses', incidentId],
    queryFn: () => api.listHypotheses(incidentId),
    ...REFRESH,
  });
  const { data: guidance } = useQuery({
    queryKey: ['guidance', incidentId],
    queryFn: () => api.listGuidance(incidentId),
    ...REFRESH,
  });

  // ---- 行动区（让会议室"用起来"） ----
  const queryClient = useQueryClient();
  const [guideText, setGuideText] = useState('');
  const [lastDiag, setLastDiag] = useState('');
  const diagnose = useMutation({
    mutationFn: () => api.runDiagnose(incidentId),
    onSuccess: () => {
      setLastDiag('AI 诊断完成：headless-diagnoser 已提交假设（10s 内出现在右侧面板）');
      queryClient.invalidateQueries({ queryKey: ['hypotheses', incidentId] });
      queryClient.invalidateQueries({ queryKey: ['events', incidentId] });
    },
    onError: (e: Error) => setLastDiag(`诊断失败：${e.message.slice(0, 80)}`),
  });
  const postGuidance = useMutation({
    mutationFn: (text: string) => api.createGuidance(incidentId, { from_ic: incident?.ic_name ?? 'ic', text, priority: 'directive' }),
    onSuccess: () => {
      setGuideText('');
      queryClient.invalidateQueries({ queryKey: ['guidance', incidentId] });
      queryClient.invalidateQueries({ queryKey: ['events', incidentId] });
    },
  });

  const agents = new Map<string, { roles: string[]; leases: string[]; active: boolean }>();
  for (const l of leases ?? []) {
    const a = agents.get(l.holder) ?? { roles: [], leases: [], active: false };
    a.leases.push(l.work_node_id);
    a.active = a.active || l.status === 'active';
    agents.set(l.holder, a);
  }

  const flow = [...(events ?? [])].sort((a, b) => a.at.localeCompare(b.at));

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* 顶部事故条 */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-zinc-800/70 px-5 py-2.5">
        <span className="rounded bg-rose-500/15 px-1.5 py-0.5 text-[10px] font-bold text-rose-300">
          {incident?.severity ?? 'P?'}
        </span>
        <h1 className="text-sm font-semibold text-zinc-100">{incident?.title ?? '加载中…'}</h1>
        <span className="rounded border border-zinc-700 px-1.5 py-0.5 text-[10px] text-zinc-400">
          {incident?.status ?? '…'}
        </span>
        <span className="text-[10px] text-zinc-500">IC：{incident?.ic_name ?? '—'}</span>
        <span className="ml-auto flex items-center gap-3 text-[10px] text-zinc-400">
          <span>
            事实 <span className="font-mono text-emerald-300">{stats?.facts_confirmed ?? 0}</span>
          </span>
          <span>
            未解 <span className="font-mono text-amber-300">{stats?.open_questions ?? 0}</span>
          </span>
          <span>
            去重 <span className="font-mono text-sky-300">{((stats?.dedup_rate ?? 0) * 100).toFixed(0)}%</span>
          </span>
          <span>
            决策延迟 <span className="font-mono text-violet-300">{stats?.decision_latency_min ?? 0}m</span>
          </span>
        </span>
      </div>

      {/* 三栏主体 */}
      <div className="grid min-h-0 flex-1 grid-cols-12">
        {/* 左：agent 状态 */}
        <aside className="col-span-2 min-w-0 border-r border-zinc-800/70 p-3">
          <h3 className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500">与会 Agent</h3>
          <ul className="mt-2 space-y-2">
            {[...agents.entries()].map(([name, a]) => (
              <li key={name} className="rounded-lg border border-zinc-800 bg-zinc-900/40 p-2">
                <div className="flex items-center gap-1.5">
                  <span className={`h-1.5 w-1.5 rounded-full ${a.active ? 'bg-emerald-400' : 'bg-zinc-600'}`} />
                  <span className="truncate text-[11px] font-medium text-zinc-200">{name}</span>
                </div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {a.roles.length === 0 && (
                    <span className="rounded bg-zinc-800 px-1 py-0.5 text-[9px] text-zinc-400">role—</span>
                  )}
                </div>
                <div className="mt-1 text-[9px] text-zinc-500">
                  {a.active ? `在查 ${a.leases.length} 个工作单元` : '空闲'}
                </div>
              </li>
            ))}
            {agents.size === 0 && (
              <li className="text-[10px] text-zinc-600">暂无 agent 在线（租约为空）</li>
            )}
          </ul>
        </aside>

        {/* 中：协同诊断流（会议纪要） */}
        <main className="col-span-7 min-h-0 overflow-y-auto border-r border-zinc-800/70 px-5 py-3">
          <div className="flex items-center justify-between">
            <h3 className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500">
              协同诊断流（{flow.length}）
            </h3>
            <span className="text-[9px] text-zinc-600">只有结构化证据与决策改变事故状态</span>
          </div>
          <ul className="mt-3">
            {flow.map((e) => (
              <EventCard key={e.seq} e={e} />
            ))}
            {flow.length === 0 && (
              <li className="py-8 text-center text-xs text-zinc-600">会议室暂无活动，等待 agent 进场…</li>
            )}
          </ul>
        </main>

        {/* 右：假设面板 */}
        <aside className="col-span-3 min-w-0 overflow-y-auto p-3">
          <h3 className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500">根因假设</h3>
          <p className="mt-1 rounded border border-amber-500/30 bg-amber-500/5 px-2 py-1 text-[9px] leading-snug text-amber-200/80">
            AI 仅做根因定位与辅助分析；止血/修复方案由 IC 决策
          </p>
          <ul className="mt-2 space-y-2">
            {[...(hypotheses ?? [])]
              .sort((a, b) => b.confidence - a.confidence)
              .map((h) => {
                const st = HYP_STATUS[h.status];
                const hasConflict = h.supporting.length > 0 && h.refuting.length > 0;
                return (
                  <li
                    key={h.id}
                    className={`rounded-lg border p-2.5 ${
                      h.status === 'refuted' ? 'border-zinc-800 bg-zinc-900/30 opacity-60' : 'border-violet-500/30 bg-violet-500/5'
                    }`}
                  >
                    <div className="flex items-center gap-1.5">
                      <span className={`text-[9px] font-semibold uppercase ${st.cls}`}>{st.label}</span>
                      {hasConflict && (
                        <span className="rounded border border-rose-500/40 px-1 text-[8px] text-rose-300">冲突</span>
                      )}
                    </div>
                    <p className="mt-1 text-[11px] leading-snug text-zinc-200">{h.topic}</p>
                    <div className="mt-1.5 flex items-center gap-2">
                      <div className="h-1 flex-1 overflow-hidden rounded bg-zinc-800">
                        <div className="h-full rounded bg-violet-400" style={{ width: `${h.confidence * 100}%` }} />
                      </div>
                      <span className="font-mono text-[9px] text-violet-300">{(h.confidence * 100).toFixed(0)}%</span>
                    </div>
                    <div className="mt-1 text-[9px] text-zinc-500">
                      {h.proposed_by} · {fmtTime(h.updated_at)}
                    </div>
                  </li>
                );
              })}
            {(hypotheses ?? []).length === 0 && (
              <li className="text-[10px] text-zinc-600">暂无假设</li>
            )}
          </ul>

          {/* 行动区：一键 AI 诊断 + IC 发言 */}
          <h3 className="mt-4 text-[10px] font-semibold uppercase tracking-wider text-zinc-500">行动</h3>
          <div className="mt-2 space-y-2">
            <button
              onClick={() => diagnose.mutate()}
              disabled={diagnose.isPending}
              className="w-full rounded-lg bg-emerald-600 px-3 py-2 text-xs font-semibold text-white transition-colors hover:bg-emerald-500 disabled:cursor-wait disabled:opacity-60"
            >
              {diagnose.isPending ? 'AI 诊断中（GLM-5.2 分析真实日志…）' : '🤖 触发 AI 诊断（headless-diagnoser 进场）'}
            </button>
            {lastDiag && <p className="text-[9px] leading-snug text-zinc-500">{lastDiag}</p>}
            <textarea
              value={guideText}
              onChange={(e) => setGuideText(e.target.value)}
              placeholder="给全部 agent 的指示（IC 发言）…"
              rows={2}
              className="w-full rounded-lg border border-zinc-700 bg-zinc-900/70 px-2.5 py-2 text-xs text-zinc-200 placeholder:text-zinc-600 focus:border-emerald-500 focus:outline-none"
            />
            <button
              onClick={() => guideText.trim() && postGuidance.mutate(guideText.trim())}
              disabled={!guideText.trim() || postGuidance.isPending}
              className="w-full rounded-lg border border-violet-500/50 bg-violet-500/10 px-3 py-1.5 text-xs font-medium text-violet-300 transition-colors hover:bg-violet-500/20 disabled:cursor-not-allowed disabled:opacity-50"
            >
              发布 IC 指示
            </button>
          </div>

          <h3 className="mt-4 text-[10px] font-semibold uppercase tracking-wider text-zinc-500">IC 发言</h3>
          <ul className="mt-2 space-y-2">
            {(guidance ?? []).map((g) => (
              <li key={g.id} className="rounded-lg border border-zinc-800 bg-zinc-900/40 p-2.5">
                <div className="flex items-center gap-1.5">
                  <span className={`rounded border px-1 py-0.5 text-[8px] ${GUIDANCE_STYLE[g.priority]}`}>
                    {g.priority === 'urgent' ? '紧急' : g.priority === 'directive' ? '指令' : '知会'}
                  </span>
                  <span className="text-[9px] text-zinc-500">{g.from_ic}</span>
                </div>
                <p className="mt-1 text-[11px] leading-snug text-zinc-200">{g.text}</p>
              </li>
            ))}
            {(guidance ?? []).length === 0 && <li className="text-[10px] text-zinc-600">IC 尚未发言</li>}
          </ul>
        </aside>
      </div>
    </div>
  );
}
