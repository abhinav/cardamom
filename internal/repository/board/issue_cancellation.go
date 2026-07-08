package board

import (
	"context"
	"errors"
	"slices"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// CancelIssues atomically cancels requested roots and their dependent closure.
func (r *Repository) CancelIssues(ctx context.Context, command execution.CancelIssues) (out execution.IssuesCancelled, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	roots := make([]issue.State, 0, len(command.Roots))
	for _, id := range command.Roots {
		state, _, err := r.readIssueState(ctx, mutation.change, id)
		if err == nil {
			roots = append(roots, state)
		} else if errkind.Of(err) != errkind.NotFound {
			return out, err
		}
	}
	closure, err := r.dependentClosure(ctx, mutation.change, command.Roots)
	if err != nil {
		return out, err
	}
	parents, err := r.terminalParentSnapshots(ctx, mutation.change, closure)
	if err != nil {
		return out, err
	}
	board, err := execution.LoadCancellation(execution.CancellationSnapshot{
		BoardID: r.boardID, Revision: mutation.current, Roots: roots,
		Closure: closure, TerminalParents: parents, OccurredAt: mutation.occurredAt,
	})
	if err != nil {
		return out, err
	}
	out, err = board.CancelIssues(command)
	if err != nil || len(out.Issues) == 0 {
		return out, err
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	before := make(map[issue.ID]issue.State, len(closure))
	for _, state := range closure {
		before[state.ID()] = state
	}
	for index := range out.Issues {
		state := out.Issues[index]
		snapshotID, err := r.insertChangedStateSnapshot(
			ctx,
			mutation,
			before[state.ID()],
			command.Actor,
		)
		if err != nil {
			return out, err
		}
		if snapshotID != nil && state.RecoveryStateRecord() != nil {
			state = state.WithRecoveryStateSnapshot(snapshotID)
		}
		state, err = r.persistIssueState(ctx, mutation, state)
		if err != nil {
			return out, err
		}
		if err := r.replaceActiveClaim(ctx, mutation, state); err != nil {
			return out, err
		}
		out.Issues[index] = state
	}
	issueIDs := make([]issue.ID, len(out.Issues))
	for index, state := range out.Issues {
		issueIDs[index] = state.ID()
	}
	if err := mutation.commit(ctx, issueIDs...); err != nil {
		return out, err
	}
	out.CommittedRevision = executionCommittedRevision(mutation)
	return out, nil
}

func (r *Repository) dependentClosure(ctx context.Context, scope queryScope, roots []issue.ID) (out []issue.State, err error) {
	rows, err := query.New(scope).BoardListCancellationDependencyEdges(
		ctx,
		r.boardID.String(),
	)
	if err != nil {
		return nil, err
	}
	dependents := make(map[issue.ID][]issue.ID)
	for _, row := range rows {
		child := issue.ID(row.IssueID)
		parent := issue.ID(row.PrerequisiteID)
		dependents[parent] = append(dependents[parent], child)
	}
	queue := slices.Clone(roots)
	seen := make(map[issue.ID]struct{})
	var closure []issue.State
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		state, _, err := r.readIssueState(ctx, scope, id)
		if errkind.Of(err) == errkind.NotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		closure = append(closure, state)
		queue = append(queue, dependents[id]...)
	}
	return closure, nil
}
