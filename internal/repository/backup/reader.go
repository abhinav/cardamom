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
	boards   boardSnapshots // required
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

// boardSnapshots reads semantic board state without ending the retained view.
type boardSnapshots interface {
	ReadBackupSnapshot(
		context.Context,
		*store.View,
		board.ID,
		configuration.Overrides,
		repositoryboard.BackupSource,
	) (boardcopy.CopySnapshot, error)
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

// Capture reads project metadata and every selected semantic board snapshot
// from one retained canonical revision.
func (r *Reader) Capture(
	ctx context.Context,
	selection domainbackup.Selection,
	storeOverrides configuration.Overrides,
) (captured domainbackup.Capture, err error) {
	if err := storeOverrides.Validate(); err != nil {
		return captured, fmt.Errorf("source store configuration: %w", err)
	}
	view, err := r.store.View(ctx)
	if err != nil {
		return captured, fmt.Errorf("begin backup capture: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	lineageID, err := view.LineageID(ctx)
	if err != nil {
		return captured, err
	}
	revision, err := view.CanonicalRevision(ctx)
	if err != nil {
		return captured, fmt.Errorf("read backup source revision: %w", err)
	}
	selected, err := r.projects.ReadBackupBoards(ctx, view, selection)
	if err != nil {
		return captured, err
	}
	source := repositoryboard.BackupSource{
		LineageID: lineageID,
		Revision:  revision,
	}
	captured.SourceLineageID = lineageID
	captured.SourceRevision = revision
	projectIDs := make(map[project.ID]struct{})
	for _, selectedBoard := range selected {
		if _, found := projectIDs[selectedBoard.Project.ID]; !found {
			captured.Projects = append(captured.Projects, selectedBoard.Project)
			projectIDs[selectedBoard.Project.ID] = struct{}{}
		}
		snapshot, err := r.boards.ReadBackupSnapshot(
			ctx,
			view,
			selectedBoard.BoardID,
			storeOverrides,
			source,
		)
		if err != nil {
			return captured, err
		}
		captured.Boards = append(captured.Boards, domainbackup.CapturedBoard{
			ProjectID: selectedBoard.Project.ID,
			Snapshot:  snapshot,
		})
	}
	return captured, nil
}
