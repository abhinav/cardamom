package store

import (
	"context"
	"database/sql"
	"sync"
)

// View is one read-only snapshot transaction.
//
// A View retains the snapshot established by its first query until Done.
// Callers must defer Done immediately after a successful Store.View call.
type View struct {
	// tx owns the read-only database transaction.
	tx readTransaction // required

	// databaseSchemaVersion is the version observed before the Store was
	// published.
	databaseSchemaVersion int64

	// completion serializes and remembers the first Done call.
	completion scopeCompletion
}

// Change is one read-write transaction.
//
// A Change publishes writes only through Commit.
// Callers must defer Done immediately after a successful Store.Change call so
// every path that does not commit rolls back.
type Change struct {
	// tx owns the read-write database transaction.
	tx changeTransaction // required

	// completion serializes Commit and Done.
	completion scopeCompletion
}

// scopeCompletion records terminal scope cleanup without exposing transaction
// state to callers.
type scopeCompletion struct {
	// mu serializes terminal operations and protects the remembered result.
	mu sync.Mutex

	// done records that Commit or Done completed the transaction.
	done bool

	// cleanupErr preserves a rollback failure for repeated Done calls.
	cleanupErr error
}

// readTransaction is the database behavior required by a View.
type readTransaction interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	Rollback() error
}

// changeTransaction is the database behavior required by a Change.
type changeTransaction interface {
	readTransaction
	Commit() error
}

// View starts one deferred read-only transaction.
func (s *Store) View(ctx context.Context) (*View, error) {
	tx, err := s.readDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return &View{
		tx:                    tx,
		databaseSchemaVersion: s.databaseSchemaVersion,
	}, nil
}

// Change starts one immediate read-write transaction.
func (s *Store) Change(ctx context.Context) (*Change, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Change{tx: tx}, nil
}

// QueryContext executes a query in the View's retained snapshot.
func (v *View) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return v.tx.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a single-row query in the View's retained snapshot.
func (v *View) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return v.tx.QueryRowContext(ctx, query, args...)
}

// ExecContext submits a statement to the View's query-only transaction.
//
// SQLite rejects statements that would mutate the database.
func (v *View) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return v.tx.ExecContext(ctx, query, args...)
}

// PrepareContext prepares a statement in the View's retained snapshot.
func (v *View) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return v.tx.PrepareContext(ctx, query)
}

// Done releases the View's snapshot.
//
// Done is idempotent and returns the first rollback error on every call.
func (v *View) Done() error {
	v.completion.mu.Lock()
	defer v.completion.mu.Unlock()
	if v.completion.done {
		return v.completion.cleanupErr
	}
	v.completion.done = true
	v.completion.cleanupErr = v.tx.Rollback()
	return v.completion.cleanupErr
}

// ExecContext executes a statement in the Change transaction.
func (c *Change) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.tx.ExecContext(ctx, query, args...)
}

// PrepareContext prepares a statement in the Change transaction.
func (c *Change) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return c.tx.PrepareContext(ctx, query)
}

// QueryContext executes a query in the Change transaction.
func (c *Change) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.tx.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a single-row query in the Change transaction.
func (c *Change) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.tx.QueryRowContext(ctx, query, args...)
}

// Commit atomically publishes every write made through Change.
//
// Commit completes the Change even when the database reports a commit error.
// A second Commit or operations attempted afterward report sql.ErrTxDone.
func (c *Change) Commit() error {
	c.completion.mu.Lock()
	defer c.completion.mu.Unlock()
	if c.completion.done {
		return sql.ErrTxDone
	}
	c.completion.done = true
	return c.tx.Commit()
}

// Done rolls back an uncommitted Change.
//
// Done is idempotent, becomes a no-op after Commit, and returns the first
// rollback error on every call.
func (c *Change) Done() error {
	c.completion.mu.Lock()
	defer c.completion.mu.Unlock()
	if c.completion.done {
		return c.completion.cleanupErr
	}
	c.completion.done = true
	c.completion.cleanupErr = c.tx.Rollback()
	return c.completion.cleanupErr
}
