package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ValidateRestoreProjects checks archived project identities without mutation.
func (r *Repository) ValidateRestoreProjects(
	ctx context.Context,
	snapshots []project.Snapshot,
) (err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return fmt.Errorf("begin project restore validation: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	_, err = inspectRestoreProjects(ctx, query.New(view), snapshots)
	return err
}

// RestoreProjects atomically creates missing archived projects after rechecking
// every retained identity for compatible metadata.
func (r *Repository) RestoreProjects(
	ctx context.Context,
	snapshots []project.Snapshot,
) (err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return fmt.Errorf("begin project restore: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	queries := query.New(change)

	missing, err := inspectRestoreProjects(ctx, queries, snapshots)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return change.Commit()
	}
	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return err
	}
	for _, snapshot := range missing {
		if err := queries.ProjectInsertRestoredProject(
			ctx,
			query.ProjectInsertRestoredProjectParams{
				ID: snapshot.ID.String(), Name: snapshot.Name,
				CreatedAt: snapshot.Created,
			},
		); err != nil {
			return fmt.Errorf("create restored project %q: %w", snapshot.ID, err)
		}
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return err
	}
	return change.Commit()
}

func inspectRestoreProjects(
	ctx context.Context,
	queries *query.Queries,
	snapshots []project.Snapshot,
) ([]project.Snapshot, error) {
	missing := make([]project.Snapshot, 0, len(snapshots))
	seen := make(map[project.ID]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		state, err := project.Load(snapshot)
		if err != nil {
			return nil, fmt.Errorf("validate archived project: %w", err)
		}
		if _, exists := seen[state.ID()]; exists {
			return nil, errkind.Errorf(
				errkind.InvalidInput,
				"archived project %q is duplicated",
				state.ID(),
			)
		}
		seen[state.ID()] = struct{}{}
		canonical := project.Snapshot{
			ID: state.ID(), Name: state.Name(), Created: state.Created(),
		}

		row, err := queries.ProjectGetRestoreProject(ctx, state.ID().String())
		if errors.Is(err, sql.ErrNoRows) {
			missing = append(missing, canonical)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read retained project %q: %w", state.ID(), err)
		}
		if row.Name != state.Name() || !row.CreatedAt.Equal(state.Created()) {
			return nil, errkind.Errorf(
				errkind.Conflict,
				"project %q conflicts with archived metadata",
				state.ID(),
			)
		}
	}
	return missing, nil
}
