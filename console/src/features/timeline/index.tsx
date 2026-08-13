import { useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { api, type EventType } from '../../lib/api';
import { fmtTime } from '../../lib/format';
import { useUiStore } from '../../app/store';

/**
 * 生命周期时间线：告警 → 分诊 → 调查 → 候选。
 * 横轴在自身容器内滚动（页面不横向滚动）；点击节点联动定位到对应证据/工作单元。
 */

const TYPE_STYLE: Record<EventType, { label: string; dot: string; text: string }> = {
  'incident.created': { label: '事故', dot: 'bg-rose-400', text: 'text-rose-300' },
  'work_node.created': { label: '工作单元', dot: 'bg-sky-400', text: 'text-sky-300' },
  'work_node.updated': { label: '工作单元', dot: 'bg-sky-400', text: 'text-sky-300' },
  'lease.claimed': { label: '租约/去重', dot: 'bg-amber-400', text: 'text-amber-300' },
  'lease.heartbeat': { label: '租约/去重', dot: 'bg-amber-400', text: 'text-amber-300' },
  'lease.released': { label: '租约/去重', dot: 'bg-amber-400', text: 'text-amber-300' },
  'operation.registered': { label: '查询', dot: 'bg-amber-300', text: 'text-amber-200' },
  'evidence.appended': { label: '证据', dot: 'bg-emerald-400', text: 'text-emerald-300' },
  'fact.posted': { label: '事实确认', dot: 'bg-emerald-300', text: 'text-emerald-200' },
  'hypothesis.posted': { label: '假设', dot: 'bg-purple-400', text: 'text-purple-300' },
  'guidance.posted': { label: 'IC 决策', dot: 'bg-violet-400', text: 'text-violet-300' },
};

/** 事件 ref 前缀 → 定位目标视图（兼容 mock 连字符 id 与后端下划线 id） */
function targetView(refId: string | null): string | null {
  if (!refId) return null;
  if (/^(ev[-_]|op[-_]|fact[-_]|hyp[-_])/.test(refId)) return 'evidence';
  if (refId.startsWith('wn-') || refId.startsWith('wn_')) return 'work-graph';
  return null;
}

export function TimelineView() {
  const { incidentId = '' } = useParams();
  const navigate = useNavigate();
  const setHighlightRef = useUiStore((s) => s.setHighlightRef);

  const { data: events } = useQuery({
    queryKey: ['events', incidentId],
    queryFn: () => api.listEvents(incidentId),
  });

  const locate = (refId: string | null) => {
    const view = targetView(refId);
    if (!view || !refId) return;
    setHighlightRef(refId);
    navigate(`/incidents/${incidentId}/${view}`);
  };

  return (
    <div className="flex h-full flex-col p-5">
      <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">
        生命周期时间线（点击节点定位到对应证据 / 工作单元）
      </h2>
      <div className="mt-6 flex-1 overflow-x-auto overflow-y-hidden">
        <div className="relative flex h-full min-w-max items-start gap-0 px-2 pt-6">
          <div className="absolute left-0 right-0 top-8 h-px bg-zinc-700" />
          {(events ?? []).map((e) => {
            const st = TYPE_STYLE[e.type];
            const clickable = targetView(e.ref_id) !== null;
            return (
              <div key={e.seq} className="relative flex w-52 shrink-0 flex-col items-center px-2">
                <span className="absolute -top-1 font-mono text-[10px] text-zinc-500">{fmtTime(e.at)}</span>
                <button
                  onClick={() => locate(e.ref_id)}
                  disabled={!clickable}
                  className={`relative z-10 mt-4 h-3.5 w-3.5 rounded-full border-2 border-zinc-950 ${st.dot} ${
                    clickable ? 'cursor-pointer hover:scale-125' : 'cursor-default opacity-80'
                  } transition-transform`}
                  title={clickable ? '点击定位' : undefined}
                />
                <div className="mt-3 w-full rounded-lg border border-zinc-800 bg-zinc-900/60 p-2.5">
                  <div className="flex items-center justify-between">
                    <span className={`text-[10px] font-semibold ${st.text}`}>{st.label}</span>
                    <span className="font-mono text-[9px] text-zinc-600">#{e.seq}</span>
                  </div>
                  <div className="mt-0.5 text-[10px] text-sky-400">{e.actor}</div>
                  <p className="mt-1 text-[11px] leading-snug text-zinc-300">{e.summary}</p>
                  {clickable && (
                    <div className="mt-1.5 font-mono text-[9px] text-zinc-600">→ {e.ref_id}</div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
      <div className="mt-4 flex flex-wrap gap-3 border-t border-zinc-800/60 pt-3 text-[10px] text-zinc-500">
        {(Object.keys(TYPE_STYLE) as EventType[]).map((t) => (
          <span key={t} className="flex items-center gap-1">
            <span className={`h-2 w-2 rounded-full ${TYPE_STYLE[t].dot}`} />
            {TYPE_STYLE[t].label}
          </span>
        ))}
      </div>
    </div>
  );
}
