package evidence

import (
	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

// 独立性评分（防从众）：按采集链路 / 权限域 / 底层数据源计算，不按 agent 数。
// 核心规则：**同一数据源的重复观测不叠加**。
//
// 设计（P0 实现）：
//   - 独立性键 = 数据源 + 权限域（若提供）。同一数据源不同权限域视为更独立。
//   - 每个独立键的"总独立信号"上限为 1.0；该键下 n 条证据各自贡献 1/n。
//   - 一组证据的总独立性 = 不同独立键的数量（每个键至多贡献 1.0）。

// IndependenceKey 返回证据的独立性键（数据源 + 权限域）。
func IndependenceKey(ev *iam.Evidence) string {
	if ev.PermissionDomain != "" {
		return ev.DataSource + "|" + ev.PermissionDomain
	}
	return ev.DataSource
}

// IndependenceForNew 计算新增证据的边际独立性评分（0..1）。
// 给定已有证据集，若该独立键下已有 k 条，则新增的边际独立 = 1/(k+1)。
// 首条来自某键的证据获得 1.0；重复观测迅速衰减，防止从众。
func IndependenceForNew(existing []*iam.Evidence, ev *iam.Evidence) float64 {
	key := IndependenceKey(ev)
	var same int
	for _, e := range existing {
		if IndependenceKey(e) == key {
			same++
		}
	}
	return 1.0 / float64(same+1)
}

// ScoreGroup 按独立键聚合独立性：返回每个键的总独立信号（恒为 1.0）
// 以及去重后的独立键总数。
func ScoreGroup(evs []*iam.Evidence) (perKey map[string]float64, total float64) {
	perKey = make(map[string]float64)
	for _, e := range evs {
		perKey[IndependenceKey(e)] = 1.0 // 同一键重复观测不叠加
	}
	return perKey, float64(len(perKey))
}

// ChainIndependence 计算一组证据（如假设的支持链/事实的证据链）的总独立性。
// = 不同独立键的数量。
func ChainIndependence(evs []*iam.Evidence) float64 {
	_, total := ScoreGroup(evs)
	return total
}
