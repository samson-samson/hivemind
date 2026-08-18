import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import { api } from '../../lib/api';
import { SEVERITY_STYLE, STATUS_COLOR, STATUS_LABEL, fmtTime } from '../../lib/format';

/** 侧栏：活跃事故列表 */
export function IncidentSidebar({ activeId }: { activeId: string }) {
  const { data: incidents } = useQuery({ queryKey: ['incidents'], queryFn: () => api.listIncidents() });
  return (
    <aside className="flex w-64 shrink-0 flex-col border-r border-zinc-800 bg-zinc-900/40">
      <div className="border-b border-zinc-800 px-4 py-3">
        <div className="text-sm font-bold tracking-wide text-zinc-100">
          Hivemind <span className="text-emerald-400">指挥室</span>
        </div>
        <div className="mt-0.5 text-[10px] text-zinc-500">P0 · 只读协同账本</div>
      </div>
      <div className="px-3 pt-3 text-[10px] font-medium uppercase tracking-wider text-zinc-500">
        活跃事故（{incidents?.length ?? 0}）
      </div>
      <div className="mt-1 flex-1 space-y-1 overflow-y-auto p-2">
        {(incidents ?? []).map((inc) => {
          const active = inc.id === activeId;
          return (
            <Link
              key={inc.id}
              to={`/incidents/${inc.id}/overview`}
              className={`block rounded-lg border p-2.5 transition-colors ${
                active
                  ? 'border-zinc-600 bg-zinc-800/70'
                  : 'border-transparent hover:border-zinc-800 hover:bg-zinc-900'
              }`}
            >
              <div className="flex items-center gap-1.5">
                <span className={`rounded border px-1 text-[10px] font-bold ${SEVERITY_STYLE[inc.severity]}`}>
                  {inc.severity}
                </span>
                <span className="flex items-center gap-1 text-[10px] text-zinc-400">
                  <span className={`h-1.5 w-1.5 rounded-full ${STATUS_COLOR[inc.status]}`} />
                  {STATUS_LABEL[inc.status]}
                </span>
              </div>
              <div className="mt-1 line-clamp-2 text-xs font-medium text-zinc-200">{inc.title}</div>
              <div className="mt-1 text-[10px] text-zinc-500">
                IC {inc.ic_name} · {fmtTime(inc.time_range.start)} 起
              </div>
            </Link>
          );
        })}
      </div>
    </aside>
  );
}

/** 事故总览视图 */
export function IncidentOverview() {
  return (
    <div className="p-5">
      <IncidentDetail />
    </div>
  );
}

function IncidentDetail() {
  const { incidentId = '' } = useParams();
  const { data: inc } = useQuery({
    queryKey: ['incident', incidentId],
    queryFn: () => api.getIncident(incidentId),
  });
  const { data: stats } = useQuery({
    queryKey: ['stats', incidentId],
    queryFn: () => api.getStats(incidentId),
  });
  if (!inc) return <div className="text-sm text-zinc-500">加载中…</div>;

  return (
    <div className="grid max-w-5xl grid-cols-1 gap-4 lg:grid-cols-2">
      <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">事故信息</h2>
        <dl className="mt-3 space-y-2 text-sm">
          <Row k="状态" v={STATUS_LABEL[inc.status]} />
          <Row k="严重度" v={inc.severity} />
          <Row k="IC" v={inc.ic_name} />
          <Row k="时间窗" v={`${fmtTime(inc.time_range.start)} — 进行中`} />
          <Row k="来源" v={inc.source} />
        </dl>
      </section>

      <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">
          告警集（{inc.alert_ids.length}）
        </h2>
        <ul className="mt-3 space-y-1.5">
          {inc.alert_ids.map((a) => (
            <li key={a} className="rounded border border-zinc-800 bg-zinc-950 px-2.5 py-1.5 font-mono text-xs text-zinc-300">
              {a}
            </li>
          ))}
        </ul>
      </section>

      <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4 lg:col-span-2">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">症状集</h2>
        <ul className="mt-3 grid grid-cols-1 gap-2 md:grid-cols-2">
          {inc.symptom_set.map((s) => (
            <li key={s} className="rounded border border-rose-500/20 bg-rose-500/5 px-3 py-2 text-xs text-rose-200/90">
              {s}
            </li>
          ))}
        </ul>
      </section>

      {stats && (
        <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4 lg:col-span-2">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">协同指标</h2>
          <div className="mt-3 grid grid-cols-2 gap-3 md:grid-cols-4">
            <Metric label="无意重复率（已去重）" value={`${(stats.dedup_rate * 100).toFixed(0)}%`} hint={`${stats.duplicates_avoided}/${stats.operations_total} 次查询被去重`} />
            <Metric label="已确认事实" value={String(stats.facts_confirmed)} hint={`${stats.open_questions} 个问题未解`} />
            <Metric label="决策延迟" value={`${stats.decision_latency_min} min`} hint="首告警 → 最新决策" />
            <Metric label="单位成本信息增益" value={stats.info_gain_per_cost.toFixed(2)} hint="证据 bit / 工具成本" />
          </div>
        </section>
      )}
    </div>
  );
}

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="flex gap-3">
      <dt className="w-16 shrink-0 text-zinc-500">{k}</dt>
      <dd className="text-zinc-200">{v}</dd>
    </div>
  );
}

function Metric({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="rounded border border-zinc-800 bg-zinc-950 p-3">
      <div className="text-[10px] text-zinc-500">{label}</div>
      <div className="mt-1 font-mono text-lg font-semibold text-zinc-100">{value}</div>
      <div className="mt-0.5 text-[10px] text-zinc-500">{hint}</div>
    </div>
  );
}
