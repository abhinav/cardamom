package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestMigrationProviderAppliesMixedRegistryInVersionOrder(t *testing.T) {
	db := openMigrationTestDatabase(t)
	files := fstest.MapFS{
		"20260718120001_first.sql": {
			Data: gooseSQL(`
				CREATE TABLE migration_order (
					position INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL
				);
				INSERT INTO migration_order (name) VALUES ('sql-first');
			`),
		},
		"20260718120003_third.sql": {
			Data: gooseSQL(`
				INSERT INTO migration_order (name) VALUES ('sql-third');
			`),
		},
	}
	goMigration := goose.NewGoMigration(
		20260718120002,
		&goose.GoFunc{
			RunTx: func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(
					ctx,
					`INSERT INTO migration_order (name) VALUES ('go-second')`,
				)
				return err
			},
		},
		nil,
	)

	provider, err := newMigrationProvider(db, files, goMigration)
	require.NoError(t, err)
	assert.Equal(t, int64(20260718120003), migrationSchemaVersion(provider))
	_, err = provider.Up(t.Context())
	require.NoError(t, err)

	rows, err := db.Query(`SELECT name FROM migration_order ORDER BY position`)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()
	var got []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got = append(got, name)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"sql-first", "go-second", "sql-third"}, got)
}

func TestNewMigrationProviderRejectsInvalidRegistry(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		_, err := newMigrationProvider(openMigrationTestDatabase(t), nil)
		assert.ErrorIs(t, err, goose.ErrNoMigrations)
	})

	t.Run("MalformedSQLFilename", func(t *testing.T) {
		files := fstest.MapFS{
			"20260718_bad.sql": {Data: gooseSQL(`SELECT 1;`)},
		}
		_, err := newMigrationProvider(openMigrationTestDatabase(t), files)
		assert.ErrorContains(t, err, `malformed migration filename "20260718_bad.sql"`)
	})

	t.Run("InvalidGoVersion", func(t *testing.T) {
		_, err := newMigrationProvider(
			openMigrationTestDatabase(t),
			nil,
			goose.NewGoMigration(0, nil, nil),
		)
		assert.ErrorContains(t, err, "invalid go migration")
	})

	t.Run("DuplicateSQLVersion", func(t *testing.T) {
		files := fstest.MapFS{
			"20260718120001_first.sql":  {Data: gooseSQL(`SELECT 1;`)},
			"20260718120001_second.sql": {Data: gooseSQL(`SELECT 1;`)},
		}
		_, err := newMigrationProvider(openMigrationTestDatabase(t), files)
		assert.ErrorContains(t, err, "duplicate migration version 20260718120001")
	})

	t.Run("DuplicateGoVersion", func(t *testing.T) {
		first := goose.NewGoMigration(20260718120001, nil, nil)
		second := goose.NewGoMigration(20260718120001, nil, nil)
		_, err := newMigrationProvider(
			openMigrationTestDatabase(t),
			nil,
			first,
			second,
		)
		assert.ErrorContains(t, err, "version 20260718120001 already registered")
	})

	t.Run("DuplicateMixedVersion", func(t *testing.T) {
		files := fstest.MapFS{
			"20260718120001_schema.sql": {Data: gooseSQL(`SELECT 1;`)},
		}
		_, err := newMigrationProvider(
			openMigrationTestDatabase(t),
			files,
			goose.NewGoMigration(20260718120001, nil, nil),
		)
		assert.ErrorContains(t, err, "duplicate migration version 20260718120001")
	})
}

func TestMigrationProviderIgnoresGlobalGoMigrations(t *testing.T) {
	goose.ResetGlobalMigrations()
	t.Cleanup(goose.ResetGlobalMigrations)
	require.NoError(t, goose.SetGlobalMigrations(
		goose.NewGoMigration(20260718120001, nil, nil),
	))

	provider, err := newMigrationProvider(
		openMigrationTestDatabase(t),
		nil,
		goose.NewGoMigration(20260718120002, nil, nil),
	)
	require.NoError(t, err)

	sources := provider.ListSources()
	require.Len(t, sources, 1)
	assert.Equal(t, int64(20260718120002), sources[0].Version)
}

func TestMigrationProviderRollsBackFailingSQLMigration(t *testing.T) {
	db := openMigrationTestDatabase(t)
	files := fstest.MapFS{
		"20260718120001_failing.sql": {
			Data: gooseSQL(`
				CREATE TABLE migration_parent (id INTEGER PRIMARY KEY);
				CREATE TABLE migration_effect (
					parent_id INTEGER NOT NULL
						REFERENCES migration_parent (id)
				);
				INSERT INTO migration_effect (parent_id) VALUES (1);
			`),
		},
	}
	provider, err := newMigrationProvider(db, files)
	require.NoError(t, err)

	_, err = provider.Up(t.Context())
	assert.ErrorContains(t, err, "FOREIGN KEY constraint failed")
	assert.False(t, tableExists(t, db, "migration_effect"))
	assert.False(t, tableExists(t, db, "migration_parent"))
	version, err := provider.GetDBVersion(t.Context())
	require.NoError(t, err)
	assert.Zero(t, version)
}

func TestMigrationProviderRollsBackFailingGoMigration(t *testing.T) {
	db := openMigrationTestDatabase(t)
	files := fstest.MapFS{
		"20260718120001_schema.sql": {
			Data: gooseSQL(`
				CREATE TABLE migration_effect (value TEXT NOT NULL);
			`),
		},
	}
	goMigration := goose.NewGoMigration(
		20260718120002,
		&goose.GoFunc{
			RunTx: func(ctx context.Context, tx *sql.Tx) error {
				if _, err := tx.ExecContext(
					ctx,
					`INSERT INTO migration_effect (value) VALUES ('rolled back')`,
				); err != nil {
					return err
				}
				return errors.New("stop migration")
			},
		},
		nil,
	)
	provider, err := newMigrationProvider(db, files, goMigration)
	require.NoError(t, err)

	_, err = provider.Up(t.Context())
	assert.ErrorContains(t, err, "stop migration")
	var effects int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM migration_effect`).Scan(&effects))
	assert.Zero(t, effects)
	version, err := provider.GetDBVersion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(20260718120001), version)
}

func TestMigrationProviderRecordsNoOpBaseline(t *testing.T) {
	db := openMigrationTestDatabase(t)
	provider, err := newMigrationProvider(
		db,
		nil,
		goose.NewGoMigration(20260718120001, nil, nil),
	)
	require.NoError(t, err)

	results, err := provider.Up(t.Context())
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Empty)
	version, err := provider.GetDBVersion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(20260718120001), version)

	var applicationTables int
	require.NoError(t, db.QueryRow(`
		SELECT count(*)
		FROM sqlite_schema
		WHERE type = 'table'
			AND name NOT LIKE 'sqlite_%'
			AND name <> 'goose_db_version'
	`).Scan(&applicationTables))
	assert.Zero(t, applicationTables)
}

func TestStoreMigrationProviderUsesVersionIdentity(t *testing.T) {
	db := openMigrationTestDatabase(t)
	legacyProvider, err := newMigrationProvider(
		db,
		nil,
		goose.NewGoMigration(20260723150000, nil, nil),
		goose.NewGoMigration(20260726143816, nil, nil),
		goose.NewGoMigration(20260726181403, nil, nil),
	)
	require.NoError(t, err)
	_, err = legacyProvider.Up(t.Context())
	require.NoError(t, err)
	// The legacy Go migration marks the baseline version without creating its
	// schema. Supply the tables required by later migrations while preserving
	// the test's cross-source version-identity boundary.
	_, err = db.ExecContext(
		t.Context(),
		`CREATE TABLE projects (id TEXT PRIMARY KEY);
		CREATE TABLE boards (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			name TEXT
		);
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			board_id TEXT NOT NULL,
			UNIQUE (board_id, id)
		)`,
	)
	require.NoError(t, err)

	provider, err := newStoreMigrationProvider(db)
	require.NoError(t, err)
	results, err := provider.Up(t.Context())
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, int64(20260729090000), results[0].Source.Version)
	assert.Equal(t, int64(20260811090000), results[1].Source.Version)
	assert.Equal(t, int64(20260812120000), results[2].Source.Version)

	var appliedVersions int
	require.NoError(t, db.QueryRow(`
		SELECT count(*)
		FROM goose_db_version
		WHERE is_applied
			AND version_id IN (
				20260723150000,
				20260726143816,
				20260726181403,
				20260729090000,
				20260811090000,
				20260812120000
			)
	`).Scan(&appliedVersions))
	assert.Equal(t, 6, appliedVersions)
}

func TestBoardCopyMigrationPreservesBaselineStore(t *testing.T) {
	db := openMigrationTestDatabase(t)
	baseline, err := migrationFiles.ReadFile(
		"migrations/20260726181403_baseline.sql",
	)
	require.NoError(t, err)
	legacy, err := newMigrationProvider(db, fstest.MapFS{
		"20260726181403_baseline.sql": {Data: baseline},
	})
	require.NoError(t, err)
	_, err = legacy.Up(t.Context())
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO projects (id, name, created_at)
VALUES ('project-existing', 'Existing project', 1000);
INSERT INTO boards (id, project_id, name, created_at)
VALUES ('board-existing', 'project-existing', 'Existing board', 1000)`)
	require.NoError(t, err)

	current, err := newStoreMigrationProvider(db)
	require.NoError(t, err)
	results, err := current.Up(t.Context())
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, int64(20260729090000), results[0].Source.Version)
	assert.Equal(t, int64(20260811090000), results[1].Source.Version)
	assert.Equal(t, int64(20260812120000), results[2].Source.Version)

	var projectName, boardName, lineage string
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT project.name, board.name, lineage.id
FROM projects AS project
JOIN boards AS board ON board.project_id = project.id
CROSS JOIN store_lineage AS lineage
WHERE project.id = 'project-existing'
    AND lineage.singleton = 1`,
	).Scan(&projectName, &boardName, &lineage))
	assert.Equal(t, "Existing project", projectName)
	assert.Equal(t, "Existing board", boardName)
	assert.Regexp(t, `^store_[0-9a-f]{32}$`, lineage)
}

func gooseSQL(statements string) []byte {
	return []byte("-- +goose Up\n" + statements)
}

func openMigrationTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open(
		"sqlite",
		filepath.Join(t.TempDir(), "migration.db")+
			"?_pragma=foreign_keys(1)&_txlock=immediate",
	)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(t.Context()))
	t.Cleanup(func() { assert.NoError(t, db.Close()) })
	return db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = ?
		)
	`, name).Scan(&exists))
	return exists
}
