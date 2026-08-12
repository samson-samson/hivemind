package lease

import (
	"context"
	"testing"
	"time"

	"github.com/ops-hive/control-plane/internal/iam"
)

func TestClaimHeartbeatSweep(t *testing.T) {
	clock := time.Now()
	m := NewManager(time.Minute)
	m.now = func() time.Time { return clock }
	ctx := context.Background()

	l, err := m.Claim(ctx, "inc1", "wn1", "agent-a", iam.RoleExplorer)
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsActive(l.ID) {
		t.Fatal("fresh lease should be active")
	}

	// 心跳续约
	clock = clock.Add(30 * time.Second)
	if err := m.Heartbeat(ctx, "inc1", l.ID); err != nil {
		t.Fatalf("heartbeat within ttl should succeed: %v", err)
	}

	// 超时未心跳 → sweep 标记过期，IsActive 为 false
	clock = clock.Add(2 * time.Minute)
	expired := m.Sweep(ctx, "inc1")
	if len(expired) != 1 {
		t.Fatalf("sweep expired = %d, want 1", len(expired))
	}
	if m.IsActive(l.ID) {
		t.Fatal("expired lease must not be active")
	}

	// 过期后心跳失败
	if err := m.Heartbeat(ctx, "inc1", l.ID); err != ErrNotActive {
		t.Fatalf("heartbeat after expiry: got %v, want ErrNotActive", err)
	}
}

func TestAdvisoryClaimNotExclusive(t *testing.T) {
	m := NewManager(time.Minute)
	ctx := context.Background()
	// 同一工作单元可被多个 agent 认领（咨询性，不互斥）。
	if _, err := m.Claim(ctx, "inc1", "wn1", "agent-a", iam.RoleExplorer); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Claim(ctx, "inc1", "wn1", "agent-b", iam.RoleVerifier); err != nil {
		t.Fatal("advisory lease must allow parallel claim")
	}
}
