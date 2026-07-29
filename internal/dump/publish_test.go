package dump

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishCreatesOwnedFilesAndSafelyReruns(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	rendered := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md":      {identity: "board", body: "# Board\n"},
		"issues/an-a.md": {identity: "issue:an-a", body: "# Issue A\n"},
	})

	result, err := publishDump(t, Publication{Destination: destination, Rendered: rendered})
	require.NoError(t, err)
	assert.Equal(t, PublicationResult{Written: 2}, result)
	metadata, body := readGeneratedFile(t, filepath.Join(destination, "issues/an-a.md"))
	assert.Equal(t, ownershipVersion, metadata.Version)
	assert.Equal(t, "project-1", metadata.ProjectID)
	assert.Equal(t, "Project one", metadata.ProjectName)
	assert.Equal(t, "board-1", metadata.BoardID)
	assert.Equal(t, "Board one", metadata.BoardName)
	assert.Equal(t, "issue:an-a", metadata.Identity)
	assert.Equal(t, digest([]byte(body)), metadata.BodySHA256)
	assert.Equal(t, "# Issue A\n", body)
	assert.NoFileExists(t, filepath.Join(destination, ".cardamom-json"))

	issuePath := filepath.Join(destination, "issues/an-a.md")
	before, err := os.Stat(issuePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(destination, "states.txt"), []byte("unowned\n"), 0o644))
	time.Sleep(20 * time.Millisecond)

	result, err = publishDump(t, Publication{Destination: destination, Rendered: rendered})
	require.NoError(t, err)
	assert.Equal(t, PublicationResult{Unchanged: 2}, result)
	after, err := os.Stat(issuePath)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime())
	assert.Equal(t, "unowned\n", readFile(t, filepath.Join(destination, "states.txt")))
}

func TestPublishRejectsGeneratedFileWithoutOwnershipMetadata(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	_, err := publishDump(t, Publication{
		Destination: destination,
		Rendered: RenderedDump{
			Provenance: testDumpProvenance("board-1"),
			Selection:  WholeBoard(),
			Files: []*GeneratedFile{
				testGeneratedFile(t, "README.md", "board", []byte("# Board\n")),
			},
		},
	})

	assert.ErrorContains(t, err, `generated path "README.md" has invalid ownership metadata`)
	assert.NoDirExists(t, destination)
}

func TestPublishStreamsAndClosesGeneratedFile(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	body := bytes.Repeat([]byte("streamed body\n"), 16*1024)
	content, err := encodeOwnedFile(testDumpProvenance("board-1"), "board", body)
	require.NoError(t, err)
	source := &publicationReadRecorder{
		Reader:  bytes.NewReader(content),
		maxRead: 32 * 1024,
	}
	generated, err := NewGeneratedFile(GeneratedFileConfig{
		Path: "README.md", Identity: "board", Size: int64(len(content)),
		Open: func() (io.ReadCloser, error) { return source, nil },
	})
	require.NoError(t, err)

	result, err := publishDump(t, Publication{
		Destination: destination,
		Rendered: RenderedDump{
			Provenance: testDumpProvenance("board-1"),
			Selection:  WholeBoard(),
			Files:      []*GeneratedFile{generated},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, PublicationResult{Written: 1}, result)
	assert.True(t, source.closed)
	assert.Greater(t, source.readCalls, 1)
	_, publishedBody := readGeneratedFile(t, filepath.Join(destination, "README.md"))
	assert.Equal(t, string(body), publishedBody)
}

func TestPublishPreservesGeneratedSourceReadFailure(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	readErr := errors.New("source read failed")
	source := &errorReadCloser{err: readErr}
	generated, err := NewGeneratedFile(GeneratedFileConfig{
		Path: "README.md", Identity: "board", Size: 1,
		Open: func() (io.ReadCloser, error) { return source, nil },
	})
	require.NoError(t, err)

	_, err = publishDump(t, Publication{
		Destination: destination,
		Rendered: RenderedDump{
			Provenance: testDumpProvenance("board-1"),
			Selection:  WholeBoard(),
			Files:      []*GeneratedFile{generated},
		},
	})
	assert.ErrorIs(t, err, readErr)
	assert.True(t, source.closed)
	assert.NoDirExists(t, destination)
}

func TestPublishSelectedFilesIsAdditive(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	whole := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md":      {identity: "board", body: "whole\n"},
		"issues/an-a.md": {identity: "issue:an-a", body: "a old\n"},
		"issues/an-b.md": {identity: "issue:an-b", body: "b\n"},
	})
	_, err := publishDump(t, Publication{Destination: destination, Rendered: whole})
	require.NoError(t, err)

	selected := testRenderedDump(t, "board-1", NamedIssuesOnly("an-a"), map[string]renderedFile{
		"issues/an-a.md": {identity: "issue:an-a", body: "a new\n"},
	})
	result, err := publishDump(t, Publication{Destination: destination, Rendered: selected})
	require.NoError(t, err)
	assert.Equal(t, PublicationResult{Written: 1}, result)
	_, readme := readGeneratedFile(t, filepath.Join(destination, "README.md"))
	_, unselected := readGeneratedFile(t, filepath.Join(destination, "issues/an-b.md"))
	assert.Equal(t, "whole\n", readme)
	assert.Equal(t, "b\n", unselected)
}

func TestPublishRequiresForceForModifiedGeneratedFile(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	prior := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md": {identity: "board", body: "old\n"},
	})
	_, err := publishDump(t, Publication{Destination: destination, Rendered: prior})
	require.NoError(t, err)
	path := filepath.Join(destination, "README.md")
	require.NoError(t, os.WriteFile(path, []byte(readFile(t, path)+"maintainer edit\n"), 0o644))

	next := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md": {identity: "board", body: "new\n"},
	})
	_, err = publishDump(t, Publication{Destination: destination, Rendered: next})
	var generated *GeneratedFileError
	require.ErrorAs(t, err, &generated)
	assert.Equal(t, "README.md", generated.Path)
	assert.Equal(t, GeneratedFileModified, generated.Kind)

	result, err := publishDump(t, Publication{
		Destination: destination, Rendered: next, Force: ForceGenerated,
	})
	require.NoError(t, err)
	assert.Equal(t, PublicationResult{Written: 1}, result)
	_, body := readGeneratedFile(t, path)
	assert.Equal(t, "new\n", body)
}

func TestPublishRejectsCollisionsAndCrossBoardOwnership(t *testing.T) {
	t.Run("Unowned", func(t *testing.T) {
		destination := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(destination, "README.md"), []byte("mine\n"), 0o644))
		_, err := publishDump(t, Publication{
			Destination: destination,
			Rendered: testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
				"README.md": {identity: "board", body: "generated\n"},
			}),
		})
		assert.ErrorContains(t, err, `generated path "README.md" collides with an unowned file`)
		assert.Equal(t, "mine\n", readFile(t, filepath.Join(destination, "README.md")))
	})

	t.Run("CrossBoard", func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "dump")
		prior := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
			"README.md": {identity: "board", body: "one\n"},
		})
		_, err := publishDump(t, Publication{Destination: destination, Rendered: prior})
		require.NoError(t, err)
		next := testRenderedDump(t, "board-2", WholeBoard(), map[string]renderedFile{
			"README.md": {identity: "board", body: "two\n"},
		})
		_, err = publishDump(t, Publication{Destination: destination, Rendered: next, Force: ForceGenerated})
		assert.ErrorContains(t, err, `generated path "README.md" belongs to board "board-1", not "board-2"`)
	})
}

func TestPublishMigratesLegacyMissionPage(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	writeGeneratedFile(t, filepath.Join(destination, "missions/an-a.md"), "board-1", "issue:an-a", "mission\n")

	next := testRenderedDump(t, "board-1", NamedIssuesOnly("an-a"), map[string]renderedFile{
		"issues/an-a.md": {identity: "issue:an-a", body: "workstream\n"},
	})
	result, err := publishDump(t, Publication{Destination: destination, Rendered: next})
	require.NoError(t, err)
	assert.Equal(t, PublicationResult{Written: 1, Removed: 1}, result)
	assert.NoFileExists(t, filepath.Join(destination, "missions/an-a.md"))
	_, body := readGeneratedFile(t, filepath.Join(destination, "issues/an-a.md"))
	assert.Equal(t, "workstream\n", body)
}

func TestPublishLeavesUnownedLegacyMissionPageUntouched(t *testing.T) {
	destination := t.TempDir()
	oldPath := filepath.Join(destination, "missions/an-a.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(oldPath), 0o755))
	require.NoError(t, os.WriteFile(oldPath, []byte("maintainer page\n"), 0o644))
	next := testRenderedDump(t, "board-1", NamedIssuesOnly("an-a"), map[string]renderedFile{
		"issues/an-a.md": {identity: "issue:an-a", body: "workstream\n"},
	})

	result, err := publishDump(t, Publication{Destination: destination, Rendered: next})
	require.NoError(t, err)
	assert.Equal(t, PublicationResult{Written: 1}, result)
	assert.Equal(t, "maintainer page\n", readFile(t, oldPath))
}

func TestPublishRequiresForceToMigrateModifiedLegacyMissionPage(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	oldPath := filepath.Join(destination, "missions/an-a.md")
	writeGeneratedFile(t, oldPath, "board-1", "issue:an-a", "mission\n")
	require.NoError(t, os.WriteFile(oldPath, []byte(readFile(t, oldPath)+"edit\n"), 0o644))

	next := testRenderedDump(t, "board-1", NamedIssuesOnly("an-a"), map[string]renderedFile{
		"issues/an-a.md": {identity: "issue:an-a", body: "workstream\n"},
	})
	_, err := publishDump(t, Publication{Destination: destination, Rendered: next})
	var generated *GeneratedFileError
	require.ErrorAs(t, err, &generated)
	assert.Equal(t, "missions/an-a.md", generated.Path)

	result, err := publishDump(t, Publication{
		Destination: destination, Rendered: next, Force: ForceGenerated,
	})
	require.NoError(t, err)
	assert.Equal(t, PublicationResult{Written: 1, Removed: 1}, result)
}

func TestPublishRollsBackFailedLegacyMissionMigration(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	writeGeneratedFile(t, filepath.Join(destination, "missions/an-a.md"), "board-1", "issue:an-a", "mission\n")
	next := testRenderedDump(t, "board-1", NamedIssuesOnly("an-a"), map[string]renderedFile{
		"issues/an-a.md": {identity: "issue:an-a", body: "workstream\n"},
	})
	failing := &failingFS{
		fileSystem: osFileSystem{}, failBackupFromSuffix: filepath.FromSlash("missions/an-a.md"),
		err: errors.New("injected legacy-migration failure"),
	}

	_, err := publish(t.Context(), Publication{Destination: destination, Rendered: next}, failing)
	assert.ErrorContains(t, err, "injected legacy-migration failure")
	assert.NoFileExists(t, filepath.Join(destination, "issues/an-a.md"))
	_, body := readGeneratedFile(t, filepath.Join(destination, "missions/an-a.md"))
	assert.Equal(t, "mission\n", body)
}

func TestPublishRejectsUnsafePathsAndSymlinkTraversal(t *testing.T) {
	t.Run("UnsafeRenderedPath", func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "dump")
		_, err := publishDump(t, Publication{
			Destination: destination,
			Rendered: testRenderedDump(t, "board-1", NamedIssuesOnly("an-a"), map[string]renderedFile{
				"../escape.md": {identity: "issue:an-a", body: "escape\n"},
			}),
		})
		assert.ErrorContains(t, err, `generated path "../escape.md" is not a canonical dump path`)
		assert.NoDirExists(t, destination)
	})

	t.Run("SymlinkParent", func(t *testing.T) {
		destination := t.TempDir()
		require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(destination, "issues")))
		_, err := publishDump(t, Publication{
			Destination: destination,
			Rendered: testRenderedDump(t, "board-1", NamedIssuesOnly("an-a"), map[string]renderedFile{
				"issues/an-a.md": {identity: "issue:an-a", body: "issue\n"},
			}),
		})
		assert.ErrorContains(t, err, `generated path "issues/an-a.md" traverses symbolic link "issues"`)
	})
}

func TestFilePublisher_PublishCanceledBeforeMutation(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := (&FilePublisher{}).Publish(ctx, Publication{
		Destination: destination,
		Rendered: testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
			"README.md": {identity: "board", body: "readme\n"},
		}),
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.NoDirExists(t, destination)
}

func TestPublishRollsBackFailure(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	prior := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md":      {identity: "board", body: "old readme\n"},
		"issues/an-a.md": {identity: "issue:an-a", body: "old issue\n"},
	})
	_, err := publishDump(t, Publication{Destination: destination, Rendered: prior})
	require.NoError(t, err)
	next := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md":      {identity: "board", body: "new readme\n"},
		"issues/an-a.md": {identity: "issue:an-a", body: "new issue\n"},
	})
	failing := &failingFS{
		fileSystem: osFileSystem{}, failRenameToSuffix: filepath.FromSlash("issues/an-a.md"),
		err: errors.New("injected rename failure"),
	}
	_, err = publish(t.Context(), Publication{Destination: destination, Rendered: next}, failing)
	assert.ErrorContains(t, err, "injected rename failure")
	var partial *PartialRecoveryError
	assert.False(t, errors.As(err, &partial))
	_, readme := readGeneratedFile(t, filepath.Join(destination, "README.md"))
	_, issue := readGeneratedFile(t, filepath.Join(destination, "issues/an-a.md"))
	assert.Equal(t, "old readme\n", readme)
	assert.Equal(t, "old issue\n", issue)
}

func TestPublishReportsAndRetainsStagingWhenRollbackFails(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	prior := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md":      {identity: "board", body: "old\n"},
		"issues/an-a.md": {identity: "issue:an-a", body: "old\n"},
	})
	_, err := publishDump(t, Publication{Destination: destination, Rendered: prior})
	require.NoError(t, err)
	next := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md":      {identity: "board", body: "new\n"},
		"issues/an-a.md": {identity: "issue:an-a", body: "new\n"},
	})
	failing := &failingFS{
		fileSystem: osFileSystem{}, failRenameToSuffix: filepath.FromSlash("issues/an-a.md"),
		failRollback: true, err: errors.New("injected publication failure"),
	}
	_, err = publish(t.Context(), Publication{Destination: destination, Rendered: next}, failing)
	var partial *PartialRecoveryError
	require.ErrorAs(t, err, &partial)
	assert.DirExists(t, partial.RecoveryDirectory)
}

func TestPublishRollbackRestoresAbsentDestination(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	rendered := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md":      {identity: "board", body: "readme\n"},
		"issues/an-a.md": {identity: "issue:an-a", body: "issue\n"},
	})
	failing := &failingFS{
		fileSystem: osFileSystem{}, failRenameToSuffix: filepath.FromSlash("issues/an-a.md"),
		err: errors.New("injected first-publication failure"),
	}

	_, err := publish(t.Context(), Publication{Destination: destination, Rendered: rendered}, failing)
	assert.ErrorContains(t, err, "injected first-publication failure")
	assert.NoDirExists(t, destination)
}

func TestPublishCleansAbsentDestinationWhenStagingCreationFails(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "dump")
	rendered := testRenderedDump(t, "board-1", WholeBoard(), map[string]renderedFile{
		"README.md": {identity: "board", body: "readme\n"},
	})
	stagingErr := errors.New("injected staging creation failure")
	failing := &failingFS{
		fileSystem:    osFileSystem{},
		failMkdirTemp: true,
		err:           stagingErr,
	}

	_, err := publish(t.Context(), Publication{
		Destination: destination,
		Rendered:    rendered,
	}, failing)
	assert.ErrorIs(t, err, stagingErr)
	assert.NoDirExists(t, destination)
}

type failingFS struct {
	fileSystem
	failRenameToSuffix   string
	failBackupFromSuffix string
	failMkdirTemp        bool
	failRollback         bool
	failed               bool
	err                  error
}

func (f *failingFS) MkdirTemp(directory, pattern string) (string, error) {
	if f.failMkdirTemp {
		return "", f.err
	}
	return f.fileSystem.MkdirTemp(directory, pattern)
}

type publicationReadRecorder struct {
	*bytes.Reader
	maxRead   int
	readCalls int
	closed    bool
}

type errorReadCloser struct {
	err    error
	closed bool
}

func (r *errorReadCloser) Read([]byte) (int, error) { return 0, r.err }

func (r *errorReadCloser) Close() error {
	r.closed = true
	return nil
}

func (r *publicationReadRecorder) Read(body []byte) (int, error) {
	if len(body) > r.maxRead {
		return 0, fmt.Errorf("read buffer is %d bytes, maximum is %d", len(body), r.maxRead)
	}
	r.readCalls++
	return r.Reader.Read(body)
}

func (r *publicationReadRecorder) Close() error {
	r.closed = true
	return nil
}

func (f *failingFS) Rename(oldPath, newPath string) error {
	if !f.failed && f.failRenameToSuffix != "" && strings.HasSuffix(newPath, f.failRenameToSuffix) && strings.Contains(oldPath, string(filepath.Separator)+"next"+string(filepath.Separator)) {
		f.failed = true
		return f.err
	}
	if !f.failed && f.failBackupFromSuffix != "" && strings.HasSuffix(oldPath, f.failBackupFromSuffix) && strings.Contains(newPath, string(filepath.Separator)+"backup"+string(filepath.Separator)) {
		f.failed = true
		return f.err
	}
	if f.failed && f.failRollback && strings.Contains(oldPath, string(filepath.Separator)+"backup"+string(filepath.Separator)) {
		return errors.New("injected rollback failure")
	}
	return f.fileSystem.Rename(oldPath, newPath)
}

type renderedFile struct {
	identity string
	body     string
}

func testRenderedDump(
	t *testing.T,
	boardID string,
	selection Selection,
	files map[string]renderedFile,
) RenderedDump {
	t.Helper()
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	rendered := RenderedDump{
		Provenance: testDumpProvenance(boardID), Selection: selection,
	}
	for _, path := range paths {
		file := files[path]
		body, err := encodeOwnedFile(
			rendered.Provenance,
			file.identity,
			[]byte(file.body),
		)
		require.NoError(t, err)
		rendered.Files = append(rendered.Files, testGeneratedFile(
			t,
			path,
			file.identity,
			body,
		))
		if strings.HasPrefix(file.identity, "issue:") {
			rendered.IssueCount++
		}
	}
	return rendered
}

func testGeneratedFile(t *testing.T, path, identity string, body []byte) *GeneratedFile {
	t.Helper()
	file, err := NewGeneratedFile(GeneratedFileConfig{
		Path: path, Identity: identity, Size: int64(len(body)),
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		},
	})
	require.NoError(t, err)
	return file
}

func publishDump(t *testing.T, publication Publication) (PublicationResult, error) {
	t.Helper()
	return (&FilePublisher{}).Publish(t.Context(), publication)
}

func readGeneratedFile(t *testing.T, path string) (ownershipMetadata, string) {
	t.Helper()
	metadata, body, err := decodeOwnedFile([]byte(readFile(t, path)))
	require.NoError(t, err)
	return metadata, string(body)
}

func writeGeneratedFile(t *testing.T, path, boardID, identity, body string) {
	t.Helper()
	encoded, err := encodeOwnedFile(testDumpProvenance(boardID), identity, []byte(body))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, encoded, 0o644))
}

func testDumpProvenance(boardID string) Provenance {
	return Provenance{
		ProjectID: "project-1", ProjectName: "Project one",
		BoardID: boardID, BoardName: "Board one",
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}
