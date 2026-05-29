package store

import (
	"fmt"
	"strings"
	"testing"
)

func TestBatchCreateHappyGraph(t *testing.T) {
	s := newTestStore(t)
	s.SetActor("gen")
	issues := []BatchIssue{
		{Alias: "design", Title: "Design", Priority: 1, Capabilities: []string{"architecture"}},
		{Alias: "impl", Title: "Implement", Priority: 1, Needs: []string{"design"}, Capabilities: []string{"go"}},
		{Alias: "tests", Title: "Tests", Needs: []string{"impl"}},
		{Alias: "docs", Title: "Docs", Needs: []string{"impl"}},
		{Alias: "ship", Title: "Ship", Priority: 0, Needs: []string{"tests", "docs"}},
	}
	mapping, stats, err := s.BatchCreate(ctx, issues)
	if err != nil {
		t.Fatal(err)
	}
	if len(mapping) != 5 {
		t.Fatalf("mapping has %d entries, want 5", len(mapping))
	}
	if stats.Issues != 5 || stats.Edges != 5 {
		t.Fatalf("stats = %+v", stats)
	}
	// design has no prereqs → root; ship is nothing's prereq → leaf.
	if stats.Roots != 1 || stats.Leaves != 1 {
		t.Fatalf("roots/leaves = %d/%d, want 1/1 (%+v)", stats.Roots, stats.Leaves, stats)
	}
	// depth: design→impl→tests→ship = 4 nodes.
	if stats.MaxDepth != 4 {
		t.Fatalf("max depth = %d, want 4", stats.MaxDepth)
	}

	// Real issues exist with resolved deps.
	shipID := mapping["ship"]
	parents, _, err := s.Deps(ctx, shipID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parents) != 2 {
		t.Fatalf("ship has %d parents, want 2", len(parents))
	}
	// cap label applied.
	labels, _ := s.LabelsForIssue(ctx, mapping["impl"])
	if strings.Join(labels, ",") != "cap:go" {
		t.Fatalf("impl labels = %v, want [cap:go]", labels)
	}
	// created event recorded with actor.
	evs, _ := s.History(ctx, mapping["design"])
	if len(evs) != 1 || evs[0].Kind != "created" || evs[0].Actor == nil || *evs[0].Actor != "gen" {
		t.Fatalf("design events = %+v", evs)
	}
}

func TestBatchExternalRef(t *testing.T) {
	s := newTestStore(t)
	// Pre-existing epic.
	epic, _ := s.Create(ctx, "epic", "epic", 1, nil)
	issues := []BatchIssue{
		{Alias: "sub", Title: "Subtask", Needs: []string{epic.ID}}, // external real ID
	}
	mapping, stats, err := s.BatchCreate(ctx, issues)
	if err != nil {
		t.Fatal(err)
	}
	if stats.External != 1 {
		t.Fatalf("external = %d, want 1", stats.External)
	}
	parents, _, _ := s.Deps(ctx, mapping["sub"])
	if len(parents) != 1 || parents[0] != epic.ID {
		t.Fatalf("sub parents = %v, want [%s]", parents, epic.ID)
	}
}

func TestBatchCycleDetected(t *testing.T) {
	s := newTestStore(t)
	issues := []BatchIssue{
		{Alias: "a", Title: "A", Needs: []string{"c"}},
		{Alias: "b", Title: "B", Needs: []string{"a"}},
		{Alias: "c", Title: "C", Needs: []string{"b"}},
	}
	_, _, err := s.BatchCreate(ctx, issues)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error should mention cycle: %v", err)
	}
	// Nothing written.
	n, _ := s.db.NewSelect().Model((*Issue)(nil)).Count(ctx)
	if n != 0 {
		t.Fatalf("cycle batch wrote %d issues, want 0", n)
	}
}

func TestBatchAllErrorsAtOnce(t *testing.T) {
	s := newTestStore(t)
	issues := []BatchIssue{
		{Alias: "a", Title: ""},                            // missing title
		{Alias: "a", Title: "dup"},                         // duplicate alias
		{Alias: "b", Title: "B", Needs: []string{"ghost"}}, // unknown ref
		{Alias: "c", Title: "C", Priority: 9},              // bad priority
	}
	_, _, err := s.BatchCreate(ctx, issues)
	if err == nil {
		t.Fatal("expected validation errors")
	}
	msg := err.Error()
	for _, want := range []string{"title required", "duplicated", "unknown issue", "priority"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error missing %q:\n%s", want, msg)
		}
	}
}

func TestBatchValidateNoWrite(t *testing.T) {
	s := newTestStore(t)
	issues := []BatchIssue{
		{Alias: "a", Title: "A"},
		{Alias: "b", Title: "B", Needs: []string{"a"}},
	}
	stats, err := s.BatchValidate(ctx, issues)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Issues != 2 || stats.Edges != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	n, _ := s.db.NewSelect().Model((*Issue)(nil)).Count(ctx)
	if n != 0 {
		t.Fatalf("dry-run wrote %d issues, want 0", n)
	}
}

func TestBatchSelfDep(t *testing.T) {
	s := newTestStore(t)
	_, _, err := s.BatchCreate(ctx, []BatchIssue{{Alias: "a", Title: "A", Needs: []string{"a"}}})
	if err == nil || !strings.Contains(err.Error(), "itself") {
		t.Fatalf("expected self-dep error, got %v", err)
	}
}

// TestBatchScale builds a thousand-node graph with a chain + fan-in to
// exercise chunked inserts, ID allocation, and cycle/stats at size.
func TestBatchScale(t *testing.T) {
	s := newTestStore(t)
	const n = 1000
	issues := make([]BatchIssue, n)
	for i := 0; i < n; i++ {
		bi := BatchIssue{Alias: fmt.Sprintf("t%d", i), Title: fmt.Sprintf("Task %d", i)}
		if i > 0 {
			// chain to previous + fan-in to t0 to create real edges.
			bi.Needs = []string{fmt.Sprintf("t%d", i-1)}
			if i > 1 {
				bi.Needs = append(bi.Needs, "t0")
			}
		}
		issues[i] = bi
	}
	mapping, stats, err := s.BatchCreate(ctx, issues)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Issues != n {
		t.Fatalf("created %d, want %d", stats.Issues, n)
	}
	if len(mapping) != n {
		t.Fatalf("mapping %d, want %d", len(mapping), n)
	}
	// All IDs unique.
	seen := map[string]bool{}
	for _, id := range mapping {
		if seen[id] {
			t.Fatalf("duplicate id allocated: %s", id)
		}
		seen[id] = true
	}
	count, _ := s.db.NewSelect().Model((*Issue)(nil)).Count(ctx)
	if count != n {
		t.Fatalf("db has %d issues, want %d", count, n)
	}
}
