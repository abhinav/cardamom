package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// CreateBoard atomically establishes one board at a canonical revision.
func (r *Repository) CreateBoard(
	ctx context.Context,
	request board.CreateRequest,
) (*board.State, error) {
	projectID, err := project.NewID(request.ProjectID)
	if err != nil {
		return nil, err
	}
	boardID, err := r.idSource.NewID("board")
	if err != nil {
		return nil, fmt.Errorf("generate board identity: %w", err)
	}
	state, err := board.Load(board.Snapshot{
		ID: board.ID(boardID), ProjectID: projectID.String(),
		Name: request.Name, Description: request.Description, Created: r.clock(),
	})
	if err != nil {
		return nil, err
	}
	err = r.commitRevision(
		ctx,
		func(change *store.Change) error {
			queries := query.New(change)
			projectExists, err := queries.ProjectExists(ctx, state.ProjectID())
			if err != nil {
				return err
			}
			if !projectExists {
				return errkind.Errorf(
					errkind.NotFound,
					"project not found: project %q",
					state.ProjectID(),
				)
			}
			return queries.ProjectCreateBoard(ctx, query.ProjectCreateBoardParams{
				ID:          state.ID().String(),
				ProjectID:   state.ProjectID(),
				Name:        state.Name(),
				Description: state.Description(),
				CreatedAt:   state.Created(),
			})
		},
	)
	if err != nil {
		return nil, err
	}
	return state, nil
}

// EditBoardSettings atomically changes one board's name and Markdown description.
func (r *Repository) EditBoardSettings(
	ctx context.Context,
	request board.EditRequest,
) (out *board.State, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	queries := query.New(change)
	row, err := queries.ProjectGetBoard(ctx, request.BoardID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "board not found")
	}
	if err != nil {
		return out, err
	}
	current, err := loadBoard(
		row.ID,
		row.ProjectID,
		row.Name,
		row.Description,
		row.CreatedAt,
	)
	if err != nil {
		return out, err
	}
	out, err = current.EditSettings(request.Settings)
	if err != nil {
		return out, err
	}
	if boardsEqual(current, out) {
		return out, change.Commit()
	}

	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return out, err
	}
	if err := queries.ProjectUpdateBoardSettings(
		ctx,
		query.ProjectUpdateBoardSettingsParams{
			Name:        out.Name(),
			Description: out.Description(),
			ID:          out.ID().String(),
		},
	); err != nil {
		return out, err
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return out, err
	}
	return out, change.Commit()
}

func boardsEqual(left, right *board.State) bool {
	if left.ID() != right.ID() || left.ProjectID() != right.ProjectID() ||
		left.Name() != right.Name() || !left.Created().Equal(right.Created()) {
		return false
	}
	leftDescription := left.Description()
	rightDescription := right.Description()
	if leftDescription == nil || rightDescription == nil {
		return leftDescription == nil && rightDescription == nil
	}
	return *leftDescription == *rightDescription
}
