package iam

import (
	"context"
	"sort"
	"sync"
)

// Store 是 IOM 图存储的统一抽象。
//
// P0 由 MemoryStore（纯内存）实现，保证任何机器可跑、可测试；
// 后续接入 PostgreSQL 时实现同一接口即可无缝替换（关系表 + 边表模拟图语义）。
// 所有方法接收 context.Context，为未来接入长连接/事务预留。
type Store interface {
	// ---- Incident ----
	CreateIncident(ctx context.Context, inc *Incident) error
	ListIncidents(ctx context.Context) ([]*Incident, error)
	GetIncident(ctx context.Context, id string) (*Incident, error)
	UpdateIncident(ctx context.Context, inc *Incident) error

	// ---- WorkNode ----
	AddWorkNode(ctx context.Context, incidentID string, wn *WorkNode) error
	ListWorkNodes(ctx context.Context, incidentID string) ([]*WorkNode, error)
	GetWorkNode(ctx context.Context, incidentID, id string) (*WorkNode, error)
	UpdateWorkNode(ctx context.Context, incidentID string, wn *WorkNode) error

	// ---- Operation ----
	AddOperation(ctx context.Context, incidentID string, op *Operation) error
	ListOperations(ctx context.Context, incidentID string) ([]*Operation, error)
	GetOperation(ctx context.Context, incidentID, id string) (*Operation, error)

	// ---- Evidence ----
	AddEvidence(ctx context.Context, incidentID string, ev *Evidence) error
	ListEvidence(ctx context.Context, incidentID string) ([]*Evidence, error)

	// ---- Fact ----
	AddFact(ctx context.Context, incidentID string, f *Fact) error
	ListFacts(ctx context.Context, incidentID string) ([]*Fact, error)

	// ---- Hypothesis ----
	AddHypothesis(ctx context.Context, incidentID string, h *Hypothesis) error
	ListHypotheses(ctx context.Context, incidentID string) ([]*Hypothesis, error)

	// ---- Guidance ----
	AddGuidance(ctx context.Context, incidentID string, g *Guidance) error
	ListGuidance(ctx context.Context, incidentID string) ([]*Guidance, error)

	// ---- Edge ----
	AddEdge(ctx context.Context, e *Edge) error
	ListEdges(ctx context.Context, incidentID string) ([]*Edge, error)
}

// MemoryStore 纯内存实现。所有集合以 map 存储，以互斥锁保护并发访问。
// 不依赖任何外部 DB，进程重启即清空（P0 单实例可接受）。
type MemoryStore struct {
	mu sync.RWMutex

	incidents  map[string]*Incident
	workNodes  map[string]*WorkNode
	operations map[string]*Operation
	evidence   map[string]*Evidence
	facts      map[string]*Fact
	hypotheses map[string]*Hypothesis
	guidance   map[string]*Guidance
	edges      map[string]*Edge

	// 按事故作用域索引（incidentID -> 节点 ID 列表）
	incWorkNodes  map[string][]string
	incOperations map[string][]string
	incEvidence   map[string][]string
	incFacts      map[string][]string
	incHypotheses map[string][]string
	incGuidance   map[string][]string
	incEdges      map[string][]string
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore 构造一个空的纯内存存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		incidents:     make(map[string]*Incident),
		workNodes:     make(map[string]*WorkNode),
		operations:    make(map[string]*Operation),
		evidence:      make(map[string]*Evidence),
		facts:         make(map[string]*Fact),
		hypotheses:    make(map[string]*Hypothesis),
		guidance:      make(map[string]*Guidance),
		edges:         make(map[string]*Edge),
		incWorkNodes:  make(map[string][]string),
		incOperations: make(map[string][]string),
		incEvidence:   make(map[string][]string),
		incFacts:      make(map[string][]string),
		incHypotheses: make(map[string][]string),
		incGuidance:   make(map[string][]string),
		incEdges:      make(map[string][]string),
	}
}

// requireIncident 检查事故存在。
func (m *MemoryStore) requireIncident(incidentID string) error {
	if _, ok := m.incidents[incidentID]; !ok {
		return &NotFoundError{Kind: "incident", ID: incidentID}
	}
	return nil
}

// appendIndex 向作用域索引追加 id。
func appendIndex(idx map[string][]string, incidentID, id string) {
	idx[incidentID] = append(idx[incidentID], id)
}

// ---- Incident ----

func (m *MemoryStore) CreateIncident(_ context.Context, inc *Incident) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.incidents[inc.ID]; ok {
		return &ConflictError{Kind: "incident", ID: inc.ID}
	}
	m.incidents[inc.ID] = inc
	return nil
}

func (m *MemoryStore) ListIncidents(_ context.Context) ([]*Incident, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Incident, 0, len(m.incidents))
	for _, inc := range m.incidents {
		out = append(out, inc)
	}
	// map 遍历顺序不稳定：按时间倒序（最新事故在前），保证前端列表稳定。
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out, nil
}

func (m *MemoryStore) GetIncident(_ context.Context, id string) (*Incident, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inc, ok := m.incidents[id]
	if !ok {
		return nil, &NotFoundError{Kind: "incident", ID: id}
	}
	return inc, nil
}

func (m *MemoryStore) UpdateIncident(_ context.Context, inc *Incident) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.incidents[inc.ID]; !ok {
		return &NotFoundError{Kind: "incident", ID: inc.ID}
	}
	m.incidents[inc.ID] = inc
	return nil
}

// ---- WorkNode ----

func (m *MemoryStore) AddWorkNode(_ context.Context, incidentID string, wn *WorkNode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireIncident(incidentID); err != nil {
		return err
	}
	m.workNodes[wn.ID] = wn
	appendIndex(m.incWorkNodes, incidentID, wn.ID)
	return nil
}

func (m *MemoryStore) ListWorkNodes(_ context.Context, incidentID string) ([]*WorkNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.incWorkNodes[incidentID]
	out := make([]*WorkNode, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.workNodes[id])
	}
	return out, nil
}

func (m *MemoryStore) GetWorkNode(_ context.Context, incidentID, id string) (*WorkNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	wn, ok := m.workNodes[id]
	if !ok {
		return nil, &NotFoundError{Kind: "work_node", ID: id}
	}
	return wn, nil
}

func (m *MemoryStore) UpdateWorkNode(_ context.Context, incidentID string, wn *WorkNode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workNodes[wn.ID]; !ok {
		return &NotFoundError{Kind: "work_node", ID: wn.ID}
	}
	m.workNodes[wn.ID] = wn
	return nil
}

// ---- Operation ----

func (m *MemoryStore) AddOperation(_ context.Context, incidentID string, op *Operation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireIncident(incidentID); err != nil {
		return err
	}
	m.operations[op.ID] = op
	appendIndex(m.incOperations, incidentID, op.ID)
	return nil
}

func (m *MemoryStore) ListOperations(_ context.Context, incidentID string) ([]*Operation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.incOperations[incidentID]
	out := make([]*Operation, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.operations[id])
	}
	return out, nil
}

func (m *MemoryStore) GetOperation(_ context.Context, incidentID, id string) (*Operation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	op, ok := m.operations[id]
	if !ok {
		return nil, &NotFoundError{Kind: "operation", ID: id}
	}
	return op, nil
}

// ---- Evidence ----

func (m *MemoryStore) AddEvidence(_ context.Context, incidentID string, ev *Evidence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireIncident(incidentID); err != nil {
		return err
	}
	m.evidence[ev.ID] = ev
	appendIndex(m.incEvidence, incidentID, ev.ID)
	return nil
}

func (m *MemoryStore) ListEvidence(_ context.Context, incidentID string) ([]*Evidence, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.incEvidence[incidentID]
	out := make([]*Evidence, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.evidence[id])
	}
	return out, nil
}

// ---- Fact ----

func (m *MemoryStore) AddFact(_ context.Context, incidentID string, f *Fact) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireIncident(incidentID); err != nil {
		return err
	}
	m.facts[f.ID] = f
	appendIndex(m.incFacts, incidentID, f.ID)
	return nil
}

func (m *MemoryStore) ListFacts(_ context.Context, incidentID string) ([]*Fact, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.incFacts[incidentID]
	out := make([]*Fact, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.facts[id])
	}
	return out, nil
}

// ---- Hypothesis ----

func (m *MemoryStore) AddHypothesis(_ context.Context, incidentID string, h *Hypothesis) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireIncident(incidentID); err != nil {
		return err
	}
	m.hypotheses[h.ID] = h
	appendIndex(m.incHypotheses, incidentID, h.ID)
	return nil
}

func (m *MemoryStore) ListHypotheses(_ context.Context, incidentID string) ([]*Hypothesis, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.incHypotheses[incidentID]
	out := make([]*Hypothesis, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.hypotheses[id])
	}
	return out, nil
}

// ---- Guidance ----

func (m *MemoryStore) AddGuidance(_ context.Context, incidentID string, g *Guidance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireIncident(incidentID); err != nil {
		return err
	}
	m.guidance[g.ID] = g
	appendIndex(m.incGuidance, incidentID, g.ID)
	return nil
}

func (m *MemoryStore) ListGuidance(_ context.Context, incidentID string) ([]*Guidance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.incGuidance[incidentID]
	out := make([]*Guidance, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.guidance[id])
	}
	return out, nil
}

// ---- Edge ----

func (m *MemoryStore) AddEdge(_ context.Context, e *Edge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edges[e.ID] = e
	appendIndex(m.incEdges, e.IncidentID, e.ID)
	return nil
}

func (m *MemoryStore) ListEdges(_ context.Context, incidentID string) ([]*Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.incEdges[incidentID]
	out := make([]*Edge, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.edges[id])
	}
	return out, nil
}
