package store

import "testing"

func TestStartedAtLifecycle(t *testing.T) {
	s := newTestStore(t)
	i, _ := s.Create(ctx, "task", "task", 2, nil)
	if i.StartedAt != nil {
		t.Fatalf("fresh issue should have nil started_at")
	}
	// Claim stamps started_at.
	c, err := s.ClaimByID(ctx, i.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if c.StartedAt == nil {
		t.Fatalf("claim should set started_at")
	}
	started := *c.StartedAt
	// Close preserves it (cycle time = closed - started_at).
	cl, _ := s.MarkClosed(ctx, i.ID)
	if cl.StartedAt == nil || *cl.StartedAt != started {
		t.Fatalf("close should preserve started_at: %v", cl.StartedAt)
	}
	// Reopen clears it.
	ro, _ := s.Reopen(ctx, i.ID)
	if ro.StartedAt != nil {
		t.Fatalf("reopen should clear started_at, got %v", ro.StartedAt)
	}
}

func TestStartedAtViaClaimRace(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Create(ctx, "a", "task", 2, nil); err != nil {
		t.Fatal(err)
	}
	// The lane Claim path (raw SQL RETURNING) also stamps started_at.
	got, err := s.Claim(ctx, "dev", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.StartedAt == nil {
		t.Fatalf("Claim should set started_at, got nil")
	}
}
