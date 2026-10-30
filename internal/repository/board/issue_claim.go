package board

import (
	"context"
	"errors"

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
	presentation, err := r.readIssuePresentation(ctx, mutation.change)
	if err != nil {
		return out, err
	}
	board, err := execution.LoadClaimIssue(execution.ClaimIssueSnapshot{
		BoardID: r.boardID, Revision: mutation.current, Candidate: candidate,
		Prerequisites: presentation.ClaimPrerequisites(command.IssueID), OccurredAt: mutation.occurredAt,
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
func (r *Repository) ClaimNext(ctx context.Context, command execution.ClaimNext) (out execution.NextIssueClaimed, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	snapshot, err := r.selectClaimCandidate(ctx, mutation, command)
	if err != nil {
		return out, err
	}
	snapshot.BoardID, snapshot.Revision, snapshot.OccurredAt = r.boardID, mutation.current, mutation.occurredAt
	board, err := execution.LoadClaimNext(snapshot)
	if err != nil {
		return out, err
	}
	out, err = board.ClaimNext(command)
	if err != nil {
		return out, err
	}
	if out.Issue == nil {
		return out, nil
	}
	committed, err := r.commitClaim(ctx, mutation, execution.IssueClaimed{Issue: *out.Issue})
	if err != nil {
		return out, err
	}
	out.Issue = &committed.Issue
	out.CommittedRevision = committed.CommittedRevision
	return out, nil
}

func (r *Repository) selectClaimCandidate(
	ctx context.Context,
	mutation *mutation,
	command execution.ClaimNext,
) (execution.ClaimNextSnapshot, error) {
	index, err := r.readIssuePresentation(ctx, mutation.change)
	if err != nil {
		return execution.ClaimNextSnapshot{}, err
	}
	if command.UnderID != "" {
		if !index.Contains(command.UnderID) {
			return execution.ClaimNextSnapshot{}, errkind.Errorf(errkind.NotFound, "issue not found: %s", command.UnderID)
		}
	}
	values := make([]issue.BoardIssueSummary, 0)
	skipped := 0
	for id := range index.IDs() {
		if !index.CouldBeUnder(id, command.UnderID) {
			continue
		}
		summary := index.Summary(id)
		if !execution.MatchesKnownClaimLabels(summary, command) {
			continue
		}
		eligibility := index.Eligibility(id, summary)
		if eligibility.SkippedForConflict() {
			skipped++
			continue
		}
		if !eligibility.ReadyForClaim() {
			continue
		}
		values = append(values, issue.BoardIssueSummary{
			BoardID: r.boardID.String(),
			Summary: summary,
		})
	}
	values = issue.OrderSummaries(issue.ListRequest{Sort: "priority"}, values)
	if len(values) == 0 {
		return execution.ClaimNextSnapshot{SkippedConflicts: skipped}, nil
	}
	selectedID, err := issue.NewID(values[0].Summary.Issue.ID)
	if err != nil {
		return execution.ClaimNextSnapshot{}, err
	}
	selected, _, err := r.readIssueState(ctx, mutation.change, selectedID)
	if err != nil {
		return execution.ClaimNextSnapshot{}, err
	}
	labelValues, err := labelsFromStrings(values[0].Summary.Labels)
	if err != nil {
		return execution.ClaimNextSnapshot{}, err
	}
	return execution.ClaimNextSnapshot{Candidate: &selected, Labels: labelValues, Prerequisites: index.ClaimPrerequisites(selected.ID()), SkippedConflicts: skipped}, nil
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
	var commitErr error
	if mutation.hasHistory() {
		commitErr = mutation.commit(ctx, out.Issue.ID())
	} else {
		commitErr = mutation.commitLocal(ctx, out.Issue.ID())
	}
	if commitErr != nil {
		return out, commitErr
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
	if err := queries.BoardDeleteActiveClaim(ctx, state.UID().Bytes()); err != nil {
		return err
	}
	claim := state.ActiveClaim()
	if claim == nil {
		return nil
	}
	return queries.BoardInsertActiveClaim(
		ctx,
		query.BoardInsertActiveClaimParams{
			IssueUid:        state.UID().Bytes(),
			BoardID:         r.boardID.String(),
			Actor:           claim.Actor.String(),
			StartedAt:       claim.StartedAt,
			StartedRevision: mutation.reservation.Revision(),
		},
	)
}
