package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	i, err := s.Create("first task", "task", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(i.ID, "bd-") || len(i.ID) != 7 {
		t.Fatalf("unexpected id: %q", i.ID)
	}
	if i.Title != "first task" || i.Priority != 1 || i.Status != "open" {
		t.Fatalf("unexpected issue: %+v", i)
	}
	got, err := s.Get(i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != i.ID {
		t.Fatalf("get returned %q", got.ID)
	}
}

func TestGetMissing(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get("bd-zzzz"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReadyExcludesBlocked(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create("a", "task", 1)
	b, _ := s.Create("b", "task", 1)
	if err := s.AddDep(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	ready, err := s.Ready(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0].ID != a.ID {
		t.Fatalf("expected only %s ready, got %+v", a.ID, ready)
	}
	if _, err := s.Close_(a.ID, ""); err != nil {
		t.Fatal(err)
	}
	ready, _ = s.Ready(10)
	if len(ready) != 1 || ready[0].ID != b.ID {
		t.Fatalf("expected b ready after closing a, got %+v", ready)
	}
}

func TestReadyExcludesAssigned(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create("a", "task", 1)
	if _, err := s.Claim("alice"); err != nil {
		t.Fatal(err)
	}
	ready, _ := s.Ready(10)
	if len(ready) != 0 {
		t.Fatalf("expected nothing ready after claim, got %+v", ready)
	}
	got, _ := s.Get(a.ID)
	if got.Status != "in_progress" || !got.Assignee.Valid || got.Assignee.String != "alice" {
		t.Fatalf("expected claimed by alice, got %+v", got)
	}
}

func TestReadyOrderedByPriority(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Create("low", "task", 5)
	hi, _ := s.Create("hi", "task", 0)
	_, _ = s.Create("mid", "task", 2)
	ready, _ := s.Ready(10)
	if ready[0].ID != hi.ID {
		t.Fatalf("expected hi first, got %s", ready[0].ID)
	}
}

func TestClaimAtomicityRace(t *testing.T) {
	s := newTestStore(t)
	const n = 50
	for i := 0; i < n; i++ {
		if _, err := s.Create("task", "task", 1); err != nil {
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
				_, err := s.Claim("w")
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
	a, _ := s.Create("a", "task", 1)
	b, _ := s.Create("b", "task", 1)
	c, _ := s.Create("c", "task", 1)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.AddDep(b.ID, a.ID)) // b depends on a
	must(s.AddDep(c.ID, b.ID)) // c depends on b
	// adding a -> c would form a cycle (a depends on c, c on b, b on a)
	if err := s.AddDep(a.ID, c.ID); err == nil {
		t.Fatal("expected cycle error")
	}
	// self-dep
	if err := s.AddDep(a.ID, a.ID); err == nil {
		t.Fatal("expected self-dep rejection")
	}
}

func TestUpdateFields(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create("orig", "task", 2)
	newTitle := "renamed"
	newPri := 0
	newStatus := "in_progress"
	got, err := s.Update(a.ID, UpdateFields{
		Title:    &newTitle,
		Priority: &newPri,
		Status:   &newStatus,
		Assignee: &sql.NullString{String: "bob", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "renamed" || got.Priority != 0 || got.Status != "in_progress" || got.Assignee.String != "bob" {
		t.Fatalf("update did not apply: %+v", got)
	}
}

func TestCloseTwice(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create("a", "task", 1)
	if _, err := s.Close_(a.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Close_(a.ID, ""); err == nil {
		t.Fatal("expected error closing already-closed issue")
	}
}

func TestClaimByID(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create("a", "task", 1)
	if _, err := s.ClaimByID(a.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimByID(a.ID, "bob"); err == nil {
		t.Fatal("expected re-claim to fail")
	}
}

func TestListFilter(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Create("a", "task", 1)
	b, _ := s.Create("b", "task", 1)
	_, _ = s.Close_(a.ID, "")
	open, _ := s.List("open")
	if len(open) != 1 || open[0].ID != b.ID {
		t.Fatalf("expected only b open, got %+v", open)
	}
	all, _ := s.List("")
	if len(all) != 2 {
		t.Fatalf("expected 2 issues total, got %d", len(all))
	}
}
