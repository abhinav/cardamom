package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func (r *Repository) replaceDependencies(
	ctx context.Context,
	mutation *mutation,
	issueID issue.ID,
	dependencies []issue.ID,
) error {
	queries := query.New(mutation.change)
	if err := queries.BoardDeleteIssueDependencies(
		ctx,
		query.BoardDeleteIssueDependenciesParams{
			BoardID: r.boardID.String(),
			IssueID: issueID.String(),
		},
	); err != nil {
		return err
	}
	for _, prerequisite := range dependencies {
		if err := queries.BoardInsertIssueDependency(
			ctx,
			query.BoardInsertIssueDependencyParams{
				BoardID:        r.boardID.String(),
				IssueID:        issueID.String(),
				PrerequisiteID: prerequisite.String(),
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) replaceParent(
	ctx context.Context,
	mutation *mutation,
	child issue.ID,
	parent *issue.ID,
) error {
	queries := query.New(mutation.change)
	if err := queries.BoardDeleteIssueParent(
		ctx,
		query.BoardDeleteIssueParentParams{
			BoardID: r.boardID.String(),
			ChildID: child.String(),
		},
	); err != nil {
		return err
	}
	if parent == nil {
		return nil
	}
	return queries.BoardInsertIssueParent(
		ctx,
		query.BoardInsertIssueParentParams{
			BoardID:  r.boardID.String(),
			ChildID:  child.String(),
			ParentID: parent.String(),
		},
	)
}

func (r *Repository) readDirectChildren(ctx context.Context, scope queryScope, parent issue.ID) (out []issue.State, err error) {
	ids, err := query.New(scope).BoardListDirectChildIDs(
		ctx,
		query.BoardListDirectChildIDsParams{
			BoardID:  r.boardID.String(),
			ParentID: parent.String(),
		},
	)
	if err != nil {
		return nil, err
	}
	children := make([]issue.State, 0, len(ids))
	for _, value := range ids {
		id := issue.ID(value)
		state, _, err := r.readIssueState(ctx, scope, id)
		if err != nil {
			return nil, err
		}
		children = append(children, state)
	}
	return children, nil
}

func (r *Repository) dependencyAncestors(ctx context.Context, scope queryScope, start issue.ID) (out []issue.ID, err error) {
	values, err := query.New(scope).BoardListEditDependencyAncestorIDs(
		ctx,
		query.BoardListEditDependencyAncestorIDsParams{
			ScopeBoardID: r.boardID.String(),
			StartID:      start.String(),
		},
	)
	if err != nil {
		return nil, err
	}
	return issueIDsFromStrings(values), nil
}

func (r *Repository) containmentAncestors(ctx context.Context, scope queryScope, start issue.ID) (out []issue.ID, err error) {
	values, err := query.New(scope).BoardListEditContainmentAncestorIDs(
		ctx,
		query.BoardListEditContainmentAncestorIDsParams{
			ScopeBoardID: r.boardID.String(),
			StartID:      start.String(),
		},
	)
	if err != nil {
		return nil, err
	}
	return issueIDsFromStrings(values), nil
}

func (r *Repository) issueExists(ctx context.Context, scope queryScope, id issue.ID) (bool, error) {
	return query.New(scope).BoardIssueExists(
		ctx,
		query.BoardIssueExistsParams{
			BoardID: r.boardID.String(),
			IssueID: id.String(),
		},
	)
}

func (r *Repository) readContainmentParents(
	ctx context.Context,
	scope queryScope,
) (out map[issue.ID]issue.ID, err error) {
	rows, err := query.New(scope).BoardListContainmentParents(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	parents := make(map[issue.ID]issue.ID)
	for _, row := range rows {
		parents[issue.ID(row.ChildID)] = issue.ID(row.ParentID)
	}
	return parents, nil
}

func (r *Repository) descendantSet(ctx context.Context, scope queryScope, under string) (out map[issue.ID]struct{}, err error) {
	if under == "" {
		return nil, nil
	}
	root, err := issue.NewID(under)
	if err != nil {
		return nil, err
	}
	exists, err := r.issueExists(ctx, scope, root)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errkind.Errorf(errkind.NotFound, "issue not found: %s", root)
	}
	values, err := query.New(scope).BoardListDescendantIDs(
		ctx,
		query.BoardListDescendantIDsParams{
			ScopeBoardID: r.boardID.String(),
			RootID:       root.String(),
		},
	)
	if err != nil {
		return nil, err
	}
	result := make(map[issue.ID]struct{})
	for _, value := range values {
		result[issue.ID(value)] = struct{}{}
	}
	return result, nil
}

func (r *Repository) readPrerequisiteStates(ctx context.Context, scope queryScope, id issue.ID) ([]issue.State, error) {
	ids, err := query.New(scope).BoardListPrerequisiteIDs(
		ctx,
		query.BoardListPrerequisiteIDsParams{
			BoardID: r.boardID.String(),
			IssueID: id.String(),
		},
	)
	if err != nil {
		return nil, err
	}
	states := make([]issue.State, 0, len(ids))
	for _, value := range ids {
		state, _, err := r.readIssueState(ctx, scope, issue.ID(value))
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}
