// Package store owns the SQLite database lifetime, schema migrations, and
// explicit snapshot and change scopes used by repository implementations.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // Register the SQLite database/sql driver.
)

const readerConnectionLimit = 4

var storeOpenGates = openGates{
	byPath: make(map[string]*openGate),
}

// Config defines the physical SQLite database.
type Config struct {
	// Path is the local SQLite database path.
	Path string
}

// Store owns one writer connection and a bounded pool of read-only
// connections for one physical SQLite database.
type Store struct {
	// db is the process-lifetime immediate writer connection.
	db *sql.DB

	// readDB supplies deferred query-only connections for View snapshots.
	readDB *sql.DB

	// databaseSchemaVersion is the applied version observed through Goose before
	// the Store was published.
	databaseSchemaVersion int64

	// closeOnce serializes and remembers process-lifetime cleanup.
	closeOnce sync.Once

	// closeErr is the joined reader and writer close result.
	closeErr error
}

// Open migrates and verifies one local SQLite database before publishing its
// Store lifetime to the caller.
func Open(ctx context.Context, cfg Config) (*Store, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		return nil, errors.New("store path is required")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve store path %q: %w", path, err)
	}
	releaseOpenGate, err := storeOpenGates.acquire(ctx, absolutePath)
	if err != nil {
		return nil, err
	}
	defer releaseOpenGate()

	writer, err := openDatabase(ctx, sqliteDSN(absolutePath, false), 1)
	if err != nil {
		return nil, err
	}
	// WAL setup, migration, and verification run before reader connections can
	// observe the database or a partially initialized Store can escape.
	if err := configureWAL(ctx, writer); err != nil {
		return nil, closeAfterOpenFailure(err, writer)
	}
	databaseSchemaVersion, err := migrateStore(ctx, writer)
	if err != nil {
		return nil, closeAfterOpenFailure(err, writer)
	}
	if err := verifyStore(ctx, writer, databaseSchemaVersion); err != nil {
		return nil, closeAfterOpenFailure(err, writer)
	}

	readers, err := openDatabase(
		ctx,
		sqliteDSN(absolutePath, true),
		readerConnectionLimit,
	)
	if err != nil {
		return nil, closeAfterOpenFailure(err, writer)
	}
	return &Store{
		db:                    writer,
		readDB:                readers,
		databaseSchemaVersion: databaseSchemaVersion,
	}, nil
}

// openGates serializes initialization of each physical database without
// coupling unrelated store paths.
type openGates struct {
	// mu protects byPath and each openGate user count.
	mu sync.Mutex

	// byPath retains gates while an Open call owns or waits for each path.
	byPath map[string]*openGate
}

// openGate is one context-aware initialization permit for a database path.
type openGate struct {
	// token carries the permit while no Open call owns it.
	token chan struct{}

	// users counts the current owner and waiters so idle gates can be removed.
	users int
}

func (g *openGates) acquire(
	ctx context.Context,
	path string,
) (release func(), _ error) {
	g.mu.Lock()
	gate := g.byPath[path]
	if gate == nil {
		gate = &openGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		g.byPath[path] = gate
	}
	gate.users++
	g.mu.Unlock()

	select {
	case <-ctx.Done():
		g.removeUser(path, gate)
		return nil, fmt.Errorf("wait to initialize store %q: %w", path, ctx.Err())
	case <-gate.token:
		return func() {
			gate.token <- struct{}{}
			g.removeUser(path, gate)
		}, nil
	}
}

func (g *openGates) removeUser(path string, gate *openGate) {
	g.mu.Lock()
	defer g.mu.Unlock()
	gate.users--
	if gate.users == 0 {
		delete(g.byPath, path)
	}
}

// Close releases every connection owned by Store.
//
// Close is idempotent and returns the same joined close error on every call.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = errors.Join(s.readDB.Close(), s.db.Close())
	})
	return s.closeErr
}

func openDatabase(ctx context.Context, dsn string, connectionLimit int) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(connectionLimit)
	db.SetMaxIdleConns(connectionLimit)
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("connect to SQLite database: %w", err), db.Close())
	}
	return db, nil
}

// sqliteDSN applies connection-local locking, integrity, and durability policy.
func sqliteDSN(path string, queryOnly bool) string {
	parameters := url.Values{}
	// Persist time.Time values as Unix seconds and decode integer values from
	// declared timestamp columns back into time.Time.
	parameters.Set("_time_integer_format", "unix")
	parameters.Set("_inttotime", "1")
	// Contending local operations wait for the short expected writer interval;
	// every pooled connection must independently enforce foreign keys and durable
	// WAL synchronization.
	parameters.Add("_pragma", "busy_timeout(5000)")
	parameters.Add("_pragma", "foreign_keys(1)")
	parameters.Add("_pragma", "synchronous(FULL)")
	if queryOnly {
		// Views cannot mutate and defer lock acquisition so concurrent snapshots do
		// not reserve the database's single writer.
		parameters.Add("_pragma", "query_only(1)")
		parameters.Set("_txlock", "deferred")
	} else {
		// Changes reserve the writer at BeginTx instead of failing later while
		// upgrading a transaction that has already read mutable state.
		parameters.Set("_txlock", "immediate")
	}
	return (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: parameters.Encode(),
	}).String()
}

// configureWAL selects the database-wide journal mode before readers are opened.
// WAL permits read snapshots to coexist with the process's single writer.
func configureWAL(ctx context.Context, db *sql.DB) error {
	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&journalMode); err != nil {
		return fmt.Errorf("enable WAL journal mode: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("enable WAL journal mode: SQLite selected %q", journalMode)
	}
	return nil
}

func closeAfterOpenFailure(cause error, databases ...*sql.DB) error {
	for _, db := range databases {
		cause = errors.Join(cause, db.Close())
	}
	return cause
}
