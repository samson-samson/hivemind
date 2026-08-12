import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { api } from '../../lib/api';
import { GUIDANCE_STYLE, HYP_STATUS, fmtTime } from '../../lib/format';
import { useUiStore } from '../../app/store';

/**
 * 协商视图（P0 只读结构）：提案（假设）列表 + 冲突标记 + 升级对象 + 仲裁记录（Guidance）。
 * 冲突 = 同一假设同时存在支撑与反证证据，或置信度处于中间区间需 IC 裁决。
 */
export function DeliberationView() {
  const { incidentId = '' } = useParams();
  const setHighlightRef = useUiStore((s) => s.setHighlightRef);

  const { data: hypotheses } = useQuery({
    queryKey: ['hypotheses', incidentId],
    queryFn: () => api.listHypotheses(incidentId),
  });
  const { data: guidance } = useQuery({
    queryKey: ['guidance', incidentId],
    queryFn: () => api.listGuidance(incidentId),
  });
  const { data: incident } = useQuery({
    queryKey: ['incident', incidentId],
    queryFn: () => api.getIncident(incidentId),
  });

  return (
    <div className="grid grid-cols-1 gap-4 p-5 lg:grid-cols-3">
      <section className="lg:col-span-2">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">
          提案流（谁提了什么 · 证据对比 · 冲突标记）
        </h2>
        <ul className="mt-3 space-y-3">
          {(hypotheses ?? []).map((h) => {
            const st = HYP_STATUS[h.status];
            const hasConflict = h.supporting.length > 0 && h.refuting.length > 0;
            const needsEscalation = hasConflict || (h.confidence >= 0.25 && h.confidence <= 0.7);
            return (
              <li
                key={h.id}
                className={`rounded-lg border-2 border-dashed p-4 ${
                  h.status === 'refuted'
                    ? 'border-zinc-700 bg-zinc-900/40 opacity-70'
                    : 'border-violet-500/50 bg-violet-500/5'
                }`}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded bg-violet-500/20 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-violet-300">
                    Hypothesis
                  </span>
                  <span className={`text-[10px] font-medium ${st.cls}`}>{st.label}</span>
                  <span className="text-[10px] text-zinc-500">提案人：{h.proposed_by}</span>
                  {hasConflict && (
                    <span className="rounded border border-rose-500/50 bg-rose-500/10 px-1.5 py-0.5 text-[10px] font-medium text-rose-300">
                      冲突：支撑 {h.supporting.length} / 反证 {h.refuting.length}
                    </span>
                  )}
                  {needsEscalation && incident && (
                    <span className="rounded border border-amber-500/50 bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-300">
                      升级给 IC：{incident.ic_name}
                    </span>
                  )}
                  <button
                    onClick={() => setHighlightRef(h.id)}
                    className="ml-auto rounded border border-zinc-700 px-1.5 py-0.5 text-[10px] text-zinc-400 hover:bg-zinc-800"
                  >
                    在血缘图中定位
                  </button>
                </div>
                <p className="mt-2 text-sm text-zinc-100">{h.topic}</p>
                <div className="mt-3 flex items-center gap-3">
                  <div className="h-1.5 flex-1 overflow-hidden rounded bg-zinc-800">
                    <div className="h-full rounded bg-violet-400" style={{ width: `${h.confidence * 100}%` }} />
                  </div>
                  <span className="font-mono text-xs text-violet-300">{(h.confidence * 100).toFixed(0)}%</span>
                </div>
                <div className="mt-2 flex gap-4 text-[10px] text-zinc-500">
                  <span>
                    支撑证据 <span className="text-emerald-300">{h.supporting.join(', ') || '—'}</span>
                  </span>
                  <span>
                    反证证据 <span className="text-rose-300">{h.refuting.join(', ') || '—'}</span>
                  </span>
                  <span>独立性权重 {h.independence_weight.toFixed(2)}</span>
                  <span className="ml-auto font-mono">{fmtTime(h.updated_at)} 更新</span>
                </div>
              </li>
            );
          })}
        </ul>
        <p className="mt-3 text-[10px] text-zinc-600">
          P0 只读：投票与仲裁动作在 P1。聊天不进事实层——只有结构化证据与决策改变事故状态。
        </p>
      </section>

      <section>
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">仲裁记录（IC Guidance）</h2>
        <ul className="mt-3 space-y-2.5">
          {(guidance ?? []).map((g) => (
            <li key={g.id} className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-3">
              <div className="flex items-center gap-2">
                <span className={`rounded border px-1.5 py-0.5 text-[10px] font-medium ${GUIDANCE_STYLE[g.priority]}`}>
                  {g.priority === 'directive' ? '指令' : g.priority === 'urgent' ? '紧急' : '知会'}
                </span>
                <span className="text-[10px] text-zinc-500">{g.from_ic}</span>
                <span className="ml-auto font-mono text-[10px] text-zinc-500">{fmtTime(g.created_at)}</span>
              </div>
              <p className="mt-2 text-xs leading-relaxed text-zinc-200">{g.text}</p>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
