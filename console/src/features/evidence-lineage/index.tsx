import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import ReactFlow, {
  Background,
  Controls,
  Handle,
  MiniMap,
  Position,
  type Edge,
  type Node,
  type NodeProps,
} from 'reactflow';
import 'reactflow/dist/style.css';
import {
  api,
  type Evidence,
  type Fact,
  type Hypothesis,
  type Operation,
} from '../../lib/api';
import { layoutGraph } from '../../lib/layout';
import { fmtTime, HYP_STATUS } from '../../lib/format';
import { useUiStore } from '../../app/store';

/**
 * 证据血缘图：Operation → Evidence → Fact / Hypothesis。
 * - Evidence 边框颜色按独立性评分着色（低 = 琥珀，高 = 翠绿）
 * - Fact（已确认）= 实底翠绿；Hypothesis = 虚线边框 + 置信度
 * - supports = 实线绿边，refutes = 虚线红边
 * - 节点带生长动画（追加式入场）
 */

type NodeData =
  | { kind: 'op'; op: Operation; width: number; height: number; idx: number; highlight: boolean }
  | { kind: 'ev'; ev: Evidence; width: number; height: number; idx: number; highlight: boolean }
  | { kind: 'fact'; fact: Fact; width: number; height: number; idx: number; highlight: boolean }
  | { kind: 'hyp'; hyp: Hypothesis; width: number; height: number; idx: number; highlight: boolean };

const independenceColor = (score: number) =>
  score >= 0.75 ? 'border-emerald-500' : score >= 0.5 ? 'border-amber-500' : 'border-rose-500';

function LineageNode({ data }: NodeProps<NodeData>) {
  const delay = `${data.idx * 120}ms`;
  const ring = data.highlight ? 'ring-2 ring-amber-400' : '';
  const inner = (() => {
    switch (data.kind) {
      case 'op': {
        const op = data.op;
        return (
          <div className={`rounded-lg border border-zinc-600 bg-zinc-900 p-2 ${ring}`} style={{ width: data.width }}>
            <div className="text-[9px] uppercase tracking-wider text-zinc-500">
              Operation · {op.data_source}
              {op.dedup_status !== 'fresh' && (
                <span className="ml-1 rounded bg-amber-500/20 px-1 text-amber-300">
                  {op.dedup_status === 'single_flight' ? '已去重' : '已复用'}
                </span>
              )}
            </div>
            <div className="mt-0.5 line-clamp-2 text-[10px] text-zinc-300">{op.summary}</div>
            <div className="mt-0.5 text-[9px] text-zinc-500">
              {op.registered_by} · {fmtTime(op.registered_at)}
            </div>
          </div>
        );
      }
      case 'ev': {
        const ev = data.ev;
        return (
          <div
            className={`rounded-lg border-2 bg-zinc-900 p-2 ${independenceColor(ev.independence_score)} ${ring}`}
            style={{ width: data.width }}
            title={`独立性评分 ${ev.independence_score.toFixed(2)}（按采集链路/数据源计算）`}
          >
            <div className="flex items-center justify-between text-[9px] text-zinc-500">
              <span>Evidence · {ev.source}</span>
              <span className="font-mono">独立性 {ev.independence_score.toFixed(2)}</span>
            </div>
            <div className="mt-0.5 line-clamp-2 text-[10px] text-zinc-200">{ev.summary}</div>
          </div>
        );
      }
      case 'fact':
        return (
          <div
            className={`rounded-lg border-2 border-emerald-400 bg-emerald-500/15 p-2 ${ring}`}
            style={{ width: data.width }}
          >
            <div className="text-[9px] font-semibold uppercase tracking-wider text-emerald-300">
              Fact · 已确认事实
            </div>
            <div className="mt-0.5 line-clamp-3 text-[10px] font-medium text-emerald-100">
              {data.fact.statement}
            </div>
            <div className="mt-0.5 text-[9px] text-emerald-400/70">确认人：{data.fact.confirmed_by}</div>
          </div>
        );
      case 'hyp': {
        const hyp = data.hyp;
        const st = HYP_STATUS[hyp.status];
        return (
          <div
            className={`rounded-lg border-2 border-dashed border-violet-400 bg-violet-500/10 p-2 ${ring}`}
            style={{ width: data.width }}
          >
            <div className="flex items-center justify-between text-[9px]">
              <span className="font-semibold uppercase tracking-wider text-violet-300">Hypothesis · 假设</span>
              <span className={st.cls}>{st.label}</span>
            </div>
            <div className="mt-0.5 line-clamp-3 text-[10px] text-violet-100">{hyp.topic}</div>
            <div className="mt-1 flex items-center gap-1">
              <div className="h-1 flex-1 overflow-hidden rounded bg-zinc-800">
                <div
                  className="h-full rounded bg-violet-400"
                  style={{ width: `${hyp.confidence * 100}%` }}
                />
              </div>
              <span className="font-mono text-[9px] text-violet-300">
                {(hyp.confidence * 100).toFixed(0)}%
              </span>
            </div>
          </div>
        );
      }
    }
  })();
  return (
    <div className="lineage-node" style={{ animationDelay: delay }}>
      <Handle type="target" position={Position.Left} className="!bg-zinc-600" />
      {inner}
      <Handle type="source" position={Position.Right} className="!bg-zinc-600" />
    </div>
  );
}

const nodeTypes = { lineage: LineageNode };

export function EvidenceLineageView() {
  const { incidentId = '' } = useParams();
  const highlightRef = useUiStore((s) => s.highlightRef);

  const { data: operations } = useQuery({
    queryKey: ['operations', incidentId],
    queryFn: () => api.listOperations(incidentId),
  });
  const { data: evidence } = useQuery({
    queryKey: ['evidence', incidentId],
    queryFn: () => api.listEvidence(incidentId),
  });
  const { data: facts } = useQuery({
    queryKey: ['facts', incidentId],
    queryFn: () => api.listFacts(incidentId),
  });
  const { data: hypotheses } = useQuery({
    queryKey: ['hypotheses', incidentId],
    queryFn: () => api.listHypotheses(incidentId),
  });

  const { flowNodes, edges } = useMemo(() => {
    const nodes: Node<NodeData>[] = [];
    const edges: Edge[] = [];
    let idx = 0;
    const hl = (id: string) => highlightRef === id;

    for (const op of operations ?? []) {
      nodes.push({
        id: op.id,
        type: 'lineage',
        position: { x: 0, y: 0 },
        data: { kind: 'op', op, width: 210, height: 84, idx: idx++, highlight: hl(op.id) },
      });
    }
    for (const ev of evidence ?? []) {
      nodes.push({
        id: ev.id,
        type: 'lineage',
        position: { x: 0, y: 0 },
        data: { kind: 'ev', ev, width: 240, height: 78, idx: idx++, highlight: hl(ev.id) },
      });
      edges.push({ id: `${ev.operation_id}->${ev.id}`, source: ev.operation_id, target: ev.id });
      for (const p of ev.parent_ids) {
        edges.push({
          id: `${p}->${ev.id}`,
          source: p,
          target: ev.id,
          style: { stroke: '#71717a', strokeDasharray: '4 3' },
          label: 'derived_from',
          labelStyle: { fill: '#71717a', fontSize: 8 },
          labelBgStyle: { fill: '#09090b' },
        });
      }
    }
    for (const f of facts ?? []) {
      nodes.push({
        id: f.id,
        type: 'lineage',
        position: { x: 0, y: 0 },
        data: { kind: 'fact', fact: f, width: 230, height: 96, idx: idx++, highlight: hl(f.id) },
      });
      for (const evId of f.evidence_ids) {
        edges.push({
          id: `${evId}->${f.id}`,
          source: evId,
          target: f.id,
          animated: true,
          style: { stroke: '#34d399' },
        });
      }
    }
    for (const h of hypotheses ?? []) {
      nodes.push({
        id: h.id,
        type: 'lineage',
        position: { x: 0, y: 0 },
        data: { kind: 'hyp', hyp: h, width: 250, height: 110, idx: idx++, highlight: hl(h.id) },
      });
      for (const evId of h.supporting) {
        edges.push({
          id: `${evId}-supports-${h.id}`,
          source: evId,
          target: h.id,
          label: 'supports',
          labelStyle: { fill: '#34d399', fontSize: 8 },
          labelBgStyle: { fill: '#09090b' },
          style: { stroke: '#34d399' },
        });
      }
      for (const evId of h.refuting) {
        edges.push({
          id: `${evId}-refutes-${h.id}`,
          source: evId,
          target: h.id,
          label: 'refutes',
          labelStyle: { fill: '#fb7185', fontSize: 8 },
          labelBgStyle: { fill: '#09090b' },
          style: { stroke: '#fb7185', strokeDasharray: '5 4' },
        });
      }
    }
    return { flowNodes: layoutGraph(nodes, edges, 'LR'), edges };
  }, [operations, evidence, facts, hypotheses, highlightRef]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-zinc-800/60 px-4 py-2 text-[10px] text-zinc-500">
        <span>Operation → Evidence → Fact / Hypothesis（生长式追加）</span>
        <span className="text-emerald-300">■ 事实（已确认）</span>
        <span className="text-violet-300">▨ 假设（虚线 + 置信度）</span>
        <span className="text-emerald-400">— supports</span>
        <span className="text-rose-400">┅ refutes</span>
        <span>证据边框 = 独立性评分（绿 ≥0.75 / 琥珀 ≥0.5 / 红 &lt;0.5）</span>
      </div>
      <div className="min-h-0 flex-1">
        <ReactFlow nodes={flowNodes} edges={edges} nodeTypes={nodeTypes} fitView proOptions={{ hideAttribution: true }} nodesConnectable={false}>
          <Background color="#27272a" gap={24} />
          <Controls showInteractive={false} />
          <MiniMap pannable zoomable nodeColor="#3f3f46" maskColor="rgba(9,9,11,0.7)" />
        </ReactFlow>
      </div>
    </div>
  );
}
