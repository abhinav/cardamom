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

func TestEventLabelRecordsOnlyChanged(t *testing.T) {
	s := newTestStore(t)
	i, _ := s.Create(ctx, "t", "task", 2, nil)
	mustPayload := func(kind, want string) {
		t.Helper()
		evs, _ := s.History(ctx, i.ID)
		last := evs[len(evs)-1]
		if last.Kind != kind {
			t.Fatalf("last kind = %q, want %q", last.Kind, kind)
		}
		if last.Payload == nil || !strings.Contains(*last.Payload, want) {
			t.Fatalf("%s payload = %v, want contains %q", kind, last.Payload, want)
		}
	}
	if _, err := s.AddLabels(ctx, i.ID, []string{"alpha", "beta"}); err != nil {
		t.Fatal(err)
	}
	mustPayload("labeled", `["alpha","beta"]`)
	// alpha already present → only gamma is the real change.
	if _, err := s.AddLabels(ctx, i.ID, []string{"alpha", "gamma"}); err != nil {
		t.Fatal(err)
	}
	mustPayload("labeled", `"added":["gamma"]`)
	// missing-label not present → only beta is the real removal.
	if _, err := s.RemoveLabels(ctx, i.ID, []string{"beta", "missing-label"}); err != nil {
		t.Fatal(err)
	}
	mustPayload("unlabeled", `"removed":["beta"]`)

	// Adding only already-present labels records no event.
	before, _ := s.History(ctx, i.ID)
	if _, err := s.AddLabels(ctx, i.ID, []string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	after, _ := s.History(ctx, i.ID)
	if len(after) != len(before) {
		t.Fatalf("no-op label add recorded an event: %d → %d", len(before), len(after))
	}
}

func TestEventDuplicateDepNoSecondEvent(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 2, nil)
	b, _ := s.Create(ctx, "b", "task", 2, nil)
	if err := s.AddDep(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDep(ctx, a.ID, b.ID); err != nil { // duplicate, no-op
		t.Fatal(err)
	}
	var n int
	for _, k := range kindsFor(t, s, a.ID) {
		if k == "dep_added" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("duplicate dep add recorded %d dep_added events, want 1", n)
	}
}

func TestEventCreatedRecordsExtras(t *testing.T) {
	s := newTestStore(t)
	parent, _ := s.Create(ctx, "parent", "task", 2, nil)
	child, err := s.CreateWithLinks(ctx, "child", "task", 2, ptr("routed"), CreateOpts{
		Caps:        []string{"go-review"},
		Parents:     []string{parent.ID},
		Description: "desc",
		Notes:       "note",
	})
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := s.History(ctx, child.ID)
	p := *evs[0].Payload
	for _, want := range []string{`"labels":["cap:go-review"]`, `"depends_on":["` + parent.ID + `"]`, `"description":true`, `"notes":true`, `"assignee":"routed"`} {
		if !strings.Contains(p, want) {
			t.Fatalf("created payload missing %q:\n%s", want, p)
		}
	}
}

func TestEventCommentEditRemove(t *testing.T) {
	s := newTestStore(t)
	i, _ := s.Create(ctx, "t", "task", 2, nil)
	cm, err := s.AddComment(ctx, i.ID, "alice", "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EditComment(ctx, cm.ID, "edited"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveComment(ctx, cm.ID); err != nil {
		t.Fatal(err)
	}
	got := kindsFor(t, s, i.ID)
	want := []string{"created", "commented", "comment_edited", "comment_removed"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("comment event kinds:\n got %v\nwant %v", got, want)
	}
}

func TestEventUpdateUsesSemanticStatusKind(t *testing.T) {
	s := newTestStore(t)
	i, _ := s.Create(ctx, "t", "task", 2, nil)
	if _, err := s.Update(ctx, i.ID, UpdateFields{Status: ptr("closed")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, i.ID, UpdateFields{Status: ptr("open")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, i.ID, UpdateFields{Status: ptr("cancelled")}); err != nil {
		t.Fatal(err)
	}
	got := kindsFor(t, s, i.ID)
	want := []string{"created", "closed", "reopened", "cancelled"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("update status kinds:\n got %v\nwant %v", got, want)
	}
	// And they are reachable by kind filter.
	closed, _ := s.EventLog(ctx, EventFilter{IssueID: i.ID, Kind: "closed"})
	if len(closed) != 1 {
		t.Fatalf("log --kind closed found %d, want 1", len(closed))
	}
}
