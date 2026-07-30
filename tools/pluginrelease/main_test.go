package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_Materialize(t *testing.T) {
	root := newPluginFixture(t, "0.1.0-beta.2")

	require.NoError(t, run(root, io.Discard, []string{
		"materialize",
		"v1.2.3-beta.4",
	}))

	assert.JSONEq(t, `{
		"name": "cardamom",
		"version": "1.2.3-beta.4",
		"description": "preserved"
	}`, readFile(t, filepath.Join(
		root,
		"plugins/cardamom/.claude-plugin/plugin.json",
	)))
	assert.JSONEq(t, `{
		"name": "cardamom",
		"version": "1.2.3-beta.4",
		"skills": "./skills/"
	}`, readFile(t, filepath.Join(
		root,
		"plugins/cardamom/.codex-plugin/plugin.json",
	)))
	assert.Equal(t, "1.2.3-beta.4\n", readFile(t, filepath.Join(
		root,
		"plugins/cardamom/VERSION",
	)))
}

func TestRun_MaterializeIsIdempotent(t *testing.T) {
	root := newPluginFixture(t, "0.1.0-beta.2")

	require.NoError(t, run(root, io.Discard, []string{
		"materialize",
		"v1.2.3",
	}))
	before := readPluginFiles(t, root)
	require.NoError(t, run(root, io.Discard, []string{
		"materialize",
		"v1.2.3",
	}))

	assert.Equal(t, before, readPluginFiles(t, root))
}

func TestRun_MaterializeValidatesBeforeWriting(t *testing.T) {
	root := newPluginFixture(t, "0.1.0-beta.2")
	codexManifest := filepath.Join(
		root,
		"plugins/cardamom/.codex-plugin/plugin.json",
	)
	require.NoError(t, os.WriteFile(codexManifest, []byte("{"), 0o644))
	before := readPluginFiles(t, root)

	err := run(root, io.Discard, []string{
		"materialize",
		"v1.2.3",
	})

	require.Error(t, err)
	assert.Equal(t, before, readPluginFiles(t, root))
}

func TestRun_MaterializeRejectsInvalidCurrentVersion(t *testing.T) {
	root := newPluginFixture(t, "0.1.0-beta.2")
	claudeManifest := filepath.Join(
		root,
		"plugins/cardamom/.claude-plugin/plugin.json",
	)
	writeFile(t, claudeManifest, `{
  "name": "cardamom",
  "version": 12
}
`)
	before := readPluginFiles(t, root)

	err := run(root, io.Discard, []string{
		"materialize",
		"v1.2.3",
	})

	require.Error(t, err)
	assert.Equal(t, before, readPluginFiles(t, root))
}

func TestRun_Check(t *testing.T) {
	root := newPluginFixture(t, "0.1.0-beta.2")

	assert.NoError(t, run(root, io.Discard, []string{"check"}))
	assert.NoError(t, run(root, io.Discard, []string{
		"check",
		"v0.1.0-beta.2",
	}))
}

func TestRun_CheckReportsMismatch(t *testing.T) {
	root := newPluginFixture(t, "0.1.0-beta.2")

	err := run(root, io.Discard, []string{"check", "v1.0.0"})

	require.Error(t, err)
	assert.ErrorContains(t, err, "expected version")
}

func TestRun_RequiresPrefixedReleaseVersion(t *testing.T) {
	root := newPluginFixture(t, "0.1.0-beta.2")

	err := run(root, io.Discard, []string{
		"materialize",
		"1.2.3",
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, `must start with "v"`)
}

func TestRun_RejectsInvalidSemVer(t *testing.T) {
	root := newPluginFixture(t, "0.1.0-beta.2")

	err := run(root, io.Discard, []string{
		"materialize",
		"v1.2.3-01",
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "not valid SemVer")
}

func TestPOSIXLauncher_PrefersPath(t *testing.T) {
	root := repositoryRoot(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "card"), `#!/bin/sh
printf 'path:%s\n' "$*"
`)
	writeExecutable(t, filepath.Join(binDir, "curl"), `#!/bin/sh
echo "curl must not run" >&2
exit 99
`)

	cmd := exec.CommandContext(
		t.Context(),
		filepath.Join(
			root,
			"plugins/cardamom/skills/cardamom/scripts/cardamom",
		),
		"show",
		"cm-task",
	)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin:/sbin",
		"HOME="+t.TempDir(),
	)
	out, err := cmd.CombinedOutput()

	require.NoError(t, err, string(out))
	assert.Equal(t, "path:show cm-task\n", string(out))
}

func TestPOSIXLauncher_DownloadsAndCachesRelease(t *testing.T) {
	root := repositoryRoot(t)
	fixtures := t.TempDir()
	binDir := t.TempDir()
	home := t.TempDir()
	archiveName := "cardamom.Darwin-arm64.tar.gz"
	archive := filepath.Join(fixtures, archiveName)
	writeCardArchive(t, archive, `#!/bin/sh
printf 'downloaded:%s\n' "$*"
`)
	writeChecksums(t, fixtures, archiveName, archive)
	writeFakeReleaseCommands(t, binDir, "Darwin", "arm64")

	runLauncher := func() string {
		t.Helper()
		cmd := exec.CommandContext(
			t.Context(),
			filepath.Join(
				root,
				"plugins/cardamom/skills/cardamom/scripts/cardamom",
			),
			"show",
			"cm-task",
		)
		cmd.Env = append(os.Environ(),
			"PATH="+binDir+":/usr/bin:/bin:/sbin",
			"HOME="+home,
			"FIXTURE_DIR="+fixtures,
			"CURL_LOG="+filepath.Join(fixtures, "curl.log"),
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}

	assert.Equal(t, "downloaded:show cm-task\n", runLauncher())
	assert.Equal(t, "downloaded:show cm-task\n", runLauncher())
	assert.Len(t, strings.Split(strings.TrimSpace(readFile(
		t,
		filepath.Join(fixtures, "curl.log"),
	)), "\n"), 2)

	cached := filepath.Join(
		home,
		".cache/cardamom-skill/versions/0.1.0-beta.2/cardamom",
	)
	info, err := os.Stat(cached)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111)
}

func TestPOSIXLauncher_RejectsChecksumMismatch(t *testing.T) {
	root := repositoryRoot(t)
	fixtures := t.TempDir()
	binDir := t.TempDir()
	home := t.TempDir()
	archiveName := "cardamom.Linux-x86_64.tar.gz"
	archive := filepath.Join(fixtures, archiveName)
	writeCardArchive(t, archive, "#!/bin/sh\nexit 0\n")
	require.NoError(t, os.WriteFile(
		filepath.Join(fixtures, "checksums.txt"),
		[]byte(strings.Repeat("0", 64)+"  "+archiveName+"\n"),
		0o644,
	))
	writeFakeReleaseCommands(t, binDir, "Linux", "x86_64")

	cmd := exec.CommandContext(
		t.Context(),
		filepath.Join(
			root,
			"plugins/cardamom/skills/cardamom/scripts/cardamom",
		),
	)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin:/sbin",
		"HOME="+home,
		"FIXTURE_DIR="+fixtures,
		"CURL_LOG="+filepath.Join(fixtures, "curl.log"),
	)
	out, err := cmd.CombinedOutput()

	require.Error(t, err)
	assert.Contains(t, string(out), "checksum mismatch")
	_, statErr := os.Stat(filepath.Join(
		home,
		".cache/cardamom-skill/versions/0.1.0-beta.2/cardamom",
	))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func newPluginFixture(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(
		root,
		"plugins/cardamom/.claude-plugin/plugin.json",
	), fmt.Sprintf(`{
  "name": "cardamom",
  "version": %q,
  "description": "preserved"
}
`, version))
	writeFile(t, filepath.Join(
		root,
		"plugins/cardamom/.codex-plugin/plugin.json",
	), fmt.Sprintf(`{
  "name": "cardamom",
  "version": %q,
  "skills": "./skills/"
}
`, version))
	writeFile(t, filepath.Join(
		root,
		"plugins/cardamom/VERSION",
	), version+"\n")
	return root
}

func readPluginFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := []string{
		"plugins/cardamom/.claude-plugin/plugin.json",
		"plugins/cardamom/.codex-plugin/plugin.json",
		"plugins/cardamom/VERSION",
	}
	got := make(map[string]string, len(files))
	for _, name := range files {
		got[name] = readFile(t, filepath.Join(root, name))
	}
	return got
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	return root
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	writeFile(t, path, body)
	require.NoError(t, os.Chmod(path, 0o755))
}

func writeCardArchive(t *testing.T, path, body string) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, file.Close())
	}()

	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{
		Name:    "card",
		Mode:    0o755,
		Size:    int64(len(body)),
		ModTime: time.Unix(0, 0),
	}))
	_, err = tarWriter.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
}

func writeChecksums(
	t *testing.T,
	fixtures string,
	archiveName string,
	archive string,
) {
	t.Helper()
	body, err := os.ReadFile(archive)
	require.NoError(t, err)
	sum := sha256.Sum256(body)
	writeFile(
		t,
		filepath.Join(fixtures, "checksums.txt"),
		hex.EncodeToString(sum[:])+"  "+archiveName+"\n",
	)
}

func writeFakeReleaseCommands(
	t *testing.T,
	binDir string,
	goos string,
	goarch string,
) {
	t.Helper()
	writeExecutable(t, filepath.Join(binDir, "uname"), fmt.Sprintf(`#!/bin/sh
case "$1" in
  -s) printf '%%s\n' %q ;;
  -m) printf '%%s\n' %q ;;
  *) exit 2 ;;
esac
`, goos, goarch))
	writeExecutable(t, filepath.Join(binDir, "curl"), `#!/bin/sh
output=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      output=$2
      shift 2
      ;;
    --fail|--location|--silent|--show-error)
      shift
      ;;
    *)
      url=$1
      shift
      ;;
  esac
done
printf '%s\n' "$url" >> "$CURL_LOG"
cp "$FIXTURE_DIR/${url##*/}" "$output"
`)
}
