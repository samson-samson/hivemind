import { useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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
  type NodeDragHandler,
} from 'reactflow';
import 'reactflow/dist/style.css';
import { api, type WorkNode } from '../../lib/api';
import { layoutGraph } from '../../lib/layout';
import { COST_LABEL, WN_STATUS } from '../../lib/format';
import { useUiStore } from '../../app/store';

type WnData = { node: WorkNode; width: number; height: number; highlight: boolean };

const W = 260;
const H = 108;

const STATUS_BORDER: Record<WorkNode['status'], string> = {
  pending: 'border-zinc-600',
  claimed: 'border-sky-500',
  in_progress: 'border-sky-400',
  done: 'border-emerald-600',
  stale: 'border-amber-500',
  cancelled: 'border-zinc-700',
};

function WorkNodeCard({ data }: NodeProps<WnData>) {
  const { node, highlight } = data;
  const st = WN_STATUS[node.status];
  return (
    <div
      className={`rounded-lg border-2 bg-zinc-900 p-2.5 shadow-lg ${STATUS_BORDER[node.status]} ${
        highlight ? 'ring-2 ring-amber-400' : ''
      } ${node.status === 'in_progress' ? 'lease-pulse' : ''}`}
      style={{ width: W, height: H }}
    >
      <Handle type="target" position={Position.Left} className="!bg-zinc-600" />
      <div className="line-clamp-2 text-[11px] font-medium leading-snug text-zinc-100">{node.question}</div>
      <div className="mt-1 truncate font-mono text-[9px] text-zinc-500" title={node.scope}>
        {node.scope}
      </div>
      <div className="mt-1.5 flex items-center gap-1.5 text-[9px]">
        <span className={`rounded border px-1 py-px ${st.cls}`}>{st.label}</span>
        <span className="rounded border border-zinc-700 px-1 py-px text-zinc-400">{COST_LABEL[node.cost]}</span>
        <span className="ml-auto truncate text-zinc-400">{node.assignee ?? '待认领'}</span>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-zinc-600" />
    </div>
  );
}

const nodeTypes = { workNode: WorkNodeCard };

export function WorkGraphView() {
  const { incidentId = '' } = useParams();
  const queryClient = useQueryClient();
  const highlightRef = useUiStore((s) => s.highlightRef);

  const { data: nodes } = useQuery({
    queryKey: ['work-nodes', incidentId],
    queryFn: () => api.listWorkNodes(incidentId),
  });

  const layoutMutation = useMutation({
    mutationFn: (positions: Record<string, { x: number; y: number }>) =>
      api.saveWorkGraphLayout(incidentId, positions),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['work-nodes', incidentId] }),
  });

  const { flowNodes, edges } = useMemo(() => {
    const list = nodes ?? [];
    const edges: Edge[] = list.flatMap((n) =>
      n.depends_on.map((dep) => ({
        id: `${dep}->${n.id}`,
        source: dep,
        target: n.id,
        animated: n.status === 'in_progress',
      })),
    );
    const flowNodes: Node<WnData>[] = list.map((n) => ({
      id: n.id,
      type: 'workNode',
      position: { x: 0, y: 0 },
      data: { node: n, width: W, height: H, highlight: highlightRef === n.id },
    }));
    return { flowNodes: layoutGraph(flowNodes, edges, 'LR'), edges };
  }, [nodes, highlightRef]);

  const onNodeDragStop: NodeDragHandler = (_e, node) => {
    layoutMutation.mutate({ [node.id]: node.position });
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-4 border-b border-zinc-800/60 px-4 py-2 text-[10px] text-zinc-500">
        <span>节点 = 工作单元（question / 成本 / 负责人 / 租约态），边 = 依赖</span>
        <span className="text-sky-400">脉冲 = 进行中（持有租约）</span>
        <span className="ml-auto">IC 可拖拽调整布局</span>
      </div>
      <div className="min-h-0 flex-1">
        <ReactFlow
          nodes={flowNodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodeDragStop={onNodeDragStop}
          fitView
          proOptions={{ hideAttribution: true }}
          nodesConnectable={false}
        >
          <Background color="#27272a" gap={24} />
          <Controls showInteractive={false} />
          <MiniMap pannable zoomable nodeColor="#3f3f46" maskColor="rgba(9,9,11,0.7)" />
        </ReactFlow>
      </div>
    </div>
  );
}
