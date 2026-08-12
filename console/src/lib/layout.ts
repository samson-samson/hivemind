import dagre from '@dagrejs/dagre';
import type { Edge, Node } from 'reactflow';

export interface LayoutNodeData {
  width: number;
  height: number;
}

/**
 * dagre 自动布局：输入 reactflow 节点/边，返回带坐标的节点。
 */
export function layoutGraph<T extends { width: number; height: number }>(
  nodes: Node<T>[],
  edges: Edge[],
  direction: 'TB' | 'LR' = 'LR',
): Node<T>[] {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: direction, nodesep: 40, ranksep: 90, marginx: 20, marginy: 20 });

  nodes.forEach((n) => g.setNode(n.id, { width: n.data.width, height: n.data.height }));
  edges.forEach((e) => g.setEdge(e.source, e.target));

  dagre.layout(g);

  return nodes.map((n) => {
    const pos = g.node(n.id);
    return {
      ...n,
      position: { x: pos.x - n.data.width / 2, y: pos.y - n.data.height / 2 },
    };
  });
}
