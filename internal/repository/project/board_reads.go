package project

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ListAllBoards returns active and archived boards in stable project, name, and
// identity order.
func (r *Repository) ListAllBoards(ctx context.Context) (out []*board.State, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	rows, err := query.New(view).ProjectListAllBoards(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		state, err := loadBoard(
			row.ID,
			row.ProjectID,
			row.Name,
			row.Description,
			row.CreatedAt,
			row.ArchivedAt,
			row.ArchivedBy,
			row.ArchiveReason,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, nil
}

// ListProjectBoards returns one project's active boards in stable name and
// identity order. Archived boards are omitted from project aggregate scopes.
// An existing project with no boards returns an empty slice.
// The read does not verify project existence; callers resolve projectID before
// using it.
func (r *Repository) ListProjectBoards(
	ctx context.Context,
	projectID project.ID,
) (out []*board.State, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	rows, err := query.New(view).ProjectListProjectBoards(ctx, projectID.String())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		state, err := loadBoard(
			row.ID,
			row.ProjectID,
			row.Name,
			row.Description,
			row.CreatedAt,
			row.ArchivedAt,
			row.ArchivedBy,
			row.ArchiveReason,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, nil
}

// Board returns an active or archived board by stable identity.
func (r *Repository) Board(ctx context.Context, id board.ID) (out *board.State, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	row, err := query.New(view).ProjectGetBoard(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "board not found")
	}
	if err != nil {
		return out, err
	}
	return loadBoard(
		row.ID,
		row.ProjectID,
		row.Name,
		row.Description,
		row.CreatedAt,
		row.ArchivedAt,
		row.ArchivedBy,
		row.ArchiveReason,
	)
}

// SoleBoard returns the only active board in the store. Archived boards do not
// make implicit selection ambiguous.
func (r *Repository) SoleBoard(ctx context.Context) (out *board.State, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	rows, err := query.New(view).ProjectListBoardsForSoleSelection(ctx)
	if err != nil {
		return out, err
	}
	var boards []*board.State
	for _, row := range rows {
		state, err := loadBoard(
			row.ID,
			row.ProjectID,
			row.Name,
			row.Description,
			row.CreatedAt,
			row.ArchivedAt,
			row.ArchivedBy,
			row.ArchiveReason,
		)
		if err != nil {
			return out, err
		}
		boards = append(boards, state)
	}
	switch len(boards) {
	case 0:
		return out, errkind.Errorf(errkind.NotFound, "board not found")
	case 1:
		return boards[0], nil
	default:
		return out, errkind.Errorf(errkind.Conflict, "board selection is ambiguous")
	}
}

// loadBoard restores one domain board from selected persisted values. Archive
// time and actor form one logical presence marker; the database invariant keeps
// them both null for active boards and both populated for archived boards.
func loadBoard(
	id string,
	projectIDValue string,
	name string,
	description *string,
	createdAt time.Time,
	archivedAt *time.Time,
	archivedBy *string,
	archiveReason *string,
) (*board.State, error) {
	projectID, err := project.NewID(projectIDValue)
	if err != nil {
		return nil, err
	}
	var archived *board.Archive
	if archivedAt != nil && archivedBy != nil {
		archived = &board.Archive{Actor: *archivedBy, At: *archivedAt, Reason: archiveReason}
	}
	return board.Load(board.Snapshot{
		ID:          board.ID(id),
		ProjectID:   projectID.String(),
		Name:        name,
		Description: description,
		Created:     createdAt,
		Archived:    archived,
	})
}
