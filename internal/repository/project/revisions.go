package project

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/repository/store"
)

// commitRevision reserves one canonical revision, applies all projections,
// publishes the revision, and commits them as one transaction.
func (r *Repository) commitRevision(
	ctx context.Context,
	write func(*store.Change) error,
) (err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return err
	}
	if err := write(change); err != nil {
		return err
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return err
	}
	return change.Commit()
}
