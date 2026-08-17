package evidence

import (
	"testing"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

func mkEvidence(id, source, domain string) *iam.Evidence {
	return &iam.Evidence{
		NodeBase:         iam.NodeBase{ID: id},
		DataSource:       source,
		PermissionDomain: domain,
	}
}

func TestIndependenceSameSourceNotStacking(t *testing.T) {
	// 同一数据源 prometheus 的重复观测不叠加：边际独立性递减。
	var existing []*iam.Evidence
	ev1 := mkEvidence("e1", "prometheus", "")
	if got := IndependenceForNew(existing, ev1); got != 1.0 {
		t.Fatalf("first from source = %v, want 1.0", got)
	}
	existing = append(existing, ev1)
	ev2 := mkEvidence("e2", "prometheus", "")
	if got := IndependenceForNew(existing, ev2); got != 0.5 {
		t.Fatalf("second from same source = %v, want 0.5", got)
	}
	existing = append(existing, ev2)
	ev3 := mkEvidence("e3", "prometheus", "")
	if got := IndependenceForNew(existing, ev3); got != 1.0/3.0 {
		t.Fatalf("third from same source = %v, want 1/3", got)
	}
}

func TestIndependenceDifferentSources(t *testing.T) {
	// 不同数据源各自独立。
	ev1 := mkEvidence("e1", "prometheus", "")
	ev2 := mkEvidence("e2", "k8s", "")
	evs := []*iam.Evidence{ev1, ev2}
	if got := ChainIndependence(evs); got != 2.0 {
		t.Fatalf("two distinct sources = %v, want 2.0", got)
	}
}

func TestIndependencePermissionDomainAddsVoice(t *testing.T) {
	// 同一数据源不同权限域视为更独立的观测（采集链路/权限域维度）。
	ev1 := mkEvidence("e1", "prometheus", "domain-a")
	ev2 := mkEvidence("e2", "prometheus", "domain-b")
	evs := []*iam.Evidence{ev1, ev2}
	if got := ChainIndependence(evs); got != 2.0 {
		t.Fatalf("same source, diff domain = %v, want 2.0", got)
	}
}
