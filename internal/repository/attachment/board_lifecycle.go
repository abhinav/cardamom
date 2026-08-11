package attachment

import (
	"context"
	"database/sql"
	"errors"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// requireMutableBoard enforces board lifecycle inside the attachment writer's
// transaction, so archival cannot interleave with an accepted blob mutation.
func requireMutableBoard(ctx context.Context, queries *query.Queries, boardID board.ID) error {
	archived, err := queries.AttachmentBoardArchived(ctx, boardID.String())
	if errors.Is(err, sql.ErrNoRows) {
		// Preserve the attachment operation's more specific target-not-found error;
		// its subsequent validation still prevents a write for a missing board.
		return nil
	}
	if err != nil {
		return err
	}
	if archived {
		return board.ErrArchived
	}
	return nil
}
