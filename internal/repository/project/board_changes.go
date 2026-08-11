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
		row.ArchivedAt,
		row.ArchivedBy,
		row.ArchiveReason,
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

// ArchiveBoard records archive metadata when the board has no active claim and
// reports the effective issue population observed by the same writer. An
// already archived board returns its original metadata and no new revision.
func (r *Repository) ArchiveBoard(
	ctx context.Context,
	invocation board.Invocation,
	request board.ArchiveRequest,
) (out board.ArchiveResult, err error) {
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
	out.Board, err = loadBoard(row.ID, row.ProjectID, row.Name, row.Description,
		row.CreatedAt, row.ArchivedAt, row.ArchivedBy, row.ArchiveReason)
	if err != nil {
		return out, err
	}
	// Keeping the inventory, custody check, and lifecycle write in one immediate
	// transaction prevents a claim from racing the archive decision. The counts
	// therefore describe the same board state that accepted the transition.
	out.Issues, err = countBoardIssues(ctx, queries, request.BoardID)
	if err != nil {
		return out, err
	}
	if out.Board.Archived() != nil {
		return out, change.Commit()
	}
	hasClaim, err := queries.ProjectBoardHasActiveClaim(ctx, request.BoardID.String())
	if err != nil {
		return out, err
	}
	if hasClaim {
		return out, errkind.Errorf(errkind.Conflict, "board has an active claim")
	}
	changed, err := out.Board.ArchiveBoard(invocation.Actor(), r.clock(), request.Reason)
	if err != nil {
		return out, err
	}
	out.Changed = changed
	archive := out.Board.Archived()
	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return out, err
	}
	if err := queries.ProjectArchiveBoard(ctx, query.ProjectArchiveBoardParams{
		ArchivedAt: &archive.At, ArchivedBy: &archive.Actor,
		ArchiveReason: archive.Reason, ID: request.BoardID.String(),
	}); err != nil {
		return out, err
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return out, err
	}
	return out, change.Commit()
}

// UnarchiveBoard atomically clears archive metadata and publishes the lifecycle
// revision. An already active board returns unchanged without a new revision.
func (r *Repository) UnarchiveBoard(ctx context.Context, id board.ID) (out *board.State, changed bool, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	queries := query.New(change)
	row, err := queries.ProjectGetBoard(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, errkind.Errorf(errkind.NotFound, "board not found")
	}
	if err != nil {
		return nil, false, err
	}
	out, err = loadBoard(row.ID, row.ProjectID, row.Name, row.Description,
		row.CreatedAt, row.ArchivedAt, row.ArchivedBy, row.ArchiveReason)
	if err != nil {
		return nil, false, err
	}
	changed = out.Unarchive()
	if !changed {
		return out, false, change.Commit()
	}
	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return nil, false, err
	}
	if err := queries.ProjectUnarchiveBoard(ctx, id.String()); err != nil {
		return nil, false, err
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return nil, false, err
	}
	return out, true, change.Commit()
}

// countBoardIssues snapshots the effective status partition retained by the
// caller's transaction.
func countBoardIssues(ctx context.Context, queries *query.Queries, id board.ID) (board.IssueCounts, error) {
	total, err := queries.ProjectCountBoardIssues(ctx, id.String())
	if err != nil {
		return board.IssueCounts{}, err
	}
	counts := board.IssueCounts{Total: int(total)}
	for status, target := range map[string]*int{
		"ready": &counts.Ready, "blocked": &counts.Blocked,
		"in_progress": &counts.InProgress, "waiting": &counts.Waiting,
		"closed": &counts.Closed, "cancelled": &counts.Cancelled,
	} {
		value, err := queries.ProjectCountBoardIssuesByStatus(ctx, query.ProjectCountBoardIssuesByStatusParams{
			BoardID: id.String(), Status: status,
		})
		if err != nil {
			return board.IssueCounts{}, err
		}
		*target = int(value)
	}
	return counts, nil
}

func boardsEqual(left, right *board.State) bool {
	if left.ID() != right.ID() || left.ProjectID() != right.ProjectID() ||
		left.Name() != right.Name() || !left.Created().Equal(right.Created()) {
		return false
	}
	leftArchive, rightArchive := left.Archived(), right.Archived()
	if !archivesEqual(leftArchive, rightArchive) {
		return false
	}
	leftDescription := left.Description()
	rightDescription := right.Description()
	if leftDescription == nil || rightDescription == nil {
		return leftDescription == nil && rightDescription == nil
	}
	return *leftDescription == *rightDescription
}

func archivesEqual(left, right *board.Archive) bool {
	switch {
	case left == nil || right == nil:
		return left == nil && right == nil
	case left.Actor != right.Actor || !left.At.Equal(right.At):
		return false
	default:
		return optionalStringsEqual(left.Reason, right.Reason)
	}
}

func optionalStringsEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
