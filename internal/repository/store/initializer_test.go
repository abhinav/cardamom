package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/storelocation"
)

func TestInitializerPublishesCompletedFreshStore(t *testing.T) {
	dir := t.TempDir()
	result, err := NewInitializer().Initialize(
		t.Context(),
		dir,
		func(ctx context.Context, persistence *Store) error {
			change, err := persistence.Change(ctx)
			if err != nil {
				return err
			}
			defer func() { assert.NoError(t, change.Done()) }()
			if _, err := change.ExecContext(ctx, `
				UPDATE store_state SET next_issue_number = 2 WHERE singleton = 1
			`); err != nil {
				return err
			}
			return change.Commit()
		},
	)
	require.NoError(t, err)
	assert.True(t, result.DatabaseWritten)
	assert.Equal(t, SchemaVersion(), result.SchemaVersion)

	persistence, err := Open(t.Context(), Config{
		Path: storelocation.DatabasePath(dir),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	assert.Equal(t, int64(2), readNextIssueNumber(t, persistence))
}

func TestInitializerRemovesDatabaseAfterCompletionFailure(t *testing.T) {
	dir := t.TempDir()
	wantErr := errors.New("completion failed")
	result, err := NewInitializer().Initialize(
		t.Context(),
		dir,
		func(context.Context, *Store) error { return wantErr },
	)
	assert.ErrorIs(t, err, wantErr)
	assert.False(t, result.DatabaseWritten)
	assert.Equal(t, SchemaVersion(), result.SchemaVersion)
	assertDatabaseFilesAbsent(t, storelocation.DatabasePath(dir))
}

func TestInitializerRemovesDatabaseAfterVerificationFailure(t *testing.T) {
	dir := t.TempDir()
	result, err := NewInitializer().Initialize(
		t.Context(),
		dir,
		func(ctx context.Context, persistence *Store) error {
			change, err := persistence.Change(ctx)
			if err != nil {
				return err
			}
			defer func() { assert.NoError(t, change.Done()) }()
			if _, err := change.ExecContext(ctx, `
				INSERT INTO projects(id, name, created_at)
				VALUES ('project', 'Project', 1);
				INSERT INTO boards(id, project_id, name, created_at, revision)
				VALUES ('board', 'project', 'Board', 1, 1);
			`); err != nil {
				return err
			}
			return change.Commit()
		},
	)
	assert.ErrorContains(t, err, "projection revisions")
	assert.False(t, result.DatabaseWritten)
	assert.Equal(t, SchemaVersion(), result.SchemaVersion)
	assertDatabaseFilesAbsent(t, storelocation.DatabasePath(dir))
}

func assertDatabaseFilesAbsent(t *testing.T, path string) {
	t.Helper()
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		_, err := os.Stat(candidate)
		assert.ErrorIs(t, err, os.ErrNotExist, candidate)
	}
}
