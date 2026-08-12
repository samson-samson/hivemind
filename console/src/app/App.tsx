import { Navigate, Route, Routes } from 'react-router-dom';
import { ConsoleLayout } from './Layout';
import { IncidentOverview } from '../features/incident-list';
import { WorkGraphView } from '../features/work-graph';
import { EvidenceLineageView } from '../features/evidence-lineage';
import { SwarmView } from '../features/swarm-view';
import { DeliberationView } from '../features/deliberation';
import { TimelineView } from '../features/timeline';
import { KnowledgePanel } from '../features/knowledge-panel';
import { IcControls } from '../features/ic-controls';

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/incidents/inc-20260812-001/overview" replace />} />
      <Route path="/incidents/:incidentId" element={<ConsoleLayout />}>
        <Route index element={<Navigate to="overview" replace />} />
        <Route path="overview" element={<IncidentOverview />} />
        <Route path="work-graph" element={<WorkGraphView />} />
        <Route path="evidence" element={<EvidenceLineageView />} />
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
