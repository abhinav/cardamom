package board

import (
	"context"
	"database/sql"
	"errors"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ResolveExternalKey returns the issue identified by one exact producer key.
func (r *Repository) ResolveExternalKey(
	ctx context.Context,
	value string,
) (out issue.ID, err error) {
	key, err := planning.NewExternalKey(value)
	if err != nil {
		return "", err
	}
	view, err := r.store.View(ctx)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	return r.resolveExternalKey(ctx, view, key)
}

func (r *Repository) resolveExternalKey(
	ctx context.Context,
	scope queryScope,
	key planning.ExternalKey,
) (issue.ID, error) {
	id, err := query.New(scope).BoardGetIssueIDByExternalKey(
		ctx,
		query.BoardGetIssueIDByExternalKeyParams{
			BoardID: r.boardID.String(), ExternalKey: key.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errkind.Errorf(
			errkind.NotFound,
			"issue not found: external key %q",
			key,
		)
	}
	if err != nil {
		return "", err
	}
	return issue.ID(id), nil
}

func (r *Repository) readExternalKeyOwner(
	ctx context.Context,
	scope queryScope,
	key *planning.ExternalKey,
) (*planning.ExternalKeyOwner, error) {
	if key == nil {
		return nil, nil
	}
	id, err := query.New(scope).BoardGetIssueIDByExternalKey(
		ctx,
		query.BoardGetIssueIDByExternalKeyParams{
			BoardID: r.boardID.String(), ExternalKey: key.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &planning.ExternalKeyOwner{Key: *key, IssueID: issue.ID(id)}, nil
}

func (r *Repository) readExternalKeys(
	ctx context.Context,
	scope queryScope,
) (map[planning.ExternalKey]issue.ID, error) {
	rows, err := query.New(scope).BoardListApplyExternalKeys(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	values := make(map[planning.ExternalKey]issue.ID)
	for _, row := range rows {
		values[planning.ExternalKey(row.ExternalKey)] = issue.ID(row.IssueID)
	}
	return values, nil
}

func (r *Repository) insertExternalKey(
	ctx context.Context,
	mutation *mutation,
	id issue.ID,
	key planning.ExternalKey,
) error {
	return query.New(mutation.change).BoardInsertIssueExternalKey(
		ctx,
		query.BoardInsertIssueExternalKeyParams{
			BoardID:     r.boardID.String(),
			ExternalKey: key.String(),
			IssueID:     id.String(),
		},
	)
}
