package store

import (
	"strings"
	"testing"
)

// mkMilestone creates a milestone issue depending on the given parents.
func mkMilestone(t *testing.T, s *Store, title string, parents ...string) Issue {
	t.Helper()
	m, err := s.CreateWithLinks(ctx, title, "milestone", 2, nil, CreateOpts{Parents: parents})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMilestoneAutoClosesWhenDepsClose(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 2, nil)
	b, _ := s.Create(ctx, "b", "task", 2, nil)
	m := mkMilestone(t, s, "phase done", a.ID, b.ID)

	if _, err := s.MarkClosed(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	// One dep still open → milestone stays open.
	if got, _ := s.Get(ctx, m.ID); got.Status != "open" {
		t.Fatalf("milestone closed too early: %s", got.Status)
	}
	if _, err := s.MarkClosed(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	// All deps closed → milestone auto-closed.
	got, _ := s.Get(ctx, m.ID)
	if got.Status != "closed" {
		t.Fatalf("milestone should have auto-closed, got %s", got.Status)
	}
	// Auto-close recorded with auto:true.
	evs, _ := s.History(ctx, m.ID)
	last := evs[len(evs)-1]
	if last.Kind != "closed" || last.Payload == nil || !strings.Contains(*last.Payload, `"auto":true`) {
		t.Fatalf("expected auto-close event with auto:true, got %+v", last)
	}
}

func TestMilestoneCascadeRecursive(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 2, nil)
	inner := mkMilestone(t, s, "inner", a.ID)     // closes when a closes
	outer := mkMilestone(t, s, "outer", inner.ID) // closes when inner closes

	if _, err := s.MarkClosed(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if g, _ := s.Get(ctx, inner.ID); g.Status != "closed" {
		t.Fatalf("inner milestone not closed: %s", g.Status)
	}
	if g, _ := s.Get(ctx, outer.ID); g.Status != "closed" {
		t.Fatalf("outer milestone should cascade-close: %s", g.Status)
	}
}

func TestMilestoneCancelledDepDoesNotClose(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 2, nil)
	b, _ := s.Create(ctx, "b", "task", 2, nil)
	m := mkMilestone(t, s, "gate", a.ID, b.ID)

	if _, err := s.MarkClosed(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cancel(ctx, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	// b is cancelled, not closed → the milestone must NOT auto-close. The
	// cancel cascade instead carries it to "cancelled" (the gate's chain was
	// abandoned). Either way it is never "closed".
	if g, _ := s.Get(ctx, m.ID); g.Status == "closed" {
		t.Fatalf("milestone must not auto-close on a cancelled dep, got %s", g.Status)
	}
}

func TestMilestoneClosesViaUpdateStatus(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 2, nil)
	m := mkMilestone(t, s, "m", a.ID)
	// Close the dep via the generic update path, not MarkClosed.
	if _, err := s.Update(ctx, a.ID, UpdateFields{Status: ptr("closed")}); err != nil {
		t.Fatal(err)
	}
	if g, _ := s.Get(ctx, m.ID); g.Status != "closed" {
		t.Fatalf("milestone should close via update path too, got %s", g.Status)
	}
}
