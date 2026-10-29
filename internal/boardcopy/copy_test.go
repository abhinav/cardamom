package boardcopy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestEvaluateCopyImportReturnsPublishedOutcome(t *testing.T) {
	published := CopyOutcome{
		SourceLineageID:      "store_0123456789abcdef0123456789abcdef",
		SourceBoardID:        "board-source",
		SourceRevision:       7,
		SnapshotVersion:      CopySnapshotVersion,
		SnapshotDigest:       "sha256:published",
		DestinationProjectID: "project-destination",
		DestinationBoardID:   "board-published",
		DestinationName:      "Published",
		DestinationRevision:  11,
	}

	outcome, err := EvaluateCopyImport(
		RecordIndex{},
		CopyOptions{},
		NewPublishedCopyImport(published),
	)
	require.NoError(t, err)
	assert.Equal(t, published, outcome)
	assert.False(t, outcome.AlreadyCompleted)
}

func TestEvaluateCopyImportEvaluatesConcurrentWinner(t *testing.T) {
	snapshot := copyTestSnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
		Configuration: configuration.Defaults(),
	}
	index := copyTestIndex(t, snapshot)

	outcome, err := EvaluateCopyImport(
		index,
		CopyOptions{ProjectID: "project-destination"},
		NewConcurrentCopyImport(CopyReceipt{
			SourceLineageID:      snapshot.SourceLineageID,
			SourceBoardID:        snapshot.Board.ID,
			SourceRevision:       3,
			SnapshotVersion:      index.Header.Version,
			SnapshotDigest:       index.Digest,
			DestinationProjectID: "project-destination",
			DestinationBoardID:   "board-winner",
			DestinationName:      "Source",
			DestinationRevision:  11,
		}),
	)
	require.NoError(t, err)
	assert.True(t, outcome.AlreadyCompleted)
	assert.Equal(t, "board-winner", outcome.DestinationBoardID)
	assert.Equal(t, int64(3), outcome.SourceRevision)
}

func TestEvaluateRecordReceiptRejectsChangedPublication(t *testing.T) {
	snapshot := copyTestSnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
		Configuration: configuration.Defaults(),
	}
	index := copyTestIndex(t, snapshot)
	receipt := CopyReceipt{
		SourceLineageID:      snapshot.SourceLineageID,
		SourceBoardID:        snapshot.Board.ID,
		SourceRevision:       3,
		SnapshotVersion:      index.Header.Version,
		SnapshotDigest:       "prior",
		DestinationProjectID: "project-destination",
		DestinationBoardID:   "board-prior",
		DestinationName:      "Source",
		DestinationRevision:  11,
	}

	_, err := EvaluateRecordReceipt(
		index,
		CopyOptions{ProjectID: "project-destination"},
		receipt,
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "incremental synchronization is not supported")
}

func TestEvaluateRecordReceiptRejectsDifferentDestinationOptions(t *testing.T) {
	snapshot := copyTestSnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
		Configuration: configuration.Defaults(),
	}
	index := copyTestIndex(t, snapshot)
	receipt := CopyReceipt{
		SourceLineageID:      snapshot.SourceLineageID,
		SourceBoardID:        snapshot.Board.ID,
		SourceRevision:       3,
		SnapshotVersion:      index.Header.Version,
		SnapshotDigest:       index.Digest,
		DestinationProjectID: "project-destination",
		DestinationBoardID:   "board-prior",
		DestinationName:      "Source",
		DestinationRevision:  11,
	}
	renamed := "Renamed"

	_, err := EvaluateRecordReceipt(
		index,
		CopyOptions{
			ProjectID: "project-destination",
			Name:      &renamed,
		},
		receipt,
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "different destination options")
}

type copyTestSnapshot struct {
	SourceLineageID string
	SourceRevision  int64
	Board           CopyBoard
	Configuration   configuration.Configuration
	Issues          []CopyIssue
	Labels          []CopyLabel
	Dependencies    []CopyDependency
	Containment     []CopyContainment
	ExternalKeys    []CopyExternalKey
	LogEntries      []CopyLogEntry
	States          []CopyState
	Results         []CopyResultRecord
	Checkpoints     []CopyCheckpoint
	Attachments     []CopyAttachment
}

func copyTestRecords(snapshot copyTestSnapshot) RecordSequence {
	counts := RecordCounts{
		Issues: uint64(len(snapshot.Issues)), Labels: uint64(len(snapshot.Labels)),
		Dependencies: uint64(len(snapshot.Dependencies)),
		Containment:  uint64(len(snapshot.Containment)),
		ExternalKeys: uint64(len(snapshot.ExternalKeys)),
		LogEntries:   uint64(len(snapshot.LogEntries)),
		States:       uint64(len(snapshot.States)), Results: uint64(len(snapshot.Results)),
		Checkpoints: uint64(len(snapshot.Checkpoints)),
		Attachments: uint64(len(snapshot.Attachments)),
	}
	return func(yield func(Record, error) bool) {
		if !yield(RecordHeader{
			Version: CopySnapshotVersion, SourceLineageID: snapshot.SourceLineageID,
			SourceRevision: snapshot.SourceRevision, Board: snapshot.Board,
			Configuration: snapshot.Configuration,
		}, nil) {
			return
		}
		for _, records := range [][]Record{
			copyTestRecordValues(snapshot.Issues),
			copyTestRecordValues(snapshot.Labels),
			copyTestRecordValues(snapshot.Dependencies),
			copyTestRecordValues(snapshot.Containment),
			copyTestRecordValues(snapshot.ExternalKeys),
			copyTestRecordValues(snapshot.LogEntries),
			copyTestRecordValues(snapshot.States),
			copyTestRecordValues(snapshot.Results),
			copyTestRecordValues(snapshot.Checkpoints),
			copyTestRecordValues(snapshot.Attachments),
		} {
			for _, record := range records {
				if !yield(record, nil) {
					return
				}
			}
		}
		yield(RecordTrailer{Counts: counts}, nil)
	}
}

func copyTestIndex(t *testing.T, snapshot copyTestSnapshot) RecordIndex {
	t.Helper()
	index, err := IndexRecords(copyTestRecords(snapshot))
	require.NoError(t, err)
	return index
}

func copyTestRecordValues[T Record](values []T) []Record {
	records := make([]Record, 0, len(values))
	for _, value := range values {
		records = append(records, value)
	}
	return records
}
