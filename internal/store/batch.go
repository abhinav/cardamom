package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/uptrace/bun"
)

// BatchIssue is one node in a batch graph. Alias is a local handle used
// only to wire Needs edges within the batch; it is not stored. Needs
// entries are either another issue's Alias (an internal edge) or an
// existing real issue ID (an external edge onto the committed graph).
type BatchIssue struct {
	Alias        string
	Title        string
	Type         string
	Priority     int
	Assignee     *string
	Description  *string
	Notes        *string
	Capabilities []string // bare names; stored as cap:<name> labels
	Labels       []string // arbitrary extra labels
	Needs        []string // aliases or existing real IDs

	// Checkpoint, if set, makes this issue a manual gate: type is forced
	// to "checkpoint", a checkpoint:pending label is added, and the
	// payload is written to the cp:<id> KV row — exactly what `clu run`
	// wires up, so `clu approve` / `clu checkpoint pass|fail` work.
	Checkpoint *CheckpointPayload
}

// BatchStats summarizes a validated batch graph. Returned by BatchValidate
// (dry-run) and BatchCreate so callers can show what was/would be built.
type BatchStats struct {
	Issues      int `json:"issues"`
	Edges       int `json:"edges"`         // total Needs references (internal + external)
	Roots       int `json:"roots"`         // issues with no prerequisites (ready immediately)
	Leaves      int `json:"leaves"`        // issues nothing else depends on (terminal goals)
	MaxDepth    int `json:"max_depth"`     // longest dependency chain (in nodes)
	External    int `json:"external_refs"` // edges onto pre-existing issues
	Checkpoints int `json:"checkpoints"`   // issues that are manual gates
}

// chunkInsertParams caps how many bind parameters one INSERT uses, kept
// well under SQLite's SQLITE_MAX_VARIABLE_NUMBER (999 on old builds) so
// chunked bulk inserts work on every modernc.org/sqlite version.
const chunkInsertParams = 800

// batchPlan is the validated, resolved form of a batch, ready to write.
type batchPlan struct {
	issues   []BatchIssue
	stats    BatchStats
	internal [][]int    // internal[i] = indices of batch issues that issue i needs
	external [][]string // external[i] = existing real IDs that issue i needs
}

// batchPrepare runs all validation that doesn't require allocated IDs:
// structural field checks, alias uniqueness, Needs resolution (alias vs.
// existing real ID, with a single existence query for the externals),
// cycle detection over the internal graph, and stats. Every problem is
// collected and returned together (errors.Join) so a generator can fix
// in one pass instead of fix-rerun-repeat.
func (s *Store) batchPrepare(ctx context.Context, issues []BatchIssue) (*batchPlan, error) {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if len(issues) == 0 {
		return nil, fmt.Errorf("%w: batch is empty", ErrInvalid)
	}

	// Index aliases; flag duplicates and empties.
	aliasIdx := make(map[string]int, len(issues))
	for i, iss := range issues {
		al := strings.TrimSpace(iss.Alias)
		if al == "" {
			add("issue #%d (%q): alias required", i+1, iss.Title)
			continue
		}
		if prev, ok := aliasIdx[al]; ok {
			add("alias %q: duplicated (issues #%d and #%d)", al, prev+1, i+1)
			continue
		}
		aliasIdx[al] = i
	}

	// Per-issue field validation.
	for i, iss := range issues {
		ref := iss.Alias
		if ref == "" {
			ref = fmt.Sprintf("#%d", i+1)
		}
		if strings.TrimSpace(iss.Title) == "" {
			add("%s: title required", ref)
		}
		typ := iss.Type
		if typ != "" {
			if err := ValidateType(typ); err != nil {
				add("%s: %v", ref, err)
			}
		}
		if iss.Checkpoint != nil {
			if typ != "" && typ != "checkpoint" {
				add("%s: checkpoint set but type is %q (must be checkpoint or omitted)", ref, typ)
			}
			if k := iss.Checkpoint.Kind; k != "" && k != "manual" && k != "approval" {
				add("%s: invalid checkpoint kind %q (manual or approval)", ref, k)
			}
		}
		if err := ValidatePriority(iss.Priority); err != nil {
			add("%s: %v", ref, err)
		}
		for _, c := range iss.Capabilities {
			if strings.TrimSpace(c) == "" {
				add("%s: capability cannot be empty", ref)
			}
		}
		for _, l := range iss.Labels {
			if strings.TrimSpace(l) == "" {
				add("%s: label cannot be empty", ref)
			}
		}
	}

	// Resolve Needs into internal (alias) and external (real ID) edges.
	internal := make([][]int, len(issues))
	external := make([][]string, len(issues))
	extSet := map[string]struct{}{}
	edgeCount := 0
	for i, iss := range issues {
		ref := iss.Alias
		if ref == "" {
			ref = fmt.Sprintf("#%d", i+1)
		}
		seen := map[string]struct{}{}
		for _, need := range iss.Needs {
			need = strings.TrimSpace(need)
			if need == "" {
				add("%s: empty entry in needs", ref)
				continue
			}
			if _, dup := seen[need]; dup {
				continue // tolerate a repeated need; just one edge
			}
			seen[need] = struct{}{}
			edgeCount++
			if j, ok := aliasIdx[need]; ok {
				if j == i {
					add("%s: cannot depend on itself", ref)
					continue
				}
				internal[i] = append(internal[i], j)
			} else {
				external[i] = append(external[i], need)
				extSet[need] = struct{}{}
			}
		}
	}

	// One existence query for every external ID referenced.
	if len(extSet) > 0 {
		extIDs := make([]string, 0, len(extSet))
		for id := range extSet {
			extIDs = append(extIDs, id)
		}
		found, err := s.existingIDs(ctx, extIDs)
		if err != nil {
			return nil, err
		}
		// Deterministic error ordering: report missing refs sorted.
		var missing []string
		for _, id := range extIDs {
			if _, ok := found[id]; !ok {
				missing = append(missing, id)
			}
		}
		sort.Strings(missing)
		for _, id := range missing {
			add("needs references unknown issue %q (not a batch alias, and no such issue exists)", id)
		}
	}

	// Cycle detection + stats only make sense once the graph is structurally
	// sound enough to walk. If aliases/refs already failed, bail with those.
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w:\n  %s", ErrInvalid, joinErrs(errs))
	}

	if cyclePath := detectCycle(internal, issues); cyclePath != "" {
		return nil, fmt.Errorf("%w: dependency cycle: %s", ErrInvalid, cyclePath)
	}

	plan := &batchPlan{issues: issues, internal: internal, external: external}
	plan.stats = batchStats(issues, internal, external, edgeCount)
	for _, iss := range issues {
		if iss.Checkpoint != nil {
			plan.stats.Checkpoints++
		}
	}
	return plan, nil
}

// existingIDs returns the subset of ids that exist in the issues table,
// queried in chunks to respect the bind-variable limit.
func (s *Store) existingIDs(ctx context.Context, ids []string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(ids))
	for start := 0; start < len(ids); start += chunkInsertParams {
		end := min(start+chunkInsertParams, len(ids))
		var got []string
		err := s.db.NewSelect().Model((*Issue)(nil)).
			Column("id").
			Where("id IN (?)", bun.In(ids[start:end])).
			Scan(ctx, &got)
		if err != nil {
			return nil, err
		}
		for _, id := range got {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

// detectCycle returns a human-readable cycle path (alias → alias → …) over
// the internal edge graph, or "" if the graph is acyclic. Iterative
// three-color DFS so a pathological 1k-deep chain can't blow the stack.
func detectCycle(internal [][]int, issues []BatchIssue) string {
	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS stack
		black = 2 // fully explored
	)
	color := make([]int, len(issues))
	var path []int // current DFS stack (node indices)

	var visit func(n int) []int
	visit = func(n int) []int {
		color[n] = gray
		path = append(path, n)
		for _, m := range internal[n] {
			switch color[m] {
			case white:
				if cyc := visit(m); cyc != nil {
					return cyc
				}
			case gray:
				// Back-edge: m is on the stack. Slice from m to the top.
				for k := len(path) - 1; k >= 0; k-- {
					if path[k] == m {
						cyc := append([]int(nil), path[k:]...)
						return append(cyc, m) // close the loop
					}
				}
			}
		}
		color[n] = black
		path = path[:len(path)-1]
		return nil
	}

	for n := range issues {
		if color[n] == white {
			if cyc := visit(n); cyc != nil {
				parts := make([]string, len(cyc))
				for i, idx := range cyc {
					parts[i] = issues[idx].Alias
				}
				return strings.Join(parts, " → ")
			}
		}
	}
	return ""
}

// batchStats computes the summary over a validated (acyclic) graph.
func batchStats(issues []BatchIssue, internal [][]int, external [][]string, edges int) BatchStats {
	n := len(issues)
	st := BatchStats{Issues: n, Edges: edges}

	// Roots: no prerequisites at all (ready immediately).
	// Leaves: nothing internal depends on them (terminal goals).
	hasDependent := make([]bool, n)
	for i := 0; i < n; i++ {
		if len(internal[i]) == 0 && len(external[i]) == 0 {
			st.Roots++
		}
		for _, j := range internal[i] {
			hasDependent[j] = true
		}
		st.External += len(external[i])
	}
	for i := 0; i < n; i++ {
		if !hasDependent[i] {
			st.Leaves++
		}
	}

	// Longest dependency chain (nodes) via memoized DFS over internal edges.
	depth := make([]int, n)
	var dfs func(i int) int
	dfs = func(i int) int {
		if depth[i] != 0 {
			return depth[i]
		}
		best := 1
		for _, j := range internal[i] {
			if d := dfs(j) + 1; d > best {
				best = d
			}
		}
		depth[i] = best
		return best
	}
	for i := 0; i < n; i++ {
		if d := dfs(i); d > st.MaxDepth {
			st.MaxDepth = d
		}
	}
	return st
}

// BatchGroup, when passed to BatchCreate, wraps the whole batch under a
// parent issue: the parent is created, every batched issue (and the parent)
// gets a run:<parent-id> label, and the parent depends on every leaf so it
// only closes once the whole graph is done. This reuses `clu run`'s grouping
// convention, so `clu list -l run:<id>` and the web run view work on
// batch-created graphs too.
type BatchGroup struct {
	Title       string
	Description string
}

// BatchResult is what BatchCreate returns: the alias→real-ID map, the
// optional group parent ID, and the graph stats.
type BatchResult struct {
	Mapping  map[string]string `json:"created"`
	ParentID string            `json:"group,omitempty"`
	Stats    BatchStats        `json:"stats"`
}

// BatchValidate validates a batch without writing anything and returns its
// stats. Used by `clu batch --dry-run`.
func (s *Store) BatchValidate(ctx context.Context, issues []BatchIssue) (BatchStats, error) {
	plan, err := s.batchPrepare(ctx, issues)
	if err != nil {
		return BatchStats{}, err
	}
	return plan.stats, nil
}

// BatchCreate validates, allocates IDs, and writes the whole graph in one
// transaction. With a non-nil group it also creates a parent umbrella issue
// (see BatchGroup). A single bad input aborts everything — by the time
// anything is written the graph is already proven valid.
func (s *Store) BatchCreate(ctx context.Context, issues []BatchIssue, group *BatchGroup) (BatchResult, error) {
	plan, err := s.batchPrepare(ctx, issues)
	if err != nil {
		return BatchResult{}, err
	}

	// Allocate all IDs up front, deduped against existing rows and each
	// other — the random suffix collides at batch scale otherwise.
	existing, err := s.allIDs(ctx)
	if err != nil {
		return BatchResult{}, err
	}
	alloc := func() (string, error) {
		for tries := 0; ; tries++ {
			id := newID(s.idPrefix)
			if _, clash := existing[id]; !clash {
				existing[id] = struct{}{}
				return id, nil
			}
			if tries > 1000 {
				return "", errors.New("failed to allocate unique ids; id space may be exhausted")
			}
		}
	}

	aliasToID := make(map[string]string, len(plan.issues))
	idByIdx := make([]string, len(plan.issues))
	for i, iss := range plan.issues {
		id, err := alloc()
		if err != nil {
			return BatchResult{}, err
		}
		idByIdx[i] = id
		aliasToID[iss.Alias] = id
	}

	// Optional group parent + its run:<id> label, applied to every issue.
	var parentID, runLabel string
	if group != nil {
		parentID, err = alloc()
		if err != nil {
			return BatchResult{}, err
		}
		runLabel = "run:" + parentID
	}

	// Build all rows in memory, then bulk-insert in chunks within one tx.
	t := now()
	issueRows := make([]Issue, len(plan.issues))
	var labelRows []IssueLabel
	var depRows []Dep
	var eventRows []Event
	cpKV := map[string]string{} // issue id → checkpoint payload JSON
	hasDependent := make([]bool, len(plan.issues))
	for i := range plan.issues {
		for _, j := range plan.internal[i] {
			hasDependent[j] = true
		}
	}
	for i, iss := range plan.issues {
		id := idByIdx[i]
		typ := iss.Type
		if iss.Checkpoint != nil {
			typ = "checkpoint" // checkpoint spec forces the type
		}
		if typ == "" {
			typ = "task"
		}
		issueRows[i] = Issue{
			ID: id, Title: strings.TrimSpace(iss.Title), Type: typ, Status: "open",
			Priority: iss.Priority, Assignee: iss.Assignee,
			Created: t, Updated: t,
			Description: iss.Description, Notes: iss.Notes,
		}
		// Labels: cap:<name> + arbitrary + run:<parent> + checkpoint:pending.
		var allLabels []string
		for _, c := range iss.Capabilities {
			allLabels = append(allLabels, "cap:"+c)
		}
		allLabels = append(allLabels, iss.Labels...)
		if runLabel != "" {
			allLabels = append(allLabels, runLabel)
		}
		if iss.Checkpoint != nil {
			allLabels = append(allLabels, "checkpoint:pending")
			payload := *iss.Checkpoint
			if payload.Kind == "" { // infer: approvers → approval, else manual
				if len(payload.Approvers) > 0 {
					payload.Kind = "approval"
				} else {
					payload.Kind = "manual"
				}
			}
			if b, err := json.Marshal(payload); err == nil {
				cpKV["cp:"+id] = string(b)
			}
		}
		// Dedupe effective labels — a repeated label, or a capability that
		// collides with an explicit cap:<name> label, would otherwise hit the
		// issue_labels unique constraint. Matches AddLabels' conflict-ignore
		// behavior so dry-run and commit agree.
		seenLabel := make(map[string]bool, len(allLabels))
		for _, l := range allLabels {
			if seenLabel[l] {
				continue
			}
			seenLabel[l] = true
			labelRows = append(labelRows, IssueLabel{IssueID: id, Label: l})
		}
		// Dep edges: internal (resolved alias) + external (real ID).
		var parents []string
		for _, j := range plan.internal[i] {
			parents = append(parents, idByIdx[j])
		}
		parents = append(parents, plan.external[i]...)
		for _, p := range parents {
			depRows = append(depRows, Dep{ChildID: id, ParentID: p})
		}
		// created event, mirroring CreateWithLinks' changed-fields payload.
		eventRows = append(eventRows, Event{
			IssueID: &id, Actor: s.actorPtr(), Kind: "created", TS: t,
			Payload: batchCreatedPayload(issueRows[i], allLabels, parents),
		})
	}

	// Parent umbrella: depends on every leaf, carries the run label.
	if group != nil {
		desc := group.Description
		var descPtr *string
		if desc != "" {
			descPtr = &desc
		}
		title := strings.TrimSpace(group.Title)
		if title == "" {
			title = "Batch"
		}
		// type=milestone so the umbrella self-completes when every leaf
		// closes, instead of lingering open (and showing up in `ready`).
		issueRows = append(issueRows, Issue{
			ID: parentID, Title: title, Type: "milestone", Status: "open",
			Priority: 2, Created: t, Updated: t, Description: descPtr,
		})
		labelRows = append(labelRows, IssueLabel{IssueID: parentID, Label: runLabel})
		for i := range plan.issues {
			if !hasDependent[i] { // leaf → parent waits on it
				depRows = append(depRows, Dep{ChildID: parentID, ParentID: idByIdx[i]})
			}
		}
		pid := parentID
		eventRows = append(eventRows, Event{
			IssueID: &pid, Actor: s.actorPtr(), Kind: "created", TS: t,
			Payload: batchCreatedPayload(issueRows[len(issueRows)-1], []string{runLabel}, nil),
		})
	}

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := chunkInsert(ctx, tx, issueRows, chunkInsertParams/11); err != nil {
			return err
		}
		if err := chunkInsert(ctx, tx, depRows, chunkInsertParams/2); err != nil {
			return err
		}
		if err := chunkInsert(ctx, tx, labelRows, chunkInsertParams/2); err != nil {
			return err
		}
		if err := chunkInsert(ctx, tx, eventRows, chunkInsertParams/5); err != nil {
			return err
		}
		// Checkpoint payloads → cp:<id> KV rows, in the same tx.
		for key, val := range cpKV {
			if err := KVSetTx(ctx, tx, key, val); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Mapping: aliasToID, ParentID: parentID, Stats: plan.stats}, nil
}

// allIDs loads every existing issue ID into a set for collision-free
// allocation.
func (s *Store) allIDs(ctx context.Context) (map[string]struct{}, error) {
	var ids []string
	if err := s.db.NewSelect().Model((*Issue)(nil)).Column("id").Scan(ctx, &ids); err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// actorPtr returns the store's audit actor as a pointer, or nil when unset.
func (s *Store) actorPtr() *string {
	if s.actor == "" {
		return nil
	}
	a := s.actor
	return &a
}

// batchCreatedPayload builds the changed-fields JSON for a created event,
// matching CreateWithLinks' shape (plus arbitrary labels).
func batchCreatedPayload(i Issue, labels, parents []string) *string {
	changed := map[string]any{"title": i.Title, "type": i.Type, "priority": i.Priority}
	if i.Assignee != nil {
		changed["assignee"] = *i.Assignee
	}
	if len(labels) > 0 {
		changed["labels"] = labels
	}
	if len(parents) > 0 {
		changed["depends_on"] = parents
	}
	if i.Description != nil {
		changed["description"] = true
	}
	if i.Notes != nil {
		changed["notes"] = true
	}
	b, err := json.Marshal(changed)
	if err != nil {
		return nil
	}
	p := string(b)
	return &p
}

// chunkInsert bulk-inserts rows in batches of at most perChunk to stay
// under SQLite's bind-variable limit. A zero/empty slice is a no-op.
func chunkInsert[T any](ctx context.Context, tx bun.Tx, rows []T, perChunk int) error {
	if perChunk < 1 {
		perChunk = 1
	}
	for start := 0; start < len(rows); start += perChunk {
		end := min(start+perChunk, len(rows))
		chunk := rows[start:end]
		if _, err := tx.NewInsert().Model(&chunk).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// joinErrs renders accumulated validation errors as an indented list.
func joinErrs(errs []error) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n  ")
}
