//go:build conflictpreview

package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConflictPreview preserves a disposable fixture for CLI and browser review.
// Set CARDAMOM_CONFLICT_PREVIEW_STORE to a fresh directory under /tmp or TMPDIR
// and run this test with -tags conflictpreview. Ordinary test runs cannot create it.
func TestConflictPreview(t *testing.T) {
	directory := os.Getenv("CARDAMOM_CONFLICT_PREVIEW_STORE")
	if directory == "" {
		t.Skip("set CARDAMOM_CONFLICT_PREVIEW_STORE to an explicit disposable directory")
	}
	directory = filepath.Clean(directory)
	require.True(t, strings.HasPrefix(directory, "/tmp/") || strings.HasPrefix(directory, filepath.Clean(os.TempDir())+string(os.PathSeparator)), "preview store must be in a temporary directory")
	_, err := os.Stat(filepath.Join(directory, "board.sqlite3"))
	require.ErrorIs(t, err, os.ErrNotExist, "preview must not overwrite an existing store")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	fixture := seedConflictFixture(t, directory)
	t.Logf("store=%s board=board-conflicts", directory)
	t.Log("issues: demo-title, demo-cycle-a, demo-cycle-b, demo-execution, demo-independent")
	for _, name := range []string{"title", "summary", "limit", "cycle", "execution"} {
		t.Logf("%s: %s first-assignment=%s", name, fixture.entries[name].ID(), fixture.first[name])
	}
}
