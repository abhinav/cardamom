package board

import (
	"context"
	"errors"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// EditIssue atomically applies scalar, label, dependency, and containment edits.
func (r *Repository) EditIssue(
	ctx context.Context,
	command planning.EditIssue,
) (out planning.IssueEdited, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()

	snapshot, err := r.editSnapshot(ctx, mutation, command)
	if err != nil {
		return out, err
	}
	board, err := planning.LoadEdit(snapshot)
	if err != nil {
		return out, err
	}
	out, err = board.EditIssue(command)
	if err != nil || !out.Changed {
		return out, err
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	if err := r.updateIssue(ctx, mutation, out.Issue); err != nil {
		return out, err
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

func (r *Repository) editSnapshot(
	ctx context.Context,
	mutation *mutation,
	command planning.EditIssue,
) (planning.EditSnapshot, error) {
	state, _, err := r.readIssueState(ctx, mutation.change, command.IssueID)
	if err != nil {
		return planning.EditSnapshot{}, err
	}
	children, err := r.readDirectChildren(ctx, mutation.change, command.IssueID)
	if err != nil {
		return planning.EditSnapshot{}, err
	}
	labelValues, err := r.readLabels(ctx, mutation.change, command.IssueID)
	if err != nil {
		return planning.EditSnapshot{}, err
	}
	labels, err := labelsFromStrings(labelValues)
	if err != nil {
		return planning.EditSnapshot{}, err
	}
	dependencyValues, err := query.New(mutation.change).BoardListPrerequisiteIDs(
		ctx,
		query.BoardListPrerequisiteIDsParams{
			BoardID: r.boardID.String(),
			IssueID: command.IssueID.String(),
		},
	)
	if err != nil {
		return planning.EditSnapshot{}, err
	}
	dependencies := issueIDsFromStrings(dependencyValues)
	parentValue, err := r.readParent(ctx, mutation.change, command.IssueID)
	if err != nil {
		return planning.EditSnapshot{}, err
	}
	var parent *issue.ID
	if parentValue != nil {
		value := issue.ID(*parentValue)
		parent = &value
	}
	existingIDs, err := r.listBoardIssueIDs(ctx, mutation.change)
	if err != nil {
		return planning.EditSnapshot{}, err
	}
	dependencyAncestors := make(map[issue.ID][]issue.ID, len(command.AddDependencies))
	for _, dependency := range command.AddDependencies {
		dependencyAncestors[dependency], err = r.dependencyAncestors(ctx, mutation.change, dependency)
		if err != nil {
			return planning.EditSnapshot{}, err
		}
	}
	var containmentAncestors []issue.ID
	if command.ParentSet && command.Parent != "" {
		containmentAncestors, err = r.containmentAncestors(ctx, mutation.change, command.Parent)
		if err != nil {
			return planning.EditSnapshot{}, err
		}
	}
	return planning.EditSnapshot{
		BoardID: r.boardID, Revision: mutation.current, Issue: state,
		DirectChildren: children, Labels: labels, Dependencies: dependencies,
		Parent: parent, ExistingIDs: existingIDs,
		DependencyAncestors:  dependencyAncestors,
		ContainmentAncestors: containmentAncestors,
		OccurredAt:           mutation.occurredAt,
	}, nil
}

func issueIDsFromStrings(values []string) []issue.ID {
	out := make([]issue.ID, len(values))
	for index, value := range values {
		out[index] = issue.ID(value)
	}
	return out
}

func planningCommittedRevision(mutation *mutation) planning.CommittedRevision {
	return planning.CommittedRevision{
		Revision: domainboard.Revision(mutation.reservation.Revision()),
	}
}
