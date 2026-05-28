package store

import (
	"strings"
	"testing"
)

// kindsFor returns the event kinds recorded for one issue, oldest first.
func kindsFor(t *testing.T, s *Store, id string) []string {
	t.Helper()
	evs, err := s.History(ctx, id)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

func TestEventsRecordedPerWriteVerb(t *testing.T) {
	s := newTestStore(t)
	s.SetActor("tester")

	i, err := s.Create(ctx, "audit me", "task", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddLabels(ctx, i.ID, []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveLabels(ctx, i.ID, []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimByID(ctx, i.ID, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkClosed(ctx, i.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reopen(ctx, i.ID); err != nil {
		t.Fatal(err)
	}

	got := kindsFor(t, s, i.ID)
	want := []string{"created", "labeled", "unlabeled", "claimed", "closed", "reopened"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("event kinds:\n got %v\nwant %v", got, want)
	}

	// Actor is recorded on every event.
	evs, _ := s.History(ctx, i.ID)
	for _, e := range evs {
		if e.Actor == nil || *e.Actor != "tester" {
			t.Fatalf("event %s: actor = %v, want tester", e.Kind, e.Actor)
		}
	}
}

func TestEventPayloadChangedFields(t *testing.T) {
	s := newTestStore(t)
	i, err := s.Create(ctx, "t", "task", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, i.ID, UpdateFields{Priority: ptr(0)}); err != nil {
		t.Fatal(err)
	}
	evs, _ := s.History(ctx, i.ID)
	last := evs[len(evs)-1]
	if last.Kind != "updated" {
		t.Fatalf("last kind = %q, want updated", last.Kind)
	}
	if last.Payload == nil || !strings.Contains(*last.Payload, `"priority":0`) {
		t.Fatalf("payload = %v, want priority:0 only", last.Payload)
	}
	// Changed-fields only: an unchanged field (status) must not appear.
	if strings.Contains(*last.Payload, "status") {
		t.Fatalf("payload leaked unchanged field: %s", *last.Payload)
	}
}

func TestEventLogFilters(t *testing.T) {
	s := newTestStore(t)
	s.SetActor("alice")
	a, _ := s.Create(ctx, "a", "task", 2, nil)
	s.SetActor("bob")
	b, _ := s.Create(ctx, "b", "task", 2, nil)
	if _, err := s.MarkClosed(ctx, b.ID); err != nil {
		t.Fatal(err)
	}

	// Filter by actor.
	byAlice, err := s.EventLog(ctx, EventFilter{Actor: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byAlice) != 1 || byAlice[0].IssueID == nil || *byAlice[0].IssueID != a.ID {
		t.Fatalf("actor filter: got %d events, want 1 for %s", len(byAlice), a.ID)
	}

	// Filter by kind.
	closed, err := s.EventLog(ctx, EventFilter{Kind: "closed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(closed) != 1 || *closed[0].IssueID != b.ID {
		t.Fatalf("kind filter: got %d closed events, want 1 for %s", len(closed), b.ID)
	}

	// Newest first: the global log leads with the most recent event.
	all, _ := s.EventLog(ctx, EventFilter{})
	if len(all) < 1 || all[0].Kind != "closed" {
		t.Fatalf("expected newest-first ordering, got %v", all)
	}
}

func TestEventsSurviveBlankActor(t *testing.T) {
	s := newTestStore(t) // no SetActor → NULL actor
	i, _ := s.Create(ctx, "t", "task", 2, nil)
	evs, _ := s.History(ctx, i.ID)
	if len(evs) != 1 || evs[0].Actor != nil {
		t.Fatalf("expected one event with nil actor, got %v", evs)
	}
}
