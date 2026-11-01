package board

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// checkBackupConflicts uses authoritative conflict metadata, including optional
// fields whose absent projections would otherwise resemble empty values.
func checkBackupConflicts(ctx context.Context, reader *query.Queries, selection backup.Selection) error {
	if selection.IsAll() {
		id, err := reader.BoardReadAnyBackupConflict(ctx)
		if err == nil {
			return errkind.Errorf(errkind.Conflict, "backup cannot preserve unresolved conflict %s", id)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read store backup conflicts: %w", err)
		}
	}
	for _, boardID := range selection.BoardIDs() {
		id, err := reader.BoardReadBackupConflict(ctx, boardID.String())
		if err == nil {
			return errkind.Errorf(errkind.Conflict, "backup cannot preserve unresolved conflict %s on board %q", id, boardID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read board backup conflicts: %w", err)
		}
	}
	return nil
}
