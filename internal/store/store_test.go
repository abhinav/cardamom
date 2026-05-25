package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ctx is the default context for tests. Individual tests may shadow this
// with context.WithTimeout/WithCancel when they need cancellation.
var ctx = context.Background()

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ptr[T any](v T) *T { return &v }

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	i, err := s.Create(ctx, "first task", "task", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(i.ID, "bd-") || len(i.ID) != 7 {
		t.Fatalf("unexpected id: %q", i.ID)
	}
	if i.Title != "first task" || i.Priority != 1 || i.Status != "open" {
		t.Fatalf("unexpected issue: %+v", i)
	}
	got, err := s.Get(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != i.ID {
		t.Fatalf("get returned %q", got.ID)
	}
	if got.Assignee != nil || got.Closed != nil {
		t.Fatalf("expected nil assignee/closed on fresh issue, got %+v", got)
	}
}

func TestGetMissing(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get(ctx, "bd-zzzz"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReadyExcludesBlocked(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)
	if err := s.AddDep(ctx, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	ready, err := s.Ready(ctx, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0].ID != a.ID {
		t.Fatalf("expected only %s ready, got %+v", a.ID, ready)
	}
	if _, err := s.MarkClosed(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	ready, _ = s.Ready(ctx, 10, nil)
	if len(ready) != 1 || ready[0].ID != b.ID {
		t.Fatalf("expected b ready after closing a, got %+v", ready)
	}
}

func TestReadyExcludesAssigned(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	if _, err := s.Claim(ctx, "alice", nil); err != nil {
		t.Fatal(err)
	}
	ready, _ := s.Ready(ctx, 10, nil)
	if len(ready) != 0 {
		t.Fatalf("expected nothing ready after claim, got %+v", ready)
	}
	got, _ := s.Get(ctx, a.ID)
	if got.Status != "in_progress" || got.Assignee == nil || *got.Assignee != "alice" {
		t.Fatalf("expected claimed by alice, got %+v", got)
	}
}

func TestReadyOrderedByPriority(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Create(ctx, "low", "task", 5, nil)
	hi, _ := s.Create(ctx, "hi", "task", 0, nil)
	_, _ = s.Create(ctx, "mid", "task", 2, nil)
	ready, _ := s.Ready(ctx, 10, nil)
	if ready[0].ID != hi.ID {
		t.Fatalf("expected hi first, got %s", ready[0].ID)
	}
}

func TestClaimAtomicityRace(t *testing.T) {
	s := newTestStore(t)
	const n = 50
	for i := 0; i < n; i++ {
		if _, err := s.Create(ctx, "task", "task", 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	const workers = 5
	claimed := make([]int, workers)
	for w := 0; w < workers; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				_, err := s.Claim(ctx, "w", nil)
				if err == ErrNotFound {
					return
				}
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				claimed[w]++
			}
		}()
	}
	wg.Wait()
	total := 0
	for _, c := range claimed {
		total += c
	}
	if total != n {
		t.Fatalf("expected exactly %d claims across workers, got %d (per-worker %v)", n, total, claimed)
	}
}

func TestAddDepCycleRejected(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)
	c, _ := s.Create(ctx, "c", "task", 1, nil)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.AddDep(ctx, b.ID, a.ID)) // b depends on a
	must(s.AddDep(ctx, c.ID, b.ID)) // c depends on b
	// adding a -> c would form a cycle
	if err := s.AddDep(ctx, a.ID, c.ID); err != ErrCycle {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
	if err := s.AddDep(ctx, a.ID, a.ID); err != ErrSelfDep {
		t.Fatalf("expected ErrSelfDep, got %v", err)
	}
}

func TestAddDepMissingIssue(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	if err := s.AddDep(ctx, a.ID, "bd-zzzz"); err == nil {
		t.Fatal("expected error for missing parent")
	}
	if err := s.AddDep(ctx, "bd-zzzz", a.ID); err == nil {
		t.Fatal("expected error for missing child")
	}
}

func TestUpdateFields(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "orig", "task", 2, nil)
	bob := ptr("bob")
	got, err := s.Update(ctx, a.ID, UpdateFields{
		Title:    ptr("renamed"),
		Priority: ptr(0),
		Status:   ptr("in_progress"),
		Assignee: &bob,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "renamed" || got.Priority != 0 || got.Status != "in_progress" || got.Assignee == nil || *got.Assignee != "bob" {
		t.Fatalf("update did not apply: %+v", got)
	}
}

func TestUpdateUnassign(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	_, _ = s.ClaimByID(ctx, a.ID, "alice")
	var none *string
	got, err := s.Update(ctx, a.ID, UpdateFields{Assignee: &none})
	if err != nil {
		t.Fatal(err)
	}
	if got.Assignee != nil {
		t.Fatalf("expected assignee cleared, got %v", *got.Assignee)
	}
}

func TestCloseTwice(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	if _, err := s.MarkClosed(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkClosed(ctx, a.ID); err != ErrAlreadyClosed {
		t.Fatalf("expected ErrAlreadyClosed, got %v", err)
	}
}

func TestClaimByID(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	if _, err := s.ClaimByID(ctx, a.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimByID(ctx, a.ID, "bob"); err == nil {
		t.Fatal("expected re-claim to fail")
	}
}

func TestListFilter(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)
	_, _ = s.MarkClosed(ctx, a.ID)
	open, _ := s.List(ctx, ListFilter{Status: "open"})
	if len(open) != 1 || open[0].ID != b.ID {
		t.Fatalf("expected only b open, got %+v", open)
	}
	all, _ := s.List(ctx, ListFilter{})
	if len(all) != 2 {
		t.Fatalf("expected 2 issues total, got %d", len(all))
	}
}

func TestAgentLanes(t *testing.T) {
	s := newTestStore(t)
	cr := "code-reviewer"
	wr := "writer"
	unassigned, _ := s.Create(ctx, "unassigned task", "task", 1, nil)
	reviewer, _ := s.Create(ctx, "review PR", "task", 1, &cr)
	writer, _ := s.Create(ctx, "write docs", "task", 1, &wr)

	// bd ready (no agent): only unassigned-lane issues.
	r, _ := s.Ready(ctx, 10, nil)
	if len(r) != 1 || r[0].ID != unassigned.ID {
		t.Fatalf("expected only unassigned issue ready, got %+v", r)
	}
	// bd ready -a code-reviewer: only that lane.
	r, _ = s.Ready(ctx, 10, &cr)
	if len(r) != 1 || r[0].ID != reviewer.ID {
		t.Fatalf("expected reviewer issue ready, got %+v", r)
	}
	r, _ = s.Ready(ctx, 10, &wr)
	if len(r) != 1 || r[0].ID != writer.ID {
		t.Fatalf("expected writer issue ready, got %+v", r)
	}

	// Claim respects lane.
	got, err := s.Claim(ctx, "alice", &cr)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != reviewer.ID {
		t.Fatalf("expected reviewer claim, got %s", got.ID)
	}
	if got.Agent == nil || *got.Agent != cr {
		t.Fatalf("agent lost on claim: %+v", got)
	}
}

func TestClaimNoneInLane(t *testing.T) {
	s := newTestStore(t)
	cr := "code-reviewer"
	_, _ = s.Create(ctx, "only unassigned", "task", 1, nil)
	if _, err := s.Claim(ctx, "alice", &cr); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound when nothing in lane, got %v", err)
	}
}

func TestWaitReadyReturnsImmediately(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Create(ctx, "ready now", "task", 1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	issues, err := s.WaitReady(ctx, 10, nil, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 ready issue, got %d", len(issues))
	}
}

func TestWaitReadyBlocksThenWakes(t *testing.T) {
	s := newTestStore(t)
	cr := "code-reviewer"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Insert after a small delay; WaitReady should pick it up.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = s.Create(ctx, "for cr", "task", 1, &cr)
	}()

	start := time.Now()
	issues, err := s.WaitReady(ctx, 10, &cr, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue after wait, got %d", len(issues))
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatalf("WaitReady returned too fast (didn't actually wait)")
	}
}

func TestWaitReadyCancellation(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := s.WaitReady(ctx, 10, nil, 10*time.Millisecond)
	if err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestUpdateAgent(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "task", "task", 1, nil)
	cr := ptr("code-reviewer")
	got, err := s.Update(ctx, a.ID, UpdateFields{Agent: &cr})
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent == nil || *got.Agent != "code-reviewer" {
		t.Fatalf("agent not set: %+v", got)
	}
	var none *string
	got, err = s.Update(ctx, a.ID, UpdateFields{Agent: &none})
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != nil {
		t.Fatalf("agent not cleared: %+v", got)
	}
}

func TestLabelsAddListRemove(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "with labels", "task", 1, nil)
	if err := s.AddLabels(ctx, a.ID, []string{"security", "p0"}); err != nil {
		t.Fatal(err)
	}
	// Adding the same again is a no-op.
	if err := s.AddLabels(ctx, a.ID, []string{"security"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.LabelsForIssue(ctx, a.ID)
	if len(got) != 2 || got[0] != "p0" || got[1] != "security" {
		t.Fatalf("expected [p0 security], got %v", got)
	}
	if err := s.RemoveLabels(ctx, a.ID, []string{"security"}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LabelsForIssue(ctx, a.ID)
	if len(got) != 1 || got[0] != "p0" {
		t.Fatalf("expected [p0] after rm, got %v", got)
	}
}

func TestAddLabelsMissingIssue(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddLabels(ctx, "bd-zzzz", []string{"x"}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLoadLabelsBatch(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)
	_, _ = s.Create(ctx, "c (no labels)", "task", 1, nil)
	_ = s.AddLabels(ctx, a.ID, []string{"x", "y"})
	_ = s.AddLabels(ctx, b.ID, []string{"y"})
	m, _ := s.LoadLabels(ctx, []string{a.ID, b.ID})
	if len(m[a.ID]) != 2 || len(m[b.ID]) != 1 {
		t.Fatalf("unexpected map: %+v", m)
	}
}

func TestListFilterByLabelsAnd(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)
	_ = s.AddLabels(ctx, a.ID, []string{"security", "p0"})
	_ = s.AddLabels(ctx, b.ID, []string{"security"})
	// Must have BOTH labels.
	got, _ := s.List(ctx, ListFilter{Labels: []string{"security", "p0"}})
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("expected only a, got %+v", got)
	}
}

func TestListFilterByLabelsAny(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)
	_, _ = s.Create(ctx, "c", "task", 1, nil)
	_ = s.AddLabels(ctx, a.ID, []string{"x"})
	_ = s.AddLabels(ctx, b.ID, []string{"y"})
	got, _ := s.List(ctx, ListFilter{LabelsAny: []string{"x", "y"}})
	if len(got) != 2 {
		t.Fatalf("expected 2 (a and b), got %d: %+v", len(got), got)
	}
}

func TestListFilterNoLabels(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "labeled", "task", 1, nil)
	b, _ := s.Create(ctx, "bare", "task", 1, nil)
	_ = s.AddLabels(ctx, a.ID, []string{"x"})
	got, _ := s.List(ctx, ListFilter{NoLabels: true})
	if len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("expected only b (unlabeled), got %+v", got)
	}
}

func TestListFilterPriorityRange(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Create(ctx, "p0", "task", 0, nil)
	mid, _ := s.Create(ctx, "p2", "task", 2, nil)
	_, _ = s.Create(ctx, "p4", "task", 4, nil)
	min1, max3 := 1, 3
	got, _ := s.List(ctx, ListFilter{PriorityMin: &min1, PriorityMax: &max3})
	if len(got) != 1 || got[0].ID != mid.ID {
		t.Fatalf("expected only p2, got %+v", got)
	}
}

func TestListFilterTitleContains(t *testing.T) {
	s := newTestStore(t)
	hit, _ := s.Create(ctx, "Fix the BUG today", "task", 1, nil)
	_, _ = s.Create(ctx, "tomorrow", "task", 1, nil)
	got, _ := s.List(ctx, ListFilter{TitleContains: "bug"})
	if len(got) != 1 || got[0].ID != hit.ID {
		t.Fatalf("expected only the bug issue, got %+v", got)
	}
}

func TestListFilterIDs(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	_, _ = s.Create(ctx, "b", "task", 1, nil)
	c, _ := s.Create(ctx, "c", "task", 1, nil)
	got, _ := s.List(ctx, ListFilter{IDs: []string{a.ID, c.ID}})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestListFilterNoAssignee(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Create(ctx, "unclaimed", "task", 1, nil)
	a, _ := s.Create(ctx, "claimed", "task", 1, nil)
	_, _ = s.ClaimByID(ctx, a.ID, "alice")
	got, _ := s.List(ctx, ListFilter{NoAssignee: true})
	if len(got) != 1 || got[0].Title != "unclaimed" {
		t.Fatalf("expected only unclaimed, got %+v", got)
	}
}

func TestDeferExcludesFromReady(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "deferred", "task", 1, nil)
	_, _ = s.Create(ctx, "open", "task", 1, nil)
	future := time.Now().Add(time.Hour).Unix()
	if _, err := s.SetDefer(ctx, a.ID, &future); err != nil {
		t.Fatal(err)
	}
	ready, _ := s.Ready(ctx, 10, nil)
	if len(ready) != 1 || ready[0].ID == a.ID {
		t.Fatalf("expected the deferred one to be excluded, got %+v", ready)
	}
}

func TestDeferPastDateIsReady(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "was deferred", "task", 1, nil)
	past := time.Now().Add(-time.Hour).Unix()
	_, _ = s.SetDefer(ctx, a.ID, &past)
	ready, _ := s.Ready(ctx, 10, nil)
	if len(ready) != 1 || ready[0].ID != a.ID {
		t.Fatalf("expected past-deferred to be ready, got %+v", ready)
	}
}

func TestUndeferRestoresReady(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "to undefer", "task", 1, nil)
	future := time.Now().Add(time.Hour).Unix()
	_, _ = s.SetDefer(ctx, a.ID, &future)
	if len(mustReady(t, s)) != 0 {
		t.Fatal("expected deferred to be excluded")
	}
	_, _ = s.SetDefer(ctx, a.ID, nil)
	r := mustReady(t, s)
	if len(r) != 1 || r[0].ID != a.ID {
		t.Fatalf("expected undeferred to be ready, got %+v", r)
	}
	got, _ := s.Get(ctx, a.ID)
	if got.DeferUntil != nil {
		t.Fatalf("expected DeferUntil cleared, got %v", *got.DeferUntil)
	}
}

func mustReady(t *testing.T, s *Store) []Issue {
	t.Helper()
	r, err := s.Ready(ctx, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestListFilterDeferred(t *testing.T) {
	s := newTestStore(t)
	d, _ := s.Create(ctx, "deferred", "task", 1, nil)
	_, _ = s.Create(ctx, "normal", "task", 1, nil)
	future := time.Now().Add(time.Hour).Unix()
	_, _ = s.SetDefer(ctx, d.ID, &future)
	got, _ := s.List(ctx, ListFilter{Deferred: true})
	if len(got) != 1 || got[0].ID != d.ID {
		t.Fatalf("expected only deferred, got %+v", got)
	}
}

func TestListFilterOverdue(t *testing.T) {
	s := newTestStore(t)
	od, _ := s.Create(ctx, "overdue", "task", 1, nil)
	_, _ = s.Create(ctx, "future", "task", 1, nil)
	past := time.Now().Add(-time.Hour).Unix()
	future := time.Now().Add(time.Hour).Unix()
	_, _ = s.SetDefer(ctx, od.ID, &past)
	// also defer the second one to the future to confirm it's not picked up
	other, _ := s.Create(ctx, "other future", "task", 1, nil)
	_, _ = s.SetDefer(ctx, other.ID, &future)
	got, _ := s.List(ctx, ListFilter{Overdue: true})
	if len(got) != 1 || got[0].ID != od.ID {
		t.Fatalf("expected only overdue, got %+v", got)
	}
}

func TestBlocked(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)
	_, _ = s.Create(ctx, "free", "task", 1, nil)
	_ = s.AddDep(ctx, b.ID, a.ID)
	got, _ := s.Blocked(ctx, 10, nil)
	if len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("expected only b blocked, got %+v", got)
	}
}

func TestCountWithFilter(t *testing.T) {
	s := newTestStore(t)
	cr := "code-reviewer"
	_, _ = s.Create(ctx, "1", "task", 1, &cr)
	_, _ = s.Create(ctx, "2", "task", 1, &cr)
	_, _ = s.Create(ctx, "3", "task", 1, nil)
	n, _ := s.Count(ctx, ListFilter{Agent: &cr})
	if n != 2 {
		t.Fatalf("expected 2 for code-reviewer, got %d", n)
	}
	n, _ = s.Count(ctx, ListFilter{})
	if n != 3 {
		t.Fatalf("expected 3 total, got %d", n)
	}
}

func TestStats(t *testing.T) {
	s := newTestStore(t)
	cr := "code-reviewer"
	a, _ := s.Create(ctx, "1", "task", 1, &cr)
	_, _ = s.Create(ctx, "2", "bug", 1, nil)
	_, _ = s.Create(ctx, "3", "task", 1, nil)
	_, _ = s.MarkClosed(ctx, a.ID)
	st, _ := s.Stats(ctx)
	if st.Status["closed"] != 1 || st.Status["open"] != 2 {
		t.Fatalf("status counts wrong: %+v", st.Status)
	}
	if st.Agents["<none>"] != 2 || st.Agents["code-reviewer"] != 1 {
		t.Fatalf("agent counts wrong: %+v", st.Agents)
	}
	if st.Types["task"] != 2 || st.Types["bug"] != 1 {
		t.Fatalf("type counts wrong: %+v", st.Types)
	}
}

func TestReopen(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	_, _ = s.MarkClosed(ctx, a.ID)
	got, err := s.Reopen(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "open" || got.Closed != nil {
		t.Fatalf("expected open + closed=nil, got %+v", got)
	}
}

func TestReopenAlreadyOpen(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	if _, err := s.Reopen(ctx, a.ID); err != ErrAlreadyOpen {
		t.Fatalf("expected ErrAlreadyOpen, got %v", err)
	}
}

func TestReopenMissing(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Reopen(ctx, "bd-zzzz"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDepsListing(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)
	c, _ := s.Create(ctx, "c", "task", 1, nil)
	_ = s.AddDep(ctx, b.ID, a.ID)
	_ = s.AddDep(ctx, c.ID, a.ID)
	parents, blocks, err := s.Deps(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parents) != 0 {
		t.Fatalf("a has no parents, got %+v", parents)
	}
	if len(blocks) != 2 {
		t.Fatalf("a should block 2, got %+v", blocks)
	}
}
