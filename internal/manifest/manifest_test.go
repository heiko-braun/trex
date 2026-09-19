package manifest

import "testing"

func TestAppendRevision_FirstRevisionOnZeroSlot(t *testing.T) {
	rev := Revision{Ref: Ref{Digest: "sha256:aaa", Size: 3}, ProducedBy: "planner#1", Reason: "initial"}

	slot := AppendRevision(Slot{}, rev)

	if slot.Current != rev.Ref {
		t.Errorf("Current = %+v, want %+v", slot.Current, rev.Ref)
	}
	if len(slot.Revisions) != 1 || slot.Revisions[0] != rev {
		t.Errorf("Revisions = %+v, want [%+v]", slot.Revisions, rev)
	}
}

func TestAppendRevision_IsAppendOnlyAndMovesCurrent(t *testing.T) {
	rev1 := Revision{Ref: Ref{Digest: "sha256:aaa"}, ProducedBy: "planner#1", Reason: "initial"}
	rev2 := Revision{Ref: Ref{Digest: "sha256:bbb"}, ProducedBy: "planner#2", Reason: "review feedback"}

	slot := AppendRevision(Slot{}, rev1)
	slot = AppendRevision(slot, rev2)

	if slot.Current != rev2.Ref {
		t.Errorf("Current = %+v, want %+v (latest)", slot.Current, rev2.Ref)
	}
	if len(slot.Revisions) != 2 {
		t.Fatalf("Revisions has %d entries, want 2 (append-only, nothing removed)", len(slot.Revisions))
	}
	if slot.Revisions[0] != rev1 {
		t.Errorf("Revisions[0] = %+v, want first revision %+v unchanged", slot.Revisions[0], rev1)
	}
	if slot.Revisions[1] != rev2 {
		t.Errorf("Revisions[1] = %+v, want %+v", slot.Revisions[1], rev2)
	}
}
