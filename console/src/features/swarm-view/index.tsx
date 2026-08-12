import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { api, type Operation } from '../../lib/api';
import { DEDUP_STYLE, fmtTime } from '../../lib/format';
import { useUiStore } from '../../app/store';

/**
 * 协同态势：
 * 1) 无意重复热力图 —— entity × 10 分钟时间窗，被 single-flight/复用压掉的
 *    重复查询以「已去重」形式留在图上，证明去重有效。
 * 2) agent 活动流（事件驱动）
 * 3) 租约状态（心跳脉冲）
 */

const BUCKET_MIN = 10;

function bucketOf(iso: string): number {
  const d = new Date(iso);
  return Math.floor((d.getHours() * 60 + d.getMinutes()) / BUCKET_MIN);
}

function bucketLabel(b: number): string {
  const h = Math.floor((b * BUCKET_MIN) / 60);
  const m = (b * BUCKET_MIN) % 60;
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
}

interface Cell {
  total: number;
  deduped: Operation[];
  fresh: number;
}

export function SwarmView() {
  const { incidentId = '' } = useParams();
  const setHighlightRef = useUiStore((s) => s.setHighlightRef);

  const { data: operations } = useQuery({
    queryKey: ['operations', incidentId],
    queryFn: () => api.listOperations(incidentId),
  });
  const { data: events } = useQuery({
    queryKey: ['events', incidentId],
    queryFn: () => api.listEvents(incidentId),
  });
  const { data: leases } = useQuery({
    queryKey: ['leases', incidentId],
    queryFn: () => api.listLeases(incidentId),
  });

  const { entities, buckets, grid } = useMemo(() => {
    const ops = operations ?? [];
    const entities = [...new Set(ops.map((o) => o.target_entity))];
    const bucketSet = [...new Set(ops.map((o) => bucketOf(o.registered_at)))].sort((a, b) => a - b);
    const grid = new Map<string, Cell>();
    for (const op of ops) {
      const key = `${op.target_entity}#${bucketOf(op.registered_at)}`;
      const cell = grid.get(key) ?? { total: 0, deduped: [], fresh: 0 };
      cell.total += 1;
      if (op.dedup_status === 'fresh') cell.fresh += 1;
      else cell.deduped.push(op);
      grid.set(key, cell);
    }
    return { entities, buckets: bucketSet, grid };
  }, [operations]);

  const feed = useMemo(() => [...(events ?? [])].sort((a, b) => b.seq - a.seq), [events]);

  return (
    <div className="grid h-full grid-cols-1 gap-4 overflow-y-auto p-5 xl:grid-cols-3">
      {/* 热力图 */}
      <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4 xl:col-span-2">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">
          无意重复热力图（entity × 时间窗）
        </h2>
        <div className="mt-3 overflow-x-auto">
          <table className="w-full border-collapse text-[10px]">
            <thead>
              <tr>
                <th className="p-1 text-left font-medium text-zinc-500">entity</th>
                {buckets.map((b) => (
                  <th key={b} className="p-1 font-mono font-normal text-zinc-500">
                    {bucketLabel(b)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {entities.map((ent) => (
                <tr key={ent}>
                  <td className="whitespace-nowrap p-1 font-mono text-zinc-300">{ent}</td>
                  {buckets.map((b) => {
                    const cell = grid.get(`${ent}#${b}`);
                    if (!cell) {
                      return <td key={b} className="p-1"><div className="h-12 rounded bg-zinc-900/60" /></td>;
                    }
                    const intensity = Math.min(cell.total / 3, 1);
                    const hasDedup = cell.deduped.length > 0;
                    return (
                      <td key={b} className="p-1">
                        <div
                          className={`flex h-12 min-w-16 flex-col justify-center rounded border px-1.5 ${
                            hasDedup
                              ? 'border-amber-500/60 bg-amber-500/10'
                              : 'border-sky-500/30 bg-sky-500/10'
                          }`}
                          style={{ opacity: 0.55 + intensity * 0.45 }}
                          title={cell.deduped
                            .map((o) => `${o.summary}（${DEDUP_STYLE[o.dedup_status].label} → ${o.dedup_of}）`)
                            .join('\n')}
                        >
                          <span className="font-mono text-zinc-200">×{cell.total} 查询</span>
                          {hasDedup && (
                            <button
                              onClick={() => setHighlightRef(cell.deduped[0].id)}
                              className="mt-0.5 w-fit rounded bg-amber-500/25 px-1 font-medium text-amber-300 hover:bg-amber-500/40"
                            >
                              已去重 ×{cell.deduped.length}
                            </button>
                          )}
                        </div>
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="mt-3 flex gap-4 text-[10px] text-zinc-500">
          <span className="flex items-center gap-1">
            <span className="h-2.5 w-2.5 rounded border border-sky-500/40 bg-sky-500/15" /> 实查
          </span>
          <span className="flex items-center gap-1">
            <span className="h-2.5 w-2.5 rounded border border-amber-500/60 bg-amber-500/15" /> 含被去重的重复查询
          </span>
          <span className="ml-auto">被 single-flight 压掉的查询保留在图上，证明去重有效</span>
        </div>
      </section>

      {/* 租约 + 活动流 */}
      <div className="flex min-h-0 flex-col gap-4">
        <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">咨询性租约</h2>
          <ul className="mt-3 space-y-2">
            {(leases ?? []).map((l) => (
              <li
                key={l.id}
                className={`rounded border p-2.5 text-xs ${
                  l.status === 'active' ? 'lease-pulse border-emerald-500/50 bg-emerald-500/5' : 'border-zinc-800 bg-zinc-950'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="font-medium text-zinc-200">{l.holder}</span>
                  <span
                    className={`rounded px-1.5 py-0.5 text-[10px] ${
                      l.status === 'active'
                        ? 'bg-emerald-500/20 text-emerald-300'
                        : 'bg-zinc-800 text-zinc-500'
                    }`}
                  >
                    {l.status === 'active' ? '持有中' : l.status === 'released' ? '已释放' : '已过期'}
                  </span>
                </div>
                <div className="mt-1 font-mono text-[10px] text-zinc-500">
                  {l.work_node_id} · 心跳 {fmtTime(l.last_heartbeat_at)} · {fmtTime(l.expires_at)} 到期
                </div>
              </li>
            ))}
          </ul>
        </section>

        <section className="min-h-0 flex-1 rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">agent 活动流</h2>
          <ul className="mt-3 max-h-72 space-y-1.5 overflow-y-auto pr-1">
            {feed.map((e) => (
              <li key={e.seq} className="flex gap-2 text-[11px]">
                <span className="shrink-0 font-mono text-zinc-500">{fmtTime(e.at)}</span>
                <span className="shrink-0 text-sky-400">{e.actor}</span>
                <span className="text-zinc-300">{e.summary}</span>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}
