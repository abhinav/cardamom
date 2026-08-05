package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/project"
	projectcreation "go.abhg.dev/cardamom/internal/project/creation"
	repositoryattachment "go.abhg.dev/cardamom/internal/repository/attachment"
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	repositoryproject "go.abhg.dev/cardamom/internal/repository/project"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRestoreService_preservesDestinationAndReappliesIdentically(t *testing.T) {
	createdAt := restoreTestTime()
	alpha := restoreTestProject(t, "project-alpha", "Alpha", createdAt)
	beta := restoreTestProject(t, "project-beta", "Beta", createdAt.Add(time.Hour))
	archive := restoreTestArchive(t, []project.Snapshot{alpha, beta}, []restoreTestBoard{
		{projectID: alpha.ID, id: "board-alpha", name: "Alpha board", suffix: "a"},
		{projectID: beta.ID, id: "board-beta", name: "Beta board", suffix: "b"},
	}, true)

	destination := newRestoreTestDestination(t, restoreTestIDs{
		"project": {alpha.ID.String(), "project-unrelated"},
		"board":   {"board-unrelated"},
	}, createdAt)
	_, err := destination.projects.CreateProject(t.Context(), projectcreation.Creation{
		Name: alpha.Name,
	})
	require.NoError(t, err)
	unrelated, err := destination.projects.CreateProject(
		t.Context(),
		projectcreation.Creation{Name: "Unrelated"},
	)
	require.NoError(t, err)
	_, err = destination.projects.CreateBoard(t.Context(), board.CreateRequest{
		ProjectID: unrelated.ID().String(),
		Name:      "Unrelated board",
	})
	require.NoError(t, err)

	reader := restoreTestReader(t, archive)
	result, err := destination.service.Restore(t.Context(), reader)
	require.NoError(t, err)
	assert.Equal(t, []project.Snapshot{alpha, beta}, result.Projects)
	assert.Equal(t, 1, result.BlobCount)
	require.Len(t, result.Boards, 2)
	assert.Equal(t, alpha.ID.String(), result.Boards[0].DestinationProjectID)
	assert.Equal(t, beta.ID.String(), result.Boards[1].DestinationProjectID)
	assert.False(t, result.Boards[0].AlreadyCompleted)
	assert.False(t, result.Boards[1].AlreadyCompleted)

	projects, err := destination.projects.ListProjects(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"Alpha", "Beta", "Unrelated"}, projectNames(projects))
	assert.Equal(t, alpha.ID, projects[0].ID())
	assert.True(t, alpha.Created.Equal(projects[0].Created()))
	assert.Equal(t, beta.ID, projects[1].ID())
	assert.True(t, beta.Created.Equal(projects[1].Created()))
	boards, err := destination.projects.ListAllBoards(t.Context())
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"Alpha board", "Beta board", "Unrelated board"},
		boardNames(boards),
	)
	blobs, err := os.ReadDir(filepath.Join(destination.directory, "blobs", "sha256"))
	require.NoError(t, err)
	assert.Len(t, blobs, 1)

	reapplied, err := destination.service.Restore(
		t.Context(),
		restoreTestReader(t, archive),
	)
	require.NoError(t, err)
	require.Len(t, reapplied.Boards, 2)
	assert.True(t, reapplied.Boards[0].AlreadyCompleted)
	assert.True(t, reapplied.Boards[1].AlreadyCompleted)
	projects, err = destination.projects.ListProjects(t.Context())
	require.NoError(t, err)
	assert.Len(t, projects, 3)
	boards, err = destination.projects.ListAllBoards(t.Context())
	require.NoError(t, err)
	assert.Len(t, boards, 3)
}

func TestRestoreService_rejectsIncompatibleProjectIdentity(t *testing.T) {
	createdAt := restoreTestTime()
	archived := restoreTestProject(t, "project-alpha", "Archived", createdAt)
	archive := restoreTestArchive(t, []project.Snapshot{archived}, []restoreTestBoard{
		{projectID: archived.ID, id: "board-alpha", name: "Alpha board", suffix: "a"},
	}, true)
	destination := newRestoreTestDestination(t, restoreTestIDs{
		"project": {archived.ID.String()},
	}, createdAt)
	_, err := destination.projects.CreateProject(t.Context(), projectcreation.Creation{
		Name: "Retained",
	})
	require.NoError(t, err)

	_, err = destination.service.Restore(
		t.Context(),
		restoreTestReader(t, archive),
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, `project "project-alpha" conflicts with archived metadata`)
	projects, err := destination.projects.ListProjects(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"Retained"}, projectNames(projects))
	boards, err := destination.projects.ListAllBoards(t.Context())
	require.NoError(t, err)
	assert.Empty(t, boards)
	_, err = os.Stat(filepath.Join(destination.directory, "blobs", "sha256"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRestoreService_rejectsArchiveFailuresBeforeMutation(t *testing.T) {
	createdAt := restoreTestTime()
	archived := restoreTestProject(t, "project-alpha", "Alpha", createdAt)
	complete := restoreTestArchive(t, []project.Snapshot{archived}, []restoreTestBoard{
		{projectID: archived.ID, id: "board-alpha", name: "Alpha board", suffix: "a"},
	}, true)

	t.Run("CorruptBlob", func(t *testing.T) {
		corrupt := rewriteZip(t, complete, func(name string, body []byte) ([]byte, bool) {
			if strings.HasPrefix(name, "blobs/") {
				body[0] ^= 1
			}
			return body, true
		})
		destination := newRestoreTestDestination(t, nil, createdAt)

		_, err := destination.service.Restore(
			t.Context(),
			restoreTestReader(t, corrupt),
		)
		require.Error(t, err)
		assert.ErrorContains(t, err, "content digest mismatch")
		assertRestoreDestinationEmpty(t, destination)
	})

	t.Run("UnindexedAttachmentBlob", func(t *testing.T) {
		incomplete := restoreTestArchive(t, []project.Snapshot{archived}, []restoreTestBoard{
			{projectID: archived.ID, id: "board-alpha", name: "Alpha board", suffix: "a"},
		}, false)
		destination := newRestoreTestDestination(t, nil, createdAt)

		_, err := destination.service.Restore(
			t.Context(),
			restoreTestReader(t, incomplete),
		)
		require.Error(t, err)
		assert.ErrorContains(t, err, "references unindexed blob")
		assertRestoreDestinationEmpty(t, destination)
	})
}

func TestRestoreService_resumesAfterCommittedBoard(t *testing.T) {
	createdAt := restoreTestTime()
	archived := restoreTestProject(t, "project-alpha", "Alpha", createdAt)
	archive := restoreTestArchive(t, []project.Snapshot{archived}, []restoreTestBoard{
		{projectID: archived.ID, id: "board-alpha", name: "Alpha board", suffix: "a"},
		{projectID: archived.ID, id: "board-beta", name: "Beta board", suffix: "b"},
	}, true)
	destination := newRestoreTestDestination(t, nil, createdAt)
	failingBoards := &failSecondBoardDestination{
		CopySnapshotDestination: destination.boards,
		fail:                    true,
	}
	service := NewRestoreService(RestoreServiceConfig{
		Projects: destination.projects,
		Boards:   failingBoards,
		Blobs:    destination.blobs,
	})

	_, err := service.Restore(t.Context(), restoreTestReader(t, archive))
	assert.EqualError(t, err, `restore board "board-beta": injected board import failure`)
	boards, err := destination.projects.ListAllBoards(t.Context())
	require.NoError(t, err)
	require.Len(t, boards, 1)
	assert.Equal(t, "Alpha board", boards[0].Name())

	failingBoards.fail = false
	result, err := service.Restore(t.Context(), restoreTestReader(t, archive))
	require.NoError(t, err)
	require.Len(t, result.Boards, 2)
	assert.True(t, result.Boards[0].AlreadyCompleted)
	assert.False(t, result.Boards[1].AlreadyCompleted)
	boards, err = destination.projects.ListAllBoards(t.Context())
	require.NoError(t, err)
	assert.Len(t, boards, 2)
}

type restoreTestDestination struct {
	directory string
	projects  *repositoryproject.Repository
	boards    *repositoryboard.CopyRepository
	blobs     *repositoryattachment.Repository
	service   *RestoreService
}

func newRestoreTestDestination(
	t *testing.T,
	ids restoreTestIDs,
	createdAt time.Time,
) *restoreTestDestination {
	t.Helper()
	directory := t.TempDir()
	persistence, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(directory, "cardamom.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	projects := repositoryproject.New(persistence, repositoryproject.Config{
		Clock:    restoreTestClock{now: createdAt},
		IDSource: &ids,
	})
	boards, err := repositoryboard.NewCopyRepository(
		persistence,
		repositoryboard.CopyRepositoryConfig{},
	)
	require.NoError(t, err)
	blobs, err := repositoryattachment.New(persistence, repositoryattachment.Config{
		StoreDirectory: directory,
	})
	require.NoError(t, err)
	return &restoreTestDestination{
		directory: directory,
		projects:  projects,
		boards:    boards,
		blobs:     blobs,
		service: NewRestoreService(RestoreServiceConfig{
			Projects: projects,
			Boards:   boards,
			Blobs:    blobs,
		}),
	}
}

func assertRestoreDestinationEmpty(t *testing.T, destination *restoreTestDestination) {
	t.Helper()
	projects, err := destination.projects.ListProjects(t.Context())
	require.NoError(t, err)
	assert.Empty(t, projects)
	boards, err := destination.projects.ListAllBoards(t.Context())
	require.NoError(t, err)
	assert.Empty(t, boards)
	_, err = os.Stat(filepath.Join(destination.directory, "blobs", "sha256"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

type failSecondBoardDestination struct {
	boardcopy.CopySnapshotDestination
	imports int
	fail    bool
}

func (d *failSecondBoardDestination) ImportCopySnapshot(
	ctx context.Context,
	snapshot boardcopy.CopySnapshot,
	options boardcopy.CopyOptions,
) (boardcopy.CopyImportResult, error) {
	d.imports++
	if d.fail && d.imports == 2 {
		return boardcopy.CopyImportResult{}, errors.New("injected board import failure")
	}
	return d.CopySnapshotDestination.ImportCopySnapshot(ctx, snapshot, options)
}

type restoreTestClock struct {
	now time.Time
}

func (c restoreTestClock) Now() time.Time { return c.now }

type restoreTestIDs map[string][]string

func (s *restoreTestIDs) NewID(kind string) (string, error) {
	values := (*s)[kind]
	if len(values) == 0 {
		return "", fmt.Errorf("no test %s identity available", kind)
	}
	value := values[0]
	(*s)[kind] = values[1:]
	return value, nil
}

type restoreTestBoard struct {
	projectID project.ID
	id        string
	name      string
	suffix    string
}

func restoreTestArchive(
	t *testing.T,
	projects []project.Snapshot,
	boards []restoreTestBoard,
	includeBlob bool,
) []byte {
	t.Helper()
	descriptor, body := restoreTestBlob(t)
	var archive bytes.Buffer
	writer := NewWriter(&archive)
	for _, snapshot := range projects {
		require.NoError(t, writer.AddProject(snapshot))
	}
	for _, publication := range boards {
		require.NoError(t, writer.AddBoard(
			publication.projectID,
			restoreTestBoardSnapshot(publication, descriptor),
		))
	}
	if includeBlob {
		require.NoError(t, writer.AddBlob(descriptor, bytes.NewReader(body)))
	}
	require.NoError(t, writer.Close())
	return archive.Bytes()
}

func restoreTestBoardSnapshot(
	publication restoreTestBoard,
	descriptor attachment.BlobDescriptor,
) boardcopy.CopySnapshot {
	createdAt := restoreTestTime()
	issueID := "cm-" + publication.suffix
	attachmentID := "att_" + strings.Repeat(publication.suffix, 25) + "a"
	return boardcopy.CopySnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  12,
		Board: boardcopy.CopyBoard{
			ID: publication.id, Name: publication.name, CreatedAt: createdAt,
		},
		Configuration: configuration.Defaults(),
		Issues: []boardcopy.CopyIssue{{
			ID: issueID, Title: "Issue " + publication.suffix,
			Kind: "task", Lifecycle: "open", Priority: 2,
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}},
		Attachments: []boardcopy.CopyAttachment{{
			ID: attachmentID, OriginIssueID: new(issueID), Blob: descriptor,
			Filename:  "evidence-" + publication.suffix + ".txt",
			MediaType: "text/plain", Lifecycle: "active",
			CreatedActor: "worker", CreatedAt: createdAt,
		}},
	}
}

func restoreTestProject(
	t *testing.T,
	id string,
	name string,
	createdAt time.Time,
) project.Snapshot {
	t.Helper()
	projectID, err := project.NewID(id)
	require.NoError(t, err)
	return project.Snapshot{ID: projectID, Name: name, Created: createdAt}
}

func restoreTestBlob(t *testing.T) (attachment.BlobDescriptor, []byte) {
	t.Helper()
	body := []byte("shared backup blob")
	digest, err := attachment.NewDigest(
		"sha256:669296cf96c0c165827f40bbb41d50a5ba95eca2b56dccba3f095541e8b5a0dd",
	)
	require.NoError(t, err)
	return attachment.BlobDescriptor{
		Digest: digest, SizeBytes: uint64(len(body)),
	}, body
}

func restoreTestReader(t *testing.T, archive []byte) *Reader {
	t.Helper()
	reader, err := NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	return reader
}

func restoreTestTime() time.Time {
	return time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
}

func projectNames(values []*project.State) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.Name())
	}
	return out
}

func boardNames(values []*board.State) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.Name())
	}
	return out
}
