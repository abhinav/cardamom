package boardcopy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestCopyService_CopyPublishesUniqueBlobsBeforeMetadata(t *testing.T) {
	digest, err := attachment.NewDigest(
		"sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
	)
	require.NoError(t, err)
	descriptor := attachment.BlobDescriptor{Digest: digest, SizeBytes: 4}
	dependencies := &copyDependencies{
		snapshot: copyTestSnapshot{
			SourceLineageID: "store_0123456789abcdef0123456789abcdef",
			SourceRevision:  7,
			Board: CopyBoard{
				ID: "board-source", Name: "Source",
			},
			Configuration: configuration.Defaults(),
			Attachments: []CopyAttachment{
				{ID: "att_first", Blob: descriptor},
				{ID: "att_second", Blob: descriptor},
			},
		},
		blob: []byte("data"),
		importResult: NewPublishedCopyImport(CopyOutcome{
			DestinationBoardID: "board-source",
		}),
	}
	service := NewCopyService(CopyServiceConfig{
		Source: dependencies, Destination: dependencies,
		SourceBlobs: dependencies, DestinationBlobs: dependencies,
		Configuration: dependencies,
	})

	outcome, err := service.Copy(t.Context(), CopyRequest{
		SourceBoardID: "board-source",
		Options:       CopyOptions{ProjectID: "project-destination"},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, dependencies.openBlobCalls)
	assert.Equal(t, 1, dependencies.publishBlobCalls)
	assert.True(t, dependencies.importCalled)
	assert.Equal(t, "data", string(dependencies.published))
	assert.Equal(t, 1, outcome.Counts.Blobs)
	assert.NotEmpty(t, dependencies.importIndex.Digest)
	assert.Equal(t, CopySnapshotVersion, dependencies.importIndex.Header.Version)
	assert.Equal(t, CopyReceiptKey{
		SourceLineageID: dependencies.snapshot.SourceLineageID,
		SourceBoardID:   dependencies.snapshot.Board.ID,
		SnapshotVersion: CopySnapshotVersion,
	}, dependencies.receiptKey)
}

func TestCopyService_CopyReturnsReceiptBeforeReadingBlobs(t *testing.T) {
	digest, err := attachment.NewDigest(
		"sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
	)
	require.NoError(t, err)
	snapshot := copyTestSnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
		Configuration: configuration.Defaults(),
		Attachments: []CopyAttachment{{
			ID: "att_first",
			Blob: attachment.BlobDescriptor{
				Digest: digest, SizeBytes: 4,
			},
		}},
	}
	index := copyTestIndex(t, snapshot)
	dependencies := &copyDependencies{
		snapshot:     snapshot,
		receiptFound: true,
		receipt: CopyReceipt{
			SourceLineageID:      snapshot.SourceLineageID,
			SourceBoardID:        snapshot.Board.ID,
			SourceRevision:       3,
			SnapshotVersion:      index.Header.Version,
			SnapshotDigest:       index.Digest,
			DestinationProjectID: "project-destination",
			DestinationBoardID:   "board-prior",
			DestinationName:      "Source",
			DestinationRevision:  11,
		},
	}
	service := NewCopyService(CopyServiceConfig{
		Source: dependencies, Destination: dependencies,
		SourceBlobs: dependencies, DestinationBlobs: dependencies,
		Configuration: dependencies,
	})

	outcome, err := service.Copy(t.Context(), CopyRequest{
		SourceBoardID: "board-source",
		Options:       CopyOptions{ProjectID: "project-destination"},
	})
	require.NoError(t, err)

	assert.Equal(t, "board-prior", outcome.DestinationBoardID)
	assert.Equal(t, int64(3), outcome.SourceRevision)
	assert.True(t, outcome.AlreadyCompleted)
	assert.Equal(t, 1, outcome.Counts.Blobs)
	assert.Zero(t, dependencies.openBlobCalls)
	assert.Zero(t, dependencies.publishBlobCalls)
	assert.False(t, dependencies.importCalled)
}

func TestCopyService_CopyEvaluatesIdenticalReceiptFromImportRace(t *testing.T) {
	snapshot := copyTestSnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
		Configuration: configuration.Defaults(),
	}
	index := copyTestIndex(t, snapshot)
	dependencies := &copyDependencies{
		snapshot: snapshot,
		importResult: NewConcurrentCopyImport(CopyReceipt{
			SourceLineageID:      snapshot.SourceLineageID,
			SourceBoardID:        snapshot.Board.ID,
			SourceRevision:       3,
			SnapshotVersion:      index.Header.Version,
			SnapshotDigest:       index.Digest,
			DestinationProjectID: "project-destination",
			DestinationBoardID:   "board-prior",
			DestinationName:      "Source",
			DestinationRevision:  11,
		}),
	}
	service := NewCopyService(CopyServiceConfig{
		Source: dependencies, Destination: dependencies,
		SourceBlobs: dependencies, DestinationBlobs: dependencies,
		Configuration: dependencies,
	})

	outcome, err := service.Copy(t.Context(), CopyRequest{
		SourceBoardID: "board-source",
		Options:       CopyOptions{ProjectID: "project-destination"},
	})
	require.NoError(t, err)

	assert.True(t, dependencies.importCalled)
	assert.True(t, outcome.AlreadyCompleted)
	assert.Equal(t, int64(3), outcome.SourceRevision)
	assert.Equal(t, "board-prior", outcome.DestinationBoardID)
}

func TestCopyService_CopyRejectsConflictingReceiptFromImportRace(t *testing.T) {
	snapshot := copyTestSnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
		Configuration: configuration.Defaults(),
	}
	dependencies := &copyDependencies{
		snapshot: snapshot,
		importResult: NewConcurrentCopyImport(CopyReceipt{
			SourceLineageID:      snapshot.SourceLineageID,
			SourceBoardID:        snapshot.Board.ID,
			SourceRevision:       3,
			SnapshotVersion:      CopySnapshotVersion,
			SnapshotDigest:       "sha256:prior",
			DestinationProjectID: "project-destination",
			DestinationBoardID:   "board-prior",
			DestinationName:      "Source",
			DestinationRevision:  11,
		}),
	}
	service := NewCopyService(CopyServiceConfig{
		Source: dependencies, Destination: dependencies,
		SourceBlobs: dependencies, DestinationBlobs: dependencies,
		Configuration: dependencies,
	})

	_, err := service.Copy(t.Context(), CopyRequest{
		SourceBoardID: "board-source",
		Options:       CopyOptions{ProjectID: "project-destination"},
	})
	require.Error(t, err)

	assert.True(t, dependencies.importCalled)
	assert.ErrorContains(t, err, "incremental synchronization is not supported")
}

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

func TestCopyService_CopyRejectsConfigurationChange(t *testing.T) {
	before := configuration.Overrides{}
	after := configuration.Overrides{}
	prefix, err := configuration.NewPrefix("changed-")
	require.NoError(t, err)
	after.Issue.ID.Prefix = &prefix
	dependencies := &copyDependencies{
		snapshot: copyTestSnapshot{
			SourceLineageID: "store_0123456789abcdef0123456789abcdef",
			Board: CopyBoard{
				ID: "board-source", Name: "Source",
			},
			Configuration: configuration.Defaults(),
		},
		configurations: []configuration.Overrides{before, after},
	}
	service := NewCopyService(CopyServiceConfig{
		Source: dependencies, Destination: dependencies,
		SourceBlobs: dependencies, DestinationBlobs: dependencies,
		Configuration: dependencies,
	})

	_, err = service.Copy(t.Context(), CopyRequest{
		SourceBoardID: "board-source",
		Options:       CopyOptions{ProjectID: "project-destination"},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "configuration changed")
	assert.False(t, dependencies.importCalled)
}

type copyDependencies struct {
	snapshot       copyTestSnapshot
	importIndex    RecordIndex
	importResult   CopyImportResult
	receipt        CopyReceipt
	receiptKey     CopyReceiptKey
	receiptFound   bool
	configurations []configuration.Overrides
	blob           []byte
	published      []byte

	configurationReads int
	openBlobCalls      int
	publishBlobCalls   int
	importCalled       bool
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

func (d *copyDependencies) ReadStoreConfiguration(
	context.Context,
) (configuration.Overrides, error) {
	if len(d.configurations) == 0 {
		return configuration.Overrides{}, nil
	}
	value := d.configurations[d.configurationReads]
	d.configurationReads++
	return value, nil
}

func (d *copyDependencies) ReadCopyRecords(
	context.Context,
	board.ID,
	configuration.Overrides,
) RecordSequence {
	return copyTestRecords(d.snapshot)
}

func (d *copyDependencies) ReadCopyReceipt(
	_ context.Context,
	key CopyReceiptKey,
) (CopyReceipt, bool, error) {
	d.receiptKey = key
	return d.receipt, d.receiptFound, nil
}

func (d *copyDependencies) ImportCopyRecords(
	_ context.Context,
	index RecordIndex,
	records RecordSequence,
	_ CopyOptions,
) (CopyImportResult, error) {
	d.importCalled = true
	d.importIndex = index
	_, err := IndexRecords(records)
	if err != nil {
		return nil, err
	}
	return d.importResult, nil
}

func (d *copyDependencies) OpenCopyBlob(
	context.Context,
	attachment.BlobDescriptor,
) (io.ReadCloser, error) {
	d.openBlobCalls++
	if d.blob == nil {
		return nil, errors.New("blob reads are forbidden")
	}
	return io.NopCloser(bytes.NewReader(d.blob)), nil
}

func (d *copyDependencies) PublishCopyBlob(
	_ context.Context,
	_ attachment.BlobDescriptor,
	reader io.Reader,
) error {
	d.publishBlobCalls++
	body, err := io.ReadAll(reader)
	d.published = body
	return err
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
