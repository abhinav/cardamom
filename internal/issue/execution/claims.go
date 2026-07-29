package execution

import (
	"context"
	"errors"
	"slices"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

var errNoReady = errkind.Errorf(errkind.Conflict, "no ready issues")

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
	Prerequisites []issue.State
	// OccurredAt timestamps a successful claim.
	OccurredAt time.Time
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
	Prerequisites []issue.State
	// OccurredAt timestamps a successful claim.
	OccurredAt time.Time
}

type claimSnapshot struct {
	BoardID       board.ID
	Revision      board.Revision
	Candidate     *issue.State
	Labels        []issue.Label
	Prerequisites []issue.State
	OccurredAt    time.Time
}

// ClaimIssuePolicy evaluates one explicitly selected claim candidate.
type ClaimIssuePolicy struct{ snapshot claimSnapshot }

// ClaimNextPolicy evaluates one candidate selected from the ready pool.
type ClaimNextPolicy struct{ snapshot claimSnapshot }

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
	validated, err := loadClaim(claimSnapshot(snapshot))
	if err != nil {
		return nil, err
	}
	return &ClaimNextPolicy{snapshot: validated}, nil
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
	return claimCandidate(p.snapshot, command.Assignee, claimDirect)
}

// ClaimNext applies ready-selection policy to the loaded pool candidate.
func (p *ClaimNextPolicy) ClaimNext(command ClaimNext) (IssueClaimed, error) {
	requestedLabelsAll, err := uniqueLabels(command.LabelsAll)
	if err != nil {
		return IssueClaimed{}, err
	}
	if p.snapshot.Candidate == nil || p.snapshot.Candidate.Kind() == issue.KindRoutine {
		return IssueClaimed{}, errNoReady
	}
	for _, label := range requestedLabelsAll {
		if !slices.Contains(p.snapshot.Labels, label) {
			return IssueClaimed{}, errNoReady
		}
	}
	requestedLabelsAny, err := uniqueLabels(command.LabelsAny)
	if err != nil {
		return IssueClaimed{}, err
	}
	if len(requestedLabelsAny) > 0 {
		matched := false
		for _, label := range requestedLabelsAny {
			matched = matched || slices.Contains(p.snapshot.Labels, label)
		}
		if !matched {
			return IssueClaimed{}, errNoReady
		}
	}
	requestedLabelsNone, err := uniqueLabels(command.LabelsNone)
	if err != nil {
		return IssueClaimed{}, err
	}
	for _, label := range requestedLabelsNone {
		if slices.Contains(p.snapshot.Labels, label) {
			return IssueClaimed{}, errNoReady
		}
	}
	return claimCandidate(p.snapshot, command.Assignee, claimFromPool)
}

type claimSource uint8

const (
	claimDirect claimSource = iota
	claimFromPool
)

func claimCandidate(
	snapshot claimSnapshot,
	assignee issue.Actor,
	source claimSource,
) (IssueClaimed, error) {
	if assignee == "" {
		return IssueClaimed{}, errkind.Errorf(errkind.InvalidInput, "invalid input: assignee required")
	}
	state := *snapshot.Candidate
	if state.Lifecycle() != issue.LifecycleOpen {
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
	if source == claimFromPool && state.Waiting() != nil {
		return IssueClaimed{}, errkind.Errorf(
			errkind.Conflict,
			"automatic claim excludes waiting issues; claim the issue by ID",
		)
	}
	for _, prerequisite := range snapshot.Prerequisites {
		if prerequisite.Status() != issue.StatusClosed {
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
) (ClaimIssueResult, error) {
	var underID issue.ID
	var err error
	if req.UnderID != "" {
		underID, err = issue.NewID(req.UnderID)
		if err != nil {
			return ClaimIssueResult{}, err
		}
	}
	claimLabelsAll, err := labels(req.LabelsAll)
	if err != nil {
		return ClaimIssueResult{}, err
	}
	claimLabelsAny, err := labels(req.LabelsAny)
	if err != nil {
		return ClaimIssueResult{}, err
	}
	claimLabelsNone, err := labels(req.LabelsNone)
	if err != nil {
		return ClaimIssueResult{}, err
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
		if err == nil {
			return e.claimResult(ctx, outcome, req.ContextDepth, nil)
		}
		if !req.Watch || !errors.Is(err, errNoReady) {
			return ClaimIssueResult{}, err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ClaimIssueResult{}, ctx.Err()
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
