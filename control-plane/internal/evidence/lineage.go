package evidence

import (
	"fmt"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

// LineageGraph 血缘 DAG 视图：evidenceID -> 父证据 ID 集合。
type LineageGraph struct {
	// node -> 父节点集合（derived_from 入边）
	parents map[string]map[string]struct{}
	// 全部节点 ID
	nodes map[string]struct{}
}

// BuildLineage 从证据列表构建血缘 DAG。
// 每条证据的 LineageDAG 字段即为父证据 ID 列表。
func BuildLineage(evs []*iam.Evidence) *LineageGraph {
	g := &LineageGraph{
		parents: make(map[string]map[string]struct{}),
		nodes:   make(map[string]struct{}),
	}
	for _, ev := range evs {
		g.nodes[ev.ID] = struct{}{}
		if _, ok := g.parents[ev.ID]; !ok {
			g.parents[ev.ID] = make(map[string]struct{})
		}
		for _, p := range ev.LineageDAG {
			g.parents[ev.ID][p] = struct{}{}
		}
	}
	return g
}

// ValidateDAG 校验血缘图满足 DAG 约束：
//   - 引用的父证据必须存在于现有证据集合；
//   - 不得形成环（防止证据自证/循环依赖）。
func ValidateDAG(existing []*iam.Evidence, ev *iam.Evidence) error {
	if len(ev.LineageDAG) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		known[e.ID] = struct{}{}
	}
	for _, p := range ev.LineageDAG {
		if _, ok := known[p]; !ok {
			return fmt.Errorf("lineage parent evidence %q not found", p)
		}
	}

	// 临时图 = 现有 + 新证据，做环检测（DFS）。
	all := append(append([]*iam.Evidence{}, existing...), ev)
	g := BuildLineage(all)

	const (
		white = 0 // 未访问
		gray  = 1 // 访问中
		black = 2 // 完成
	)
	color := make(map[string]int, len(g.nodes))
	for n := range g.nodes {
		color[n] = white
	}

	var dfs func(id string) bool // 返回 true 表示发现环
	dfs = func(id string) bool {
		color[id] = gray
		for p := range g.parents[id] {
			switch color[p] {
			case gray:
				return true
			case white:
				if dfs(p) {
					return true
				}
			}
		}
		color[id] = black
		return false
	}

	for n := range g.nodes {
		if color[n] == white && dfs(n) {
			return fmt.Errorf("lineage DAG contains a cycle involving evidence %q", n)
		}
	}
	return nil
}

// Ancestors 返回 id 的全部祖先（含间接父级），用于证据链闭合检查。
func (g *LineageGraph) Ancestors(id string) []string {
	seen := make(map[string]struct{})
	var walk func(n string)
	walk = func(n string) {
		for p := range g.parents[n] {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			walk(p)
		}
	}
	walk(id)
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	return out
}
