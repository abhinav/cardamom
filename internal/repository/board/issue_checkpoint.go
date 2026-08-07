package board

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ListActionableCheckpoints reads ready checkpoints whose
// prerequisites are closed in priority order.
func (r *Repository) ListActionableCheckpoints(ctx context.Context) (out []issue.CheckpointView, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	index, err := r.readBoardIssueIndex(ctx, view)
	if err != nil {
		return nil, err
	}
	values, _ := r.filterIssueSummaries(index, issue.ListRequest{Sort: "priority"}, nil)

	checkpoints := make([]issue.CheckpointView, 0)
	for _, value := range values {
		eligibility, err := execution.EvaluateEligibility(value)
		if err != nil {
			return nil, err
		}
		if !eligibility.ActionableCheckpoint() {
			continue
		}
		id, err := issue.NewID(value.Issue.ID)
		if err != nil {
			return nil, err
		}
		blockIDs, err := query.New(view).BoardListBlockIDs(
			ctx,
			query.BoardListBlockIDsParams{
				BoardID:        r.boardID.String(),
				PrerequisiteID: id.String(),
			},
		)
		if err != nil {
			return nil, err
		}
		blocks, err := index.references(blockIDs)
		if err != nil {
			return nil, err
		}
		checkpoints = append(checkpoints, issue.CheckpointView{
			Issue: value.Issue, Blocks: blocks, Labels: value.Labels,
		})
	}
	return checkpoints, nil
}

// ApproveCheckpoint closes one unblocked checkpoint and records its decision.
func (r *Repository) ApproveCheckpoint(ctx context.Context, command execution.ApproveCheckpoint) (execution.CheckpointResolved, error) {
	return r.resolveCheckpoint(ctx, checkpointResolution{
		issueID: command.IssueID, actor: command.Actor,
		approve: true, reason: command.Reason,
	})
}

// DenyCheckpoint cancels one unblocked checkpoint and its dependent closure.
func (r *Repository) DenyCheckpoint(ctx context.Context, command execution.DenyCheckpoint) (execution.CheckpointResolved, error) {
	return r.resolveCheckpoint(ctx, checkpointResolution{
		issueID: command.IssueID, actor: command.Actor,
		reason: command.Reason,
	})
}

// checkpointResolution is the normalized persistence command shared by
// checkpoint approval and denial.
type checkpointResolution struct {
	// issueID identifies the checkpoint to resolve.
	issueID issue.ID

	// actor commits changed State before terminal resolution.
	actor issue.Actor

	// approve selects closure of only the checkpoint.
	// False selects cancellation of the checkpoint and dependent closure.
	approve bool

	// reason is persisted with the immutable decision.
	reason string
}

func (r *Repository) resolveCheckpoint(
	ctx context.Context,
	command checkpointResolution,
) (out execution.CheckpointResolved, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	state, _, err := r.readIssueState(ctx, mutation.change, command.issueID)
	if err != nil {
		return out, err
	}
	prerequisites, err := r.readPrerequisiteStates(ctx, mutation.change, command.issueID)
	if err != nil {
		return out, err
	}
	var dependents []issue.State
	if !command.approve {
		closure, err := r.dependentClosure(ctx, mutation.change, []issue.ID{command.issueID})
		if err != nil {
			return out, err
		}
		if len(closure) > 0 {
			dependents = closure[1:]
		}
	}
	candidates := append([]issue.State{state}, dependents...)
	parents, err := r.terminalParentSnapshots(ctx, mutation.change, candidates)
	if err != nil {
		return out, err
	}
	if command.approve {
		policy, policyErr := execution.LoadApproveCheckpoint(execution.ApproveCheckpointSnapshot{
			BoardID: r.boardID, Revision: mutation.current, Issue: state,
			Prerequisites: prerequisites, TerminalParents: parents, OccurredAt: mutation.occurredAt,
		})
		if policyErr != nil {
			return out, policyErr
		}
		out, err = policy.ApproveCheckpoint(execution.ApproveCheckpoint{
			IssueID: command.issueID, Actor: command.actor,
			Reason: command.reason,
		})
	} else {
		policy, policyErr := execution.LoadDenyCheckpoint(execution.DenyCheckpointSnapshot{
			BoardID: r.boardID, Revision: mutation.current, Issue: state,
			Prerequisites: prerequisites, TransitiveDependents: dependents,
			TerminalParents: parents, OccurredAt: mutation.occurredAt,
		})
		if policyErr != nil {
			return out, policyErr
		}
		out, err = policy.DenyCheckpoint(execution.DenyCheckpoint{
			IssueID: command.issueID, Actor: command.actor,
			Reason: command.reason,
		})
	}
	if err != nil {
		return out, err
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	out.Decision.Revision = mutation.reservation.Revision()
	if err := query.New(mutation.change).BoardInsertCheckpointDecision(
		ctx,
		query.BoardInsertCheckpointDecisionParams{
			IssueID:   command.issueID.String(),
			BoardID:   r.boardID.String(),
			Outcome:   out.Decision.Outcome.String(),
			Reason:    out.Decision.Reason,
			DecidedAt: out.Decision.DecidedAt,
			Revision:  out.Decision.Revision,
		},
	); err != nil {
		return out, err
	}
	transitions := out.Affected
	if command.approve {
		transitions = []issue.State{out.Issue}
	}
	before := make(map[issue.ID]issue.State, len(candidates))
	for _, candidate := range candidates {
		before[candidate.ID()] = candidate
	}
	for index := range transitions {
		transition := transitions[index]
		snapshotID, err := r.insertChangedStateSnapshot(
			ctx,
			mutation,
			before[transition.ID()],
			command.actor,
		)
		if err != nil {
			return out, err
		}
		if snapshotID != nil && transition.RecoveryStateRecord() != nil {
			transition = transition.WithRecoveryStateSnapshot(snapshotID)
		}
		transition, err = r.persistIssueState(ctx, mutation, transition)
		if err != nil {
			return out, err
		}
		if err := r.replaceActiveClaim(ctx, mutation, transition); err != nil {
			return out, err
		}
		transitions[index] = transition
	}
	if command.approve {
		out.Issue = transitions[0]
	} else {
		out.Affected = transitions
	}
	issueIDs := make([]issue.ID, len(transitions))
	for index, transition := range transitions {
		issueIDs[index] = transition.ID()
	}
	if err := mutation.commit(ctx, issueIDs...); err != nil {
		return out, err
	}
	out.CommittedRevision = executionCommittedRevision(mutation)
	return out, nil
}
