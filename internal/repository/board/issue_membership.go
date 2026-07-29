package board

import (
	"context"
	"database/sql"
	"errors"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/board/selection"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// Locator resolves store-global issue identities to their owning boards.
type Locator struct {
	store *store.Store
}

// NewLocator binds issue membership reads to one physical store.
func NewLocator(persistence *store.Store) *Locator {
	must.NotBeNilf(persistence, "board membership Store is required")
	return &Locator{store: persistence}
}

// BoardForIssue returns the board that owns one store-global issue identity.
func (l *Locator) BoardForIssue(ctx context.Context, issueID string) (out board.ID, err error) {
	view, err := l.store.View(ctx)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	value, err := query.New(view).BoardGetIssueBoardID(ctx, issueID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", selection.ErrIssueNotFound
	} else if err != nil {
		return "", err
	}
	return board.ID(value), nil
}
