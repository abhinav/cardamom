package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardcopy"
	backupv1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/backup/v1"
	"google.golang.org/protobuf/proto"
)

func TestBoardRecordDecodesPublishedTrailerField(t *testing.T) {
	var record backupv1.BoardRecord
	require.NoError(t, proto.Unmarshal([]byte{0x62, 0x00}, &record))
	assert.NotNil(t, record.GetTrailer())
}

func TestBoardRecordCodecUsesDeterministicDelimitedFraming(t *testing.T) {
	_, snapshot, _, _ := archiveTestValues(t)
	records := []boardcopy.Record{
		boardcopy.RecordHeader{
			Version:         boardcopy.CopySnapshotVersion,
			SourceLineageID: snapshot.SourceLineageID,
			SourceRevision:  snapshot.SourceRevision,
			Board:           snapshot.Board,
			Configuration:   snapshot.Configuration,
		},
	}
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
	for _, value := range snapshot.Pins {
		records = append(records, value)
	}
	records = append(records, boardcopy.RecordTrailer{Counts: boardcopy.RecordCounts{
		Issues: uint64(len(snapshot.Issues)), Labels: uint64(len(snapshot.Labels)),
		Dependencies: uint64(len(snapshot.Dependencies)),
		Containment:  uint64(len(snapshot.Containment)),
		ExternalKeys: uint64(len(snapshot.ExternalKeys)),
		LogEntries:   uint64(len(snapshot.LogEntries)),
		States:       uint64(len(snapshot.States)), Results: uint64(len(snapshot.Results)),
		Checkpoints: uint64(len(snapshot.Checkpoints)),
		Attachments: uint64(len(snapshot.Attachments)),
		Pins:        uint64(len(snapshot.Pins)),
	}})

	encode := func() ([]byte, string) {
		var framed bytes.Buffer
		digest := sha256.New()
		encoder := NewBoardRecordEncoder(io.MultiWriter(&framed, digest))
		for _, record := range records {
			require.NoError(t, encoder.Write(record))
		}
		return framed.Bytes(), "sha256:" + hex.EncodeToString(digest.Sum(nil))
	}

	first, firstDigest := encode()
	second, secondDigest := encode()
	assert.Equal(t, first, second)
	assert.Equal(t, firstDigest, secondDigest)
	assert.NotEmpty(t, first)

	decoder := NewBoardRecordDecoder(bytes.NewReader(first), uint64(len(first)))
	for _, want := range records {
		got, err := decoder.Read()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, err := decoder.Read()
	assert.ErrorIs(t, err, io.EOF)
}

func TestBoardRecordDecoderRejectsMissingRecord(t *testing.T) {
	var framed bytes.Buffer
	require.NoError(t, NewBoardRecordEncoder(&framed).Write(
		boardcopy.RecordTrailer{},
	))

	encoded := framed.Bytes()
	require.Greater(t, len(encoded), 1)
	_, err := NewBoardRecordDecoder(
		bytes.NewReader(encoded[:len(encoded)-1]),
		uint64(len(encoded)-1),
	).Read()
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}
