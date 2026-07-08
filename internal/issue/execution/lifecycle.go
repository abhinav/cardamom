package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// CloseIssue transitions one issue to successful completion.
type CloseIssue struct {
	// IssueID identifies the issue to close.
	IssueID issue.ID
	// Actor commits changed State before closure.
	Actor issue.Actor
}

// ReleaseIssue relinquishes execution custody held by Actor.
type ReleaseIssue struct {
	// IssueID identifies the issue whose custody is released.
	IssueID issue.ID
	// Actor must own the active claim.
	Actor issue.Actor
	// WaitingReason enters waiting status when present.
	WaitingReason *string
}

// ReopenIssue returns one terminal issue to the open lifecycle.
type ReopenIssue struct {
	// IssueID identifies the issue to reopen.
	IssueID issue.ID
}

// CancelIssues terminates named roots and their dependent closure.
type CancelIssues struct {
	// Roots contains the requested cancellation roots in caller order.
	Roots []issue.ID
	// Actor commits changed State for every cancelled issue.
	Actor issue.Actor
}

// LifecycleSnapshot contains one issue and the related state needed to
// release, close, or reopen it.
type LifecycleSnapshot struct {
	// BoardID identifies the board loaded by the writer transaction.
	BoardID board.ID
	// Revision identifies the canonical state observed by the writer.
	Revision board.Revision
	// Issue is the complete current state of the requested issue.
	Issue issue.State
	// DirectChildren is the issue's complete immediate containment set.
	DirectChildren []issue.State
	// Prerequisites is the issue's complete readiness dependency set.
	Prerequisites []issue.State
	// TerminalParent is the optional containment snapshot for Issue.
	TerminalParent *TerminalParentSnapshot
	// OccurredAt timestamps a successful lifecycle transition.
	OccurredAt time.Time
}

// LifecyclePolicy evaluates release, close, and reopen transitions against a
// validated writer snapshot.
type LifecyclePolicy struct{ snapshot LifecycleSnapshot }

// TerminalParentSnapshot contains one direct parent and the child states
// needed to determine whether that parent has open children after a change.
type TerminalParentSnapshot struct {
	// CandidateChildren identifies transitions associated with ParentID.
	CandidateChildren []issue.ID
	// ParentID is the stable containment parent identity.
	ParentID issue.ID
	// DirectChildren is the parent's complete child state before the change.
	DirectChildren []issue.State
}

// CancellationSnapshot contains named roots and their complete dependent
// closure. Terminal roots remain in the snapshot but do not produce changes.
type CancellationSnapshot struct {
	// BoardID identifies the board loaded by the writer transaction.
	BoardID board.ID
	// Revision identifies the canonical state observed by the writer.
	Revision board.Revision
	// Roots is the complete requested root set found in the board.
	Roots []issue.State
	// Closure contains each root and every transitive dependent.
	Closure []issue.State
	// TerminalParents contains parent state for changed closure members.
	TerminalParents []TerminalParentSnapshot
	// OccurredAt timestamps every issue changed by the cancellation.
	OccurredAt time.Time
}

// CancellationPolicy evaluates one complete cancellation closure.
type CancellationPolicy struct{ snapshot CancellationSnapshot }

// IssueReleased is the semantic outcome of releasing execution custody.
type IssueReleased struct {
	// Issue is the state after custody is released.
	Issue issue.State
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// IssueClosed is the semantic outcome of successful completion.
type IssueClosed struct {
	// Issue is the state after successful completion.
	Issue issue.State
	// ParentsWithoutOpenChildren contains transaction-derived stable IDs.
	ParentsWithoutOpenChildren []issue.ID
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// PrerequisiteSnapshot identifies one prerequisite and its current status.
type PrerequisiteSnapshot struct {
	// ID is the stable prerequisite identity.
	ID issue.ID
	// Status is the prerequisite status observed in the writer snapshot.
	Status issue.Status
}

// IssueReopened is the semantic outcome of reopening one issue.
type IssueReopened struct {
	// Issue is the reopened state.
	Issue issue.State
	// UnresolvedPrerequisites reports prerequisites that still block the issue.
	UnresolvedPrerequisites []PrerequisiteSnapshot
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// IssuesCancelled is the semantic outcome of cancelling roots and dependents.
type IssuesCancelled struct {
	// Issues contains every non-terminal state changed by the cancellation.
	Issues []issue.State
	// Requested counts changed roots named by the caller.
	Requested int
	// Dependents counts changed issues reached through dependency edges.
	Dependents int
	// ParentsWithoutOpenChildren contains transaction-derived stable IDs.
	ParentsWithoutOpenChildren []issue.ID
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// ReleaseIssueResult reports the canonical post-commit issue.
type ReleaseIssueResult struct {
	// Issue is the canonical post-commit detail.
	Issue issue.Detail
}

// CloseIssuesResult reports completed issues and newly childless parents.
type CloseIssuesResult struct {
	// Issues contains successful closures in request order.
	Issues []issue.Summary
	// ParentsWithoutOpenChildren contains stable parent IDs reported by each
	// successful transaction.
	ParentsWithoutOpenChildren []string
}

// ReopenIssuesResult reports reopened issues.
type ReopenIssuesResult struct {
	// Issues contains successful reopen operations in request order.
	Issues []ReopenedIssue
}

// ReopenedIssue combines one reopened issue with unresolved prerequisites.
type ReopenedIssue struct {
	// Issue is the canonical post-commit summary.
	Issue issue.Summary
	// UnresolvedPrerequisites reports remaining blockers.
	UnresolvedPrerequisites []PrerequisiteView
}

// PrerequisiteView is the caller-facing identity and status of one issue.
type PrerequisiteView struct {
	// ID identifies the prerequisite.
	ID string
	// Status is the prerequisite lifecycle-derived status.
	Status string
}

// CancelIssuesResult reports changed issues and cancellation counts.
type CancelIssuesResult struct {
	// Issues contains every changed issue in closure order.
	Issues []issue.Issue
	// Requested counts changed roots named by the caller.
	Requested int
	// Dependents counts changed issues reached through dependency edges.
	Dependents int
	// ParentsWithoutOpenChildren contains stable parent IDs from the
	// cancellation transaction.
	ParentsWithoutOpenChildren []string
}

// CloseIssuesRequest supplies issue IDs for successful completion.
type CloseIssuesRequest struct {
	// IDs contains issues to close in caller order.
	IDs []string
}

// ReleaseIssueRequest identifies the issue whose custody is released.
type ReleaseIssueRequest struct {
	// ID identifies the issue whose custody is released.
	ID string
	// WaitingReason enters waiting status when present.
	WaitingReason *string
}

// ReopenIssuesRequest supplies terminal issue IDs to reopen.
type ReopenIssuesRequest struct {
	// IDs contains terminal issues to reopen in caller order.
	IDs []string
}

// CancelIssuesRequest supplies cancellation roots.
type CancelIssuesRequest struct {
	// Roots contains requested cancellation roots in caller order.
	Roots []string
}

// LoadLifecycle validates a writer snapshot for release, close, or reopen.
func LoadLifecycle(snapshot LifecycleSnapshot) (*LifecyclePolicy, error) {
	if err := validateIssueSnapshot(snapshot.BoardID, snapshot.Revision, snapshot.Issue); err != nil ||
		snapshot.OccurredAt.IsZero() {
		return nil, ErrIncompleteSnapshot
	}
	if snapshot.TerminalParent != nil {
		if err := validateTerminalParentSnapshots(
			[]TerminalParentSnapshot{*snapshot.TerminalParent},
			[]issue.State{snapshot.Issue},
		); err != nil {
			return nil, err
		}
	}
	return &LifecyclePolicy{snapshot: snapshot}, nil
}

// LoadCancellation validates a complete dependent closure.
func LoadCancellation(snapshot CancellationSnapshot) (*CancellationPolicy, error) {
	if err := validateSnapshot(snapshot.BoardID, snapshot.Revision); err != nil || snapshot.OccurredAt.IsZero() {
		return nil, ErrIncompleteSnapshot
	}
	for _, state := range append(slices.Clone(snapshot.Roots), snapshot.Closure...) {
		if state.ID() == "" {
			return nil, ErrIncompleteSnapshot
		}
	}
	closure := make(map[issue.ID]struct{}, len(snapshot.Closure))
	for _, state := range snapshot.Closure {
		closure[state.ID()] = struct{}{}
	}
	for _, root := range snapshot.Roots {
		if _, ok := closure[root.ID()]; !ok {
			return nil, ErrIncompleteSnapshot
		}
	}
	if err := validateTerminalParentSnapshots(snapshot.TerminalParents, snapshot.Closure); err != nil {
		return nil, err
	}
	return &CancellationPolicy{snapshot: snapshot}, nil
}

// ReleaseIssue applies release policy to the loaded writer snapshot.
func (p *LifecyclePolicy) ReleaseIssue(command ReleaseIssue) (IssueReleased, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return IssueReleased{}, ErrIncompleteSnapshot
	}
	claim := p.snapshot.Issue.ActiveClaim()
	if claim == nil {
		return IssueReleased{}, errkind.Errorf(errkind.Conflict, "issue has no active claim")
	}
	if command.Actor == "" || claim.Actor != command.Actor {
		return IssueReleased{}, errkind.Errorf(errkind.Conflict, "active claim belongs to %s", claim.Actor)
	}
	next := p.snapshot.Issue.Snapshot()
	next.ActiveClaim = nil
	if command.WaitingReason != nil {
		waiting, err := issue.NewWaitingState(
			*command.WaitingReason,
			p.snapshot.OccurredAt,
		)
		if err != nil {
			return IssueReleased{}, err
		}
		next.Waiting = waiting
	} else {
		next.Waiting = nil
	}
	next.Updated = p.snapshot.OccurredAt
	state, err := issue.Load(next)
	if err != nil {
		return IssueReleased{}, err
	}
	return IssueReleased{Issue: state}, nil
}

// CloseIssue applies completion policy to the loaded writer snapshot.
func (p *LifecyclePolicy) CloseIssue(command CloseIssue) (IssueClosed, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return IssueClosed{}, ErrIncompleteSnapshot
	}
	if p.snapshot.Issue.Lifecycle() == issue.LifecycleClosed {
		return IssueClosed{}, errkind.Errorf(
			errkind.Conflict,
			"close requires lifecycle open or cancelled; issue lifecycle is %s",
			p.snapshot.Issue.Lifecycle(),
		)
	}
	if err := validateExplicitClose(p.snapshot.Issue, p.snapshot.DirectChildren); err != nil {
		return IssueClosed{}, err
	}
	state, err := terminalState(p.snapshot.Issue, issue.LifecycleClosed, p.snapshot.OccurredAt)
	if err != nil {
		return IssueClosed{}, err
	}
	return IssueClosed{
		Issue: state,
		ParentsWithoutOpenChildren: parentIDsWithoutOpenChildren(
			terminalParentSlice(p.snapshot.TerminalParent),
			[]issue.State{state},
		),
	}, nil
}

// ReopenIssue applies reopen policy to the loaded writer snapshot.
func (p *LifecyclePolicy) ReopenIssue(command ReopenIssue) (IssueReopened, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return IssueReopened{}, ErrIncompleteSnapshot
	}
	if !p.snapshot.Issue.Lifecycle().Terminal() {
		return IssueReopened{}, errkind.Errorf(
			errkind.Conflict,
			"reopen requires lifecycle closed or cancelled; issue lifecycle is %s",
			p.snapshot.Issue.Lifecycle(),
		)
	}
	next := p.snapshot.Issue.Snapshot()
	next.Lifecycle = issue.LifecycleOpen
	next.ActiveClaim = nil
	next.Waiting = nil
	next.Updated = p.snapshot.OccurredAt
	next.ClosedAt = nil
	state, err := issue.Load(next)
	if err != nil {
		return IssueReopened{}, err
	}
	unresolved := make([]PrerequisiteSnapshot, 0, len(p.snapshot.Prerequisites))
	for _, prerequisite := range p.snapshot.Prerequisites {
		if prerequisite.Status() != issue.StatusClosed {
			unresolved = append(unresolved, PrerequisiteSnapshot{
				ID: prerequisite.ID(), Status: prerequisite.Status(),
			})
		}
	}
	return IssueReopened{
		Issue: state, UnresolvedPrerequisites: unresolved,
	}, nil
}

// CancelIssues applies cancellation policy to the loaded closure.
func (p *CancellationPolicy) CancelIssues(command CancelIssues) (IssuesCancelled, error) {
	requestedIDs := uniqueIssueIDs(command.Roots)
	roots := make(map[issue.ID]issue.State, len(p.snapshot.Roots))
	for _, state := range p.snapshot.Roots {
		roots[state.ID()] = state
	}
	for _, id := range requestedIDs {
		if _, ok := roots[id]; !ok {
			return IssuesCancelled{}, errkind.Errorf(errkind.NotFound, "%s: issue not found", id)
		}
	}
	rootSet := issueIDSet(requestedIDs)
	states := make([]issue.State, 0, len(p.snapshot.Closure))
	requested := 0
	dependents := 0
	seen := make(map[issue.ID]struct{}, len(p.snapshot.Closure))
	for _, current := range p.snapshot.Closure {
		if _, ok := seen[current.ID()]; ok || current.Lifecycle().Terminal() {
			continue
		}
		seen[current.ID()] = struct{}{}
		state, err := terminalState(current, issue.LifecycleCancelled, p.snapshot.OccurredAt)
		if err != nil {
			return IssuesCancelled{}, err
		}
		states = append(states, state)
		if _, ok := rootSet[state.ID()]; ok {
			requested++
		} else {
			dependents++
		}
	}
	return IssuesCancelled{
		Issues: states, Requested: requested, Dependents: dependents,
		ParentsWithoutOpenChildren: parentIDsWithoutOpenChildren(p.snapshot.TerminalParents, states),
	}, nil
}

// ReleaseIssue validates caller input and releases execution custody.
func (e *Executor) ReleaseIssue(
	ctx context.Context,
	inv issue.Invocation,
	req ReleaseIssueRequest,
) (ReleaseIssueResult, error) {
	id, err := issue.NewID(req.ID)
	if err != nil {
		return ReleaseIssueResult{}, err
	}
	outcome, err := e.changes.ReleaseIssue(
		ctx,
		ReleaseIssue{
			IssueID: id, Actor: issue.NewActor(inv.Actor()),
			WaitingReason: req.WaitingReason,
		},
	)
	if err != nil {
		return ReleaseIssueResult{}, err
	}
	view, err := e.readIssue(ctx, outcome.Issue.ID(), nil)
	if err != nil {
		return ReleaseIssueResult{}, err
	}
	return ReleaseIssueResult{Issue: view.Detail}, nil
}

// CloseIssues validates caller input and closes each issue independently.
func (e *Executor) CloseIssues(
	ctx context.Context,
	invocation issue.Invocation,
	req CloseIssuesRequest,
) (CloseIssuesResult, error) {
	type closeResult struct {
		issue   issue.Summary
		parents []issue.ID
	}
	results, err := mapIssueCommands(ctx, req.IDs, func(ctx context.Context, id issue.ID) (closeResult, error) {
		outcome, err := e.changes.CloseIssue(
			ctx,
			CloseIssue{
				IssueID: id,
				Actor:   issue.NewActor(invocation.Actor()),
			},
		)
		if err != nil {
			return closeResult{}, err
		}
		view, err := e.readIssue(ctx, outcome.Issue.ID(), nil)
		return closeResult{
			issue:   issueSummary(view.Detail),
			parents: outcome.ParentsWithoutOpenChildren,
		}, err
	})
	issues := make([]issue.Summary, len(results))
	parentGroups := make([][]issue.ID, len(results))
	for index, result := range results {
		issues[index] = result.issue
		parentGroups[index] = result.parents
	}
	return CloseIssuesResult{
		Issues: issues, ParentsWithoutOpenChildren: parentIDStrings(parentGroups...),
	}, err
}

// ReopenIssues validates caller input and reopens each issue independently.
func (e *Executor) ReopenIssues(
	ctx context.Context,
	_ issue.Invocation,
	req ReopenIssuesRequest,
) (ReopenIssuesResult, error) {
	issues, err := mapIssueCommands(ctx, req.IDs, func(ctx context.Context, id issue.ID) (ReopenedIssue, error) {
		outcome, err := e.changes.ReopenIssue(
			ctx,
			ReopenIssue{IssueID: id},
		)
		if err != nil {
			return ReopenedIssue{}, err
		}
		unresolved := make([]PrerequisiteView, len(outcome.UnresolvedPrerequisites))
		for index, prerequisite := range outcome.UnresolvedPrerequisites {
			unresolved[index] = PrerequisiteView{
				ID: prerequisite.ID.String(), Status: prerequisite.Status.String(),
			}
		}
		view, err := e.readIssue(ctx, outcome.Issue.ID(), nil)
		return ReopenedIssue{
			Issue:                   issueSummary(view.Detail),
			UnresolvedPrerequisites: unresolved,
		}, err
	})
	return ReopenIssuesResult{Issues: issues}, err
}

// CancelIssues validates roots and cancels their dependent closure atomically.
func (e *Executor) CancelIssues(
	ctx context.Context,
	invocation issue.Invocation,
	req CancelIssuesRequest,
) (CancelIssuesResult, error) {
	ids, err := issueIDs(req.Roots)
	if err != nil {
		return CancelIssuesResult{}, err
	}
	outcome, err := e.changes.CancelIssues(
		ctx,
		CancelIssues{
			Roots: ids,
			Actor: issue.NewActor(invocation.Actor()),
		},
	)
	if err != nil {
		return CancelIssuesResult{}, err
	}
	return CancelIssuesResult{
		Issues:    issueProjections(outcome.Issues, outcome.Revision),
		Requested: outcome.Requested, Dependents: outcome.Dependents,
		ParentsWithoutOpenChildren: parentIDStrings(outcome.ParentsWithoutOpenChildren),
	}, nil
}

func terminalState(
	state issue.State,
	lifecycle issue.Lifecycle,
	occurredAt time.Time,
) (issue.State, error) {
	snapshot := state.Snapshot()
	snapshot.Lifecycle = lifecycle
	snapshot.ActiveClaim = nil
	snapshot.Waiting = nil
	snapshot.RecoveryState = nil
	snapshot.Updated = occurredAt
	snapshot.ClosedAt = new(occurredAt)
	return issue.Load(snapshot)
}

func terminalParentSlice(parent *TerminalParentSnapshot) []TerminalParentSnapshot {
	if parent == nil {
		return nil
	}
	return []TerminalParentSnapshot{*parent}
}

// parentIDsWithoutOpenChildren overlays changed children on each writer
// snapshot before deciding which stable parent IDs to report once.
func parentIDsWithoutOpenChildren(
	snapshot []TerminalParentSnapshot,
	transitioned []issue.State,
) []issue.ID {
	transitions := make(map[issue.ID]issue.State, len(transitioned))
	for _, state := range transitioned {
		transitions[state.ID()] = state
	}
	seen := make(map[issue.ID]struct{}, len(snapshot))
	var parents []issue.ID
	for _, parent := range snapshot {
		changed := false
		for _, childID := range parent.CandidateChildren {
			if _, ok := transitions[childID]; ok {
				changed = true
				break
			}
		}
		if !changed {
			continue
		}
		if _, duplicate := seen[parent.ParentID]; duplicate {
			continue
		}
		seen[parent.ParentID] = struct{}{}
		open := false
		for _, child := range parent.DirectChildren {
			if changed, ok := transitions[child.ID()]; ok {
				child = changed
			}
			if !child.Status().Terminal() {
				open = true
				break
			}
		}
		if !open {
			parents = append(parents, parent.ParentID)
		}
	}
	return parents
}

func validateTerminalParentSnapshots(
	snapshot []TerminalParentSnapshot,
	candidates []issue.State,
) error {
	eligible := make(map[issue.ID]struct{}, len(candidates))
	for _, state := range candidates {
		eligible[state.ID()] = struct{}{}
	}
	for _, parent := range snapshot {
		if parent.ParentID == "" || len(parent.CandidateChildren) == 0 {
			return ErrIncompleteSnapshot
		}
		directChildren := make(map[issue.ID]struct{}, len(parent.DirectChildren))
		for _, child := range parent.DirectChildren {
			if child.ID() == "" {
				return ErrIncompleteSnapshot
			}
			directChildren[child.ID()] = struct{}{}
		}
		for _, candidate := range parent.CandidateChildren {
			if _, ok := eligible[candidate]; !ok {
				return ErrIncompleteSnapshot
			}
			if _, ok := directChildren[candidate]; !ok {
				return ErrIncompleteSnapshot
			}
		}
	}
	return nil
}

// validateExplicitClose enforces kind-specific completion requirements.
func validateExplicitClose(state issue.State, children []issue.State) error {
	switch state.Kind() {
	case issue.KindWorkstream, issue.KindRoutine:
		for _, child := range children {
			if !child.Status().Terminal() {
				return errkind.Errorf(errkind.InvalidInput, "%s has open children", state.Kind())
			}
		}
		return nil
	case issue.KindCheckpoint:
		return errkind.Errorf(errkind.InvalidInput, "checkpoint requires explicit resolution")
	case issue.KindTask:
		return nil
	default:
		return errkind.Errorf(errkind.InvalidInput, "invalid input: invalid issue kind %q", state.Kind())
	}
}

func mapIssueCommands[T any](
	ctx context.Context,
	values []string,
	run func(context.Context, issue.ID) (T, error),
) ([]T, error) {
	result := make([]T, 0, len(values))
	var errs []error
	for _, value := range values {
		id, err := issue.NewID(value)
		if err == nil {
			var item T
			item, err = run(ctx, id)
			if err == nil {
				result = append(result, item)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", strings.TrimSpace(value), err))
		}
	}
	return result, errors.Join(errs...)
}

func issueIDs(values []string) ([]issue.ID, error) {
	result := make([]issue.ID, len(values))
	for index, value := range values {
		id, err := issue.NewID(value)
		if err != nil {
			return nil, err
		}
		result[index] = id
	}
	return result, nil
}

func uniqueIssueIDs(ids []issue.ID) []issue.ID {
	result := make([]issue.ID, 0, len(ids))
	seen := make(map[issue.ID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func issueIDSet(ids []issue.ID) map[issue.ID]struct{} {
	set := make(map[issue.ID]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
