package board

import (
	"context"
	"database/sql"
	"errors"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

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
