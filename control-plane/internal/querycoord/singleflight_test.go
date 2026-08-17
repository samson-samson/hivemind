package querycoord

import (
	"context"
	"testing"
	"time"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

func TestDedupLifecycle(t *testing.T) {
	store := iam.NewMemoryStore()
	inc := &iam.Incident{
		NodeBase:   iam.NodeBase{ID: "inc1", Type: iam.NodeIncident, Timestamp: time.Now()},
		Status:     iam.IncidentOpen,
		SymptomSet: []string{"s"},
	}
	_ = store.CreateIncident(context.Background(), inc)

	coord := NewCoordinator(store, 5*time.Minute)
	ctx := context.Background()
	spec := iam.QuerySpec{Target: "t1", DataSource: "prometheus"}

	// fresh
	r1, err := coord.Register(ctx, "inc1", spec, "agent-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Status != iam.DedupFresh {
		t.Fatalf("r1 = %s, want fresh", r1.Status)
	}

	// in-flight → single_flight 合并
	r2, _ := coord.Register(ctx, "inc1", spec, "agent-b", "")
	if r2.Status != iam.DedupSingleFlight {
		t.Fatalf("r2 = %s, want single_flight", r2.Status)
	}

	// 推证据标记完成 → 复用
	ev := &iam.Evidence{NodeBase: iam.NodeBase{ID: "ev1"}, OperationID: r1.Operation.ID, DataSource: "prometheus"}
	if err := store.AddEvidence(ctx, "inc1", ev); err != nil {
		t.Fatal(err)
	}
	_ = coord.Complete(ctx, "inc1", r1.Operation.ID, "ev1")

	r3, _ := coord.Register(ctx, "inc1", spec, "agent-c", "")
	if r3.Status != iam.DedupReused {
		t.Fatalf("r3 = %s, want reused", r3.Status)
	}
	if r3.ReusedEvidenceID != "ev1" {
		t.Fatalf("reused evidence = %q, want ev1", r3.ReusedEvidenceID)
	}
}

func TestDedupFreshnessExpiry(t *testing.T) {
	store := iam.NewMemoryStore()
	inc := &iam.Incident{NodeBase: iam.NodeBase{ID: "inc1", Type: iam.NodeIncident}, Status: iam.IncidentOpen}
	_ = store.CreateIncident(context.Background(), inc)

	clock := time.Now()
	coord := NewCoordinator(store, time.Minute)
	coord.now = func() time.Time { return clock }
	ctx := context.Background()
	spec := iam.QuerySpec{Target: "t1", DataSource: "k8s"}

	r1, _ := coord.Register(ctx, "inc1", spec, "a", "")
	ev := &iam.Evidence{NodeBase: iam.NodeBase{ID: "ev1"}, OperationID: r1.Operation.ID}
	_ = store.AddEvidence(ctx, "inc1", ev)
	_ = coord.Complete(ctx, "inc1", r1.Operation.ID, "ev1")

	// 窗口内 → 复用
	r2, _ := coord.Register(ctx, "inc1", spec, "b", "")
	if r2.Status != iam.DedupReused {
		t.Fatalf("within window: r2 = %s, want reused", r2.Status)
	}

	// 超时后 → 允许重查（fresh）
	clock = clock.Add(2 * time.Minute)
	r3, _ := coord.Register(ctx, "inc1", spec, "c", "")
	if r3.Status != iam.DedupFresh {
		t.Fatalf("after expiry: r3 = %s, want fresh (re-query allowed)", r3.Status)
	}
}
