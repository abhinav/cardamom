package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// retainConflict stores derived conflict metadata and removes the disputed
// projections. Its caller owns the surrounding history import, revision, and
// local-custody reconciliation; this operation authors no new shared facts.
func (r *Repository) retainConflict(ctx context.Context, queries *query.Queries, entry boardconflict.Entry, revision int64) error {
	var unit boardconflict.Unit
	category := "field"
	var assignments []boardconflict.AssignmentReference
	var affected []boardconflict.AffectedIssue
	switch entry := entry.(type) {
	case *boardconflict.Conflict:
		unit = entry.Unit()
		affected = entry.AffectedIssues()
		for _, alternative := range entry.Alternatives() {
			assignments = append(assignments, boardconflict.AssignmentReference{Unit: unit, AlternativeID: boardconflict.AssignmentID(alternative.ID), Origins: alternative.Origins})
		}
	case *boardconflict.ConstraintConflict:
		category = entry.Kind().String()
		assignments = entry.Assignments()
		for _, assignment := range assignments {
			affected = append(affected, affectedConstraintIssue(assignment.Unit))
		}
	}
	if err := queries.BoardInsertConflict(ctx, query.BoardInsertConflictParams{
		ID: entry.ID(), BoardID: r.boardID.String(), Kind: string(unit.Kind), Category: category,
		IssueUid: optionalUID(unit.IssueUID), RelatedUid: optionalUID(unit.RelatedUID), MemberKey: unit.Key, Revision: revision,
	}); err != nil {
		return err
	}
	units := make(map[boardconflict.Unit]struct{})
	for _, assignment := range assignments {
		if err := retainConflictAssignment(ctx, queries, entry.ID(), assignment); err != nil {
			return err
		}
		units[assignment.Unit] = struct{}{}
	}
	seen := make(map[boardconflict.AffectedIssue]struct{})
	for _, value := range affected {
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		if err := queries.BoardInsertConflictIssue(ctx, query.BoardInsertConflictIssueParams{ConflictID: entry.ID(), IssueUid: value.UID.Bytes(), Field: value.Field.String()}); err != nil {
			return err
		}
	}
	for unit := range units {
		if err := r.writeConflictProjection(ctx, queries, unit, nil, nil, revision); err != nil {
			return err
		}
	}
	return nil
}

func retainConflictAssignment(ctx context.Context, queries *query.Queries, id string, assignment boardconflict.AssignmentReference) error {
	unit := assignment.Unit
	if err := queries.BoardInsertConflictAlternative(ctx, query.BoardInsertConflictAlternativeParams{
		ConflictID: id, AlternativeID: string(assignment.AlternativeID), Kind: string(unit.Kind),
		IssueUid: optionalUID(unit.IssueUID), RelatedUid: optionalUID(unit.RelatedUID), MemberKey: unit.Key,
		WaitingPresent: assignment.WaitingPresent,
	}); err != nil {
		return err
	}
	for _, origin := range assignment.Origins {
		if err := queries.BoardInsertConflictOrigin(ctx, query.BoardInsertConflictOriginParams{
			ConflictID: id, AlternativeID: string(assignment.AlternativeID), WriterUid: origin.Writer.Bytes(),
			Sequence: int64(origin.Sequence), Ordinal: int64(origin.Ordinal),
		}); err != nil {
			return err
		}
	}
	return nil
}

func affectedConstraintIssue(unit boardconflict.Unit) boardconflict.AffectedIssue {
	var field issue.Field
	switch unit.Kind {
	case boardhistory.EventIssueDependency:
		field = issue.FieldDependencies
	case boardhistory.EventIssueContainment:
		field = issue.FieldParent
	case boardhistory.EventIssueKindSet:
		field = issue.FieldKind
	case boardhistory.EventIssueLifecycleSet:
		field = issue.FieldLifecycle
	case boardhistory.EventIssueWaitingSet:
		field = issue.FieldWaiting
	}
	return boardconflict.AffectedIssue{UID: unit.IssueUID, Field: field}
}

func optionalUID(uid issue.UID) []byte {
	if uid == (issue.UID{}) {
		return nil
	}
	return uid.Bytes()
}
