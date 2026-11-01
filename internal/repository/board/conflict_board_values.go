package board

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func (r *Repository) writeConflictBoardValue(ctx context.Context, queries *query.Queries, unit boardconflict.Unit, payload boardhistory.Payload) error {
	switch unit.Kind {
	case boardhistory.EventBoardNameSet:
		var current *string
		if payload != nil {
			value := payload.(*boardhistory.BoardNameSet)
			current = &value.Name
		}
		return queries.BoardSetNameProjection(ctx, query.BoardSetNameProjectionParams{BoardID: r.boardID.String(), Value: current})
	case boardhistory.EventBoardDescriptionSet:
		var current *string
		if payload != nil {
			value := payload.(*boardhistory.BoardDescriptionSet)
			current = value.Description
		}
		return queries.BoardSetDescriptionProjection(ctx, query.BoardSetDescriptionProjectionParams{BoardID: r.boardID.String(), Value: current})
	case boardhistory.EventBoardIssueIDPrefixSet:
		var current *string
		if payload != nil {
			value := payload.(*boardhistory.BoardIssueIDPrefixSet)
			current = value.Prefix
		}
		return queries.BoardSetIDPrefixProjection(ctx, query.BoardSetIDPrefixProjectionParams{BoardID: r.boardID.String(), Value: current})
	case boardhistory.EventBoardIssueIDStrategySet:
		var current *string
		if payload != nil {
			value := payload.(*boardhistory.BoardIssueIDStrategySet)
			current = value.Strategy
		}
		return queries.BoardSetIDStrategyProjection(ctx, query.BoardSetIDStrategyProjectionParams{BoardID: r.boardID.String(), Value: current})
	case boardhistory.EventBoardSummaryLimitSet:
		var current *int64
		if payload != nil {
			value := payload.(*boardhistory.BoardSummaryLimitSet)
			current = value.Limit
		}
		return queries.BoardSetSummaryLimitProjection(ctx, query.BoardSetSummaryLimitProjectionParams{BoardID: r.boardID.String(), Value: current})
	case boardhistory.EventBoardAttachmentLimitSet:
		var current *int64
		if payload != nil {
			value := payload.(*boardhistory.BoardAttachmentLimitSet)
			current = value.Limit
		}
		return queries.BoardSetAttachmentLimitProjection(ctx, query.BoardSetAttachmentLimitProjectionParams{BoardID: r.boardID.String(), Value: current})
	case boardhistory.EventBoardPinLimitSet:
		var current *int64
		if payload != nil {
			value := payload.(*boardhistory.BoardPinLimitSet)
			current = value.Limit
		}
		return queries.BoardSetPinLimitProjection(ctx, query.BoardSetPinLimitProjectionParams{BoardID: r.boardID.String(), Value: current})
	case boardhistory.EventBoardArchiveSet:
		parameters := query.BoardSetArchiveProjectionParams{BoardID: r.boardID.String()}
		if payload != nil {
			value := payload.(*boardhistory.BoardArchiveSet)
			if value.Archive != nil {
				parameters.ArchivedAt, parameters.ArchivedBy = &value.Archive.At, &value.Archive.Actor
				parameters.ArchiveReason = value.Archive.Reason
			}
		}
		return queries.BoardSetArchiveProjection(ctx, parameters)
	default:
		return fmt.Errorf("unsupported board projection %s", unit.Kind)
	}
}
