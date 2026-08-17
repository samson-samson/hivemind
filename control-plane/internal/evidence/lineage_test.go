package evidence

import (
	"testing"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

func TestValidateDAGRejectsMissingParent(t *testing.T) {
	existing := []*iam.Evidence{{NodeBase: iam.NodeBase{ID: "e1"}}}
	ev := &iam.Evidence{
		NodeBase:   iam.NodeBase{ID: "e2"},
		LineageDAG: []string{"e1", "ghost"},
	}
	if err := ValidateDAG(existing, ev); err == nil {
		t.Fatal("expected error for missing lineage parent")
	}
}

func TestValidateDAGRejectsCycle(t *testing.T) {
	existing := []*iam.Evidence{
		{NodeBase: iam.NodeBase{ID: "e1"}, LineageDAG: []string{"e2"}},
		{NodeBase: iam.NodeBase{ID: "e2"}, LineageDAG: []string{"e1"}},
	}
	ev := &iam.Evidence{NodeBase: iam.NodeBase{ID: "e3"}, LineageDAG: []string{"e1"}}
	if err := ValidateDAG(existing, ev); err == nil {
		t.Fatal("expected error for lineage cycle")
	}
}

func TestValidateDAGAcceptsValid(t *testing.T) {
	existing := []*iam.Evidence{
		{NodeBase: iam.NodeBase{ID: "e1"}},
		{NodeBase: iam.NodeBase{ID: "e2"}, LineageDAG: []string{"e1"}},
	}
	ev := &iam.Evidence{NodeBase: iam.NodeBase{ID: "e3"}, LineageDAG: []string{"e2"}}
	if err := ValidateDAG(existing, ev); err != nil {
		t.Fatalf("expected valid DAG, got %v", err)
	}
}

func TestAncestors(t *testing.T) {
	evs := []*iam.Evidence{
		{NodeBase: iam.NodeBase{ID: "e1"}},
		{NodeBase: iam.NodeBase{ID: "e2"}, LineageDAG: []string{"e1"}},
		{NodeBase: iam.NodeBase{ID: "e3"}, LineageDAG: []string{"e2"}},
	}
	g := BuildLineage(evs)
	anc := g.Ancestors("e3")
	if len(anc) != 2 {
		t.Fatalf("ancestors of e3 = %v, want 2 (e1,e2)", anc)
	}
}
