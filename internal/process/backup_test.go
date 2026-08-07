package process

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainbackup "go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/cli"
)

func TestBackupCommand_selectsDefaultExplicitAndAllBoards(t *testing.T) {
	t.Setenv("CARDAMOM_STORE", "")
	t.Setenv("CARDAMOM_BOARD", "")
	cfg := testConfig(t)
	initialized := execute(t, cfg, "--json", "init", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)
	var namespace cli.InitResult
	require.NoError(t, json.Unmarshal([]byte(initialized.stdout), &namespace))
	require.NotNil(t, namespace.ProjectID)
	require.NotNil(t, namespace.BoardID)

	created := execute(
		t,
		cfg,
		"board",
		"create",
		"Engineering",
		"--project",
		*namespace.ProjectID,
	)
	require.Equal(t, cli.ExitSuccess, created.code, created.stderr)
	engineeringBoardID := strings.TrimSpace(created.stdout)
	selected := execute(t, cfg, "board", "use", engineeringBoardID)
	require.Equal(t, cli.ExitSuccess, selected.code, selected.stderr)

	t.Run("DefaultSelectedBoard", func(t *testing.T) {
		archivePath := filepath.Join(cfg.CWD, "selected.cardamom")
		result := execute(
			t,
			cfg,
			"--json",
			"backup",
			archivePath,
		)

		require.Equal(t, cli.ExitSuccess, result.code, result.stderr)
		assertBackupCounts(t, result.stdout, 1, 1, 0)
		assertBackupArchiveBoards(t, archivePath, engineeringBoardID)
	})

	t.Run("ExplicitBoardNames", func(t *testing.T) {
		archivePath := filepath.Join(cfg.CWD, "explicit.cardamom")
		result := execute(
			t,
			cfg,
			"--json",
			"backup",
			archivePath,
			"--include-board",
			"Bridge",
			"--include-board",
			"Engineering",
		)

		require.Equal(t, cli.ExitSuccess, result.code, result.stderr)
		assertBackupCounts(t, result.stdout, 1, 2, 0)
		assertBackupArchiveBoards(
			t,
			archivePath,
			*namespace.BoardID,
			engineeringBoardID,
		)
	})

	t.Run("AllBoards", func(t *testing.T) {
		archivePath := filepath.Join(cfg.CWD, "all.cardamom")
		result := execute(
			t,
			cfg,
			"--json",
			"backup",
			archivePath,
			"--all",
		)

		require.Equal(t, cli.ExitSuccess, result.code, result.stderr)
		assertBackupCounts(t, result.stdout, 1, 2, 0)
		assertBackupArchiveBoards(
			t,
			archivePath,
			*namespace.BoardID,
			engineeringBoardID,
		)
	})
}

func TestRestoreCommand_createsDestinationWithoutConfigurationOrBinding(t *testing.T) {
	t.Setenv("CARDAMOM_STORE", "")
	t.Setenv("CARDAMOM_BOARD", "")
	cfg := testConfig(t)
	initialized := execute(t, cfg, "init", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)
	archivePath := filepath.Join(cfg.CWD, "source.cardamom")
	backedUp := execute(t, cfg, "backup", archivePath)
	require.Equal(t, cli.ExitSuccess, backedUp.code, backedUp.stderr)

	destination := filepath.Join(cfg.CWD, "restored", ".cardamom")
	restored := execute(
		t,
		cfg,
		"--json",
		"--store",
		destination,
		"restore",
		archivePath,
	)

	require.Equal(t, cli.ExitSuccess, restored.code, restored.stderr)
	assertRestoreCounts(t, restored.stdout, 1, 1, 0, 0)
	assert.FileExists(t, filepath.Join(destination, "board.sqlite3"))
	assert.NoFileExists(t, filepath.Join(destination, "config.yaml"))
	assert.NoFileExists(t, filepath.Join(cfg.CWD, "restored", ".cardamom-board"))

	reapplied := execute(
		t,
		cfg,
		"--json",
		"--store",
		destination,
		"restore",
		archivePath,
	)
	require.Equal(t, cli.ExitSuccess, reapplied.code, reapplied.stderr)
	assertRestoreCounts(t, reapplied.stdout, 1, 1, 0, 1)
}

func TestRestoreCommand_rejectsCorruptArchiveBeforeCreatingDestination(t *testing.T) {
	t.Setenv("CARDAMOM_STORE", "")
	t.Setenv("CARDAMOM_BOARD", "")
	cfg := testConfig(t)
	initialized := execute(t, cfg, "init", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)
	archivePath := filepath.Join(cfg.CWD, "corrupt.cardamom")
	backedUp := execute(t, cfg, "backup", archivePath)
	require.Equal(t, cli.ExitSuccess, backedUp.code, backedUp.stderr)
	corruptBackupBoard(t, archivePath)

	destination := filepath.Join(cfg.CWD, "missing", ".cardamom")
	restored := execute(
		t,
		cfg,
		"--store",
		destination,
		"restore",
		archivePath,
	)

	assert.Equal(t, cli.ExitOperation, restored.code)
	assert.Contains(t, restored.stderr, "content digest mismatch")
	_, err := os.Stat(destination)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(destination, "board.sqlite3"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func assertBackupCounts(
	t *testing.T,
	body string,
	projects int,
	boards int,
	blobs int,
) {
	t.Helper()
	var result cli.BackupResult
	require.NoError(t, json.Unmarshal([]byte(body), &result))
	assert.Equal(t, projects, result.Projects)
	assert.Equal(t, boards, result.Boards)
	assert.Equal(t, blobs, result.Blobs)
	assert.NotEmpty(t, result.Source)
	assert.NotEmpty(t, result.Destination)
}

func assertRestoreCounts(
	t *testing.T,
	body string,
	projects int,
	boards int,
	blobs int,
	alreadyCompleted int,
) {
	t.Helper()
	var result cli.RestoreResult
	require.NoError(t, json.Unmarshal([]byte(body), &result))
	assert.Equal(t, projects, result.Projects)
	assert.Equal(t, boards, result.Boards)
	assert.Equal(t, blobs, result.Blobs)
	assert.Equal(t, alreadyCompleted, result.AlreadyCompletedBoards)
	assert.NotEmpty(t, result.Source)
	assert.NotEmpty(t, result.Destination)
}

func assertBackupArchiveBoards(
	t *testing.T,
	archivePath string,
	want ...string,
) {
	t.Helper()
	archive, err := os.Open(archivePath)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, archive.Close()) })
	info, err := archive.Stat()
	require.NoError(t, err)
	reader, err := domainbackup.NewReader(archive, info.Size())
	require.NoError(t, err)
	got := make([]string, 0, len(reader.Boards()))
	for _, publication := range reader.Boards() {
		got = append(got, publication.SourceBoardID.String())
	}
	assert.ElementsMatch(t, want, got)
}

func corruptBackupBoard(t *testing.T, archivePath string) {
	t.Helper()
	body, err := os.ReadFile(archivePath)
	require.NoError(t, err)
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	var changed bytes.Buffer
	writer := zip.NewWriter(&changed)
	found := false
	for _, file := range reader.File {
		source, err := file.Open()
		require.NoError(t, err)
		memberBody, err := io.ReadAll(source)
		require.NoError(t, err)
		require.NoError(t, source.Close())
		if strings.HasPrefix(file.Name, "boards/") {
			corrupt := bytes.Replace(memberBody, []byte("Bridge"), []byte("bridge"), 1)
			require.NotEqual(t, memberBody, corrupt)
			memberBody = corrupt
			found = true
		}
		member, err := writer.CreateHeader(&zip.FileHeader{
			Name: file.Name, Method: zip.Store,
		})
		require.NoError(t, err)
		_, err = member.Write(memberBody)
		require.NoError(t, err)
	}
	require.True(t, found)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(archivePath, changed.Bytes(), 0o600))
}
