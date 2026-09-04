package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestOpenMigratesFreshDatabaseToCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()

	information, err := view.ReadInformation(t.Context())
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion(), information.DatabaseSchemaVersion)
	assert.Equal(t, SchemaVersion(), information.CodeSchemaVersion)
}

func TestOpenExistingRejectsEmptyDatabaseWithoutModification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	persistence, err := OpenExisting(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "not an existing Cardamom store")

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Empty(t, body)
	assert.NoFileExists(t, path+"-wal")
	assert.NoFileExists(t, path+"-shm")
}

func TestOpenExistingMigratesBaselineStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	db, err := sql.Open("sqlite", sqliteDSN(path, false))
	require.NoError(t, err)
	baseline, err := migrationFiles.ReadFile(
		"migrations/20260726181403_baseline.sql",
	)
	require.NoError(t, err)
	provider, err := newMigrationProvider(db, fstest.MapFS{
		"20260726181403_baseline.sql": {Data: baseline},
	})
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO projects(id, name, created_at)
VALUES ('project', 'Project', 1);
INSERT INTO boards(id, project_id, name, created_at)
VALUES ('board', 'project', 'Board', 1);
INSERT INTO issues(
    id, board_id, title, kind, lifecycle, priority,
    created_at, updated_at, summary, details
) VALUES (
    'an-issue', 'board', 'Backfilltoken title', 'task', 'open', 2,
    1, 1, 'Backfilltoken summary', 'Backfilltoken details'
);
INSERT INTO issue_states(issue_id, board_id, body, next_action)
VALUES ('an-issue', 'board', 'Backfilltoken state', 'Backfilltoken next');
INSERT INTO issue_results(issue_id, board_id, body)
VALUES ('an-issue', 'board', 'Backfilltoken result');
INSERT INTO issue_log_entries(
    id, board_id, issue_id, kind, author, committer, body, created_at
) VALUES (
    'cmt_11111111111111111111111111111111', 'board', 'an-issue',
    'post', 'author', 'author', 'Backfilltoken log', 1
)
`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := OpenExisting(t.Context(), Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	information, err := view.ReadInformation(t.Context())
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion(), information.DatabaseSchemaVersion)
	var documents, indexed int
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_documents WHERE issue_id = 'an-issue'
`).Scan(&documents))
	assert.Equal(t, 6, documents)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_fts WHERE body MATCH 'backfilltoken'
`).Scan(&indexed))
	assert.Equal(t, 6, indexed)
}

func TestOpenProvidesNativeUnixTimestamps(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "board.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	location := time.FixedZone("test offset", -7*60*60)
	createdAt := time.Date(2026, time.July, 24, 9, 10, 11, 987_654_321, location)
	closedAt := createdAt.Add(time.Hour)

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(
		t.Context(),
		`INSERT INTO projects (id, name, created_at) VALUES ('project', 'Project', ?)`,
		createdAt,
	)
	require.NoError(t, err)
	_, err = change.ExecContext(
		t.Context(),
		`INSERT INTO boards (id, project_id, name, created_at)
		VALUES ('board', 'project', 'Board', ?)`,
		createdAt,
	)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO issues (
			id, board_id, title, kind, lifecycle, priority,
			created_at, updated_at, closed_at
		) VALUES
			('open', 'board', 'Open', 'task', 'open', 2, ?, ?, NULL),
			('closed', 'board', 'Closed', 'task', 'closed', 2, ?, ?, ?)
	`, createdAt, createdAt, createdAt, closedAt, closedAt)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()

	var (
		gotCreatedAt  time.Time
		gotClosedAt   *time.Time
		createdAtType string
		closedAtType  string
	)
	err = view.QueryRowContext(t.Context(), `
		SELECT created_at, closed_at, typeof(created_at), typeof(closed_at)
		FROM issues
		WHERE id = 'open'
	`).Scan(&gotCreatedAt, &gotClosedAt, &createdAtType, &closedAtType)
	require.NoError(t, err)
	assert.Equal(t, time.Unix(createdAt.Unix(), 0).UTC(), gotCreatedAt)
	assert.Same(t, time.UTC, gotCreatedAt.Location())
	assert.Nil(t, gotClosedAt)
	assert.Equal(t, "integer", createdAtType)
	assert.Equal(t, "null", closedAtType)

	err = view.QueryRowContext(t.Context(), `
		SELECT closed_at, typeof(closed_at)
		FROM issues
		WHERE id = 'closed'
	`).Scan(&gotClosedAt, &closedAtType)
	require.NoError(t, err)
	require.NotNil(t, gotClosedAt)
	assert.Equal(t, time.Unix(closedAt.Unix(), 0).UTC(), *gotClosedAt)
	assert.Same(t, time.UTC, gotClosedAt.Location())
	assert.Equal(t, "integer", closedAtType)
}

func TestOpenDeclaresAllPersistedInstantsAsTimestamps(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "board.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()

	tests := []struct {
		name        string
		table       string
		wantColumns []string
	}{
		{name: "Projects", table: "projects", wantColumns: []string{"created_at"}},
		{name: "Boards", table: "boards", wantColumns: []string{"created_at", "archived_at"}},
		{name: "Issues", table: "issues", wantColumns: []string{
			"created_at", "updated_at", "closed_at", "waiting_since",
		}},
		{name: "ActiveClaims", table: "active_claims", wantColumns: []string{"started_at"}},
		{name: "IssueLogEntries", table: "issue_log_entries", wantColumns: []string{"created_at"}},
		{name: "IssueStates", table: "issue_states", wantColumns: []string{"updated_at"}},
		{name: "CheckpointDecisions", table: "checkpoint_decisions", wantColumns: []string{"decided_at"}},
		{name: "Attachments", table: "attachments", wantColumns: []string{"created_at", "removed_at"}},
		{name: "AttachmentUploads", table: "attachment_uploads", wantColumns: []string{"expires_at"}},
		{name: "Mailbox", table: "mailbox", wantColumns: []string{"created_at", "expires_at", "read_at"}},
		{name: "Subscriptions", table: "subscriptions", wantColumns: []string{"created_at", "expires_at"}},
		{name: "Leases", table: "leases", wantColumns: []string{"acquired_at", "expires_at"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := view.QueryContext(
				t.Context(),
				`SELECT name, type FROM pragma_table_info(?) ORDER BY cid`,
				tt.table,
			)
			require.NoError(t, err)

			var gotColumns []string
			for rows.Next() {
				var name, declaredType string
				require.NoError(t, rows.Scan(&name, &declaredType))
				if !strings.HasSuffix(name, "_at") && name != "waiting_since" {
					continue
				}
				gotColumns = append(gotColumns, name)
				assert.Equal(t, "TIMESTAMP", declaredType, name)
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
			assert.Equal(t, tt.wantColumns, gotColumns)
		})
	}
}

func TestOpenCurrentDatabasePerformsNoMigrationWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	first, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, first.Close())

	observer, err := sql.Open("sqlite", sqliteDSN(path, true))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, observer.Close()) })
	var before int64
	require.NoError(t, observer.QueryRow(`PRAGMA data_version`).Scan(&before))

	second, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, second.Close())

	var after int64
	require.NoError(t, observer.QueryRow(`PRAGMA data_version`).Scan(&after))
	assert.Equal(t, before, after)
}

func TestOpenConcurrentFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			persistence, err := Open(t.Context(), Config{Path: path})
			if err == nil {
				err = persistence.Close()
			}
			results <- err
		}()
	}

	close(start)
	assert.NoError(t, <-results)
	assert.NoError(t, <-results)
}

func TestOpenConfiguresWriterAndReaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	assertConnectionPolicy(t, change, false)

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	assertConnectionPolicy(t, view, true)

	err = view.QueryRowContext(
		t.Context(),
		`UPDATE store_state SET next_issue_number = 2 RETURNING next_issue_number`,
	).Scan(new(int64))
	assert.Error(t, err)
}

func TestChangeAcquiresWriterBeforeFirstStatement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	first, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, first.Close()) })
	second, err := sql.Open(
		"sqlite",
		path+"?_pragma=busy_timeout(1)&_txlock=immediate",
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, second.Close()) })

	change, err := first.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()

	blocked, err := second.BeginTx(t.Context(), nil)
	assert.Error(t, err)
	assert.Nil(t, blocked)
}

func TestOpenRejectsDatabaseNewerThanBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	db, err := sql.Open("sqlite", sqliteDSN(path, false))
	require.NoError(t, err)
	provider, err := newMigrationProvider(
		db,
		nil,
		goose.NewGoMigration(SchemaVersion()+1, nil, nil),
	)
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(
		t,
		err,
		fmt.Sprintf(
			"verify schema version: database has %d; binary has %d",
			SchemaVersion()+1,
			SchemaVersion(),
		),
	)

	db, err = sql.Open("sqlite", sqliteDSN(path, false))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, db.Close()) })
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, tx.Rollback())
}

func TestCloseIsIdempotent(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "board.sqlite3"),
	})
	require.NoError(t, err)

	assert.NoError(t, persistence.Close())
	assert.NoError(t, persistence.Close())
}

func assertConnectionPolicy(t *testing.T, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, wantQueryOnly bool,
) {
	t.Helper()
	for _, check := range []struct {
		pragma string
		want   any
	}{
		{pragma: "journal_mode", want: "wal"},
		{pragma: "foreign_keys", want: 1},
		{pragma: "busy_timeout", want: 5000},
		{pragma: "synchronous", want: 2},
		{pragma: "query_only", want: boolInt(wantQueryOnly)},
	} {
		t.Run(check.pragma, func(t *testing.T) {
			switch want := check.want.(type) {
			case string:
				var got string
				require.NoError(t, query.QueryRowContext(t.Context(), "PRAGMA "+check.pragma).Scan(&got))
				assert.Equal(t, want, got)
			case int:
				var got int
				require.NoError(t, query.QueryRowContext(t.Context(), "PRAGMA "+check.pragma).Scan(&got))
				assert.Equal(t, want, got)
			}
		})
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
