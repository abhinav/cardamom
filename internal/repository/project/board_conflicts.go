package project

import (
	"context"

	"go.abhg.dev/cardamom/internal/boardconflict"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// readBoardConflicts batches compact metadata for catalog reads. A non-nil
// identity limits the query to a single board without loading alternatives.
func readBoardConflicts(ctx context.Context, queries *query.Queries, boardID *string) (map[string][]board.ConflictReference, error) {
	rows, err := queries.ProjectReadBoardConflicts(ctx, boardID)
	if err != nil {
		return nil, err
	}
	byBoard := make(map[string][]board.ConflictReference)
	for _, row := range rows {
		field, err := (boardconflict.Unit{Kind: boardhistory.EventKind(row.Kind)}).BoardField()
		if err != nil {
			return nil, err
		}
		byBoard[row.BoardID] = append(byBoard[row.BoardID], board.ConflictReference{ID: row.ID, Field: field})
	}
	return byBoard, nil
}

// configurationConflicts keeps override disputes separate from board identity
// and archive disputes. Configuration resolution can still display known
// values when the board has an unrelated conflict.
func configurationConflicts(references []board.ConflictReference) []configuration.ConflictReference {
	var out []configuration.ConflictReference
	for _, reference := range references {
		var field configuration.Field
		switch reference.Field {
		case board.FieldIssueIDPrefix:
			field = configuration.FieldIssueIDPrefix
		case board.FieldIssueIDStrategy:
			field = configuration.FieldIssueIDStrategy
		case board.FieldIssueSummaryMaxBytes:
			field = configuration.FieldIssueSummaryMaxBytes
		case board.FieldAttachmentMaxBytes:
			field = configuration.FieldAttachmentMaxBytes
		case board.FieldPinsMaxCount:
			field = configuration.FieldBoardPinsMaxCount
		default:
			continue
		}
		out = append(out, configuration.ConflictReference{ID: reference.ID, Field: field})
	}
	return out
}
