package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"

	"github.com/pressly/goose/v3"
	"go.abhg.dev/cardamom/internal/must"
)

var migrationFilenamePattern = regexp.MustCompile(`^[0-9]{14}_[a-z0-9_]+[.]sql$`)

// migrationFiles is the complete forward-only schema sequence shipped with
// Cardamom.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

var schemaVersion = mustMigrationSchemaVersion()

// SchemaVersion reports the latest SQL or Go migration understood by Cardamom.
func SchemaVersion() int64 {
	return schemaVersion
}

// newMigrationProvider assembles Cardamom's mixed SQL and Go registry.
func newMigrationProvider(
	db *sql.DB,
	sqlMigrations fs.FS,
	goMigrations ...*goose.Migration,
) (*goose.Provider, error) {
	if err := validateSQLMigrationFiles(sqlMigrations); err != nil {
		return nil, err
	}
	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		db,
		sqlMigrations,
		goose.WithGoMigrations(goMigrations...),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return nil, fmt.Errorf("build migration registry: %w", err)
	}
	return provider, nil
}

func validateSQLMigrationFiles(files fs.FS) error {
	if files == nil {
		return nil
	}
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !migrationFilenamePattern.MatchString(entry.Name()) {
			return fmt.Errorf("malformed migration filename %q", entry.Name())
		}
	}
	return nil
}

func migrationSchemaVersion(provider *goose.Provider) int64 {
	sources := provider.ListSources()
	return sources[len(sources)-1].Version
}

// migrateStore applies every pending registered migration and reports the
// resulting database schema version.
//
// The database version is observed through Provider so the rest of the store
// package does not depend on Goose's ledger representation.
func migrateStore(ctx context.Context, db *sql.DB) (int64, error) {
	provider, err := newStoreMigrationProvider(db)
	if err != nil {
		return 0, err
	}
	databaseVersion, err := provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read database schema version: %w", err)
	}
	if databaseVersion > SchemaVersion() {
		return 0, fmt.Errorf(
			"verify schema version: database has %d; binary has %d",
			databaseVersion,
			SchemaVersion(),
		)
	}
	if _, err := provider.Up(ctx); err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	databaseVersion, err = provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read migrated database schema version: %w", err)
	}
	return databaseVersion, nil
}

func newStoreMigrationProvider(db *sql.DB) (*goose.Provider, error) {
	sqlMigrations, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("open embedded migration directory: %w", err)
	}
	return newMigrationProvider(db, sqlMigrations, goMigrations...)
}

func mustMigrationSchemaVersion() int64 {
	// Provider construction discovers and validates sources without connecting
	// to its required database handle.
	db, err := sql.Open("sqlite", ":memory:")
	must.NotErrorf(err, "SQLite migration registry handle must open")
	provider, err := newStoreMigrationProvider(db)
	must.NotErrorf(err, "production migration registry must be valid")
	version := migrationSchemaVersion(provider)
	must.NotErrorf(db.Close(), "SQLite migration registry handle must close")
	return version
}
