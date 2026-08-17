package yamlstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestStore_ReadStoreConfiguration_missingAndCommentOnlyInheritBuiltIns(t *testing.T) {
	directory := newStoreDirectory(t)
	store := &Store{Directory: directory}

	overrides, err := store.ReadStoreConfiguration(t.Context())
	require.NoError(t, err)
	assert.True(t, overrides.Empty())
	hasDocument, err := store.HasDocument()
	require.NoError(t, err)
	assert.False(t, hasDocument)

	require.NoError(t, store.WriteInitializationTemplate())
	overrides, err = store.ReadStoreConfiguration(t.Context())
	require.NoError(t, err)
	assert.True(t, overrides.Empty())
	hasDocument, err = store.HasDocument()
	require.NoError(t, err)
	assert.True(t, hasDocument)

	body, err := os.ReadFile(store.path())
	require.NoError(t, err)
	for line := range strings.SplitSeq(string(body), "\n") {
		assert.True(t, line == "" || strings.HasPrefix(line, "#"), line)
	}
	assert.Contains(t, string(body), "# version: 1")
	assert.Contains(t, string(body), "#     max_bytes: 2048")
	assert.Contains(t, string(body), "#   max_bytes: 104857600")
	assert.Contains(t, string(body), "#     max_count: 8")
	assert.Contains(t, string(body), "card config set --scope store")
	assert.Contains(t, string(body), "card config unset --scope store")
}

func TestStore_ReadStoreConfiguration_readsPhysicalStoreConfiguration(t *testing.T) {
	projectDirectory := t.TempDir()
	storeDirectory := filepath.Join(projectDirectory, ".cardamom")
	require.NoError(t, os.Mkdir(storeDirectory, 0o755))
	store := &Store{Directory: storeDirectory}
	require.NoError(t, os.WriteFile(
		store.path(),
		[]byte("version: 1\nissue:\n  id:\n    prefix: project-\n"),
		0o644,
	))

	overrides, err := store.ReadStoreConfiguration(t.Context())
	require.NoError(t, err)
	require.NotNil(t, overrides.Issue.ID.Prefix)
	assert.Equal(t, "project-", overrides.Issue.ID.Prefix.String())
}

func TestTrackedSettingsTemplate_matchesRenderedDefaults(t *testing.T) {
	tracked, err := os.ReadFile(filepath.Join("..", "..", "..", ".cardamom", filename))
	require.NoError(t, err)

	assert.Equal(t, string(render(configuration.Overrides{})), string(tracked))
}

func TestStore_ReadStoreConfiguration_nestedOverrides(t *testing.T) {
	directory := newStoreDirectory(t)
	store := &Store{Directory: directory}
	require.NoError(t, os.WriteFile(
		store.path(),
		[]byte(`version: 1
issue:
  id:
    prefix: mission-
    strategy: sequential
  summary:
    max_bytes: 4096
attachment:
  max_bytes: 8192
board:
  pins:
    max_count: 5
`),
		0o644,
	))

	overrides, err := store.ReadStoreConfiguration(t.Context())
	require.NoError(t, err)
	require.NotNil(t, overrides.Issue.ID.Prefix)
	require.NotNil(t, overrides.Issue.ID.Strategy)
	require.NotNil(t, overrides.Issue.Summary.MaxBytes)
	require.NotNil(t, overrides.Attachment.MaxBytes)
	require.NotNil(t, overrides.Board.Pins.MaxCount)
	assert.Equal(t, "mission-", overrides.Issue.ID.Prefix.String())
	assert.Equal(t, configuration.IDStrategySequential, *overrides.Issue.ID.Strategy)
	assert.Equal(t, uint64(4096), overrides.Issue.Summary.MaxBytes.Uint64())
	assert.Equal(t, uint64(8192), overrides.Attachment.MaxBytes.Uint64())
	assert.Equal(t, uint64(5), overrides.Board.Pins.MaxCount.Uint64())
}

func TestStore_ReadStoreConfiguration_requiresVersionForActiveValues(t *testing.T) {
	directory := newStoreDirectory(t)
	store := &Store{Directory: directory}
	require.NoError(t, os.WriteFile(
		store.path(),
		[]byte("issue:\n  id:\n    prefix: mission-\n"),
		0o644,
	))

	_, err := store.ReadStoreConfiguration(t.Context())

	assert.ErrorContains(t, err, "version must be 1 when configuration values are active")
}

func TestStore_ReadStoreConfiguration_rejectsUnknownNestedKey(t *testing.T) {
	directory := newStoreDirectory(t)
	store := &Store{Directory: directory}
	require.NoError(t, os.WriteFile(
		store.path(),
		[]byte("version: 1\nissue:\n  id:\n    unknown: true\n"),
		0o644,
	))

	_, err := store.ReadStoreConfiguration(t.Context())

	assert.ErrorContains(t, err, `unknown field "unknown"`)
}

func TestStore_UpdateStoreConfiguration_activePrefixUsesVersionedNestedShape(t *testing.T) {
	directory := newStoreDirectory(t)
	store := &Store{Directory: directory}
	prefix, err := configuration.NewPrefix("mission-")
	require.NoError(t, err)

	_, err = store.UpdateStoreConfiguration(t.Context(), configuration.Patch{
		Fields: []configuration.Field{configuration.FieldIssueIDPrefix},
		Overrides: configuration.Overrides{
			Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{Prefix: &prefix},
			},
		},
	})
	require.NoError(t, err)

	body, err := os.ReadFile(store.path())
	require.NoError(t, err)
	assert.Contains(t, string(body), "version: 1\n")
	assert.Contains(t, string(body), "issue:\n  id:\n    prefix: mission-\n")
	assert.NotContains(t, string(body), "id_prefix:")
	assert.NotContains(t, string(body), "id_strategy:")
	assert.Contains(t, string(body), "#     strategy: random")
}

func newStoreDirectory(t *testing.T) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), ".cardamom")
	require.NoError(t, os.Mkdir(directory, 0o755))
	return directory
}
