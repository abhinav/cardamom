package storelocation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAncestorDirectory(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, ".cardamom")
	child := filepath.Join(root, "a", "b")
	mustMkdirAll(t, store)
	mustMkdirAll(t, child)

	got, err := Resolve("", child)
	require.NoError(t, err)
	assert.Equal(t, store, got)
}

func TestInitTarget_defaultsToCardamomStore(t *testing.T) {
	got, err := InitTarget("", "")
	require.NoError(t, err)
	assert.Equal(t, ".cardamom", got)
}

func TestStoreBelowWorktreeRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo", "linked")
	tests := []struct {
		name  string
		store string
		want  bool
	}{
		{name: "Nested", store: filepath.Join(root, "project", ".cardamom"), want: true},
		{name: "WorktreeRoot", store: filepath.Join(root, ".cardamom")},
		{name: "Parent", store: filepath.Join(filepath.Dir(root), ".cardamom")},
		{name: "SimilarSibling", store: filepath.Join(root+"-other", ".cardamom")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, storeBelowWorktreeRoot(test.store, root))
		})
	}
}

func TestResolveRelativeRedirect(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	store := filepath.Join(root, "stores", "board")
	mustMkdirAll(t, project)
	mustMkdirAll(t, store)
	require.NoError(t, os.WriteFile(filepath.Join(project, ".cardamom"), []byte("../stores/board\n"), 0o600))

	got, err := Resolve("", project)
	require.NoError(t, err)
	assert.Equal(t, store, got)
}

func TestResolveRejectsMalformedRedirect(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".cardamom"), []byte("first\nsecond\n"), 0o600))

	_, err := Resolve("", root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must contain one path")
}

func TestBoardBindingPathUsesNearestNonGitStore(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, ".cardamom"))
	child := filepath.Join(root, "a", "b")
	mustMkdirAll(t, child)

	got, err := BoardBindingPath(child)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".cardamom-board"), got)
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o755))
}
