package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestReadSettings_missingAndCommentOnlyInheritBuiltIns(t *testing.T) {
	directory := newSettingsStoreDirectory(t)

	overrides, err := readSettings(directory)
	require.NoError(t, err)
	assert.True(t, overrides.Empty())

	require.NoError(t, writeSettings(
		settingsPath(directory),
		configuration.Overrides{},
	))
	overrides, err = readSettings(directory)
	require.NoError(t, err)
	assert.True(t, overrides.Empty())

	body, err := os.ReadFile(settingsPath(directory))
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

func TestReadSettings_readsPhysicalStoreConfiguration(t *testing.T) {
	projectDirectory := t.TempDir()
	storeDirectory := filepath.Join(projectDirectory, ".cardamom")
	require.NoError(t, os.Mkdir(storeDirectory, 0o755))
	require.NoError(t, os.WriteFile(
		settingsPath(storeDirectory),
		[]byte("version: 1\nissue:\n  id:\n    prefix: project-\n"),
		0o644,
	))

	overrides, err := readSettings(storeDirectory)
	require.NoError(t, err)
	require.NotNil(t, overrides.Issue.ID.Prefix)
	assert.Equal(t, "project-", overrides.Issue.ID.Prefix.String())
}

func TestTrackedSettingsTemplate_matchesRenderedDefaults(t *testing.T) {
	tracked, err := os.ReadFile(filepath.Join("..", "..", ".cardamom", settingsFilename))
	require.NoError(t, err)

	assert.Equal(t, string(renderSettings(configuration.Overrides{})), string(tracked))
}

func TestReadSettings_nestedOverrides(t *testing.T) {
	directory := newSettingsStoreDirectory(t)
	require.NoError(t, os.WriteFile(
		settingsPath(directory),
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

	overrides, err := readSettings(directory)
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

func TestReadSettings_requiresVersionForActiveValues(t *testing.T) {
	directory := newSettingsStoreDirectory(t)
	require.NoError(t, os.WriteFile(
		settingsPath(directory),
		[]byte("issue:\n  id:\n    prefix: mission-\n"),
		0o644,
	))

	_, err := readSettings(directory)

	assert.ErrorContains(t, err, "version must be 1 when configuration values are active")
}

func TestReadSettings_rejectsUnknownNestedKey(t *testing.T) {
	directory := newSettingsStoreDirectory(t)
	require.NoError(t, os.WriteFile(
		settingsPath(directory),
		[]byte("version: 1\nissue:\n  id:\n    unknown: true\n"),
		0o644,
	))

	_, err := readSettings(directory)

	assert.ErrorContains(t, err, `unknown field "unknown"`)
}

func TestWriteSettings_activePrefixUsesVersionedNestedShape(t *testing.T) {
	directory := newSettingsStoreDirectory(t)
	prefix, err := configuration.NewPrefix("mission-")
	require.NoError(t, err)

	require.NoError(t, writeSettings(
		settingsPath(directory),
		configuration.Overrides{
			Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{Prefix: &prefix},
			},
		},
	))

	body, err := os.ReadFile(settingsPath(directory))
	require.NoError(t, err)
	assert.Contains(t, string(body), "version: 1\n")
	assert.Contains(t, string(body), "issue:\n  id:\n    prefix: mission-\n")
	assert.NotContains(t, string(body), "id_prefix:")
	assert.NotContains(t, string(body), "id_strategy:")
	assert.Contains(t, string(body), "#     strategy: random")
}

func newSettingsStoreDirectory(t *testing.T) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), ".cardamom")
	require.NoError(t, os.Mkdir(directory, 0o755))
	return directory
}
