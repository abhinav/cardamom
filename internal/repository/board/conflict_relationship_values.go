package board

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func (r *Repository) writeConflictRelationship(ctx context.Context, queries *query.Queries, unit boardconflict.Unit, payload boardhistory.Payload) error {
	switch unit.Kind {
	case boardhistory.EventIssueLabel:
		if err := queries.BoardDeleteLabelProjection(ctx, query.BoardDeleteLabelProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Label: unit.Key}); err != nil {
			return err
		}
		if payload == nil || !payload.(*boardhistory.IssueLabel).Present {
			return nil
		}
		return queries.BoardInsertIssueLabel(ctx, query.BoardInsertIssueLabelParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Label: unit.Key})
	case boardhistory.EventIssueDependency:
		if err := queries.BoardDeleteDependencyProjection(ctx, query.BoardDeleteDependencyProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), PrerequisiteUid: unit.RelatedUID.Bytes()}); err != nil {
			return err
		}
		if payload == nil || !payload.(*boardhistory.IssueDependency).Present {
			return nil
		}
		return queries.BoardInsertIssueDependency(ctx, query.BoardInsertIssueDependencyParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), PrerequisiteUid: unit.RelatedUID.Bytes()})
	case boardhistory.EventIssueContainment:
		if err := queries.BoardDeleteIssueParent(ctx, query.BoardDeleteIssueParentParams{BoardID: r.boardID.String(), ChildUid: unit.IssueUID.Bytes()}); err != nil {
			return err
		}
		if payload == nil || !payload.(*boardhistory.IssueContainment).Present {
			return nil
		}
		return queries.BoardInsertIssueParent(ctx, query.BoardInsertIssueParentParams{BoardID: r.boardID.String(), ChildUid: unit.IssueUID.Bytes(), ParentUid: payload.(*boardhistory.IssueContainment).ParentUID.Bytes()})
	case boardhistory.EventExternalKey:
		if err := queries.BoardDeleteExternalKeyProjection(ctx, query.BoardDeleteExternalKeyProjectionParams{BoardID: r.boardID.String(), ExternalKey: unit.Key}); err != nil {
			return err
		}
		if payload == nil || !payload.(*boardhistory.ExternalKey).Present {
			return nil
		}
		return queries.BoardInsertIssueExternalKey(ctx, query.BoardInsertIssueExternalKeyParams{BoardID: r.boardID.String(), ExternalKey: unit.Key, IssueUid: payload.(*boardhistory.ExternalKey).IssueUID.Bytes()})
	case boardhistory.EventBoardPin:
		if _, err := queries.BoardDeletePin(ctx, query.BoardDeletePinParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes()}); err != nil {
			return err
		}
		if payload == nil || !payload.(*boardhistory.BoardPin).Present {
			return nil
		}
		return queries.BoardInsertPinAtPosition(ctx, query.BoardInsertPinAtPositionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Position: payload.(*boardhistory.BoardPin).Position})
	default:
		return fmt.Errorf("unsupported relationship projection %s", unit.Kind)
	}
}
