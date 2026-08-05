package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	backupv1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/backup/v1"
	"go.abhg.dev/cardamom/internal/project"
	"google.golang.org/protobuf/proto"
)

func TestArchiveRoundTrip(t *testing.T) {
	projectSnapshot, boardSnapshot, descriptor, body := archiveTestValues(t)
	var archive bytes.Buffer
	writer := NewWriter(&archive)
	require.NoError(t, writer.AddProject(projectSnapshot))
	require.NoError(t, writer.AddBoard(projectSnapshot.ID, boardSnapshot))
	stream := &boundedRead{reader: bytes.NewReader(body), max: 64 << 10}
	require.NoError(t, writer.AddBlob(descriptor, stream))
	require.NoError(t, writer.Close())
	assert.LessOrEqual(t, stream.largest, 64<<10)

	zipReader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	require.NoError(t, err)
	names := make([]string, 0, len(zipReader.File))
	for _, file := range zipReader.File {
		names = append(names, file.Name)
		assert.Empty(t, memberExtension(file.Name))
	}
	assert.Equal(t, ManifestMember, zipReader.File[len(zipReader.File)-1].Name)
	slices.Sort(names)
	assert.Equal(t, []string{
		"blobs/sha256/3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
		"boards/sha256/" + strings.TrimPrefix(
			boardcopy.PrepareSnapshot(boardSnapshot).Digest,
			"sha256:",
		),
		ManifestMember,
	}, names)

	reader, err := NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	require.NoError(t, err)
	assert.Equal(t, []project.Snapshot{projectSnapshot}, reader.Projects())
	require.Len(t, reader.Boards(), 1)
	assert.Equal(t, projectSnapshot.ID, reader.Boards()[0].ProjectID)
	assert.Equal(t, boardSnapshot.Board.ID, reader.Boards()[0].SourceBoardID.String())
	assert.Equal(t, []attachment.BlobDescriptor{descriptor}, reader.Blobs())

	gotBoard, err := reader.ReadBoard(reader.Boards()[0])
	require.NoError(t, err)
	assert.Equal(t, boardcopy.PrepareSnapshot(boardSnapshot), gotBoard)

	blob, err := reader.OpenBlob(reader.Blobs()[0])
	require.NoError(t, err)
	gotBody, err := io.ReadAll(blob)
	require.NoError(t, err)
	require.NoError(t, blob.Close())
	assert.Equal(t, body, gotBody)
}

func TestReaderRejectsValuesOutsideCatalog(t *testing.T) {
	archive := archiveTestBytes(t)
	reader, err := NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)

	publication := reader.Boards()[0]
	publication.SourceRevision++
	_, err = reader.ReadBoard(publication)
	assert.EqualError(t, err, `archive board "board-source" is not in this catalog`)

	descriptor := reader.Blobs()[0]
	descriptor.SizeBytes++
	_, err = reader.OpenBlob(descriptor)
	assert.EqualError(t, err, "archive blob sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7 is not in this catalog")
}

func TestWriterStreamsBlobContent(t *testing.T) {
	body := bytes.Repeat([]byte("streaming-content"), 1<<17)
	digestValue := sha256.Sum256(body)
	digest, err := attachment.NewDigest(fmt.Sprintf("sha256:%x", digestValue))
	require.NoError(t, err)
	descriptor := attachment.BlobDescriptor{
		Digest: digest, SizeBytes: uint64(len(body)),
	}
	stream := &boundedRead{reader: bytes.NewReader(body), max: 64 << 10}

	var archive bytes.Buffer
	writer := NewWriter(&archive)
	require.NoError(t, writer.AddBlob(descriptor, stream))
	require.NoError(t, writer.Close())
	assert.LessOrEqual(t, stream.largest, 64<<10)
}

func TestWriterCloseEnforcesManifestSizeLimit(t *testing.T) {
	tests := []struct {
		name    string
		give    int
		wantErr string
	}{
		{name: "AtLimit", give: maxManifestBytes},
		{
			name:    "OverLimit",
			give:    maxManifestBytes + 1,
			wantErr: "archive manifest size 67108865 exceeds maximum 67108864",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := NewWriter(&archive)
			setManifestSize(t, &writer.manifest, tt.give)

			err := writer.Close()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}

			reader, openErr := zip.NewReader(
				bytes.NewReader(archive.Bytes()),
				int64(archive.Len()),
			)
			require.NoError(t, openErr)
			assert.Equal(t, tt.wantErr == "", slices.ContainsFunc(
				reader.File,
				func(file *zip.File) bool { return file.Name == ManifestMember },
			))
		})
	}
}

func TestWriterRejectsInvalidContentAndDuplicateRecords(t *testing.T) {
	projectSnapshot, boardSnapshot, descriptor, body := archiveTestValues(t)

	t.Run("UnknownProject", func(t *testing.T) {
		writer := NewWriter(io.Discard)
		err := writer.AddBoard(projectSnapshot.ID, boardSnapshot)
		assert.EqualError(t, err, `archive project "project-source" is not registered`)
	})

	t.Run("DuplicateProject", func(t *testing.T) {
		writer := NewWriter(io.Discard)
		require.NoError(t, writer.AddProject(projectSnapshot))
		err := writer.AddProject(projectSnapshot)
		assert.EqualError(t, err, `archive project "project-source" is duplicated`)
	})

	t.Run("DuplicateBoard", func(t *testing.T) {
		writer := NewWriter(io.Discard)
		require.NoError(t, writer.AddProject(projectSnapshot))
		require.NoError(t, writer.AddBoard(projectSnapshot.ID, boardSnapshot))
		err := writer.AddBoard(projectSnapshot.ID, boardSnapshot)
		assert.EqualError(t, err, `archive board "board-source" is duplicated`)
	})

	t.Run("BlobSize", func(t *testing.T) {
		writer := NewWriter(io.Discard)
		err := writer.AddBlob(descriptor, bytes.NewReader(body[:3]))
		assert.EqualError(t, err, "write blob: size 3 does not match expected size 4")
		assert.EqualError(t, writer.Close(), "write blob: size 3 does not match expected size 4")
	})

	t.Run("BlobDigest", func(t *testing.T) {
		writer := NewWriter(io.Discard)
		err := writer.AddBlob(descriptor, strings.NewReader("Data"))
		assert.EqualError(t, err, "write blob: content digest does not match sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7")
	})

	t.Run("DuplicateBlob", func(t *testing.T) {
		writer := NewWriter(io.Discard)
		require.NoError(t, writer.AddBlob(descriptor, bytes.NewReader(body)))
		err := writer.AddBlob(descriptor, bytes.NewReader(body))
		assert.EqualError(t, err, "archive blob sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7 is duplicated")
	})
}

func TestReaderRejectsInvalidManifestAndLayout(t *testing.T) {
	archive := archiveTestBytes(t)

	tests := []struct {
		name string
		edit func(*backupv1.Manifest)
		want string
	}{
		{
			name: "Version",
			edit: func(manifest *backupv1.Manifest) { manifest.Version = 2 },
			want: "unsupported archive version 2",
		},
		{
			name: "DuplicateProject",
			edit: func(manifest *backupv1.Manifest) {
				manifest.Projects = append(manifest.Projects, proto.Clone(manifest.Projects[0]).(*backupv1.Project))
			},
			want: `archive project "project-source" is duplicated`,
		},
		{
			name: "MissingProject",
			edit: func(manifest *backupv1.Manifest) { manifest.Boards[0].ProjectId = "missing" },
			want: `archive board "board-source" references unknown project "missing"`,
		},
		{
			name: "BoardMember",
			edit: func(manifest *backupv1.Manifest) { manifest.Boards[0].Member.Name = "boards/snapshot.pb" },
			want: `archive board "board-source" uses noncanonical member "boards/snapshot.pb"`,
		},
		{
			name: "DuplicateBlob",
			edit: func(manifest *backupv1.Manifest) {
				manifest.Blobs = append(manifest.Blobs, proto.Clone(manifest.Blobs[0]).(*backupv1.Blob))
			},
			want: "archive blob sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7 is duplicated",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := rewriteManifest(t, archive, tt.edit)
			_, err := NewReader(bytes.NewReader(changed), int64(len(changed)))
			assert.EqualError(t, err, tt.want)
		})
	}

	t.Run("UnexpectedMember", func(t *testing.T) {
		changed := rewriteZip(t, archive, func(_ string, body []byte) ([]byte, bool) {
			return body, true
		}, zipEntry{name: "notes", body: []byte("unexpected")})
		_, err := NewReader(bytes.NewReader(changed), int64(len(changed)))
		assert.EqualError(t, err, `archive contains unexpected member "notes"`)
	})

	t.Run("MissingMember", func(t *testing.T) {
		changed := rewriteZip(t, archive, func(name string, body []byte) ([]byte, bool) {
			return body, !strings.HasPrefix(name, "boards/")
		})
		_, err := NewReader(bytes.NewReader(changed), int64(len(changed)))
		assert.Contains(t, err.Error(), "archive is missing member")
	})
}

func TestReaderVerifiesMemberIntegrity(t *testing.T) {
	archive := archiveTestBytes(t)

	t.Run("Board", func(t *testing.T) {
		changed := rewriteZip(t, archive, func(name string, body []byte) ([]byte, bool) {
			if strings.HasPrefix(name, "boards/") {
				body[len(body)-1] ^= 1
			}
			return body, true
		})
		reader, err := NewReader(bytes.NewReader(changed), int64(len(changed)))
		require.NoError(t, err)
		_, err = reader.ReadBoard(reader.Boards()[0])
		assert.Contains(t, err.Error(), "content digest mismatch")
	})

	t.Run("BlobCloseDrains", func(t *testing.T) {
		changed := rewriteZip(t, archive, func(name string, body []byte) ([]byte, bool) {
			if strings.HasPrefix(name, "blobs/") {
				body[0] ^= 1
			}
			return body, true
		})
		reader, err := NewReader(bytes.NewReader(changed), int64(len(changed)))
		require.NoError(t, err)
		blob, err := reader.OpenBlob(reader.Blobs()[0])
		require.NoError(t, err)
		assert.Contains(t, blob.Close().Error(), "content digest mismatch")
	})

	t.Run("PublicationMetadata", func(t *testing.T) {
		changed := rewriteManifest(t, archive, func(manifest *backupv1.Manifest) {
			manifest.Boards[0].SourceRevision++
		})
		reader, err := NewReader(bytes.NewReader(changed), int64(len(changed)))
		require.NoError(t, err)
		_, err = reader.ReadBoard(reader.Boards()[0])
		assert.EqualError(t, err, `archive board "board-source" does not match its manifest publication`)
	})
}

type boundedRead struct {
	reader  io.Reader
	max     int
	largest int
}

func (r *boundedRead) Read(p []byte) (int, error) {
	if len(p) > r.max {
		return 0, fmt.Errorf("read buffer %d exceeds %d", len(p), r.max)
	}
	r.largest = max(r.largest, len(p))
	return r.reader.Read(p)
}

type zipEntry struct {
	name string
	body []byte
}

func archiveTestBytes(t *testing.T) []byte {
	t.Helper()
	projectSnapshot, boardSnapshot, descriptor, body := archiveTestValues(t)
	var archive bytes.Buffer
	writer := NewWriter(&archive)
	require.NoError(t, writer.AddProject(projectSnapshot))
	require.NoError(t, writer.AddBoard(projectSnapshot.ID, boardSnapshot))
	require.NoError(t, writer.AddBlob(descriptor, bytes.NewReader(body)))
	require.NoError(t, writer.Close())
	return archive.Bytes()
}

func setManifestSize(t *testing.T, manifest *backupv1.Manifest, size int) {
	t.Helper()
	// A 2 MiB name uses the same protobuf length-prefix widths as both tested
	// boundaries, so its framing cost yields the final name length directly.
	const probeNameBytes = 1 << 21
	manifest.Version = Version
	manifest.Projects = []*backupv1.Project{{
		Name: strings.Repeat("x", probeNameBytes),
	}}
	framingBytes := proto.Size(manifest) - probeNameBytes
	manifest.Projects[0].Name = strings.Repeat("x", size-framingBytes)
	require.Equal(t, size, proto.Size(manifest))
}

func archiveTestValues(t *testing.T) (
	project.Snapshot,
	boardcopy.CopySnapshot,
	attachment.BlobDescriptor,
	[]byte,
) {
	t.Helper()
	createdAt := time.Date(2026, time.August, 5, 9, 30, 0, 123, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	description := "Board context"
	summary := "Summary"
	details := "Details"
	actor := "worker"
	digest, err := attachment.NewDigest("sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7")
	require.NoError(t, err)
	projectID, err := project.NewID("project-source")
	require.NoError(t, err)
	descriptor := attachment.BlobDescriptor{Digest: digest, SizeBytes: 4}
	return project.Snapshot{ID: projectID, Name: "Source Project", Created: createdAt}, boardcopy.CopySnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  12,
		Board: boardcopy.CopyBoard{
			ID: "board-source", Name: "Source Board",
			Description: &description, CreatedAt: createdAt,
		},
		Configuration: configuration.Defaults(),
		Issues: []boardcopy.CopyIssue{
			{
				ID: "cm-1", Title: "Issue", Kind: "task", Lifecycle: "closed",
				Priority: 2, CreatedAt: createdAt, UpdatedAt: updatedAt,
				ClosedAt: &updatedAt, WaitingReason: new("acceptance"),
				WaitingSince: &createdAt, Summary: &summary, Details: &details,
			},
			{
				ID: "cm-2", Title: "Workstream", Kind: "workstream",
				Lifecycle: "closed", Priority: 1, CreatedAt: createdAt,
				UpdatedAt: updatedAt, ClosedAt: &updatedAt,
			},
		},
		Labels:       []boardcopy.CopyLabel{{IssueID: "cm-1", Label: "area:backup"}},
		Dependencies: []boardcopy.CopyDependency{{IssueID: "cm-1", PrerequisiteID: "cm-2"}},
		Containment:  []boardcopy.CopyContainment{{ChildID: "cm-1", ParentID: "cm-2"}},
		ExternalKeys: []boardcopy.CopyExternalKey{{Key: "external", IssueID: "cm-1"}},
		LogEntries: []boardcopy.CopyLogEntry{{
			Order: 1, ID: "log_source", IssueID: "cm-1", Kind: "post",
			Author: &actor, Committer: &actor, Body: "Log", CreatedAt: &createdAt,
			NextAction: new("Continue"),
		}},
		States: []boardcopy.CopyState{{
			IssueID: "cm-1", Body: "State", Author: &actor,
			UpdatedAt: &updatedAt, SnapshotLogEntryID: new("log_source"),
			NextAction: new("Accept"),
		}},
		Results: []boardcopy.CopyResultRecord{{IssueID: "cm-1", Body: "Result"}},
		Checkpoints: []boardcopy.CopyCheckpoint{{
			IssueID: "cm-1", Outcome: "approved", Reason: "Ready",
			DecidedAt: updatedAt,
		}},
		Attachments: []boardcopy.CopyAttachment{{
			ID: "att_source", OriginIssueID: new("cm-1"), Blob: descriptor,
			Filename: "evidence.txt", MediaType: "text/plain", Lifecycle: "removed",
			CreatedActor: actor, CreatedAt: createdAt,
			RemovedActor: &actor, RemovedAt: &updatedAt,
		}},
	}, descriptor, []byte("data")
}

func rewriteManifest(
	t *testing.T,
	archive []byte,
	edit func(*backupv1.Manifest),
) []byte {
	t.Helper()
	return rewriteZip(t, archive, func(name string, body []byte) ([]byte, bool) {
		if name != ManifestMember {
			return body, true
		}
		var manifest backupv1.Manifest
		require.NoError(t, proto.Unmarshal(body, &manifest))
		edit(&manifest)
		changed, err := proto.MarshalOptions{Deterministic: true}.Marshal(&manifest)
		require.NoError(t, err)
		return changed, true
	})
}

func rewriteZip(
	t *testing.T,
	archive []byte,
	edit func(string, []byte) ([]byte, bool),
	extra ...zipEntry,
) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	var changed bytes.Buffer
	writer := zip.NewWriter(&changed)
	for _, file := range reader.File {
		body, err := readZipFile(file)
		require.NoError(t, err)
		body, keep := edit(file.Name, body)
		if !keep {
			continue
		}
		member, err := writer.CreateHeader(&zip.FileHeader{Name: file.Name, Method: zip.Store})
		require.NoError(t, err)
		_, err = member.Write(body)
		require.NoError(t, err)
	}
	for _, entry := range extra {
		member, err := writer.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		require.NoError(t, err)
		_, err = member.Write(entry.body)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return changed.Bytes()
}

func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(reader)
	return body, errors.Join(readErr, reader.Close())
}

func memberExtension(name string) string {
	base := name
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		base = name[slash+1:]
	}
	if dot := strings.LastIndexByte(base, '.'); dot >= 0 {
		return base[dot:]
	}
	return ""
}
