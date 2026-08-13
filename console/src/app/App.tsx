import { useCallback, useEffect, useState } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { api } from '../lib/api';
import { ConsoleLayout } from './Layout';
import { MeetingRoom } from '../features/meeting-room';
import { IncidentOverview } from '../features/incident-list';
import { WorkGraphView } from '../features/work-graph';
import { EvidenceLineageView } from '../features/evidence-lineage';
import { SwarmView } from '../features/swarm-view';
import { DeliberationView } from '../features/deliberation';
import { TimelineView } from '../features/timeline';
import { KnowledgePanel } from '../features/knowledge-panel';
import { IcControls } from '../features/ic-controls';

/** 根路径：加载事故列表后跳第一个真实事故（替代硬编码 mock id）。 */
function DefaultRedirect() {
  const [firstId, setFirstId] = useState<string | null>(null);
  const [empty, setEmpty] = useState(false);

  const load = useCallback(() => {
    api
      .listIncidents()
      .then((xs) => {
        if (xs.length > 0) setFirstId(xs[0].id);
        else setEmpty(true);
      })
      .catch(() => setEmpty(true));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  if (firstId) return <Navigate to={`/incidents/${firstId}`} replace />;
  if (empty) return <div style={{ padding: 48, color: 'var(--text-dim)' }}>暂无事故。请先在后端创建事故。</div>;
  return <div style={{ padding: 48, color: 'var(--text-dim)' }}>加载中…</div>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<DefaultRedirect />} />
      <Route path="/incidents/:incidentId" element={<ConsoleLayout />}>
        <Route index element={<MeetingRoom />} />
        <Route path="meeting" element={<MeetingRoom />} />
        <Route path="work-graph" element={<WorkGraphView />} />
        <Route path="evidence" element={<EvidenceLineageView />} />
        <Route path="overview" element={<IncidentOverview />} />
        <Route path="swarm" element={<SwarmView />} />
        <Route path="deliberation" element={<DeliberationView />} />
        <Route path="timeline" element={<TimelineView />} />
        <Route path="knowledge" element={<KnowledgePanel />} />
        <Route path="controls" element={<IcControls />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
