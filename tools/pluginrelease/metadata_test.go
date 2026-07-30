package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReleaseVersion_AcceptsPrefixedAndUnprefixed(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Prefixed",
			input: "v1.2.3-beta.4",
		},
		{
			name:  "Unprefixed",
			input: "1.2.3-beta.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseReleaseVersion(tt.input)

			require.NoError(t, err)
			assert.Equal(t, pluginVersion("1.2.3-beta.4"), got)
		})
	}
}

func TestParseReleaseVersion_RejectsInvalidSemVer(t *testing.T) {
	_, err := parseReleaseVersion("v1.2.3-01")

	require.Error(t, err)
	assert.ErrorContains(t, err, "not valid SemVer")
}

func TestPluginMetadata_Check(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")
	metadata := pluginMetadata{root: root}
	expected := pluginVersion("0.1.0-beta.2")

	got, err := metadata.check(&expected)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestPluginMetadata_CheckReportsMismatch(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")
	metadata := pluginMetadata{root: root}
	expected := pluginVersion("1.0.0")

	_, err := metadata.check(&expected)

	require.Error(t, err)
	assert.ErrorContains(t, err, "expected version")
	assert.ErrorContains(t, err, versionFilePath)
}

func TestPluginMetadata_MaterializeCanonicalOutput(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")
	metadata := pluginMetadata{root: root}

	require.NoError(t, metadata.materialize("1.2.3-beta.4"))

	assert.Equal(t, `{
  "description": "preserved",
  "name": "cardamom",
  "settings": {
    "channel": "beta"
  },
  "version": "1.2.3-beta.4"
}
`, readMetadataFile(t, filepath.Join(
		root,
		"plugins/cardamom/.claude-plugin/plugin.json",
	)))
	assert.Equal(t, `{
  "name": "cardamom",
  "skills": "./skills/",
  "version": "1.2.3-beta.4"
}
`, readMetadataFile(t, filepath.Join(
		root,
		"plugins/cardamom/.codex-plugin/plugin.json",
	)))
	assert.Equal(t, "1.2.3-beta.4\n", readMetadataFile(t, filepath.Join(
		root,
		versionFilePath,
	)))
}

func TestPluginMetadata_MaterializeIsIdempotent(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")
	metadata := pluginMetadata{root: root}
	require.NoError(t, metadata.materialize("1.2.3"))

	oldTime := time.Unix(1_000_000, 0)
	for _, path := range metadataPaths() {
		path := filepath.Join(root, path)
		require.NoError(t, os.Chtimes(path, oldTime, oldTime))
	}

	require.NoError(t, metadata.materialize("1.2.3"))

	for _, path := range metadataPaths() {
		info, err := os.Stat(filepath.Join(root, path))
		require.NoError(t, err)
		assert.Equal(t, oldTime, info.ModTime(), path)
	}
}

func TestPluginMetadata_MaterializeValidatesBeforeWriting(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")
	codexManifest := filepath.Join(
		root,
		"plugins/cardamom/.codex-plugin/plugin.json",
	)
	require.NoError(t, os.WriteFile(codexManifest, []byte("{"), 0o644))
	before := readMetadataFiles(t, root)
	metadata := pluginMetadata{root: root}

	err := metadata.materialize("1.2.3")

	require.Error(t, err)
	assert.ErrorContains(
		t,
		err,
		"plugins/cardamom/.codex-plugin/plugin.json",
	)
	assert.Equal(t, before, readMetadataFiles(t, root))
}

func TestPluginMetadata_MaterializeRejectsInvalidCurrentVersion(t *testing.T) {
	root := newMetadataFixture(t, "0.1.0-beta.2")
	claudeManifest := filepath.Join(
		root,
		"plugins/cardamom/.claude-plugin/plugin.json",
	)
	writeMetadataFile(t, claudeManifest, `{
  "name": "cardamom",
  "version": 12
}
`)
	before := readMetadataFiles(t, root)
	metadata := pluginMetadata{root: root}

	err := metadata.materialize("1.2.3")

	require.Error(t, err)
	assert.ErrorContains(t, err, "parse version")
	assert.Equal(t, before, readMetadataFiles(t, root))
}

func newMetadataFixture(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	writeMetadataFile(t, filepath.Join(
		root,
		"plugins/cardamom/.claude-plugin/plugin.json",
	), fmt.Sprintf(`{
  "name": "cardamom",
  "version": %q,
  "description": "preserved",
  "settings": {
    "channel": "beta"
  }
}
`, version))
	writeMetadataFile(t, filepath.Join(
		root,
		"plugins/cardamom/.codex-plugin/plugin.json",
	), fmt.Sprintf(`{
  "name": "cardamom",
  "version": %q,
  "skills": "./skills/"
}
`, version))
	writeMetadataFile(
		t,
		filepath.Join(root, versionFilePath),
		version+"\n",
	)
	return root
}

func metadataPaths() []string {
	paths := append([]string(nil), manifestPaths...)
	return append(paths, versionFilePath)
}

func readMetadataFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	got := make(map[string]string, len(metadataPaths()))
	for _, name := range metadataPaths() {
		got[name] = readMetadataFile(t, filepath.Join(root, name))
	}
	return got
}

func writeMetadataFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readMetadataFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}
