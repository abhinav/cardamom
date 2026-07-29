package board

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ReadChangeCursor reads the selected board's current scalar revision.
func (r *Repository) ReadChangeCursor(ctx context.Context) (out issue.ChangeCursor, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	err = r.readChangeCursor(ctx, view, &out)
	return out, err
}

func (r *Repository) readChangeCursor(
	ctx context.Context,
	scope queryScope,
	out *issue.ChangeCursor,
) error {
	revision, err := query.New(scope).BoardReadChangeCursor(ctx, r.boardID.String())
	out.Revision = revision
	return err
}
