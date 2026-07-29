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

// ClaimIssue acquires custody for one explicitly selected issue.
func (r *Repository) ClaimIssue(ctx context.Context, command execution.ClaimIssue) (out execution.IssueClaimed, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	var candidate *issue.State
	state, _, stateErr := r.readIssueState(ctx, mutation.change, command.IssueID)
	if stateErr == nil {
		candidate = &state
	} else if errkind.Of(stateErr) != errkind.NotFound {
		return out, stateErr
	}
	prerequisites, err := r.readPrerequisiteStates(ctx, mutation.change, command.IssueID)
	if err != nil {
		return out, err
	}
	board, err := execution.LoadClaimIssue(execution.ClaimIssueSnapshot{
		BoardID: r.boardID, Revision: mutation.current, Candidate: candidate,
		Prerequisites: prerequisites, OccurredAt: mutation.occurredAt,
	})
	if err != nil {
		return out, err
	}
	out, err = board.ClaimIssue(command)
	if err != nil {
		return out, err
	}
	return r.commitClaim(ctx, mutation, out)
}

// ClaimNext atomically selects and claims the first eligible pool issue.
func (r *Repository) ClaimNext(ctx context.Context, command execution.ClaimNext) (out execution.IssueClaimed, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	candidate, labels, prerequisites, err := r.selectClaimCandidate(ctx, mutation, command)
	if err != nil {
		return out, err
	}
	board, err := execution.LoadClaimNext(execution.ClaimNextSnapshot{
		BoardID: r.boardID, Revision: mutation.current, Candidate: candidate,
		Labels: labels, Prerequisites: prerequisites, OccurredAt: mutation.occurredAt,
	})
	if err != nil {
		return out, err
	}
	out, err = board.ClaimNext(command)
	if err != nil {
		return out, err
	}
	return r.commitClaim(ctx, mutation, out)
}

func (r *Repository) selectClaimCandidate(
	ctx context.Context,
	mutation *mutation,
	command execution.ClaimNext,
) (*issue.State, []issue.Label, []issue.State, error) {
	index, err := r.readBoardIssueIndex(ctx, mutation.change)
	if err != nil {
		return nil, nil, nil, err
	}
	descendants, err := r.descendantSet(ctx, mutation.change, command.UnderID.String())
	if err != nil {
		return nil, nil, nil, err
	}
	values := make([]issue.BoardIssueSummary, 0)
	for id := range index.states {
		if descendants != nil {
			if _, ok := descendants[id]; !ok {
				continue
			}
		}
		summary := index.summary(id)
		eligibility, err := execution.EvaluateEligibility(summary)
		if err != nil {
			return nil, nil, nil, err
		}
		if !eligibility.ReadyForClaim() {
			continue
		}
		matched := true
		for _, label := range command.LabelsAll {
			matched = matched && slices.Contains(summary.Labels, label.String())
		}
		if !matched {
			continue
		}
		if len(command.LabelsAny) > 0 {
			matched = false
			for _, label := range command.LabelsAny {
				matched = matched || slices.Contains(summary.Labels, label.String())
			}
			if !matched {
				continue
			}
		}
		for _, label := range command.LabelsNone {
			matched = matched && !slices.Contains(summary.Labels, label.String())
		}
		if !matched {
			continue
		}
		values = append(values, issue.BoardIssueSummary{
			BoardID: r.boardID.String(),
			Summary: summary,
		})
	}
	values = issue.OrderSummaries(issue.ListRequest{}, values)
	if len(values) == 0 {
		return nil, nil, nil, nil
	}
	selectedID, err := issue.NewID(values[0].Summary.Issue.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	selected := index.states[selectedID].state
	labelValues, err := labelsFromStrings(values[0].Summary.Labels)
	if err != nil {
		return nil, nil, nil, err
	}
	prerequisites, err := r.readPrerequisiteStates(ctx, mutation.change, selected.ID())
	return &selected, labelValues, prerequisites, err
}

func (r *Repository) commitClaim(ctx context.Context, mutation *mutation, out execution.IssueClaimed) (execution.IssueClaimed, error) {
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	if err := r.updateIssue(ctx, mutation, out.Issue); err != nil {
		return out, err
	}
	if err := r.replaceActiveClaim(ctx, mutation, out.Issue); err != nil {
		return out, err
	}
	if err := mutation.commit(ctx, out.Issue.ID()); err != nil {
		return out, err
	}
	out.CommittedRevision = executionCommittedRevision(mutation)
	return out, nil
}

// ReleaseIssue relinquishes custody only for the current owner.
func (r *Repository) ReleaseIssue(ctx context.Context, command execution.ReleaseIssue) (out execution.IssueReleased, err error) {
	mutation, board, before, err := r.lifecycleMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.ReleaseIssue(command)
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

func (r *Repository) replaceActiveClaim(ctx context.Context, mutation *mutation, state issue.State) error {
	queries := query.New(mutation.change)
	if err := queries.BoardDeleteActiveClaim(ctx, state.ID().String()); err != nil {
		return err
	}
	claim := state.ActiveClaim()
	if claim == nil {
		return nil
	}
	return queries.BoardInsertActiveClaim(
		ctx,
		query.BoardInsertActiveClaimParams{
			IssueID:         state.ID().String(),
			BoardID:         r.boardID.String(),
			Actor:           claim.Actor.String(),
			StartedAt:       claim.StartedAt,
			StartedRevision: mutation.reservation.Revision(),
		},
	)
}
