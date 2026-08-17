// Package stats P0 指标：去重率 / 信息增益 / 决策延迟。
//
// 对齐设计 §13 主指标：
//   - 无意重复率：single-flight 压掉的重复查询占比（P0）。
//   - 有效独立验证覆盖率：被 ≥2 个独立数据源验证的高风险结论比例（近似值）。
//   - 单位工具成本的信息增益：每个 Operation 新增的已确认事实（P1 完整化）。
//   - 决策延迟：从告警到 IC 做出根因判断/止血决策的时间（P0 用"首个已确认事实/首个 IC 发言"近似）。
package stats

import (
	"context"
	"time"

	"github.com/samson-samson/hivemind/control-plane/internal/evidence"
	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

// Snapshot 事故统计快照。
type Snapshot struct {
	IncidentID string `json:"incident_id"`

	// 查询去重（无意重复率）
	TotalOperations   int     `json:"total_operations"`
	FreshOperations   int     `json:"fresh_operations"`
	SingleFlight      int     `json:"single_flight_deduped"`
	Reused            int     `json:"reused_deduped"`
	DedupedOperations int     `json:"deduped_operations"`
	DedupRate         float64 `json:"dedup_rate"` // deduped / total

	// 证据血缘与独立性
	TotalEvidence     int     `json:"total_evidence"`
	UniqueSources     int     `json:"unique_data_sources"`
	IndependenceScore float64 `json:"independence_score"` // 独立键数量

	// 信息增益（单位工具成本）
	ConfirmedFacts int     `json:"confirmed_facts"`
	TotalFacts     int     `json:"total_facts"`
	InfoGainPerOp  float64 `json:"info_gain_per_operation"` // 已确认事实 / 总操作数

	// 假设
	TotalHypotheses int `json:"total_hypotheses"`
	SupportedHyp    int `json:"supported_hypotheses"`
	RefutedHyp      int `json:"refuted_hypotheses"`

	// 工作图
	WorkNodesOpen   int `json:"work_nodes_open"`
	WorkNodesActive int `json:"work_nodes_active"`
	WorkNodesStale  int `json:"work_nodes_stale"`

	// 决策延迟（毫秒）；未产生决策时为 nil
	DecisionLatencyMs *int64 `json:"decision_latency_ms,omitempty"`
}

// Collector 从 Store + Ledger 计算统计快照。
type Collector struct {
	store  iam.Store
	ledger *evidence.Ledger
	now    func() time.Time
}

// NewCollector 构造统计采集器。
func NewCollector(store iam.Store, ledger *evidence.Ledger) *Collector {
	return &Collector{store: store, ledger: ledger, now: time.Now}
}

// Collect 计算某事故的统计快照。
func (c *Collector) Collect(ctx context.Context, incidentID string) (*Snapshot, error) {
	inc, err := c.store.GetIncident(ctx, incidentID)
	if err != nil {
		return nil, err
	}

	s := &Snapshot{IncidentID: incidentID}

	ops, _ := c.store.ListOperations(ctx, incidentID)
	s.TotalOperations = len(ops)
	for _, op := range ops {
		switch op.DedupStatus {
		case iam.DedupFresh:
			s.FreshOperations++
		case iam.DedupSingleFlight:
			s.SingleFlight++
			s.DedupedOperations++
		case iam.DedupReused:
			s.Reused++
			s.DedupedOperations++
		}
	}
	if s.TotalOperations > 0 {
		s.DedupRate = float64(s.DedupedOperations) / float64(s.TotalOperations)
	}

	evs, _ := c.store.ListEvidence(ctx, incidentID)
	s.TotalEvidence = len(evs)
	perKey, indep := evidence.ScoreGroup(evs)
	_ = perKey
	s.UniqueSources = len(perKey)
	s.IndependenceScore = indep

	facts, _ := c.store.ListFacts(ctx, incidentID)
	s.TotalFacts = len(facts)
	for _, f := range facts {
		if f.IsConfirmed {
			s.ConfirmedFacts++
		}
	}
	if s.TotalOperations > 0 {
		s.InfoGainPerOp = float64(s.ConfirmedFacts) / float64(s.TotalOperations)
	}

	hyps, _ := c.store.ListHypotheses(ctx, incidentID)
	s.TotalHypotheses = len(hyps)
	for _, h := range hyps {
		switch h.Status {
		case iam.HypothesisSupported, iam.HypothesisConfirmed:
			s.SupportedHyp++
		case iam.HypothesisRefuted:
			s.RefutedHyp++
		}
	}

	wns, _ := c.store.ListWorkNodes(ctx, incidentID)
	for _, wn := range wns {
		switch wn.Status {
		case iam.WorkNodeOpen:
			s.WorkNodesOpen++
		case iam.WorkNodeActive:
			s.WorkNodesActive++
		case iam.WorkNodeStale:
			s.WorkNodesStale++
		}
	}

	// 决策延迟：首个已确认事实 或 首个 IC 发言，距离事故创建的时间。
	first := time.Time{}
	for _, f := range facts {
		if f.IsConfirmed && (first.IsZero() || f.Timestamp.Before(first)) {
			first = f.Timestamp
		}
	}
	gds, _ := c.store.ListGuidance(ctx, incidentID)
	for _, g := range gds {
		if first.IsZero() || g.Timestamp.Before(first) {
			first = g.Timestamp
		}
	}
	if !first.IsZero() {
		ms := first.Sub(inc.Timestamp).Milliseconds()
		if ms < 0 {
			ms = 0
		}
		s.DecisionLatencyMs = &ms
	}

	return s, nil
}
