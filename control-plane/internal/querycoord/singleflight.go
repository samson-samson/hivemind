package querycoord

import (
	"context"
	"sync"
	"time"

	"github.com/ops-hive/control-plane/internal/iam"
)

// Flight 代表一次正在处理（或已完成）的"同指纹查询执行"。
// P0 无真实执行引擎：登记即代表一次查询意图，证据推送标记其完成。
type flight struct {
	op          *iam.Operation // 首登记的 fresh 操作
	complete    bool           // 是否已有结果（证据推送后置 true）
	evidenceID  string         // 完成后的结果证据 ID
	completedAt time.Time      // 完成时间（用于新鲜度判断）
}

// Coordinator 查询协调器：维护 incident × fingerprint → flight 表，
// 在并发登记同指纹查询时合并执行。
type Coordinator struct {
	mu sync.Mutex

	// flights[incidentID][fingerprint] -> flight
	flights map[string]map[string]*flight

	store iam.Store

	// freshWindow 结果新鲜度窗口：窗口内完成的结果可复用（reused），
	// 超出则允许重查（fresh）。0 表示不限制（始终复用）。
	freshWindow time.Duration

	now func() time.Time
}

// Result 登记结果：返回登记的 Operation 与去重结论。
type Result struct {
	Fingerprint       string          `json:"fingerprint"`
	Status            iam.DedupStatus `json:"dedup_status"`
	Operation         *iam.Operation  `json:"operation"`
	ReusedOperationID string          `json:"reused_operation_id,omitempty"`
	ReusedEvidenceID  string          `json:"reused_evidence_id,omitempty"`
}

// NewCoordinator 构造查询协调器。
// freshWindow：默认 5 分钟，0 表示始终复用。
func NewCoordinator(store iam.Store, freshWindow time.Duration) *Coordinator {
	if freshWindow == 0 {
		freshWindow = 5 * time.Minute
	}
	return &Coordinator{
		flights:     make(map[string]map[string]*flight),
		store:       store,
		freshWindow: freshWindow,
		now:         time.Now,
	}
}

// Register 登记一次查询。
//
// workNodeID 为可选的关联工作单元 ID（为空表示不关联）。
// 去重逻辑（对应 dedup_status）：
//   - fresh：该指纹首次出现，或上次结果已超出新鲜度窗口（允许重查）。
//   - single_flight：该指纹正在处理中（in-flight），本次并入既有执行，不重复查询。
//   - reused：该指纹已有窗口内完成的结果，复用既有结果，不重复查询。
func (c *Coordinator) Register(ctx context.Context, incidentID string, spec iam.QuerySpec, source, workNodeID string) (*Result, error) {
	fp, err := Fingerprint(spec)
	if err != nil {
		return nil, err
	}

	now := c.now()
	op := &iam.Operation{
		NodeBase: iam.NodeBase{
			ID:        iam.NewID("op"),
			Type:      iam.NodeOperation,
			Source:    source,
			Timestamp: now,
		},
		Fingerprint:  fp,
		Query:        spec,
		RegisteredAt: now,
		Tenant:       spec.Tenant,
		ToolVersion:  spec.ToolVersion,
		WorkNodeID:   workNodeID,
	}

	c.mu.Lock()
	if _, ok := c.flights[incidentID]; !ok {
		c.flights[incidentID] = make(map[string]*flight)
	}
	fl := c.flights[incidentID][fp]

	switch {
	case fl == nil:
		// 首次登记 → fresh，进入 in-flight。
		op.DedupStatus = iam.DedupFresh
		c.flights[incidentID][fp] = &flight{op: op}
	case !fl.complete:
		// 同一查询正在处理中 → single-flight 合并。
		op.DedupStatus = iam.DedupSingleFlight
		op.MergedIntoID = fl.op.ID
	case c.now().Sub(fl.completedAt) <= c.freshWindow:
		// 已有窗口内完成结果 → 兼容复用。
		op.DedupStatus = iam.DedupReused
		op.MergedIntoID = fl.op.ID
		op.ResultRef = fl.evidenceID
	default:
		// 结果已过期（需新鲜度）→ 允许重查，新建 flight。
		op.DedupStatus = iam.DedupFresh
		c.flights[incidentID][fp] = &flight{op: op}
	}
	res := &Result{
		Fingerprint: fp,
		Status:      op.DedupStatus,
		Operation:   op,
	}
	if op.DedupStatus == iam.DedupReused {
		res.ReusedOperationID = op.MergedIntoID
		res.ReusedEvidenceID = fl.evidenceID
	}
	c.mu.Unlock()

	if err := c.store.AddOperation(ctx, incidentID, op); err != nil {
		return nil, err
	}
	return res, nil
}

// Complete 标记某操作完成：证据推送后调用，记录结果证据 ID 与完成时间。
// 找不到对应 flight 时静默忽略（例如操作来自重建/历史数据）。
func (c *Coordinator) Complete(ctx context.Context, incidentID, operationID, evidenceID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, fl := range c.flights[incidentID] {
		if fl.op.ID == operationID {
			fl.complete = true
			fl.evidenceID = evidenceID
			fl.completedAt = c.now()
			return nil
		}
	}
	return nil
}
