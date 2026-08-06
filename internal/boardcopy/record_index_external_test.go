package boardcopy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestRecordIndexUsesDeterministicSemanticFrames(t *testing.T) {
	createdAt := time.Unix(1000, 0).UTC()
	records := []boardcopy.Record{
		boardcopy.RecordHeader{
			Version: boardcopy.CopySnapshotVersion, SourceLineageID: "lineage-one",
			SourceRevision: 7,
			Board: boardcopy.CopyBoard{
				ID: "board-source", Name: "Source", CreatedAt: createdAt,
			},
			Configuration: configuration.Defaults(),
		},
		copyRecordTestIssue("cm-one", "One", createdAt),
		boardcopy.RecordTrailer{Counts: boardcopy.RecordCounts{Issues: 1}},
	}
	first, err := boardcopy.IndexRecords(
		recordSequence(records...),
	)
	require.NoError(t, err)

	changedView := append([]boardcopy.Record(nil), records...)
	header := changedView[0].(boardcopy.RecordHeader)
	header.SourceLineageID = "lineage-two"
	header.SourceRevision = 19
	changedView[0] = header
	second, err := boardcopy.IndexRecords(
		recordSequence(changedView...),
	)
	require.NoError(t, err)

	assert.Equal(t, first.Digest, second.Digest)
	assert.Equal(t, "lineage-one", first.Header.SourceLineageID)
	assert.Equal(t, int64(7), first.Header.SourceRevision)
	assert.Equal(t, []string{"cm-one"}, first.IssueIDs)

	changedRecord := append([]boardcopy.Record(nil), records...)
	issue := changedRecord[1].(boardcopy.CopyIssue)
	issue.Title = "Changed"
	changedRecord[1] = issue
	third, err := boardcopy.IndexRecords(
		recordSequence(changedRecord...),
	)
	require.NoError(t, err)
	assert.NotEqual(t, first.Digest, third.Digest)
}

func TestRecordIndexUsesVersionTwoDigestContract(t *testing.T) {
	createdAt := time.Date(2026, time.July, 29, 9, 30, 0, 123, time.UTC)
	closedAt := createdAt.Add(time.Hour)
	summary := "Summary"
	actor := "worker"
	digest, err := attachment.NewDigest(
		"sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
	)
	require.NoError(t, err)

	index, err := boardcopy.IndexRecords(
		recordSequence(
			boardcopy.RecordHeader{
				Version:         boardcopy.CopySnapshotVersion,
				SourceLineageID: "store_0123456789abcdef0123456789abcdef",
				SourceRevision:  12,
				Board: boardcopy.CopyBoard{
					ID: "board-source", Name: "Source", CreatedAt: createdAt,
				},
				Configuration: configuration.Defaults(),
			},
			boardcopy.CopyIssue{
				ID: "cm-1", Title: "First", Kind: "workstream",
				Lifecycle: "open", Priority: 2,
				CreatedAt: createdAt, UpdatedAt: createdAt,
			},
			boardcopy.CopyIssue{
				ID: "cm-2", Title: "Second", Kind: "task",
				Lifecycle: "closed", Priority: 3,
				CreatedAt: createdAt, UpdatedAt: closedAt,
				ClosedAt: &closedAt, Summary: &summary,
			},
			boardcopy.CopyLabel{IssueID: "cm-1", Label: "alpha"},
			boardcopy.CopyLabel{IssueID: "cm-2", Label: "beta"},
			boardcopy.CopyDependency{
				IssueID: "cm-2", PrerequisiteID: "cm-1",
			},
			boardcopy.CopyContainment{ChildID: "cm-2", ParentID: "cm-1"},
			boardcopy.CopyExternalKey{Key: "external-2", IssueID: "cm-2"},
			boardcopy.CopyLogEntry{
				Order: 0, ID: "log_first", IssueID: "cm-1",
				Kind: "state_snapshot", Body: "First Log",
			},
			boardcopy.CopyLogEntry{
				Order: 1, ID: "log_second", IssueID: "cm-2",
				Kind: "post", Author: &actor, Committer: &actor,
				Body: "Second Log", CreatedAt: &closedAt,
			},
			boardcopy.CopyState{
				IssueID: "cm-1", Body: "State",
				SnapshotLogEntryID: new("log_first"),
			},
			boardcopy.CopyResultRecord{IssueID: "cm-2", Body: "Result"},
			boardcopy.CopyCheckpoint{
				IssueID: "cm-2", Outcome: "approved", Reason: "Ready",
				DecidedAt: closedAt,
			},
			boardcopy.CopyAttachment{
				ID: "att_evidence", OriginIssueID: new("cm-2"),
				Blob:     attachment.BlobDescriptor{Digest: digest, SizeBytes: 4},
				Filename: "evidence.txt", MediaType: "text/plain",
				Lifecycle: "removed", CreatedActor: actor,
				CreatedAt: createdAt, RemovedActor: &actor, RemovedAt: &closedAt,
			},
			boardcopy.RecordTrailer{Counts: boardcopy.RecordCounts{
				Issues: 2, Labels: 2, Dependencies: 1, Containment: 1,
				ExternalKeys: 1, LogEntries: 2, States: 1, Results: 1,
				Checkpoints: 1, Attachments: 1,
			}},
		),
	)
	require.NoError(t, err)
	assert.Equal(t, 2, index.Header.Version)
	assert.Equal(t, boardcopy.RecordCounts{
		Issues: 2, Labels: 2, Dependencies: 1, Containment: 1,
		ExternalKeys: 1, LogEntries: 2, States: 1, Results: 1,
		Checkpoints: 1, Attachments: 1,
	}, index.Counts)
	assert.Equal(
		t,
		"sha256:94a8bcbd08cffc175024602bce96e1d10c6790fe5903760445c747fec6c5eb6c",
		index.Digest,
	)
}

func TestRecordIndexRejectsIncompleteOrNoncanonicalRecords(t *testing.T) {
	header := boardcopy.RecordHeader{
		Version: boardcopy.CopySnapshotVersion, SourceLineageID: "lineage",
		Board: boardcopy.CopyBoard{
			ID: "board-source", Name: "Source", CreatedAt: time.Unix(1000, 0).UTC(),
		},
		Configuration: configuration.Defaults(),
	}

	t.Run("VersionOne", func(t *testing.T) {
		versionOne := header
		versionOne.Version = 1
		_, err := boardcopy.IndexRecords(recordSequence(
			versionOne,
			boardcopy.RecordTrailer{},
		))
		require.Error(t, err)
		assert.ErrorContains(t, err, "unsupported board record version 1")
	})

	t.Run("MissingRecord", func(t *testing.T) {
		_, err := boardcopy.IndexRecords(
			recordSequence(
				header,
				copyRecordTestIssue("cm-one", "One", header.Board.CreatedAt),
			),
		)
		require.Error(t, err)
		assert.ErrorContains(t, err, "trailer is required")
	})

	t.Run("MissingHeader", func(t *testing.T) {
		_, err := boardcopy.IndexRecords(recordSequence(
			copyRecordTestIssue("cm-one", "One", header.Board.CreatedAt),
			boardcopy.RecordTrailer{},
		))
		require.Error(t, err)
		assert.ErrorContains(t, err, "header is required first")
	})

	t.Run("TrailerCount", func(t *testing.T) {
		_, err := boardcopy.IndexRecords(
			recordSequence(
				header,
				copyRecordTestIssue("cm-one", "One", header.Board.CreatedAt),
				boardcopy.RecordTrailer{},
			),
		)
		require.Error(t, err)
		assert.ErrorContains(t, err, "trailer counts")
	})

	t.Run("SectionOrder", func(t *testing.T) {
		_, err := boardcopy.IndexRecords(
			recordSequence(
				header,
				boardcopy.CopyLabel{IssueID: "cm-one", Label: "area:one"},
				copyRecordTestIssue("cm-one", "One", header.Board.CreatedAt),
			),
		)
		require.Error(t, err)
		assert.ErrorContains(t, err, "follows type")
	})

	attachmentDigest, err := attachment.NewDigest(
		"sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
	)
	require.NoError(t, err)
	attachmentBlob := attachment.BlobDescriptor{
		Digest: attachmentDigest, SizeBytes: 4,
	}
	keyOrderTests := []struct {
		name    string
		records []boardcopy.Record
	}{
		{
			name: "IssuesByID",
			records: []boardcopy.Record{
				copyRecordTestIssue("cm-two", "Two", header.Board.CreatedAt),
				copyRecordTestIssue("cm-one", "One", header.Board.CreatedAt),
			},
		},
		{
			name: "LabelsByIssueAndValue",
			records: []boardcopy.Record{
				boardcopy.CopyLabel{IssueID: "cm-two", Label: "area:a"},
				boardcopy.CopyLabel{IssueID: "cm-one", Label: "area:z"},
			},
		},
		{
			name: "DependenciesByIssueAndPrerequisite",
			records: []boardcopy.Record{
				boardcopy.CopyDependency{IssueID: "cm-two", PrerequisiteID: "cm-one"},
				boardcopy.CopyDependency{IssueID: "cm-one", PrerequisiteID: "cm-two"},
			},
		},
		{
			name: "ContainmentByChild",
			records: []boardcopy.Record{
				boardcopy.CopyContainment{ChildID: "cm-two", ParentID: "cm-one"},
				boardcopy.CopyContainment{ChildID: "cm-one", ParentID: "cm-two"},
			},
		},
		{
			name: "ExternalKeysByKeyAndIssue",
			records: []boardcopy.Record{
				boardcopy.CopyExternalKey{Key: "key-two", IssueID: "cm-one"},
				boardcopy.CopyExternalKey{Key: "key-one", IssueID: "cm-two"},
			},
		},
		{
			name: "StatesByIssue",
			records: []boardcopy.Record{
				boardcopy.CopyState{IssueID: "cm-two"},
				boardcopy.CopyState{IssueID: "cm-one"},
			},
		},
		{
			name: "ResultsByIssue",
			records: []boardcopy.Record{
				boardcopy.CopyResultRecord{IssueID: "cm-two"},
				boardcopy.CopyResultRecord{IssueID: "cm-one"},
			},
		},
		{
			name: "CheckpointsByIssue",
			records: []boardcopy.Record{
				boardcopy.CopyCheckpoint{IssueID: "cm-two"},
				boardcopy.CopyCheckpoint{IssueID: "cm-one"},
			},
		},
		{
			name: "AttachmentsByID",
			records: []boardcopy.Record{
				boardcopy.CopyAttachment{ID: "att_two", Blob: attachmentBlob},
				boardcopy.CopyAttachment{ID: "att_one", Blob: attachmentBlob},
			},
		},
	}
	for _, test := range keyOrderTests {
		t.Run(test.name, func(t *testing.T) {
			records := append([]boardcopy.Record{header}, test.records...)
			_, err := boardcopy.IndexRecords(recordSequence(records...))
			require.Error(t, err)
			assert.ErrorContains(t, err, "out of order")
		})
	}

	t.Run("LogEntriesByContiguousOrder", func(t *testing.T) {
		_, err := boardcopy.IndexRecords(recordSequence(
			header,
			boardcopy.CopyLogEntry{Order: 1, ID: "log_second"},
		))
		require.Error(t, err)
		assert.ErrorContains(t, err, "order is 1, expected 0")
	})

	t.Run("RecordAfterTrailer", func(t *testing.T) {
		_, err := boardcopy.IndexRecords(recordSequence(
			header,
			boardcopy.RecordTrailer{},
			boardcopy.RecordTrailer{},
		))
		require.Error(t, err)
		assert.ErrorContains(t, err, "record follows trailer")
	})
}

func TestRecordTypeOfUsesCanonicalSectionOrder(t *testing.T) {
	records := []boardcopy.Record{
		boardcopy.RecordHeader{},
		boardcopy.CopyIssue{},
		boardcopy.CopyLabel{},
		boardcopy.CopyDependency{},
		boardcopy.CopyContainment{},
		boardcopy.CopyExternalKey{},
		boardcopy.CopyLogEntry{},
		boardcopy.CopyState{},
		boardcopy.CopyResultRecord{},
		boardcopy.CopyCheckpoint{},
		boardcopy.CopyAttachment{},
		boardcopy.RecordTrailer{},
	}
	want := []boardcopy.RecordType{
		boardcopy.RecordTypeHeader,
		boardcopy.RecordTypeIssue,
		boardcopy.RecordTypeLabel,
		boardcopy.RecordTypeDependency,
		boardcopy.RecordTypeContainment,
		boardcopy.RecordTypeExternalKey,
		boardcopy.RecordTypeLogEntry,
		boardcopy.RecordTypeState,
		boardcopy.RecordTypeResult,
		boardcopy.RecordTypeCheckpoint,
		boardcopy.RecordTypeAttachment,
		boardcopy.RecordTypeTrailer,
	}

	got := make([]boardcopy.RecordType, 0, len(records))
	for _, record := range records {
		got = append(got, boardcopy.RecordTypeOf(record))
	}
	assert.Equal(t, want, got)
	assert.Equal(t, boardcopy.RecordTypeUnknown, boardcopy.RecordTypeOf(nil))
}

func copyRecordTestIssue(
	id string,
	title string,
	createdAt time.Time,
) boardcopy.CopyIssue {
	return boardcopy.CopyIssue{
		ID: id, Title: title, Kind: "task", Lifecycle: "open", Priority: 2,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func recordSequence(records ...boardcopy.Record) boardcopy.RecordSequence {
	return func(yield func(boardcopy.Record, error) bool) {
		for _, record := range records {
			if !yield(record, nil) {
				return
			}
		}
	}
}
