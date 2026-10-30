package execution

import (
	"context"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// ClaimIssue acquires execution custody of one eligible issue.
type ClaimIssue struct {
	// IssueID identifies the issue whose custody changes.
	IssueID issue.ID
	// Assignee receives execution custody.
	Assignee issue.Actor
}

// ClaimNext selects and acquires custody of the next eligible pool issue.
// Routines require direct claim and are never eligible for pool selection.
type ClaimNext struct {
	// UnderID limits unqualified selection to strict containment descendants.
	UnderID issue.ID

	// Assignee receives execution custody.
	Assignee issue.Actor

	// LabelsAll requires the selected issue to have every listed label.
	LabelsAll []issue.Label

	// LabelsAny requires the selected issue to have at least one listed label.
	LabelsAny []issue.Label

	// LabelsNone excludes issues carrying any listed label.
	LabelsNone []issue.Label
}

// ClaimIssueSnapshot contains one requested issue and the related state needed
// to verify that it remains claimable under the writer transaction.
type ClaimIssueSnapshot struct {
	// BoardID identifies the board loaded by the writer transaction.
	BoardID board.ID
	// Revision identifies the canonical state observed by the writer.
	Revision board.Revision
	// Candidate is the requested issue, or nil when it does not exist.
	Candidate *issue.State
	// Prerequisites is the candidate's complete readiness dependency set.
	Prerequisites []ClaimPrerequisite
	// OccurredAt timestamps a successful claim.
	OccurredAt time.Time
}

// ClaimPrerequisite contains only the dependency values consumed by claiming.
// Result and other record bodies are not part of claim admission.
type ClaimPrerequisite struct {
	// Lifecycle is nil when the prerequisite's completion state is unresolved.
	Lifecycle *issue.Lifecycle
	// Conflicts retains recovery references for disputed lifecycle or decisions.
	Conflicts []issue.ConflictReference
}

// ClaimNextSnapshot contains the candidate selected under the writer transaction
// and the related state needed to verify the selection remains eligible.
type ClaimNextSnapshot struct {
	// BoardID identifies the board loaded by the writer transaction.
	BoardID board.ID
	// Revision identifies the canonical state observed by the writer.
	Revision board.Revision
	// Candidate is the selected issue, or nil when no issue matched.
	Candidate *issue.State
	// Labels is the candidate's complete label set.
	Labels []issue.Label
	// Prerequisites is the candidate's complete readiness dependency set.
	Prerequisites []ClaimPrerequisite
	// SkippedConflicts counts otherwise eligible candidates excluded by conflict.
	SkippedConflicts int
	// OccurredAt timestamps a successful claim.
	OccurredAt time.Time
}

type claimSnapshot struct {
	BoardID       board.ID
	Revision      board.Revision
	Candidate     *issue.State
	Labels        []issue.Label
	Prerequisites []ClaimPrerequisite
	OccurredAt    time.Time
}

// ClaimIssuePolicy evaluates one explicitly selected claim candidate.
type ClaimIssuePolicy struct{ snapshot claimSnapshot }

// ClaimNextPolicy evaluates one candidate selected from the ready pool.
type ClaimNextPolicy struct {
	snapshot         claimSnapshot
	skippedConflicts int
}

// IssueClaimed is the semantic outcome of issue claimed.
type IssueClaimed struct {
	// Issue is the state after custody is acquired.
	Issue issue.State
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// ClaimIssueResult reports the committed result of claim issue.
type ClaimIssueResult struct {
	// Issue is the canonical post-commit view with requested context.
	Issue issue.View
}

// NextIssueClaimed reports an automatic selection, including an empty pool.
// A nil Issue means no custody was acquired; SkippedConflicts still describes
// the observed pool. Errors are reserved for unsuccessful operations.
type NextIssueClaimed struct {
	// Issue is the selected state after custody is acquired, or nil.
	Issue *issue.State
	// SkippedConflicts counts otherwise eligible candidates excluded by conflict.
	SkippedConflicts int
	// CommittedRevision is populated only when custody changed.
	CommittedRevision
}

// ClaimNextResult preserves pool diagnostics when no issue was selected.
type ClaimNextResult struct {
	// Issue contains the claimed view and requested context, or nil.
	Issue *issue.View
	// SkippedConflicts counts otherwise eligible candidates excluded by conflict.
	SkippedConflicts int
}

// ClaimIssueRequest supplies caller input for claiming a named issue.
type ClaimIssueRequest struct {
	// ID identifies the issue to claim.
	ID string
	// Assignee identifies the new custody owner.
	Assignee string
	// ContextDepth requests inherited context in the returned issue view.
	ContextDepth *int
}

// ClaimNextRequest supplies caller input for selecting and claiming ready work.
type ClaimNextRequest struct {
	// UnderID limits unqualified selection to strict containment descendants.
	UnderID string

	// Assignee identifies the new custody owner.
	Assignee string

	// LabelsAll requires every listed label during selection.
	LabelsAll []string

	// LabelsAny requires at least one listed label during selection.
	LabelsAny []string

	// LabelsNone excludes issues carrying any listed label.
	LabelsNone []string

	// Watch retries until matching ready work exists or the context ends.
	Watch bool

	// Interval controls watch retry cadence; zero uses the default.
	Interval time.Duration

	// ContextDepth requests inherited context in the returned issue view.
	ContextDepth *int
}

// LoadClaimIssue validates snapshot for claiming one requested issue.
func LoadClaimIssue(snapshot ClaimIssueSnapshot) (*ClaimIssuePolicy, error) {
	validated, err := loadClaim(claimSnapshot{
		BoardID: snapshot.BoardID, Revision: snapshot.Revision, Candidate: snapshot.Candidate,
		Prerequisites: snapshot.Prerequisites, OccurredAt: snapshot.OccurredAt,
	})
	if err != nil {
		return nil, err
	}
	return &ClaimIssuePolicy{snapshot: validated}, nil
}

// LoadClaimNext validates snapshot for selecting and claiming ready work.
func LoadClaimNext(snapshot ClaimNextSnapshot) (*ClaimNextPolicy, error) {
	if snapshot.SkippedConflicts < 0 {
		return nil, ErrIncompleteSnapshot
	}
	validated, err := loadClaim(claimSnapshot{
		BoardID: snapshot.BoardID, Revision: snapshot.Revision, Candidate: snapshot.Candidate,
		Labels: snapshot.Labels, Prerequisites: snapshot.Prerequisites, OccurredAt: snapshot.OccurredAt,
	})
	if err != nil {
		return nil, err
	}
	return &ClaimNextPolicy{snapshot: validated, skippedConflicts: snapshot.SkippedConflicts}, nil
}

func loadClaim(snapshot claimSnapshot) (claimSnapshot, error) {
	if err := validateSnapshot(snapshot.BoardID, snapshot.Revision); err != nil || snapshot.OccurredAt.IsZero() {
		return claimSnapshot{}, ErrIncompleteSnapshot
	}
	if snapshot.Candidate != nil && snapshot.Candidate.ID() == "" {
		return claimSnapshot{}, ErrIncompleteSnapshot
	}
	if _, err := uniqueLabels(snapshot.Labels); err != nil {
		return claimSnapshot{}, ErrIncompleteSnapshot
	}
	return snapshot, nil
}

// ClaimIssue applies claim policy to one explicitly selected candidate.
func (p *ClaimIssuePolicy) ClaimIssue(command ClaimIssue) (IssueClaimed, error) {
	if p.snapshot.Candidate == nil {
		return IssueClaimed{}, errkind.Errorf(errkind.NotFound, "issue not found")
	}
	if command.IssueID != p.snapshot.Candidate.ID() {
		return IssueClaimed{}, ErrIncompleteSnapshot
	}
	return claimCandidate(p.snapshot, command.Assignee)
}

// ClaimNext applies ready-selection policy to the loaded pool candidate.
func (p *ClaimNextPolicy) ClaimNext(command ClaimNext) (NextIssueClaimed, error) {
	result := NextIssueClaimed{SkippedConflicts: p.skippedConflicts}
	requestedLabelsAll, err := uniqueLabels(command.LabelsAll)
	if err != nil {
		return result, err
	}
	if p.snapshot.Candidate == nil {
		return result, nil
	}
	requestedLabelsAny, err := uniqueLabels(command.LabelsAny)
	if err != nil {
		return result, err
	}
	requestedLabelsNone, err := uniqueLabels(command.LabelsNone)
	if err != nil {
		return result, err
	}
	labelSummary := issue.Summary{Issue: issueProjection(*p.snapshot.Candidate, p.snapshot.Revision)}
	for _, label := range p.snapshot.Labels {
		labelSummary.Labels = append(labelSummary.Labels, label.String())
	}
	if !MatchesKnownClaimLabels(labelSummary, ClaimNext{LabelsAll: requestedLabelsAll, LabelsAny: requestedLabelsAny, LabelsNone: requestedLabelsNone}) {
		return result, nil
	}
	candidate := p.snapshot.Candidate
	input := EligibilityInput{
		Kind: candidate.Kind(), Lifecycle: candidate.Lifecycle(),
		Claimed: candidate.ActiveClaim() != nil, Waiting: candidate.Waiting() != nil,
		Conflicted: len(candidate.Conflicts()) > 0,
	}
	for _, prerequisite := range p.snapshot.Prerequisites {
		input.Conflicted = input.Conflicted || prerequisite.Lifecycle == nil ||
			issue.Unresolved(prerequisite.Conflicts, issue.FieldCheckpoint)
		if prerequisite.Lifecycle != nil && *prerequisite.Lifecycle != issue.LifecycleClosed {
			input.Blocked = true
		}
	}
	eligibility := ClassifyEligibility(input)
	if eligibility.SkippedForConflict() {
		result.SkippedConflicts++
		return result, nil
	}
	if !eligibility.ReadyForClaim() {
		return result, nil
	}
	claimed, err := claimCandidate(p.snapshot, command.Assignee)
	if err != nil {
		return result, err
	}
	result.Issue = &claimed.Issue
	return result, nil
}

func claimCandidate(
	snapshot claimSnapshot,
	assignee issue.Actor,
) (IssueClaimed, error) {
	if assignee == "" {
		return IssueClaimed{}, errkind.Errorf(errkind.InvalidInput, "invalid input: assignee required")
	}
	state := *snapshot.Candidate
	if err := state.RequireResolved(issue.FieldKind, issue.FieldLifecycle, issue.FieldWaiting, issue.FieldDependencies); err != nil {
		return IssueClaimed{}, err
	}
	if *state.Lifecycle() != issue.LifecycleOpen {
		return IssueClaimed{}, errkind.Errorf(
			errkind.Conflict,
			"claim requires lifecycle open; issue lifecycle is %s",
			state.Lifecycle(),
		)
	}
	if claim := state.ActiveClaim(); claim != nil {
		return IssueClaimed{}, errkind.Errorf(
			errkind.Conflict,
			"claim requires an unclaimed issue; active claim belongs to %q",
			claim.Actor,
		)
	}
	if !state.Kind().Executable() {
		return IssueClaimed{}, errkind.Errorf(
			errkind.Conflict,
			"claim requires an executable issue; issue type is %s",
			state.Kind(),
		)
	}
	for _, prerequisite := range snapshot.Prerequisites {
		for _, conflict := range prerequisite.Conflicts {
			if conflict.Field == issue.FieldLifecycle || conflict.Field == issue.FieldCheckpoint {
				return IssueClaimed{}, errkind.Errorf(errkind.Conflict, "%s is unresolved in conflict %s", conflict.Field, conflict.ID)
			}
		}
		if prerequisite.Lifecycle == nil {
			return IssueClaimed{}, errkind.Errorf(errkind.Conflict, "prerequisite lifecycle is unresolved")
		}
		if *prerequisite.Lifecycle != issue.LifecycleClosed {
			return IssueClaimed{}, errkind.Errorf(
				errkind.Conflict,
				"claim requires all dependencies to be closed",
			)
		}
	}
	next := state.Snapshot()
	next.ActiveClaim = &issue.ClaimState{Actor: assignee, StartedAt: snapshot.OccurredAt}
	next.Waiting = nil
	next.Updated = snapshot.OccurredAt
	next.ClosedAt = nil
	state, err := issue.Load(next)
	if err != nil {
		return IssueClaimed{}, err
	}
	return IssueClaimed{Issue: state}, nil
}

// ClaimIssue validates caller input and claims one named issue.
func (e *Executor) ClaimIssue(
	ctx context.Context,
	_ issue.Invocation,
	req ClaimIssueRequest,
) (ClaimIssueResult, error) {
	id, err := issue.NewID(req.ID)
	if err != nil {
		return ClaimIssueResult{}, err
	}
	outcome, err := e.changes.ClaimIssue(
		ctx,
		ClaimIssue{IssueID: id, Assignee: issue.NewActor(req.Assignee)},
	)
	return e.claimResult(ctx, outcome, req.ContextDepth, err)
}

// ClaimNext validates caller input and atomically claims the next ready issue.
func (e *Executor) ClaimNext(
	ctx context.Context,
	_ issue.Invocation,
	req ClaimNextRequest,
) (ClaimNextResult, error) {
	var underID issue.ID
	var err error
	if req.UnderID != "" {
		underID, err = issue.NewID(req.UnderID)
		if err != nil {
			return ClaimNextResult{}, err
		}
	}
	claimLabelsAll, err := labels(req.LabelsAll)
	if err != nil {
		return ClaimNextResult{}, err
	}
	claimLabelsAny, err := labels(req.LabelsAny)
	if err != nil {
		return ClaimNextResult{}, err
	}
	claimLabelsNone, err := labels(req.LabelsNone)
	if err != nil {
		return ClaimNextResult{}, err
	}
	interval := req.Interval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for {
		outcome, err := e.changes.ClaimNext(
			ctx,
			ClaimNext{
				UnderID: underID, Assignee: issue.NewActor(req.Assignee), LabelsAll: claimLabelsAll,
				LabelsAny: claimLabelsAny, LabelsNone: claimLabelsNone,
			},
		)
		result := ClaimNextResult{SkippedConflicts: outcome.SkippedConflicts}
		if err != nil {
			return result, err
		}
		if outcome.Issue != nil {
			view, err := e.readIssue(ctx, outcome.Issue.ID(), req.ContextDepth)
			if err != nil {
				return result, err
			}
			result.Issue = &view
			return result, nil
		}
		if !req.Watch {
			return result, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, ctx.Err()
		case <-timer.C:
		}
	}
}

func (e *Executor) claimResult(
	ctx context.Context,
	outcome IssueClaimed,
	contextDepth *int,
	err error,
) (ClaimIssueResult, error) {
	if err != nil {
		return ClaimIssueResult{}, err
	}
	view, err := e.readIssue(ctx, outcome.Issue.ID(), contextDepth)
	if err != nil {
		return ClaimIssueResult{}, err
	}
	return ClaimIssueResult{Issue: view}, nil
}

func labels(values []string) ([]issue.Label, error) {
	result := make([]issue.Label, len(values))
	for index, value := range values {
		label, err := issue.NewLabel(value)
		if err != nil {
			return nil, err
		}
		result[index] = label
	}
	return result, nil
}

func uniqueLabels(values []issue.Label) ([]issue.Label, error) {
	result := make([]issue.Label, 0, len(values))
	seen := make(map[issue.Label]struct{}, len(values))
	for _, value := range values {
		validated, err := issue.NewLabel(value.String())
		if err != nil || validated != value {
			return nil, errkind.Errorf(errkind.InvalidInput, "invalid input: label must be normalized")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}
