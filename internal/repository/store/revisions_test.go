package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangePublishesCanonicalRevision(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "revision.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	reservation, err := change.ReserveRevision(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(0), reservation.CurrentRevision())
	assert.Equal(t, int64(1), reservation.Revision())
	require.NoError(t, change.PublishRevision(t.Context(), reservation))
	require.NoError(t, change.Commit())

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	revision, err := view.CanonicalRevision(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), revision)
}

func TestStoreCanonicalRevisionOnlyReadsCommittedHead(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "committed-head.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	reservation, err := change.ReserveRevision(t.Context())
	require.NoError(t, err)
	require.NoError(t, change.PublishRevision(t.Context(), reservation))

	revision, err := persistence.CanonicalRevision(t.Context())
	require.NoError(t, err)
	assert.Zero(t, revision)

	require.NoError(t, change.Commit())
	revision, err = persistence.CanonicalRevision(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), revision)
}

func TestChangeDoesNotPublishReservedNoOpRevision(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "discard.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	reservation, err := change.ReserveRevision(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), reservation.Revision())
	require.NoError(t, change.Commit())

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	revision, err := view.CanonicalRevision(t.Context())
	require.NoError(t, err)
	assert.Zero(t, revision)
}

func TestChangeRejectsFabricatedRevisionReservation(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "fabricated.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	err = change.PublishRevision(t.Context(), RevisionReservation{
		currentRevision: 1,
		revision:        2,
	})
	assert.ErrorContains(t, err, "canonical head changed")

	revision, err := change.CanonicalRevision(t.Context())
	require.NoError(t, err)
	assert.Zero(t, revision)
}

func TestChangeReservesSequentialIssueNumbers(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "issues.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	first, err := change.ReserveIssueNumber(t.Context())
	require.NoError(t, err)
	second, err := change.ReserveIssueNumber(t.Context())
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	assert.Equal(t, int64(1), first)
	assert.Equal(t, int64(2), second)
}
