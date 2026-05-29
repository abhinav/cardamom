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
	res, err := s.BatchCreate(ctx, issues, nil, BatchSkip)
	mapping, stats := res.Mapping, res.Stats
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
	res, err := s.BatchCreate(ctx, issues, nil, BatchSkip)
	mapping, stats := res.Mapping, res.Stats
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
	_, err := s.BatchCreate(ctx, issues, nil, BatchSkip)
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
	_, err := s.BatchCreate(ctx, issues, nil, BatchSkip)
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
	_, err := s.BatchCreate(ctx, []BatchIssue{{Alias: "a", Title: "A", Needs: []string{"a"}}}, nil, BatchSkip)
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
	res, err := s.BatchCreate(ctx, issues, nil, BatchSkip)
	mapping, stats := res.Mapping, res.Stats
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

func TestBatchCheckpoint(t *testing.T) {
	s := newTestStore(t)
	issues := []BatchIssue{
		{Alias: "impl", Title: "Implement"},
		{Alias: "gate", Title: "Approve deploy", Needs: []string{"impl"},
			Checkpoint: &CheckpointPayload{Approvers: []string{"alice"}}},
		{Alias: "manual-gate", Title: "Manual gate", Checkpoint: &CheckpointPayload{}},
	}
	res, err := s.BatchCreate(ctx, issues, nil, BatchSkip)
	mapping, stats := res.Mapping, res.Stats
	if err != nil {
		t.Fatal(err)
	}
	if stats.Checkpoints != 2 {
		t.Fatalf("checkpoints = %d, want 2", stats.Checkpoints)
	}

	// Checkpoint issue: type forced, pending label, cp KV with inferred kind.
	gateID := mapping["gate"]
	gate, _ := s.Get(ctx, gateID)
	if gate.Type != "checkpoint" {
		t.Fatalf("gate type = %q, want checkpoint", gate.Type)
	}
	labels, _ := s.LabelsForIssue(ctx, gateID)
	if !contains(labels, "checkpoint:pending") {
		t.Fatalf("gate labels missing checkpoint:pending: %v", labels)
	}
	pay, err := s.GetCheckpointPayload(ctx, gateID)
	if err != nil {
		t.Fatal(err)
	}
	if pay.Kind != "approval" || len(pay.Approvers) != 1 || pay.Approvers[0] != "alice" {
		t.Fatalf("gate payload = %+v, want approval/[alice]", pay)
	}
	// No-approvers checkpoint infers manual.
	mpay, err := s.GetCheckpointPayload(ctx, mapping["manual-gate"])
	if err != nil {
		t.Fatal(err)
	}
	if mpay.Kind != "manual" {
		t.Fatalf("manual-gate kind = %q, want manual", mpay.Kind)
	}

	// The gate blocks until its prereq closes — batch-created deps drive
	// the same checkpoint gating as `clu run`.
	if _, err := s.ResolveCheckpoint(ctx, gateID, "alice", true, "ok"); err == nil {
		t.Fatal("expected checkpoint to block on its open prerequisite")
	}
	if _, err := s.MarkClosed(ctx, mapping["impl"]); err != nil {
		t.Fatal(err)
	}
	// Now it resolves like a real checkpoint.
	if _, err := s.ResolveCheckpoint(ctx, gateID, "alice", true, "ok"); err != nil {
		t.Fatalf("resolve checkpoint: %v", err)
	}
	g2, _ := s.Get(ctx, gateID)
	if g2.Status != "closed" {
		t.Fatalf("passed checkpoint status = %q, want closed", g2.Status)
	}
}

func TestBatchCheckpointTypeConflict(t *testing.T) {
	s := newTestStore(t)
	_, err := s.BatchCreate(ctx, []BatchIssue{
		{Alias: "x", Title: "X", Type: "task", Checkpoint: &CheckpointPayload{}},
	}, nil, BatchSkip)
	if err == nil || !strings.Contains(err.Error(), "checkpoint set but type") {
		t.Fatalf("expected type-conflict error, got %v", err)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestBatchGroup(t *testing.T) {
	s := newTestStore(t)
	issues := []BatchIssue{
		{Alias: "a", Title: "A"},
		{Alias: "b", Title: "B", Needs: []string{"a"}},
		{Alias: "c", Title: "C", Needs: []string{"a"}}, // two leaves: b, c
	}
	res, err := s.BatchCreate(ctx, issues, &BatchGroup{Title: "My Rollout", Description: "desc"}, BatchSkip)
	if err != nil {
		t.Fatal(err)
	}
	if res.ParentID == "" {
		t.Fatal("expected a group parent id")
	}
	parent, err := s.Get(ctx, res.ParentID)
	if err != nil {
		t.Fatal(err)
	}
	if parent.Title != "My Rollout" {
		t.Fatalf("parent title = %q", parent.Title)
	}
	runLabel := "run:" + res.ParentID
	// Parent + every issue carry the run label.
	for _, id := range []string{res.ParentID, res.Mapping["a"], res.Mapping["b"], res.Mapping["c"]} {
		labels, _ := s.LabelsForIssue(ctx, id)
		if !contains(labels, runLabel) {
			t.Fatalf("%s missing %s: %v", id, runLabel, labels)
		}
	}
	// Parent depends on both leaves (b, c) — not on a.
	parents, _, _ := s.Deps(ctx, res.ParentID)
	if len(parents) != 2 {
		t.Fatalf("parent should depend on 2 leaves, got %d: %v", len(parents), parents)
	}
	for _, p := range parents {
		if p == res.Mapping["a"] {
			t.Fatalf("parent should not depend on non-leaf a")
		}
	}
	// The whole group is addressable by the run label.
	grouped, err := s.List(ctx, ListFilter{Labels: []string{runLabel}, Statuses: []string{"open", "in_progress", "closed", "cancelled"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped) != 4 { // 3 issues + parent
		t.Fatalf("run label should match 4 issues, got %d", len(grouped))
	}
}

func TestBatchDedupesLabels(t *testing.T) {
	s := newTestStore(t)
	// Repeated label, and a capability that collides with an explicit
	// cap: label — both must be deduped, not crash on the unique constraint.
	issues := []BatchIssue{
		{Alias: "a", Title: "A", Labels: []string{"x", "x"}},
		{Alias: "b", Title: "B", Capabilities: []string{"go"}, Labels: []string{"cap:go"}},
	}
	res, err := s.BatchCreate(ctx, issues, nil, BatchSkip)
	if err != nil {
		t.Fatalf("dedupe should avoid constraint error: %v", err)
	}
	la, _ := s.LabelsForIssue(ctx, res.Mapping["a"])
	if len(la) != 1 || la[0] != "x" {
		t.Fatalf("a labels = %v, want [x]", la)
	}
	lb, _ := s.LabelsForIssue(ctx, res.Mapping["b"])
	if len(lb) != 1 || lb[0] != "cap:go" {
		t.Fatalf("b labels = %v, want [cap:go]", lb)
	}
}

func TestBatchIdempotentSkip(t *testing.T) {
	s := newTestStore(t)
	graph := []BatchIssue{
		{Alias: "a", Title: "Ticket A", Key: "linear:ENG-1"},
		{Alias: "b", Title: "Ticket B", Key: "linear:ENG-2", Needs: []string{"a"}},
	}
	// First run: both new.
	r1, err := s.BatchCreate(ctx, graph, nil, BatchSkip)
	if err != nil {
		t.Fatal(err)
	}
	if r1.New != 2 || r1.Existing != 0 {
		t.Fatalf("run1 new/existing = %d/%d, want 2/0", r1.New, r1.Existing)
	}

	// Second run, same keys: both skipped, no duplicates, aliases resolve to
	// the SAME ids.
	r2, err := s.BatchCreate(ctx, graph, nil, BatchSkip)
	if err != nil {
		t.Fatal(err)
	}
	if r2.New != 0 || r2.Existing != 2 {
		t.Fatalf("run2 new/existing = %d/%d, want 0/2", r2.New, r2.Existing)
	}
	if r2.Mapping["a"] != r1.Mapping["a"] || r2.Mapping["b"] != r1.Mapping["b"] {
		t.Fatalf("re-run resolved to different ids: %v vs %v", r2.Mapping, r1.Mapping)
	}
	// Total issues in the DB is still 2.
	n, _ := s.db.NewSelect().Model((*Issue)(nil)).Count(ctx)
	if n != 2 {
		t.Fatalf("re-run duplicated issues: db has %d, want 2", n)
	}
}

func TestBatchIdempotentUpdate(t *testing.T) {
	s := newTestStore(t)
	_, err := s.BatchCreate(ctx, []BatchIssue{
		{Alias: "a", Title: "Old title", Priority: 3, Key: "linear:ENG-1"},
	}, nil, BatchSkip)
	if err != nil {
		t.Fatal(err)
	}
	// Re-run in update mode with a changed title/priority.
	r, err := s.BatchCreate(ctx, []BatchIssue{
		{Alias: "a", Title: "New title", Priority: 0, Key: "linear:ENG-1"},
	}, nil, BatchUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if r.New != 0 || r.Updated != 1 {
		t.Fatalf("new/updated = %d/%d, want 0/1", r.New, r.Updated)
	}
	got, _ := s.Get(ctx, r.Mapping["a"])
	if got.Title != "New title" || got.Priority != 0 {
		t.Fatalf("update didn't sync fields: %+v", got)
	}
}

func TestBatchNewIssueCanNeedExistingKeyed(t *testing.T) {
	s := newTestStore(t)
	r1, _ := s.BatchCreate(ctx, []BatchIssue{{Alias: "a", Title: "A", Key: "k:a"}}, nil, BatchSkip)
	// Second batch: a already exists (by key); b is new and needs a.
	r2, err := s.BatchCreate(ctx, []BatchIssue{
		{Alias: "a", Title: "A", Key: "k:a"},
		{Alias: "b", Title: "B", Needs: []string{"a"}},
	}, nil, BatchSkip)
	if err != nil {
		t.Fatal(err)
	}
	if r2.New != 1 || r2.Existing != 1 {
		t.Fatalf("new/existing = %d/%d, want 1/1", r2.New, r2.Existing)
	}
	// b's dep resolves to the EXISTING a.
	parents, _, _ := s.Deps(ctx, r2.Mapping["b"])
	if len(parents) != 1 || parents[0] != r1.Mapping["a"] {
		t.Fatalf("b should depend on existing a (%s), got %v", r1.Mapping["a"], parents)
	}
}

func TestBatchDuplicateKeyInBatch(t *testing.T) {
	s := newTestStore(t)
	_, err := s.BatchCreate(ctx, []BatchIssue{
		{Alias: "a", Title: "A", Key: "dup"},
		{Alias: "b", Title: "B", Key: "dup"},
	}, nil, BatchSkip)
	if err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("expected duplicate-key error, got %v", err)
	}
}
