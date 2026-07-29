package board

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ListLabels reads distinct board labels in lexical order.
func (r *Repository) ListLabels(ctx context.Context) (out []string, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	rows, err := query.New(view).BoardListCompletionLabels(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	return append([]string{}, rows...), nil
}

// ListIssueIDs reads every issue ID in canonical default list order.
func (r *Repository) ListIssueIDs(ctx context.Context) (out []string, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	rows, err := query.New(view).BoardListCompletionIssueIDs(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	return append([]string{}, rows...), nil
}

// ListActors reads distinct actors retained by current board product records in
// lexical order.
func (r *Repository) ListActors(ctx context.Context) (out []string, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	rows, err := query.New(view).BoardListCompletionActors(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	actors := make([]string, 0, len(rows))
	for _, actor := range rows {
		if actor != nil {
			actors = append(actors, *actor)
		}
	}
	return actors, nil
}
