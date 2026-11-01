package board

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"maps"
	"slices"

	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// readIssuePresentation reads only scalar and relationship metadata. It supplies
// Reference status and automatic eligibility without loading record bodies or
// conflict alternatives. The caller retains the read or mutation scope.
func (r *Repository) readIssuePresentation(ctx context.Context, scope queryScope) (*issuePresentation, error) {
	queries := query.New(scope)
	rows, err := queries.BoardListIssuePresentation(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	conflicts, err := r.readIssueConflictReferences(ctx, scope, nil)
	if err != nil {
		return nil, err
	}
	labels, err := r.readBoardIssueLabels(ctx, scope)
	if err != nil {
		return nil, err
	}
	parents, err := r.readContainmentParents(ctx, scope)
	if err != nil {
		return nil, err
	}
	dependencies, err := queries.BoardListSnapshotDependencies(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	pins, err := queries.BoardListPinIDs(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	contextConflicts, err := queries.BoardListContextConflicts(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	presentation := &issuePresentation{
		issues:  make(map[issue.ID]issuePresentationEntry, len(rows)),
		parents: parents,
	}
	for _, row := range rows {
		value, err := presentedIssue(row, conflicts[string(row.Uid)])
		if err != nil {
			return nil, fmt.Errorf("present issue %q: %w", row.ID, err)
		}
		id := issue.ID(row.ID)
		issueLabels := labels[id]
		if issueLabels == nil {
			issueLabels = []string{}
		}
		value.summary.Labels = issueLabels
		presentation.issues[id] = value
	}
	prerequisites := make(map[issue.ID][]issue.ID)
	for _, edge := range dependencies {
		id := issue.ID(edge.IssueID)
		prerequisites[id] = append(prerequisites[id], issue.ID(edge.PrerequisiteID))
	}
	var shared []issue.ConflictReference
	for _, conflict := range contextConflicts {
		field := issue.FieldPins
		if boardhistory.EventKind(conflict.Kind) == boardhistory.EventBoardDescriptionSet {
			field = issue.FieldBoardDescription
		}
		shared = append(shared, issue.ConflictReference{ID: conflict.ID, Field: field})
	}
	for _, pin := range pins {
		shared = appendPresentationConflicts(shared, presentation.issues[issue.ID(pin)].summary.Issue.Conflicts,
			issue.FieldTitle, issue.FieldSummary, issue.FieldState)
	}
	presentation.prerequisites = prerequisites
	presentation.sharedConflicts = shared
	return presentation, nil
}

// issuePresentation owns compact projections from one coherent board snapshot.
// Record bodies are intentionally absent; callers needing content load the
// selected issue separately. Inherited conflicts are derived per lookup rather
// than retaining a copy of every ancestor conflict for every descendant.
type issuePresentation struct {
	issues          map[issue.ID]issuePresentationEntry
	parents         map[issue.ID]issue.ID
	prerequisites   map[issue.ID][]issue.ID
	sharedConflicts []issue.ConflictReference
}

// IDs iterates the compact projection's issue identities.
func (p *issuePresentation) IDs() iter.Seq[issue.ID] { return maps.Keys(p.issues) }

// Len reports the number of issue projections.
func (p *issuePresentation) Len() int { return len(p.issues) }

// Contains reports whether the board includes the public issue identity.
func (p *issuePresentation) Contains(id issue.ID) bool { _, found := p.issues[id]; return found }

// Summary distinguishes execution prerequisites from inherited
// context. Both can withhold automatic work, but an uncertain ancestor Summary
// does not make a known execution status unknown. Known blockers take precedence
// over uncertain prerequisites because they already exclude the candidate.
func (p *issuePresentation) Summary(id issue.ID) issue.Summary {
	summary := p.issues[id].summary
	inherited := slices.Clone(p.sharedConflicts)
	for ancestor := p.parents[id]; ancestor != ""; ancestor = p.parents[ancestor] {
		inherited = appendPresentationConflicts(inherited, p.issues[ancestor].summary.Issue.Conflicts,
			issue.FieldSummary, issue.FieldState, issue.FieldParent)
	}
	uncertainPrerequisite := false
	for _, prerequisiteID := range p.prerequisites[id] {
		prerequisite := p.issues[prerequisiteID].summary.Issue
		if prerequisite.Lifecycle != nil && *prerequisite.Lifecycle != issue.LifecycleClosed.String() {
			summary.Blocked = true
		}
		inherited = appendPresentationConflicts(inherited, prerequisite.Conflicts,
			issue.FieldLifecycle, issue.FieldCheckpoint, issue.FieldResult)
		uncertainPrerequisite = uncertainPrerequisite || prerequisite.Lifecycle == nil ||
			issue.Unresolved(prerequisite.Conflicts, issue.FieldCheckpoint)
	}
	if summary.Issue.Status == issue.StatusReady.String() || summary.Issue.Status == issue.StatusUnresolved.String() {
		switch {
		case summary.Blocked && summary.Issue.Lifecycle != nil && *summary.Issue.Lifecycle == issue.LifecycleOpen.String() && !issue.Unresolved(summary.Issue.Conflicts, issue.FieldWaiting):
			summary.Issue.Status = issue.StatusBlocked.String()
		case uncertainPrerequisite || summary.Issue.Type == nil ||
			issue.Unresolved(summary.Issue.Conflicts, issue.FieldDependencies) ||
			issue.Unresolved(summary.Issue.Conflicts, issue.FieldCheckpoint):
			summary.Issue.Status = issue.StatusUnresolved.String()
		}
	}
	slices.SortFunc(inherited, comparePresentationConflicts)
	summary.EligibilityConflicts = slices.Compact(inherited)
	return summary
}

// Eligibility consumes the same derived summary used for scope and label
// selection, with waiting presence retained separately from its unloaded body.
func (p *issuePresentation) Eligibility(id issue.ID, summary issue.Summary) execution.Eligibility {
	input := p.issues[id].eligibility
	input.Blocked = summary.Blocked
	input.Conflicted = len(summary.Issue.Conflicts) > 0 || len(summary.EligibilityConflicts) > 0
	return execution.ClassifyEligibility(input)
}

// ClaimPrerequisites supplies only the completion metadata needed by the claim
// policy, leaving dependency bodies outside the selected issue mutation.
func (p *issuePresentation) ClaimPrerequisites(id issue.ID) []execution.ClaimPrerequisite {
	values := make([]execution.ClaimPrerequisite, 0, len(p.prerequisites[id]))
	for _, prerequisite := range p.prerequisites[id] {
		entry := p.issues[prerequisite]
		values = append(values, execution.ClaimPrerequisite{
			Lifecycle: entry.eligibility.Lifecycle,
			Conflicts: entry.summary.Issue.Conflicts,
		})
	}
	return values
}

func (p *issuePresentation) Reference(id issue.ID) (issue.Reference, error) {
	if _, ok := p.issues[id]; !ok {
		return issue.Reference{}, errkind.Errorf(errkind.NotFound, "issue not found: %s", id)
	}
	value := p.Summary(id).Issue
	return issue.Reference{
		ID: value.ID, Title: value.Title, Type: value.Type, Status: value.Status,
		Priority: value.Priority, Conflicts: value.Conflicts,
	}, nil
}

// CouldBeUnder excludes only known non-descendants. A disputed parent leaves
// scope membership uncertain and lets eligibility report a conflict skip; no
// alternate containment graphs need to be loaded or enumerated.
func (p *issuePresentation) CouldBeUnder(id, root issue.ID) bool {
	if root == "" {
		return true
	}
	if id == root {
		return false
	}
	for current := id; current != ""; current = p.parents[current] {
		if current == root || issue.Unresolved(p.issues[current].summary.Issue.Conflicts, issue.FieldParent) {
			return true
		}
	}
	return false
}

func appendPresentationConflicts(out, values []issue.ConflictReference, fields ...issue.Field) []issue.ConflictReference {
	for _, value := range values {
		if slices.Contains(fields, value.Field) {
			out = append(out, value)
		}
	}
	return out
}

func comparePresentationConflicts(left, right issue.ConflictReference) int {
	if order := cmp.Compare(left.ID, right.ID); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Field, right.Field); order != 0 {
		return order
	}
	return cmp.Compare(left.Key, right.Key)
}

// issuePresentationEntry retains the waiting presence needed by eligibility
// while its summary omits record bodies.
type issuePresentationEntry struct {
	summary     issue.Summary
	eligibility execution.EligibilityInput
}

func presentedIssue(row query.BoardListIssuePresentationRow, conflicts []issue.ConflictReference) (issuePresentationEntry, error) {
	var kind *issue.Kind
	if row.Kind != nil {
		value, err := issue.NewKind(*row.Kind)
		if err != nil {
			return issuePresentationEntry{}, err
		}
		kind = &value
	}
	var lifecycle *issue.Lifecycle
	if row.Lifecycle != nil {
		value, err := issue.NewLifecycle(*row.Lifecycle)
		if err != nil {
			return issuePresentationEntry{}, err
		}
		lifecycle = &value
	}
	value := issue.Issue{
		ID: row.ID, Title: row.Title, Type: row.Kind, Lifecycle: row.Lifecycle,
		Status: issue.DeriveStatus(issue.StatusInput{
			Lifecycle: lifecycle, Claimed: row.ClaimActor != nil,
			Waiting: row.WaitingSince != nil, Conflicts: conflicts,
		}).String(),
		Created: row.CreatedAt.Unix(), Updated: row.UpdatedAt.Unix(),
		Closed: unixPointer(row.ClosedAt), Revision: row.Revision, Conflicts: conflicts,
	}
	if row.Priority != nil {
		value.Priority = new(int(*row.Priority))
	}
	if row.ClaimActor != nil {
		value.Assignee = row.ClaimActor
		value.StartedAt = unixPointer(row.ClaimStartedAt)
		value.ActiveClaim = &issue.ActiveClaim{Actor: *row.ClaimActor, StartedAt: row.ClaimStartedAt.Unix()}
	}
	return issuePresentationEntry{
		summary: issue.Summary{Issue: value},
		eligibility: execution.EligibilityInput{
			Kind: kind, Lifecycle: lifecycle, Claimed: row.ClaimActor != nil,
			Waiting: row.WaitingSince != nil,
		},
	}, nil
}
