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
		snapshot: CopySnapshot{
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
	assert.NotEmpty(t, dependencies.importSnapshot.Digest)
	assert.Equal(t, CopySnapshotVersion, dependencies.importSnapshot.Version)
}

func TestCopyService_CopyReturnsReceiptBeforeReadingBlobs(t *testing.T) {
	digest, err := attachment.NewDigest(
		"sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
	)
	require.NoError(t, err)
	snapshot := CopySnapshot{
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
	digestSnapshot := canonicalCopySnapshot(snapshot)
	digestSnapshot.Version = CopySnapshotVersion
	digestSnapshot.Digest = snapshotDigest(digestSnapshot)
	dependencies := &copyDependencies{
		snapshot:     snapshot,
		receiptFound: true,
		receipt: CopyReceipt{
			SourceLineageID:      snapshot.SourceLineageID,
			SourceBoardID:        snapshot.Board.ID,
			SourceRevision:       3,
			SnapshotVersion:      digestSnapshot.Version,
			SnapshotDigest:       digestSnapshot.Digest,
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
	snapshot := CopySnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
		Configuration: configuration.Defaults(),
	}
	digestSnapshot := snapshot
	digestSnapshot.Version = CopySnapshotVersion
	digestSnapshot = canonicalCopySnapshot(digestSnapshot)
	digestSnapshot.Digest = snapshotDigest(digestSnapshot)
	dependencies := &copyDependencies{
		snapshot: snapshot,
		importResult: NewCompetingCopyImport(CopyReceipt{
			SourceLineageID:      snapshot.SourceLineageID,
			SourceBoardID:        snapshot.Board.ID,
			SourceRevision:       3,
			SnapshotVersion:      digestSnapshot.Version,
			SnapshotDigest:       digestSnapshot.Digest,
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
	snapshot := CopySnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
		Configuration: configuration.Defaults(),
	}
	dependencies := &copyDependencies{
		snapshot: snapshot,
		importResult: NewCompetingCopyImport(CopyReceipt{
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

func TestEvaluateCopyReceiptRejectsChangedPublication(t *testing.T) {
	snapshot := CopySnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Version:         CopySnapshotVersion,
		Digest:          "current",
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
	}
	receipt := CopyReceipt{
		SourceLineageID:      snapshot.SourceLineageID,
		SourceBoardID:        snapshot.Board.ID,
		SourceRevision:       3,
		SnapshotVersion:      snapshot.Version,
		SnapshotDigest:       "prior",
		DestinationProjectID: "project-destination",
		DestinationBoardID:   "board-prior",
		DestinationName:      "Source",
		DestinationRevision:  11,
	}

	_, err := EvaluateCopyReceipt(
		snapshot,
		CopyOptions{ProjectID: "project-destination"},
		receipt,
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "incremental synchronization is not supported")
}

func TestEvaluateCopyReceiptRejectsDifferentDestinationOptions(t *testing.T) {
	snapshot := CopySnapshot{
		SourceLineageID: "store_0123456789abcdef0123456789abcdef",
		SourceRevision:  7,
		Version:         CopySnapshotVersion,
		Digest:          "same",
		Board: CopyBoard{
			ID: "board-source", Name: "Source",
		},
	}
	receipt := CopyReceipt{
		SourceLineageID:      snapshot.SourceLineageID,
		SourceBoardID:        snapshot.Board.ID,
		SourceRevision:       3,
		SnapshotVersion:      snapshot.Version,
		SnapshotDigest:       snapshot.Digest,
		DestinationProjectID: "project-destination",
		DestinationBoardID:   "board-prior",
		DestinationName:      "Source",
		DestinationRevision:  11,
	}
	renamed := "Renamed"

	_, err := EvaluateCopyReceipt(
		snapshot,
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
		snapshot: CopySnapshot{
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
	snapshot       CopySnapshot
	importSnapshot CopySnapshot
	importResult   CopyImportResult
	receipt        CopyReceipt
	receiptFound   bool
	configurations []configuration.Overrides
	blob           []byte
	published      []byte

	configurationReads int
	openBlobCalls      int
	publishBlobCalls   int
	importCalled       bool
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

func (d *copyDependencies) ReadCopySnapshot(
	context.Context,
	board.ID,
	configuration.Overrides,
) (CopySnapshot, error) {
	return d.snapshot, nil
}

func (d *copyDependencies) ReadCopyReceipt(
	context.Context,
	CopyReceiptKey,
) (CopyReceipt, bool, error) {
	return d.receipt, d.receiptFound, nil
}

func (d *copyDependencies) ImportCopySnapshot(
	_ context.Context,
	snapshot CopySnapshot,
	_ CopyOptions,
) (CopyImportResult, error) {
	d.importCalled = true
	d.importSnapshot = snapshot
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
