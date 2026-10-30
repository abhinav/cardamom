package board

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// CheckBackup rejects selected boards whose reference markers the current
// backup format cannot preserve. It borrows view; callers must retain that
// same view while reading records so admission and capture share one snapshot.
func (r *BackupReader) CheckBackup(ctx context.Context, view *store.View, selection backup.Selection) error {
	reader := query.New(view)
	if selection.IsAll() {
		marker, err := reader.BoardReadAnyBackupReferenceMarker(ctx)
		if err == nil {
			return errkind.Errorf(errkind.Conflict, "backup cannot preserve reference marker %q", marker)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read store backup compatibility: %w", err)
		}
	}
	for _, boardID := range selection.BoardIDs() {
		marker, err := reader.BoardReadBackupReferenceMarker(ctx, boardID.String())
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read backup compatibility for board %q: %w", boardID, err)
		}
		return errkind.Errorf(errkind.Conflict,
			"backup cannot preserve reference marker %q on board %q", marker, boardID)
	}
	return nil
}
