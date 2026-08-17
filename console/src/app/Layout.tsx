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
    <div className="flywheel">
      <div className="flywheel-main" title="上下文飞轮：已确认事实占全部问题之比">
        <div
          className="flywheel-ring flywheel-spin"
          style={{
            background: `conic-gradient(var(--ok) ${ratio * 360}deg, var(--surface-3) ${ratio * 360}deg)`,
            mask: 'radial-gradient(circle, transparent 48%, black 50%)',
            WebkitMask: 'radial-gradient(circle, transparent 48%, black 50%)',
          }}
        />
        <div>
          <div className="flywheel-label">上下文飞轮</div>
          <div className="flywheel-value" style={{ color: 'var(--ok)' }}>{fmtPct(ratio)} 已收敛</div>
        </div>
      </div>
      <Stat label="已确认事实" value={String(stats.facts_confirmed)} tone="var(--ok)" />
      <Stat label="未解问题" value={String(stats.open_questions)} tone="var(--warn)" />
      <Stat label="决策延迟" value={`${stats.decision_latency_min}m`} tone="var(--agent)" />
      <Stat label="去重率" value={fmtPct(stats.dedup_rate)} tone="var(--hyp)" />
    </div>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone: string }) {
  return (
    <div className="shell-stat">
      <div className="shell-stat-label">{label}</div>
      <div className="shell-stat-value" style={{ color: tone }}>{value}</div>
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
      className="theme-toggle"
    >
      {theme === 'dark' ? '浅色模式' : '深色模式'}
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
    <div className="console-shell">
      <IncidentSidebar activeId={incidentId} />
      <div className="console-content">
        <header className="console-header">
          <div className="console-heading">
            <div className="console-title-row">
              {incident && (
                <>
                  <span className={`rounded border px-1.5 py-0.5 text-[10px] font-bold ${SEVERITY_STYLE[incident.severity]}`}>
                    {incident.severity}
                  </span>
                  <h1 className="console-title">{incident.title}</h1>
                  <span className="console-status">
                    <span className={`console-status-dot ${STATUS_COLOR[incident.status]}`} />
                    {STATUS_LABEL[incident.status]}
                  </span>
                  <span className="console-ic">IC {incident.ic_name}</span>
                </>
              )}
            </div>
            <nav className="console-nav">
              {TABS.map((t) => (
                <NavLink
                  key={t.to}
                  to={t.to}
                  className={({ isActive }) => `console-nav-link${isActive ? ' active' : ''}`}
                >
                  {t.label}
                </NavLink>
              ))}
            </nav>
          </div>
          <Flywheel incidentId={incidentId} />
          <ThemeToggle />
        </header>
        <main className="shell-main">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
