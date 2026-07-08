package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Change(t *testing.T) {
	store := openTransactionTestStore(t, filepath.Join(t.TempDir(), "board.db"))

	t.Run("DoneRollsBack", func(t *testing.T) {
		change, err := store.Change(t.Context())
		require.NoError(t, err)
		_, err = change.ExecContext(t.Context(), `UPDATE store_state SET next_issue_number = 2`)
		require.NoError(t, err)
		require.NoError(t, change.Done())

		assert.Equal(t, int64(1), readNextIssueNumber(t, store))
	})

	t.Run("CommitPublishes", func(t *testing.T) {
		change, err := store.Change(t.Context())
		require.NoError(t, err)
		defer func() { assert.NoError(t, change.Done()) }()

		_, err = change.ExecContext(t.Context(), `UPDATE store_state SET next_issue_number = 3`)
		require.NoError(t, err)
		require.NoError(t, change.Commit())

		assert.Equal(t, int64(3), readNextIssueNumber(t, store))
		_, err = change.ExecContext(t.Context(), `UPDATE store_state SET next_issue_number = 4`)
		assert.ErrorIs(t, err, sql.ErrTxDone)
		assert.ErrorIs(t, change.QueryRowContext(t.Context(), `SELECT 1`).Scan(new(int)), sql.ErrTxDone)
	})

	t.Run("DoneIsIdempotent", func(t *testing.T) {
		change, err := store.Change(t.Context())
		require.NoError(t, err)

		assert.NoError(t, change.Done())
		assert.NoError(t, change.Done())
	})

	t.Run("CompletedScopeRejectsOperations", func(t *testing.T) {
		change, err := store.Change(t.Context())
		require.NoError(t, err)
		require.NoError(t, change.Done())

		_, err = change.ExecContext(t.Context(), `UPDATE store_state SET next_issue_number = 4`)
		assert.ErrorIs(t, err, sql.ErrTxDone)
		assert.ErrorIs(t, change.QueryRowContext(t.Context(), `SELECT 1`).Scan(new(int)), sql.ErrTxDone)
		assert.ErrorIs(t, change.Commit(), sql.ErrTxDone)
	})
}

func TestChange_DoneReportsCleanupFailure(t *testing.T) {
	wantErr := errors.New("rollback failed")
	change := &Change{tx: &stubChangeTransaction{rollbackErr: wantErr}}

	assert.ErrorIs(t, change.Done(), wantErr)
	assert.ErrorIs(t, change.Done(), wantErr)
}

func TestView_DoneReportsCleanupFailure(t *testing.T) {
	wantErr := errors.New("rollback failed")
	view := &View{tx: &stubReadTransaction{rollbackErr: wantErr}}

	assert.ErrorIs(t, view.Done(), wantErr)
	assert.ErrorIs(t, view.Done(), wantErr)
}

func TestStore_View(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.db")
	reader := openTransactionTestStore(t, path)
	writer := openTransactionTestStore(t, path)

	t.Run("RetainsOneSnapshot", func(t *testing.T) {
		view, err := reader.View(t.Context())
		require.NoError(t, err)
		defer func() { assert.NoError(t, view.Done()) }()

		var first int64
		require.NoError(t, view.QueryRowContext(t.Context(), `SELECT next_issue_number FROM store_state`).Scan(&first))

		change, err := writer.Change(t.Context())
		require.NoError(t, err)
		defer func() { assert.NoError(t, change.Done()) }()
		_, err = change.ExecContext(t.Context(), `UPDATE store_state SET next_issue_number = 5`)
		require.NoError(t, err)
		require.NoError(t, change.Commit())

		var second int64
		require.NoError(t, view.QueryRowContext(t.Context(), `SELECT next_issue_number FROM store_state`).Scan(&second))
		assert.Equal(t, first, second)
		assert.Equal(t, int64(5), readNextIssueNumber(t, writer))
	})

	t.Run("QueryRowRejectsUpdateReturning", func(t *testing.T) {
		view, err := reader.View(t.Context())
		require.NoError(t, err)
		defer func() { assert.NoError(t, view.Done()) }()

		err = view.QueryRowContext(
			t.Context(),
			`UPDATE store_state SET next_issue_number = 6 RETURNING next_issue_number`,
		).Scan(new(int64))
		assert.Error(t, err)
	})

	t.Run("QueryRejectsUpdateReturning", func(t *testing.T) {
		view, err := reader.View(t.Context())
		require.NoError(t, err)
		defer func() { assert.NoError(t, view.Done()) }()

		rows, err := view.QueryContext(
			t.Context(),
			`UPDATE store_state SET next_issue_number = 6 RETURNING next_issue_number`,
		)
		if err != nil {
			return
		}
		defer func() { assert.NoError(t, rows.Close()) }()
		for rows.Next() {
		}
		assert.Error(t, rows.Err())
	})

	t.Run("ExecRejectsUpdate", func(t *testing.T) {
		before := readNextIssueNumber(t, reader)
		view, err := reader.View(t.Context())
		require.NoError(t, err)
		defer func() { assert.NoError(t, view.Done()) }()

		_, err = view.ExecContext(
			t.Context(),
			`UPDATE store_state SET next_issue_number = 6`,
		)
		assert.Error(t, err)
		require.NoError(t, view.Done())
		assert.Equal(t, before, readNextIssueNumber(t, reader))
	})

	t.Run("CompletedScopeRejectsQueries", func(t *testing.T) {
		view, err := reader.View(t.Context())
		require.NoError(t, err)
		require.NoError(t, view.Done())

		assert.ErrorIs(t, view.QueryRowContext(t.Context(), `SELECT 1`).Scan(new(int)), sql.ErrTxDone)
	})
}

func openTransactionTestStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	return store
}

func readNextIssueNumber(t *testing.T, store *Store) int64 {
	t.Helper()
	view, err := store.View(t.Context())
	require.NoError(t, err)

	var number int64
	require.NoError(t, view.QueryRowContext(t.Context(), `SELECT next_issue_number FROM store_state`).Scan(&number))
	require.NoError(t, view.Done())
	return number
}

type stubChangeTransaction struct {
	rollbackErr error
}

type stubReadTransaction struct {
	rollbackErr error
}

func (*stubReadTransaction) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	panic("unexpected call")
}

func (*stubReadTransaction) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	panic("unexpected call")
}

func (*stubReadTransaction) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("unexpected call")
}

func (*stubReadTransaction) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("unexpected call")
}

func (s *stubReadTransaction) Rollback() error {
	return s.rollbackErr
}

func (*stubChangeTransaction) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	panic("unexpected call")
}

func (*stubChangeTransaction) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	panic("unexpected call")
}

func (*stubChangeTransaction) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("unexpected call")
}

func (*stubChangeTransaction) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("unexpected call")
}

func (*stubChangeTransaction) Commit() error {
	panic("unexpected call")
}

func (s *stubChangeTransaction) Rollback() error {
	return s.rollbackErr
}
