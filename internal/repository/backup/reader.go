// Package backup composes project-owned and board-owned backup capture from one
// retained persistence snapshot.
package backup

import (
	"context"
	"errors"
	"fmt"

	domainbackup "go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	repositoryproject "go.abhg.dev/cardamom/internal/repository/project"
	"go.abhg.dev/cardamom/internal/repository/store"
)

var _ domainbackup.Source = (*Reader)(nil)

// Reader captures selected project and board state through one retained SQLite
// view.
type Reader struct {
	store    *store.Store   // required
	projects projectCatalog // required
	boards   boardRecords   // required
}

// projectCatalog resolves selected boards and their project metadata through a
// caller-owned retained view.
type projectCatalog interface {
	ReadBackupBoards(
		context.Context,
		*store.View,
		domainbackup.Selection,
	) ([]repositoryproject.BackupBoard, error)
}

// boardRecords yields semantic board state without ending the retained view.
type boardRecords interface {
	ReadBackupRecords(
		context.Context,
		*store.View,
		board.ID,
		configuration.Overrides,
		repositoryboard.BackupSource,
	) boardcopy.RecordSequence
}

// New binds backup capture to one store and its owner-local readers.
func New(persistence *store.Store) *Reader {
	must.NotBeNilf(persistence, "backup Store is required")
	return &Reader{
		store:    persistence,
		projects: &repositoryproject.BackupReader{},
		boards:   &repositoryboard.BackupReader{},
	}
}

// Capture streams selected project and board state from one retained canonical
// revision, then releases the view before returning.
func (r *Reader) Capture(
	ctx context.Context,
	selection domainbackup.Selection,
	storeOverrides configuration.Overrides,
	destination domainbackup.CaptureDestination,
) (result domainbackup.CaptureResult, err error) {
	if err := storeOverrides.Validate(); err != nil {
		return result, fmt.Errorf("source store configuration: %w", err)
	}
	view, err := r.store.View(ctx)
	if err != nil {
		return result, fmt.Errorf("begin backup capture: %w", err)
	}
	defer func() {
		if closeErr := view.Done(); closeErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf("close backup capture: %w", closeErr),
			)
		}
	}()

	lineageID, err := view.LineageID(ctx)
	if err != nil {
		return result, err
	}
	revision, err := view.CanonicalRevision(ctx)
	if err != nil {
		return result, fmt.Errorf("read backup source revision: %w", err)
	}
	selected, err := r.projects.ReadBackupBoards(ctx, view, selection)
	if err != nil {
		return result, err
	}
	source := repositoryboard.BackupSource{
		LineageID: lineageID,
		Revision:  revision,
	}
	result = domainbackup.CaptureResult{
		SourceLineageID: lineageID,
		SourceRevision:  revision,
	}
	projectIDs := make(map[project.ID]struct{})
	for _, selectedBoard := range selected {
		if _, found := projectIDs[selectedBoard.Project.ID]; !found {
			if err := destination.AddProject(selectedBoard.Project); err != nil {
				return result, err
			}
			projectIDs[selectedBoard.Project.ID] = struct{}{}
			result.Projects++
		}
		if err := destination.AddBoard(
			selectedBoard.Project.ID,
			selectedBoard.BoardID,
			r.boards.ReadBackupRecords(
				ctx,
				view,
				selectedBoard.BoardID,
				storeOverrides,
				source,
			),
		); err != nil {
			return result, err
		}
		result.Boards++
	}
	return result, nil
}
