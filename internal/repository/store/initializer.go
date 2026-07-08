package store

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/storelocation"
)

// FreshStoreCompletion finishes product-specific initialization after Open
// establishes the clean schema and before Initializer publishes the database.
type FreshStoreCompletion func(context.Context, *Store) error

// Initialization reports the storage state established by one invocation.
type Initialization struct {
	// DatabaseWritten reports whether this invocation published a new database.
	DatabaseWritten bool

	// SchemaVersion is the on-disk schema reached by the invocation.
	SchemaVersion int64
}

// Initializer owns the lifecycle policy for stores opened during product
// initialization.
type Initializer struct{}

// NewInitializer constructs an initializer that opens and closes each store.
func NewInitializer() *Initializer {
	return &Initializer{}
}

// Initialize opens or creates the store in dir and closes it before returning.
// For a fresh database, complete runs before the completed database is
// published. Completion or post-completion verification failure removes the
// unpublished database and reports DatabaseWritten as false when cleanup
// succeeds.
func (i *Initializer) Initialize(
	ctx context.Context,
	dir string,
	complete FreshStoreCompletion,
) (Initialization, error) {
	must.NotBeNilf(complete, "store FreshStoreCompletion is required")
	path := storelocation.DatabasePath(dir)
	_, err := os.Stat(path)
	databaseExisted := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Initialization{}, err
	}

	result := Initialization{DatabaseWritten: !databaseExisted}
	persistence, err := Open(ctx, Config{Path: path})
	if err != nil {
		return result, err
	}
	result.SchemaVersion = SchemaVersion()
	if !databaseExisted {
		if err := complete(ctx, persistence); err != nil {
			filesRemoved, cleanupErr := cleanupFreshDatabase(persistence, path)
			if filesRemoved {
				result = Initialization{SchemaVersion: result.SchemaVersion}
			}
			return result, errors.Join(err, cleanupErr)
		}
		if err := verifyStore(
			ctx,
			persistence.db,
			persistence.databaseSchemaVersion,
		); err != nil {
			filesRemoved, cleanupErr := cleanupFreshDatabase(persistence, path)
			if filesRemoved {
				result = Initialization{SchemaVersion: result.SchemaVersion}
			}
			return result, errors.Join(err, cleanupErr)
		}
	}
	if err := persistence.Close(); err != nil {
		return result, err
	}
	return result, nil
}

func cleanupFreshDatabase(persistence *Store, path string) (bool, error) {
	cleanupErr := persistence.Close()
	filesRemoved := true
	for _, candidate := range []string{path + "-wal", path + "-shm", path} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			filesRemoved = false
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove fresh database file %q: %w", candidate, err))
		}
	}
	return filesRemoved, cleanupErr
}
