package project

import (
	"context"
	"database/sql"
	"errors"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// EditProjectName returns the project State committed with a validated
// replacement name. A missing ID returns NotFound, and an invalid name returns
// InvalidInput without changing the stored project or canonical revision. A
// changed name and its canonical revision commit together; a normalized no-op
// returns current State without publishing a revision.
func (r *Repository) EditProjectName(
	ctx context.Context,
	request project.EditNameRequest,
) (out *project.State, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	// Reload under the write transaction so validation and mutation both use
	// the persisted State selected by stable ID.
	queries := query.New(change)
	row, err := queries.ProjectGetProject(ctx, request.ProjectID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "project not found")
	}
	if err != nil {
		return out, err
	}
	current, err := project.Load(project.Snapshot{
		ID: project.ID(row.ID), Name: row.Name, Created: row.CreatedAt,
	})
	if err != nil {
		return out, err
	}
	out, err = current.EditName(request.Name)
	if err != nil {
		return out, err
	}
	// Canonical revisions announce observable store changes. A normalized
	// no-op returns authoritative State without publishing one.
	if current.Name() == out.Name() {
		return out, change.Commit()
	}

	// The row update and revision publication remain inside this transaction,
	// so readers cannot observe either one without the other.
	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return out, err
	}
	if err := queries.ProjectUpdateProjectName(
		ctx,
		query.ProjectUpdateProjectNameParams{
			Name: out.Name(),
			ID:   out.ID().String(),
		},
	); err != nil {
		return out, err
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return out, err
	}
	return out, change.Commit()
}
