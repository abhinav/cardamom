package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// readRelations supplies compact graph inputs from the retained transaction.
// Record bodies and unresolved alternative edges are not current graph values.
func (r *Repository) readRelations(ctx context.Context, scope queryScope) (out planning.Relations, err error) {
	queries := query.New(scope)
	rows, err := queries.BoardReadRelationIssues(ctx, r.boardID.String())
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		uid, err := issue.ParseUID(row.Uid)
		if err != nil {
			return out, err
		}
		value := planning.RelationIssue{UID: uid}
		if row.Kind != nil {
			kind, err := issue.NewKind(*row.Kind)
			if err != nil {
				return out, err
			}
			value.Kind = &kind
		}
		if row.Lifecycle != nil {
			lifecycle, err := issue.NewLifecycle(*row.Lifecycle)
			if err != nil {
				return out, err
			}
			value.Lifecycle = &lifecycle
		}
		out.Issues = append(out.Issues, value)
	}
	dependencies, err := queries.BoardReadRelationDependencies(ctx, r.boardID.String())
	if err != nil {
		return out, err
	}
	for _, row := range dependencies {
		uid, err := issue.ParseUID(row.IssueUid)
		if err != nil {
			return out, err
		}
		prerequisite, err := issue.ParseUID(row.PrerequisiteUid)
		if err != nil {
			return out, err
		}
		out.Dependencies = append(out.Dependencies, planning.RelationDependency{IssueUID: uid, PrerequisiteUID: prerequisite})
	}
	parents, err := queries.BoardReadRelationParents(ctx, r.boardID.String())
	if err != nil {
		return out, err
	}
	for _, row := range parents {
		child, err := issue.ParseUID(row.ChildUid)
		if err != nil {
			return out, err
		}
		parent, err := issue.ParseUID(row.ParentUid)
		if err != nil {
			return out, err
		}
		out.Parents = append(out.Parents, planning.RelationParent{ChildUID: child, ParentUID: parent})
	}
	return out, nil
}
