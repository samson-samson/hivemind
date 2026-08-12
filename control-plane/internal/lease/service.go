// Package lease 咨询性租约（advisory lease）。
//
// 语义：claim 不独占——指定验证者可并行复核；高风险结论强制双路独立验证（受控冗余）。
// 租约掉线超时释放；晚到结果标记 stale 不污染当前事故（见 api 证据推送路径）。
// 纯内存实现，接口预留 PostgreSQL/Redis 扩展。
package lease

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ops-hive/control-plane/internal/iam"
)

// Status 租约状态。
type Status string

const (
	StatusActive   Status = "active"
	StatusExpired  Status = "expired"
	StatusReleased Status = "released"
)

// Lease 一条咨询性租约。
type Lease struct {
	ID          string       `json:"id"`
	IncidentID  string       `json:"incident_id"`
	WorkNodeID  string       `json:"work_node_id"`
	Assignee    string       `json:"assignee"` // 认领人（agent/人）
	Role        iam.WorkRole `json:"role"`
	Advisory    bool         `json:"advisory"` // 恒为 true：咨询性，不独占
	ClaimedAt   time.Time    `json:"claimed_at"`
	HeartbeatAt time.Time    `json:"heartbeat_at"`
	ExpiresAt   time.Time    `json:"expires_at"`
	Status      Status       `json:"status"`
}

// Manager 租约管理器（内存实现）。
type Manager struct {
	mu     sync.Mutex
	leases map[string]*Lease
	ttl    time.Duration
	now    func() time.Time
}

// NewManager 构造租约管理器，ttl 为心跳超时（默认 60s）。
func NewManager(ttl time.Duration) *Manager {
	if ttl == 0 {
		ttl = time.Minute
	}
	return &Manager{
		leases: make(map[string]*Lease),
		ttl:    ttl,
		now:    time.Now,
	}
}

// ErrNotActive 表示对非活跃租约操作（已过期/已释放）。
var ErrNotActive = errors.New("lease is not active")

// Claim 认领一条咨询性租约。不拒绝并行认领（advisory）。
func (m *Manager) Claim(_ context.Context, incidentID, workNodeID, assignee string, role iam.WorkRole) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	l := &Lease{
		ID:          iam.NewID("lease"),
		IncidentID:  incidentID,
		WorkNodeID:  workNodeID,
		Assignee:    assignee,
		Role:        role,
		Advisory:    true,
		ClaimedAt:   now,
		HeartbeatAt: now,
		ExpiresAt:   now.Add(m.ttl),
		Status:      StatusActive,
	}
	m.leases[l.ID] = l
	return l, nil
}

// Heartbeat 续约。对过期/已释放租约返回 ErrNotActive。
func (m *Manager) Heartbeat(_ context.Context, incidentID, leaseID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[leaseID]
	if !ok {
		return errors.New("lease not found")
	}
	if l.Status != StatusActive {
		return ErrNotActive
	}
	if m.now().After(l.ExpiresAt) {
		l.Status = StatusExpired
		return ErrNotActive
	}
	l.HeartbeatAt = m.now()
	l.ExpiresAt = m.now().Add(m.ttl)
	return nil
}

// Release 释放租约（幂等）。
func (m *Manager) Release(_ context.Context, incidentID, leaseID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[leaseID]
	if !ok {
		return nil // 释放幂等
	}
	l.Status = StatusReleased
	return nil
}

// Sweep 将超时未心跳的活跃租约标记为过期，返回被过期的租约。
// 由服务层定期调用（如每次查询/推送证据前）。
func (m *Manager) Sweep(_ context.Context, incidentID string) []*Lease {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	var expired []*Lease
	for _, l := range m.leases {
		if l.IncidentID == incidentID && l.Status == StatusActive && now.After(l.ExpiresAt) {
			l.Status = StatusExpired
			expired = append(expired, l)
		}
	}
	return expired
}

// Get 查询租约。
func (m *Manager) Get(_ context.Context, leaseID string) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[leaseID]
	if !ok {
		return nil, errors.New("lease not found")
	}
	return l, nil
}

// IsActive 判断租约当前是否活跃。
func (m *Manager) IsActive(leaseID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[leaseID]
	return ok && l.Status == StatusActive && !m.now().After(l.ExpiresAt)
}
