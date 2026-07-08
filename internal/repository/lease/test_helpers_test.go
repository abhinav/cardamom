package lease

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func openTestRepository(
	t *testing.T,
	path string,
	clock *testClock,
) (*Repository, *store.Store) {
	t.Helper()
	persistence, err := store.Open(t.Context(), store.Config{
		Path: path,
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	return New(persistence, clock), persistence
}

func testExec(t *testing.T, persistence *store.Store, statement string) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), statement)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

type testClock struct {
	now time.Time
}

func newTestClock(now time.Time) *testClock {
	return &testClock{now: now}
}

func (c *testClock) Now() time.Time {
	return c.now
}

func (c *testClock) Set(now time.Time) {
	c.now = now
}
