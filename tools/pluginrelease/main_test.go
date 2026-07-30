package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	claudeManifestPath = "plugins/cardamom/.claude-plugin/plugin.json"
	codexManifestPath  = "plugins/cardamom/.codex-plugin/plugin.json"
	bashLauncherPath   = "plugins/cardamom/skills/cardamom/scripts/cardamom"
	powerShellPath     = "plugins/cardamom/skills/cardamom/scripts/cardamom.ps1"
)

func TestRun_MaterializesReleaseFiles(t *testing.T) {
	root := newReleaseFixture(t, "0.1.0-beta.2")
	var stderr bytes.Buffer

	assert.Equal(t, 0, run(root, &stderr, []string{"v1.2.3-beta.4"}))

	assert.Empty(t, stderr.String())
	assert.Equal(t, `{
  "name": "cardamom",
  "version": "1.2.3-beta.4",
  "description": "preserved",
  "settings": {
    "channel": "beta"
  }
}
`, readReleaseFile(t, root, claudeManifestPath))
	assert.Equal(t, `{
  "name": "cardamom",
  "version": "1.2.3-beta.4",
  "skills": "./skills/"
}
`, readReleaseFile(t, root, codexManifestPath))
	assert.Contains(
		t,
		readReleaseFile(t, root, bashLauncherPath),
		"readonly VERSION='1.2.3-beta.4'",
	)
	assert.Contains(
		t,
		readReleaseFile(t, root, powerShellPath),
		`$Version = "1.2.3-beta.4"`,
	)
}

func TestRun_AcceptsVersionWithoutPrefix(t *testing.T) {
	root := newReleaseFixture(t, "0.1.0-beta.2")

	assert.Equal(t, 0, run(root, io.Discard, []string{"1.2.3"}))

	assert.Contains(
		t,
		readReleaseFile(t, root, claudeManifestPath),
		`"version": "1.2.3"`,
	)
}

func TestRun_CheckReportsEveryChangedFileWithoutWriting(t *testing.T) {
	root := newReleaseFixture(t, "0.1.0-beta.2")
	before := snapshotReleaseFiles(t, root)
	var stderr bytes.Buffer

	assert.Equal(t, 1, run(root, &stderr, []string{"-check", "1.2.3"}))

	assert.Equal(t, `pluginrelease: would update: plugins/cardamom/.claude-plugin/plugin.json
pluginrelease: would update: plugins/cardamom/.codex-plugin/plugin.json
pluginrelease: would update: plugins/cardamom/skills/cardamom/scripts/cardamom
pluginrelease: would update: plugins/cardamom/skills/cardamom/scripts/cardamom.ps1
pluginrelease: 4 files would be changed
`, stderr.String())
	assert.Equal(t, before, snapshotReleaseFiles(t, root))
}

func TestRun_MaterializesFilesBeforeLaterFailure(t *testing.T) {
	root := newReleaseFixture(t, "0.1.0-beta.2")
	writeReleaseFile(
		t,
		root,
		powerShellPath,
		`$ErrorActionPreference = "Stop"`+"\n",
	)
	var stderr bytes.Buffer

	assert.Equal(t, 1, run(root, &stderr, []string{"1.2.3"}))

	assert.Contains(t, stderr.String(), "expected one version constant")
	assert.Contains(
		t,
		readReleaseFile(t, root, claudeManifestPath),
		`"version": "1.2.3"`,
	)
	assert.Contains(
		t,
		readReleaseFile(t, root, codexManifestPath),
		`"version": "1.2.3"`,
	)
	assert.Contains(
		t,
		readReleaseFile(t, root, bashLauncherPath),
		"readonly VERSION='1.2.3'",
	)
}

func TestRun_LeavesUnchangedFilesAlone(t *testing.T) {
	root := newReleaseFixture(t, "1.2.3")
	oldTime := time.Unix(1_000_000, 0)
	for _, path := range releaseFilePaths() {
		require.NoError(
			t,
			os.Chtimes(filepath.Join(root, path), oldTime, oldTime),
		)
	}
	var stderr bytes.Buffer

	assert.Equal(t, 0, run(root, &stderr, []string{"1.2.3"}))
	assert.Equal(t, 0, run(root, &stderr, []string{"-check", "v1.2.3"}))

	assert.Empty(t, stderr.String())
	for _, path := range releaseFilePaths() {
		info, err := os.Stat(filepath.Join(root, path))
		require.NoError(t, err)
		assert.Equal(t, oldTime, info.ModTime(), path)
	}
}

func TestRun_RejectsMalformedJSON(t *testing.T) {
	root := newReleaseFixture(t, "0.1.0-beta.2")
	writeReleaseFile(t, root, claudeManifestPath, "{")
	var stderr bytes.Buffer

	assert.Equal(t, 1, run(root, &stderr, []string{"1.2.3"}))

	assert.Contains(t, stderr.String(), "invalid JSON")
	assert.Contains(t, stderr.String(), claudeManifestPath)
}

func TestRun_RejectsMissingOrWrongVersionField(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     string
	}{
		{
			name:     "Missing",
			manifest: `{"name":"cardamom"}`,
			want:     "version field is missing",
		},
		{
			name:     "WrongType",
			manifest: `{"name":"cardamom","version":12}`,
			want:     "version field must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newReleaseFixture(t, "0.1.0-beta.2")
			writeReleaseFile(t, root, claudeManifestPath, tt.manifest)
			var stderr bytes.Buffer

			assert.Equal(t, 1, run(root, &stderr, []string{"1.2.3"}))

			assert.Contains(t, stderr.String(), tt.want)
			assert.Contains(t, stderr.String(), claudeManifestPath)
		})
	}
}

func TestRun_RejectsMissingLauncherConstant(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "Bash",
			path: bashLauncherPath,
			body: "#!/usr/bin/env bash\n",
		},
		{
			name: "PowerShell",
			path: powerShellPath,
			body: `$ErrorActionPreference = "Stop"` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newReleaseFixture(t, "1.2.3")
			writeReleaseFile(t, root, tt.path, tt.body)
			var stderr bytes.Buffer

			assert.Equal(t, 1, run(root, &stderr, []string{"1.2.3"}))

			assert.Contains(t, stderr.String(), "expected one version constant")
			assert.Contains(t, stderr.String(), tt.path)
		})
	}
}

func TestRun_RejectsInvalidVersion(t *testing.T) {
	var stderr bytes.Buffer

	assert.Equal(
		t,
		2,
		run(t.TempDir(), &stderr, []string{"v1.2.3-01"}),
	)

	assert.Contains(t, stderr.String(), "not valid SemVer")
}

func TestRun_RequiresOneVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "Missing"},
		{name: "MissingInCheck", args: []string{"-check"}},
		{name: "TooMany", args: []string{"1.2.3", "1.2.4"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer

			assert.Equal(t, 2, run(t.TempDir(), &stderr, tt.args))

			assert.Contains(
				t,
				stderr.String(),
				"usage: pluginrelease [-check] <version>",
			)
		})
	}
}

func newReleaseFixture(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	writeReleaseFile(t, root, claudeManifestPath, fmt.Sprintf(`{
  "name": "cardamom",
  "version": %q,
  "description": "preserved",
  "settings": {
    "channel": "beta"
  }
}
`, version))
	writeReleaseFile(t, root, codexManifestPath, fmt.Sprintf(`{
  "name": "cardamom",
  "version": %q,
  "skills": "./skills/"
}
`, version))
	writeReleaseFile(
		t,
		root,
		bashLauncherPath,
		fmt.Sprintf("#!/usr/bin/env bash\nreadonly VERSION='%s'\n", version),
	)
	require.NoError(
		t,
		os.Chmod(filepath.Join(root, bashLauncherPath), 0o755),
	)
	writeReleaseFile(t, root, powerShellPath, fmt.Sprintf(`$Version = %q
`, version))
	return root
}

func releaseFilePaths() []string {
	return []string{
		claudeManifestPath,
		codexManifestPath,
		bashLauncherPath,
		powerShellPath,
	}
}

func snapshotReleaseFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string, len(releaseFilePaths()))
	for _, path := range releaseFilePaths() {
		files[path] = readReleaseFile(t, root, path)
	}
	return files
}

func writeReleaseFile(t *testing.T, root, path, body string) {
	t.Helper()
	path = filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readReleaseFile(t *testing.T, root, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, path))
	require.NoError(t, err)
	return string(body)
}
