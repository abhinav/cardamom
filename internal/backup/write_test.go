package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/project"
	"go.uber.org/mock/gomock"
)

func TestOperation_WriteSelectsBoardsAndDeduplicatesBlobs(t *testing.T) {
	descriptor := testWriteBlobDescriptor(t)
	projectOne := testWriteProject("project-one")
	projectTwo := testWriteProject("project-two")
	boards := []writeTestCapturedBoard{
		testWriteCapturedBoard(projectOne.ID, "board-one", descriptor),
		testWriteCapturedBoard(projectTwo.ID, "board-two", descriptor),
	}

	tests := []struct {
		name         string
		selection    Selection
		wantProjects int
		wantBoards   []board.ID
	}{
		{
			name:         "AllBoards",
			selection:    AllBoards(),
			wantProjects: 2,
			wantBoards:   []board.ID{"board-one", "board-two"},
		},
		{
			name:         "SelectedBoards",
			selection:    mustSelectBoards(t, "board-two"),
			wantProjects: 1,
			wantBoards:   []board.ID{"board-two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			destination := filepath.Join(directory, "portable-backup")
			source := &writeTestSource{
				projects: []project.Snapshot{projectOne, projectTwo},
				boards:   boards,
			}
			blobs := &writeTestBlobs{body: []byte("data")}
			configurations := NewMockStoreConfiguration(gomock.NewController(t))
			configurations.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(
				configuration.Overrides{}, nil,
			).Times(2)
			operation := NewOperation(OperationConfig{
				Source:        source,
				Blobs:         blobs,
				Configuration: configurations,
				Publisher:     &FilePublisher{},
			})

			result, err := operation.Write(t.Context(), WriteRequest{
				Destination: destination,
				Selection:   tt.selection,
			})
			require.NoError(t, err)
			assert.Equal(t, int64(7), result.SourceRevision)
			assert.Equal(t, tt.wantProjects, result.Projects)
			assert.Equal(t, len(tt.wantBoards), result.Boards)
			assert.Equal(t, 1, result.Blobs)
			assert.Equal(t, tt.selection, source.selection)
			assert.Equal(t, 1, blobs.opens)

			file, err := os.Open(destination)
			require.NoError(t, err)
			info, err := file.Stat()
			require.NoError(t, err)
			archive, err := NewReader(file, info.Size())
			require.NoError(t, err)
			require.NoError(t, file.Close())
			require.Len(t, archive.Boards(), len(tt.wantBoards))
			for index, publication := range archive.Boards() {
				assert.Equal(t, tt.wantBoards[index], publication.SourceBoardID)
				assert.Equal(t, int64(7), publication.SourceRevision)
			}
			assert.Equal(t, []attachment.BlobDescriptor{descriptor}, archive.Blobs())
		})
	}
}

func TestOperation_WriteRejectsConfigurationChange(t *testing.T) {
	before := configuration.Overrides{}
	prefix, err := configuration.NewPrefix("changed-")
	require.NoError(t, err)
	after := configuration.Overrides{}
	after.Issue.ID.Prefix = &prefix
	destination := filepath.Join(t.TempDir(), "portable-backup")
	configurations := NewMockStoreConfiguration(gomock.NewController(t))
	gomock.InOrder(
		configurations.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(before, nil),
		configurations.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(after, nil),
	)
	operation := NewOperation(OperationConfig{
		Source: &writeTestSource{
			projects: []project.Snapshot{testWriteProject("project-one")},
			boards: []writeTestCapturedBoard{testWriteCapturedBoard(
				"project-one",
				"board-one",
				testWriteBlobDescriptor(t),
			)},
		},
		Blobs:         &writeTestBlobs{},
		Configuration: configurations,
		Publisher:     &FilePublisher{},
	})

	_, err = operation.Write(t.Context(), WriteRequest{
		Destination: destination,
		Selection:   AllBoards(),
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "source store configuration changed")
	assert.NoFileExists(t, destination)
}

func TestOperation_WriteFailurePreservesDestination(t *testing.T) {
	descriptor := testWriteBlobDescriptor(t)
	destination := filepath.Join(t.TempDir(), "portable-backup")
	require.NoError(t, os.WriteFile(destination, []byte("existing"), 0o600))
	configurations := NewMockStoreConfiguration(gomock.NewController(t))
	configurations.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(
		configuration.Overrides{}, nil,
	).Times(2)
	operation := NewOperation(OperationConfig{
		Source: &writeTestSource{
			projects: []project.Snapshot{testWriteProject("project-one")},
			boards: []writeTestCapturedBoard{testWriteCapturedBoard(
				"project-one",
				"board-one",
				descriptor,
			)},
		},
		Blobs:         &writeTestBlobs{body: []byte("bad")},
		Configuration: configurations,
		Publisher:     &FilePublisher{},
	})

	_, err := operation.Write(t.Context(), WriteRequest{
		Destination: destination,
		Selection:   AllBoards(),
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "size 3 does not match expected size 4")
	body, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	assert.Equal(t, []byte("existing"), body)
	temporary, globErr := filepath.Glob(
		filepath.Join(filepath.Dir(destination), ".portable-backup-*"),
	)
	require.NoError(t, globErr)
	assert.Empty(t, temporary)
}

func TestOperation_WriteReleasesCaptureBeforeConfigurationAndBlobs(t *testing.T) {
	descriptor := testWriteBlobDescriptor(t)
	lifetime := &writeTestCaptureLifetime{}
	source := &lifetimeWriteTestSource{
		lifetime: lifetime,
		project:  testWriteProject("project-one"),
		board:    testWriteCapturedBoard("project-one", "board-one", descriptor),
	}
	configuration := &lifetimeWriteTestConfiguration{lifetime: lifetime}
	blobs := &lifetimeWriteTestBlobs{
		lifetime: lifetime,
		body:     []byte("data"),
	}
	operation := NewOperation(OperationConfig{
		Source:        source,
		Blobs:         blobs,
		Configuration: configuration,
		Publisher:     &FilePublisher{},
	})

	_, err := operation.Write(t.Context(), WriteRequest{
		Destination: filepath.Join(t.TempDir(), "portable-backup"),
		Selection:   AllBoards(),
	})
	require.NoError(t, err)
	assert.False(t, lifetime.active)
	assert.Equal(t, 2, configuration.reads)
	assert.Equal(t, 1, blobs.opens)
}

func TestFilePublisher_PublishFailurePreservesDestination(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "portable-backup")
	require.NoError(t, os.WriteFile(destination, []byte("existing"), 0o600))
	publisher := &FilePublisher{}

	err := publisher.Publish(t.Context(), destination, func(output io.Writer) error {
		_, writeErr := output.Write([]byte("partial"))
		return errors.Join(writeErr, errors.New("generation failed"))
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "generation failed")
	body, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	assert.Equal(t, []byte("existing"), body)
	temporary, globErr := filepath.Glob(filepath.Join(directory, ".portable-backup-*"))
	require.NoError(t, globErr)
	assert.Empty(t, temporary)
}

type writeTestSource struct {
	projects  []project.Snapshot
	boards    []writeTestCapturedBoard
	selection Selection
}

type writeTestCapturedBoard struct {
	projectID project.ID
	boardID   board.ID
	records   boardcopy.RecordSequence
}

type writeTestCaptureLifetime struct {
	active bool
}

type lifetimeWriteTestSource struct {
	lifetime *writeTestCaptureLifetime
	project  project.Snapshot
	board    writeTestCapturedBoard
}

func (s *lifetimeWriteTestSource) Capture(
	_ context.Context,
	_ Selection,
	_ configuration.Overrides,
	destination CaptureDestination,
) (CaptureResult, error) {
	s.lifetime.active = true
	defer func() { s.lifetime.active = false }()
	if err := destination.AddProject(s.project); err != nil {
		return CaptureResult{}, err
	}
	if err := destination.AddBoard(
		s.board.projectID,
		s.board.boardID,
		s.board.records,
	); err != nil {
		return CaptureResult{}, err
	}
	return CaptureResult{
		SourceLineageID: "lineage-one",
		SourceRevision:  7,
		Projects:        1,
		Boards:          1,
	}, nil
}

type lifetimeWriteTestConfiguration struct {
	lifetime *writeTestCaptureLifetime
	reads    int
}

func (c *lifetimeWriteTestConfiguration) ReadStoreConfiguration(
	context.Context,
) (configuration.Overrides, error) {
	if c.reads > 0 && c.lifetime.active {
		return configuration.Overrides{}, errors.New(
			"source configuration read during capture",
		)
	}
	c.reads++
	return configuration.Overrides{}, nil
}

type lifetimeWriteTestBlobs struct {
	lifetime *writeTestCaptureLifetime
	body     []byte
	opens    int
}

func (b *lifetimeWriteTestBlobs) OpenCopyBlob(
	context.Context,
	attachment.BlobDescriptor,
) (io.ReadCloser, error) {
	if b.lifetime.active {
		return nil, errors.New("source blob opened during capture")
	}
	b.opens++
	return io.NopCloser(bytes.NewReader(b.body)), nil
}

func (s *writeTestSource) Capture(
	_ context.Context,
	selection Selection,
	_ configuration.Overrides,
	destination CaptureDestination,
) (CaptureResult, error) {
	s.selection = selection
	selected := s.boards
	projects := s.projects
	if !selection.IsAll() {
		wanted := make(map[board.ID]struct{}, len(selection.BoardIDs()))
		for _, id := range selection.BoardIDs() {
			wanted[id] = struct{}{}
		}
		selected = nil
		projectIDs := make(map[project.ID]struct{})
		for _, value := range s.boards {
			if _, found := wanted[value.boardID]; !found {
				continue
			}
			selected = append(selected, value)
			projectIDs[value.projectID] = struct{}{}
		}
		projects = nil
		for _, value := range s.projects {
			if _, found := projectIDs[value.ID]; found {
				projects = append(projects, value)
			}
		}
	}
	for _, value := range projects {
		if err := destination.AddProject(value); err != nil {
			return CaptureResult{}, err
		}
	}
	for _, value := range selected {
		if err := destination.AddBoard(
			value.projectID,
			value.boardID,
			value.records,
		); err != nil {
			return CaptureResult{}, err
		}
	}
	return CaptureResult{
		SourceLineageID: "lineage-one",
		SourceRevision:  7,
		Projects:        len(projects),
		Boards:          len(selected),
	}, nil
}

type writeTestBlobs struct {
	body  []byte
	opens int
}

func (s *writeTestBlobs) OpenCopyBlob(
	context.Context,
	attachment.BlobDescriptor,
) (io.ReadCloser, error) {
	s.opens++
	return io.NopCloser(bytes.NewReader(s.body)), nil
}

func mustSelectBoards(t *testing.T, ids ...board.ID) Selection {
	t.Helper()
	selection, err := SelectBoards(ids...)
	require.NoError(t, err)
	return selection
}

func testWriteProject(id project.ID) project.Snapshot {
	return project.Snapshot{
		ID: id, Name: id.String(), Created: time.Unix(1_000, 0).UTC(),
	}
}

func testWriteBoard(
	id board.ID,
	descriptor attachment.BlobDescriptor,
) testBoardSnapshot {
	snapshot := testBoardSnapshot{
		SourceLineageID: "lineage-one",
		SourceRevision:  7,
		Board: boardcopy.CopyBoard{
			ID: id.String(), Name: id.String(), CreatedAt: time.Unix(1_000, 0).UTC(),
		},
		Configuration: configuration.Defaults(),
	}
	if descriptor.Digest != "" {
		snapshot.Attachments = []boardcopy.CopyAttachment{{
			ID:           "att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
			Blob:         descriptor,
			Filename:     "evidence.txt",
			MediaType:    "text/plain",
			Lifecycle:    "active",
			CreatedActor: "engineer",
			CreatedAt:    time.Unix(1_001, 0).UTC(),
		}}
	}
	return snapshot
}

func testWriteCapturedBoard(
	projectID project.ID,
	boardID board.ID,
	descriptor attachment.BlobDescriptor,
) writeTestCapturedBoard {
	snapshot := testWriteBoard(boardID, descriptor)
	return writeTestCapturedBoard{
		projectID: projectID,
		boardID:   boardID,
		records:   boardRecordSequence(testBoardRecords(snapshot)),
	}
}

func testWriteBlobDescriptor(t *testing.T) attachment.BlobDescriptor {
	t.Helper()
	digest, err := attachment.NewDigest(
		"sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
	)
	require.NoError(t, err)
	return attachment.BlobDescriptor{Digest: digest, SizeBytes: 4}
}
