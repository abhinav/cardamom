package project

import (
	"context"
	"fmt"

	domainbackup "go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	domainproject "go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// BackupBoard associates one selected board ID with validated source project
// metadata from the same retained view.
type BackupBoard struct {
	// Project is the source namespace containing BoardID.
	Project domainproject.Snapshot // required

	// BoardID is the selected complete board identity.
	BoardID board.ID // required
}

// BackupReader resolves source board and project metadata from a caller-owned
// retained repository view.
type BackupReader struct{}

// ReadBackupBoards resolves all or explicitly selected boards and their source
// project metadata without ending view.
func (r *BackupReader) ReadBackupBoards(
	ctx context.Context,
	view *store.View,
	selection domainbackup.Selection,
) ([]BackupBoard, error) {
	queries := query.New(view)
	var rows []query.ProjectListBackupBoardsRow
	if selection.IsAll() {
		values, err := queries.ProjectListBackupBoards(ctx)
		if err != nil {
			return nil, fmt.Errorf("list backup boards: %w", err)
		}
		rows = values
	} else {
		ids := selection.BoardIDs()
		encoded := make([]string, len(ids))
		for index, id := range ids {
			encoded[index] = id.String()
		}
		values, err := queries.ProjectListSelectedBackupBoards(ctx, encoded)
		if err != nil {
			return nil, fmt.Errorf("list selected backup boards: %w", err)
		}
		rows = make([]query.ProjectListBackupBoardsRow, len(values))
		for index, value := range values {
			rows[index] = query.ProjectListBackupBoardsRow(value)
		}
		if len(rows) != len(ids) {
			found := make(map[string]struct{}, len(rows))
			for _, row := range rows {
				found[row.BoardID] = struct{}{}
			}
			for _, id := range ids {
				if _, exists := found[id.String()]; !exists {
					return nil, errkind.Errorf(
						errkind.NotFound,
						"board %q not found",
						id,
					)
				}
			}
		}
	}

	out := make([]BackupBoard, 0, len(rows))
	for _, row := range rows {
		projectID, err := domainproject.NewID(row.ProjectID)
		if err != nil {
			return nil, err
		}
		state, err := domainproject.Load(domainproject.Snapshot{
			ID: projectID, Name: row.ProjectName, Created: row.ProjectCreatedAt,
		})
		if err != nil {
			return nil, err
		}
		boardID, err := board.NewID(row.BoardID)
		if err != nil {
			return nil, err
		}
		out = append(out, BackupBoard{
			Project: domainproject.Snapshot{
				ID: state.ID(), Name: state.Name(), Created: state.Created(),
			},
			BoardID: boardID,
		})
	}
	return out, nil
}
