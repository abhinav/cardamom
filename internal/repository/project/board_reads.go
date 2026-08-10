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

// ListAllBoards returns every board in stable project, name, and identity
// order.
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
		)
		if err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, nil
}

// ListProjectBoards returns one project's boards in stable name and identity
// order.
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
		)
		if err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, nil
}

// Board returns one board by stable identity.
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
	)
}

// SoleBoard returns the only board in the store.
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

// loadBoard restores one domain board from selected persisted values.
func loadBoard(
	id string,
	projectIDValue string,
	name string,
	description *string,
	createdAt time.Time,
) (*board.State, error) {
	projectID, err := project.NewID(projectIDValue)
	if err != nil {
		return nil, err
	}
	return board.Load(board.Snapshot{
		ID:          board.ID(id),
		ProjectID:   projectID.String(),
		Name:        name,
		Description: description,
		Created:     createdAt,
	})
}
