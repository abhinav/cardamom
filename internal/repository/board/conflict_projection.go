package board

import (
	"context"
	"time"

	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// writeConflictProjection stores one supported assignment, or hides its current
// value when payload is nil. Conflict metadata explains the latter absence.
// changedAt is present for newly authored changes; retaining or hiding an
// existing assignment must not invent another authoring time.
func (r *Repository) writeConflictProjection(ctx context.Context, queries *query.Queries, unit boardconflict.Unit, payload boardhistory.Payload, changedAt *time.Time, revision int64) error {
	switch unit.Kind {
	case boardhistory.EventBoardNameSet, boardhistory.EventBoardDescriptionSet,
		boardhistory.EventBoardArchiveSet, boardhistory.EventBoardIssueIDPrefixSet,
		boardhistory.EventBoardIssueIDStrategySet, boardhistory.EventBoardSummaryLimitSet,
		boardhistory.EventBoardAttachmentLimitSet, boardhistory.EventBoardPinLimitSet:
		return r.writeConflictBoardValue(ctx, queries, unit, payload)
	case boardhistory.EventIssueLabel, boardhistory.EventIssueDependency,
		boardhistory.EventIssueContainment, boardhistory.EventExternalKey, boardhistory.EventBoardPin:
		return r.writeConflictRelationship(ctx, queries, unit, payload)
	default:
		return r.writeConflictIssueValue(ctx, queries, unit, payload, changedAt, revision)
	}
}
