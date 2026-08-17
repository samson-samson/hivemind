import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { NavLink, Outlet, useParams } from 'react-router-dom';
import { api } from '../lib/api';
import { SEVERITY_STYLE, STATUS_COLOR, STATUS_LABEL, fmtPct } from '../lib/format';
import { IncidentSidebar } from '../features/incident-list';

/** 主导航：会议室为默认主视图，其余收敛为深层入口（保留路由，不占主导航）。 */
const TABS = [
  { to: 'meeting', label: '会议室' },
  { to: 'work-graph', label: '工作图' },
  { to: 'evidence', label: '证据血缘' },
  { to: 'controls', label: 'IC 操作' },
];

/** 上下文飞轮指示器：已确认事实 / 未解问题 / 决策延迟 / 去重率 */
function Flywheel({ incidentId }: { incidentId: string }) {
  const { data: stats } = useQuery({
    queryKey: ['stats', incidentId],
    queryFn: () => api.getStats(incidentId),
  });
  if (!stats) return null;
  const total = stats.facts_confirmed + stats.open_questions;
  const ratio = total === 0 ? 0 : stats.facts_confirmed / total;
  return (
    <div className="flex items-center gap-4 text-xs">
      <div className="flex items-center gap-2" title="上下文飞轮：已确认事实占全部问题之比">
        <div
          className="flywheel-spin h-7 w-7 rounded-full"
          style={{
            background: `conic-gradient(#34d399 ${ratio * 360}deg, #3f3f46 ${ratio * 360}deg)`,
            mask: 'radial-gradient(circle, transparent 45%, black 46%)',
            WebkitMask: 'radial-gradient(circle, transparent 45%, black 46%)',
          }}
        />
        <div className="leading-tight">
          <div className="text-zinc-400">上下文飞轮</div>
          <div className="text-emerald-300 font-medium">{fmtPct(ratio)} 已收敛</div>
        </div>
      </div>
      <Stat label="已确认事实" value={String(stats.facts_confirmed)} tone="text-emerald-300" />
      <Stat label="未解问题" value={String(stats.open_questions)} tone="text-amber-300" />
      <Stat label="决策延迟" value={`${stats.decision_latency_min}m`} tone="text-sky-300" />
      <Stat label="去重率" value={fmtPct(stats.dedup_rate)} tone="text-violet-300" />
    </div>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone: string }) {
  return (
    <div className="leading-tight">
      <div className="text-zinc-500">{label}</div>
      <div className={`font-mono font-medium ${tone}`}>{value}</div>
    </div>
  );
}

function ThemeToggle() {
  const [theme, setTheme] = useState<'dark' | 'light'>(() =>
    (localStorage.getItem('hivemind-theme') as 'dark' | 'light') ?? 'dark',
  );
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('hivemind-theme', theme);
  }, [theme]);
  return (
    <button
      onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
      title="切换深浅主题"
      className="rounded border border-zinc-700 px-2 py-1 font-mono text-[10px] text-zinc-400 hover:bg-zinc-800"
    >
      {theme === 'dark' ? '◐ 浅色' : '◐ 深色'}
    </button>
  );
}

export function ConsoleLayout() {
  const { incidentId = '' } = useParams();
  const { data: incident } = useQuery({
    queryKey: ['incident', incidentId],
    queryFn: () => api.getIncident(incidentId),
  });

  return (
    <div className="flex h-full">
      <IncidentSidebar activeId={incidentId} />
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between gap-4 border-b border-zinc-800 px-5 py-2.5">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              {incident && (
                <>
                  <span className={`rounded border px-1.5 py-0.5 text-[10px] font-bold ${SEVERITY_STYLE[incident.severity]}`}>
                    {incident.severity}
                  </span>
                  <h1 className="truncate text-sm font-semibold text-zinc-100">{incident.title}</h1>
                  <span className="flex items-center gap-1 text-xs text-zinc-400">
                    <span className={`h-1.5 w-1.5 rounded-full ${STATUS_COLOR[incident.status]}`} />
                    {STATUS_LABEL[incident.status]}
                  </span>
                  <span className="text-xs text-zinc-500">IC：{incident.ic_name}</span>
                </>
              )}
            </div>
            <nav className="mt-1.5 flex gap-1">
              {TABS.map((t) => (
                <NavLink
                  key={t.to}
                  to={t.to}
                  className={({ isActive }) =>
                    `rounded px-2 py-1 text-xs transition-colors ${
                      isActive
                        ? 'bg-zinc-800 text-zinc-100'
                        : 'text-zinc-500 hover:bg-zinc-900 hover:text-zinc-300'
                    }`
                  }
                >
                  {t.label}
                </NavLink>
              ))}
            </nav>
          </div>
          <Flywheel incidentId={incidentId} />
          <ThemeToggle />
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
