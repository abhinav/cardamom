package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func migrate20260916180001(ctx context.Context, tx *sql.Tx) error {
	queries := query.New(tx)
	boardIDs, err := queries.BoardHistoryListBoards(ctx)
	if err != nil {
		return fmt.Errorf("list boards for history baseline: %w", err)
	}
	observedAt := time.Now().UTC()
	for _, rawBoardID := range boardIDs {
		boardID, err := board.NewID(rawBoardID)
		if err != nil {
			return fmt.Errorf("read board identity %q: %w", rawBoardID, err)
		}
		if err := repositoryhistory.InitializeV1(ctx, tx, boardID, observedAt); err != nil {
			return fmt.Errorf("baseline board %q: %w", rawBoardID, err)
		}
	}
	return nil
}
