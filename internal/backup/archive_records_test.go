package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	backupv1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/backup/v1"
	"go.abhg.dev/cardamom/internal/project"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestWriterStreamsBoardRecordMembers(t *testing.T) {
	projectSnapshot, snapshot, _, _ := archiveTestValues(t)
	records := testBoardRecords(snapshot)
	yielded := 0
	sequence := func(yield func(boardcopy.Record, error) bool) {
		for _, record := range records {
			yielded++
			if !yield(record, nil) {
				return
			}
		}
	}

	var archive bytes.Buffer
	writer := NewWriter(&archive)
	require.NoError(t, writer.AddProject(projectSnapshot))
	require.NoError(t, writer.AddBoard(
		projectSnapshot.ID,
		board.ID(snapshot.Board.ID),
		sequence,
	))
	require.NoError(t, writer.Close())
	assert.Equal(t, len(records), yielded)

	zipReader, err := zip.NewReader(
		bytes.NewReader(archive.Bytes()),
		int64(archive.Len()),
	)
	require.NoError(t, err)
	require.Len(t, zipReader.File, 2)
	assert.Equal(t, "boards/id/Ym9hcmQtc291cmNl", zipReader.File[0].Name)

	reader, err := NewReader(
		bytes.NewReader(archive.Bytes()),
		int64(archive.Len()),
	)
	require.NoError(t, err)
	sequence, err = reader.OpenBoard(reader.Boards()[0])
	require.NoError(t, err)
	assert.Equal(t, records, collectBoardRecords(t, sequence))
}

func TestWriterSeparatesMemberAndSnapshotDigests(t *testing.T) {
	projectSnapshot, snapshot, _, _ := archiveTestValues(t)
	firstRecords := testBoardRecords(snapshot)
	first := writeRecordArchive(t, projectSnapshot, firstRecords)
	expected, err := boardcopy.IndexRecords(
		boardRecordSequence(firstRecords),
	)
	require.NoError(t, err)

	snapshot.SourceLineageID = "store_fedcba9876543210fedcba9876543210"
	snapshot.SourceRevision++
	second := writeRecordArchive(t, projectSnapshot, testBoardRecords(snapshot))

	firstBoard := first.Boards()[0]
	secondBoard := second.Boards()[0]
	assert.Equal(t, expected.Digest, firstBoard.SnapshotDigest)
	assert.Equal(t, firstBoard.SnapshotDigest, secondBoard.SnapshotDigest)
	assert.NotEqual(t, firstBoard.member.digest, secondBoard.member.digest)
	assert.NotEqual(t, firstBoard.SnapshotDigest, firstBoard.member.digest)
}

func TestWriterRejectsIncompleteBoardRecordMember(t *testing.T) {
	projectSnapshot, snapshot, _, _ := archiveTestValues(t)
	records := testBoardRecords(snapshot)
	records = records[:len(records)-1]

	writer := NewWriter(io.Discard)
	require.NoError(t, writer.AddProject(projectSnapshot))
	err := writer.AddBoard(
		projectSnapshot.ID,
		board.ID(snapshot.Board.ID),
		boardRecordSequence(records),
	)
	assert.EqualError(t, err, `write archive board "board-source": board record trailer is required`)
}

func TestWriterRejectsNoncanonicalBoardRecordMember(t *testing.T) {
	projectSnapshot, snapshot, _, _ := archiveTestValues(t)
	records := testBoardRecords(snapshot)
	records[1], records[2] = records[2], records[1]

	writer := NewWriter(io.Discard)
	require.NoError(t, writer.AddProject(projectSnapshot))
	err := writer.AddBoard(
		projectSnapshot.ID,
		board.ID(snapshot.Board.ID),
		boardRecordSequence(records),
	)
	assert.EqualError(
		t,
		err,
		`write archive board "board-source": board record type 2 is out of order`,
	)
}

func TestWriterEncodesBoardIDInMemberPath(t *testing.T) {
	projectSnapshot, snapshot, _, _ := archiveTestValues(t)
	snapshot.Board.ID = "../board/source"

	var archive bytes.Buffer
	writer := NewWriter(&archive)
	require.NoError(t, writer.AddProject(projectSnapshot))
	require.NoError(t, writer.AddBoard(
		projectSnapshot.ID,
		board.ID(snapshot.Board.ID),
		boardRecordSequence(testBoardRecords(snapshot)),
	))
	require.NoError(t, writer.Close())

	zipReader, err := zip.NewReader(
		bytes.NewReader(archive.Bytes()),
		int64(archive.Len()),
	)
	require.NoError(t, err)
	require.Len(t, zipReader.File, 2)
	assert.Equal(t, "boards/id/Li4vYm9hcmQvc291cmNl", zipReader.File[0].Name)
}

func TestPrepareRestoreRejectsIncompleteRecordMember(t *testing.T) {
	projectSnapshot, snapshot, _, _ := archiveTestValues(t)
	archive := archiveTestBytes(t)
	records := testBoardRecords(snapshot)
	var body bytes.Buffer
	encoder := NewBoardRecordEncoder(&body)
	for _, record := range records[:len(records)-1] {
		require.NoError(t, encoder.Write(record))
	}
	digest := sha256.Sum256(body.Bytes())
	descriptor := archiveMember{
		name:      boardMember(board.ID(snapshot.Board.ID)),
		sizeBytes: uint64(body.Len()),
		digest:    digestString(digest[:]),
	}

	changed := rewriteZip(t, archive, func(name string, original []byte) ([]byte, bool) {
		switch name {
		case descriptor.name:
			return body.Bytes(), true
		case ManifestMember:
			var manifest backupv1.Manifest
			require.NoError(t, proto.Unmarshal(original, &manifest))
			require.Len(t, manifest.Boards, 1)
			manifest.Boards[0].SnapshotDigest = descriptor.digest
			manifest.Boards[0].Member = memberToProto(descriptor)
			encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(&manifest)
			require.NoError(t, err)
			return encoded, true
		default:
			return original, true
		}
	})
	reader, err := NewReader(bytes.NewReader(changed), int64(len(changed)))
	require.NoError(t, err)
	_, err = PrepareRestore(t.Context(), reader)
	assert.EqualError(
		t,
		err,
		`read archive board "board-source": board record trailer is required`,
	)
	assert.Equal(t, projectSnapshot.ID, reader.Projects()[0].ID)
}

func TestReaderRejectsBoardRecordLargerThanMember(t *testing.T) {
	archive := archiveTestBytes(t)
	forgedBody := protowire.AppendVarint(nil, 1024)
	digest := sha256.Sum256(forgedBody)
	descriptor := archiveMember{
		name:      boardMember("board-source"),
		sizeBytes: uint64(len(forgedBody)),
		digest:    digestString(digest[:]),
	}

	changed := rewriteZip(t, archive, func(name string, original []byte) ([]byte, bool) {
		switch name {
		case descriptor.name:
			return forgedBody, true
		case ManifestMember:
			var manifest backupv1.Manifest
			require.NoError(t, proto.Unmarshal(original, &manifest))
			require.Len(t, manifest.Boards, 1)
			manifest.Boards[0].Member = memberToProto(descriptor)
			encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(&manifest)
			require.NoError(t, err)
			return encoded, true
		default:
			return original, true
		}
	})
	reader, err := NewReader(bytes.NewReader(changed), int64(len(changed)))
	require.NoError(t, err)
	records, err := reader.OpenBoard(reader.Boards()[0])
	require.NoError(t, err)
	err = collectBoardRecordError(records)

	var sizeErr *protodelim.SizeTooLargeError
	require.ErrorAs(t, err, &sizeErr)
	assert.Equal(t, uint64(1024), sizeErr.Size)
	assert.Equal(t, descriptor.sizeBytes, sizeErr.MaxSize)
}

func collectBoardRecords(
	t *testing.T,
	sequence boardcopy.RecordSequence,
) []boardcopy.Record {
	t.Helper()
	var records []boardcopy.Record
	for record, err := range sequence {
		require.NoError(t, err)
		records = append(records, record)
	}
	return records
}

func collectBoardRecordError(sequence boardcopy.RecordSequence) error {
	for _, err := range sequence {
		if err != nil {
			return err
		}
	}
	return nil
}

func boardRecordSequence(records []boardcopy.Record) boardcopy.RecordSequence {
	return func(yield func(boardcopy.Record, error) bool) {
		for _, record := range records {
			if !yield(record, nil) {
				return
			}
		}
	}
}

type testBoardSnapshot struct {
	SourceLineageID string
	SourceRevision  int64
	Board           boardcopy.CopyBoard
	Configuration   configuration.Configuration
	Issues          []boardcopy.CopyIssue
	Labels          []boardcopy.CopyLabel
	Dependencies    []boardcopy.CopyDependency
	Containment     []boardcopy.CopyContainment
	ExternalKeys    []boardcopy.CopyExternalKey
	LogEntries      []boardcopy.CopyLogEntry
	States          []boardcopy.CopyState
	Results         []boardcopy.CopyResultRecord
	Checkpoints     []boardcopy.CopyCheckpoint
	Attachments     []boardcopy.CopyAttachment
}

func writeRecordArchive(
	t *testing.T,
	projectSnapshot project.Snapshot,
	records []boardcopy.Record,
) *Reader {
	t.Helper()
	var archive bytes.Buffer
	writer := NewWriter(&archive)
	require.NoError(t, writer.AddProject(projectSnapshot))
	header := records[0].(boardcopy.RecordHeader)
	require.NoError(t, writer.AddBoard(
		projectSnapshot.ID,
		board.ID(header.Board.ID),
		boardRecordSequence(records),
	))
	require.NoError(t, writer.Close())
	reader, err := NewReader(
		bytes.NewReader(archive.Bytes()),
		int64(archive.Len()),
	)
	require.NoError(t, err)
	return reader
}

func testBoardRecords(snapshot testBoardSnapshot) []boardcopy.Record {
	records := []boardcopy.Record{boardcopy.RecordHeader{
		Version:         boardcopy.CopySnapshotVersion,
		SourceLineageID: snapshot.SourceLineageID,
		SourceRevision:  snapshot.SourceRevision,
		Board:           snapshot.Board,
		Configuration:   snapshot.Configuration,
	}}
	for _, value := range snapshot.Issues {
		records = append(records, value)
	}
	for _, value := range snapshot.Labels {
		records = append(records, value)
	}
	for _, value := range snapshot.Dependencies {
		records = append(records, value)
	}
	for _, value := range snapshot.Containment {
		records = append(records, value)
	}
	for _, value := range snapshot.ExternalKeys {
		records = append(records, value)
	}
	for _, value := range snapshot.LogEntries {
		records = append(records, value)
	}
	for _, value := range snapshot.States {
		records = append(records, value)
	}
	for _, value := range snapshot.Results {
		records = append(records, value)
	}
	for _, value := range snapshot.Checkpoints {
		records = append(records, value)
	}
	for _, value := range snapshot.Attachments {
		records = append(records, value)
	}
	return append(records, boardcopy.RecordTrailer{Counts: boardcopy.RecordCounts{
		Issues:       uint64(len(snapshot.Issues)),
		Labels:       uint64(len(snapshot.Labels)),
		Dependencies: uint64(len(snapshot.Dependencies)),
		Containment:  uint64(len(snapshot.Containment)),
		ExternalKeys: uint64(len(snapshot.ExternalKeys)),
		LogEntries:   uint64(len(snapshot.LogEntries)),
		States:       uint64(len(snapshot.States)),
		Results:      uint64(len(snapshot.Results)),
		Checkpoints:  uint64(len(snapshot.Checkpoints)),
		Attachments:  uint64(len(snapshot.Attachments)),
	}})
}
