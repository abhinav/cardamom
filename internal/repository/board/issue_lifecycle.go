package board

import (
	"context"
	"errors"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/record"
)

// CloseIssue transitions one issue to successful completion.
func (r *Repository) CloseIssue(ctx context.Context, command execution.CloseIssue) (out execution.IssueClosed, err error) {
	mutation, board, before, err := r.lifecycleMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.CloseIssue(command)
	if err != nil {
		return out, err
	}
	out.Issue, err = r.commitLifecycleIssue(
		ctx,
		mutation,
		before,
		out.Issue,
		command.Actor,
	)
	if err != nil {
		return out, err
	}
	out.CommittedRevision = executionCommittedRevision(mutation)
	return out, nil
}

// ReopenIssue returns one terminal issue to open lifecycle.
func (r *Repository) ReopenIssue(ctx context.Context, command execution.ReopenIssue) (out execution.IssueReopened, err error) {
	mutation, board, before, err := r.lifecycleMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.ReopenIssue(command)
	if err != nil {
		return out, err
	}
	out.Issue, err = r.commitLifecycleIssue(
		ctx,
		mutation,
		before,
		out.Issue,
		"",
	)
	if err != nil {
		return out, err
	}
	out.CommittedRevision = executionCommittedRevision(mutation)
	return out, nil
}

// lifecyclePolicy is the finite transition surface loaded from one coherent
// lifecycle snapshot.
type lifecyclePolicy interface {
	ReleaseIssue(execution.ReleaseIssue) (execution.IssueReleased, error)
	CloseIssue(execution.CloseIssue) (execution.IssueClosed, error)
	ReopenIssue(execution.ReopenIssue) (execution.IssueReopened, error)
}

func (r *Repository) lifecycleMutation(
	ctx context.Context,
	id issue.ID,
) (*mutation, lifecyclePolicy, issue.State, error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return nil, nil, issue.State{}, err
	}
	state, _, err := r.readIssueState(ctx, mutation.change, id)
	if err != nil {
		return nil, nil, issue.State{}, errors.Join(
			err,
			mutation.change.Done(),
		)
	}
	children, err := r.readDirectChildren(ctx, mutation.change, id)
	if err != nil {
		return nil, nil, issue.State{}, errors.Join(
			err,
			mutation.change.Done(),
		)
	}
	prerequisites, err := r.readPrerequisiteStates(ctx, mutation.change, id)
	if err != nil {
		return nil, nil, issue.State{}, errors.Join(
			err,
			mutation.change.Done(),
		)
	}
	parents, err := r.terminalParentSnapshots(ctx, mutation.change, []issue.State{state})
	if err != nil {
		return nil, nil, issue.State{}, errors.Join(
			err,
			mutation.change.Done(),
		)
	}
	var parent *execution.TerminalParentSnapshot
	if len(parents) > 0 {
		parent = &parents[0]
	}
	board, err := execution.LoadLifecycle(execution.LifecycleSnapshot{
		BoardID: r.boardID, Revision: mutation.current, Issue: state,
		DirectChildren: children, Prerequisites: prerequisites,
		TerminalParent: parent, OccurredAt: mutation.occurredAt,
	})
	if err != nil {
		return nil, nil, issue.State{}, errors.Join(
			err,
			mutation.change.Done(),
		)
	}
	return mutation, board, state, nil
}

func (r *Repository) commitLifecycleIssue(
	ctx context.Context,
	mutation *mutation,
	before issue.State,
	after issue.State,
	committer issue.Actor,
) (issue.State, error) {
	if err := mutation.reserve(ctx); err != nil {
		return after, err
	}
	snapshotID, err := r.insertChangedStateSnapshot(
		ctx,
		mutation,
		before,
		committer,
	)
	if err != nil {
		return after, err
	}
	if snapshotID != nil && after.RecoveryStateRecord() != nil {
		after = after.WithRecoveryStateSnapshot(snapshotID)
	}
	after, err = r.persistIssueState(ctx, mutation, after)
	if err != nil {
		return after, err
	}
	if err := r.replaceActiveClaim(ctx, mutation, after); err != nil {
		return after, err
	}
	return after, mutation.commit(ctx, after.ID())
}

func (r *Repository) insertChangedStateSnapshot(
	ctx context.Context,
	mutation *mutation,
	state issue.State,
	committer issue.Actor,
) (*issue.LogID, error) {
	policy, err := record.Load(record.Snapshot{
		BoardID:    r.boardID,
		Revision:   mutation.current,
		Issue:      state,
		OccurredAt: mutation.occurredAt,
	})
	if err != nil {
		return nil, err
	}
	out, err := policy.CommitState(record.CommitState{
		IssueID:     state.ID(),
		Committer:   committer,
		Disposition: record.CommitStateRetain,
	})
	if err != nil {
		return nil, err
	}
	if out.LogEntry == nil {
		return nil, nil
	}
	id, err := r.insertLogEntry(ctx, mutation, logEntryWrite{
		issueID:    out.LogEntry.IssueID,
		kind:       out.LogEntry.Kind,
		author:     out.LogEntry.Author,
		committer:  out.LogEntry.Committer,
		body:       out.LogEntry.Body,
		nextAction: out.LogEntry.NextAction,
		created:    out.LogEntry.Created,
	})
	if err != nil {
		return nil, err
	}
	out.LogEntry.ID = id
	return &id, nil
}

func (r *Repository) terminalParentSnapshots(ctx context.Context, scope queryScope, candidates []issue.State) ([]execution.TerminalParentSnapshot, error) {
	groups := make(map[issue.ID][]issue.ID)
	var order []issue.ID
	for _, candidate := range candidates {
		parentValue, err := r.readParent(ctx, scope, candidate.ID())
		if err != nil {
			return nil, err
		}
		if parentValue == nil {
			continue
		}
		parent := issue.ID(*parentValue)
		if _, ok := groups[parent]; !ok {
			order = append(order, parent)
		}
		groups[parent] = append(groups[parent], candidate.ID())
	}
	snapshot := make([]execution.TerminalParentSnapshot, 0, len(order))
	for _, parent := range order {
		children, err := r.readDirectChildren(ctx, scope, parent)
		if err != nil {
			return nil, err
		}
		snapshot = append(snapshot, execution.TerminalParentSnapshot{
			CandidateChildren: groups[parent], ParentID: parent, DirectChildren: children,
		})
	}
	return snapshot, nil
}

func executionCommittedRevision(mutation *mutation) execution.CommittedRevision {
	return execution.CommittedRevision{
		Revision: domainboard.Revision(mutation.reservation.Revision()),
	}
}
