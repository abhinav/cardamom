package board

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

// CreateIssue creates one issue and its initial graph snapshot atomically.
func (r *Repository) CreateIssue(
	ctx context.Context,
	issueIDConfiguration configuration.IssueIDConfiguration,
	command planning.CreateIssue,
) (out planning.IssueCreated, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()

	allocatedID, err := r.allocateIssueID(ctx, mutation, issueIDConfiguration)
	if err != nil {
		return out, err
	}
	existingIDs, err := r.listBoardIssueIDs(ctx, mutation.change)
	if err != nil {
		return out, err
	}
	externalKeyOwner, err := r.readExternalKeyOwner(
		ctx,
		mutation.change,
		command.ExternalKey,
	)
	if err != nil {
		return out, err
	}
	board, err := planning.LoadCreate(planning.CreateSnapshot{
		BoardID: r.boardID, Revision: mutation.current,
		AllocatedID: allocatedID, ExistingIDs: existingIDs,
		ExternalKeyOwner: externalKeyOwner,
		OccurredAt:       mutation.occurredAt,
	})
	if err != nil {
		return out, err
	}
	out, err = board.CreateIssue(command)
	if err != nil {
		return out, err
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	if err := r.insertIssue(ctx, mutation, out.Issue); err != nil {
		return out, err
	}
	if out.ExternalKey != nil {
		if err := r.insertExternalKey(ctx, mutation, out.Issue.ID(), *out.ExternalKey); err != nil {
			return out, err
		}
	}
	if err := r.replaceLabels(ctx, mutation, out.Issue.ID(), out.Labels); err != nil {
		return out, err
	}
	if err := r.replaceDependencies(ctx, mutation, out.Issue.ID(), out.DependsOn); err != nil {
		return out, err
	}
	if err := r.replaceParent(ctx, mutation, out.Issue.ID(), out.Parent); err != nil {
		return out, err
	}
	if err := mutation.commit(ctx, out.Issue.ID()); err != nil {
		return out, err
	}
	out.CommittedRevision = planningCommittedRevision(mutation)
	return out, nil
}
