package board

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestRepository_RecordSequenceUsesKeysetPagesFromOneView(t *testing.T) {
	persistence := openCopyTestStore(t, t.TempDir())
	seedCopySource(t, persistence, copyTestBlobDescriptor(t))
	repository, err := New(persistence, Config{BoardID: "board-source"})
	require.NoError(t, err)
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO issue_labels (board_id, issue_id, label) VALUES
    ('board-source', 'src-1', 'area:z'),
    ('board-source', 'src-2', 'area:a');
INSERT INTO dependencies (board_id, issue_id, prerequisite_id)
VALUES ('board-source', 'src-2', 'src-4');
INSERT INTO containment (board_id, child_id, parent_id)
VALUES ('board-source', 'src-2', 'src-3');
INSERT INTO issue_external_keys (board_id, external_key, issue_id)
VALUES ('board-source', 'external-2', 'src-2');
UPDATE boards SET revision = 3 WHERE id = 'board-source';
UPDATE store_state SET current_revision = 3 WHERE singleton = 1`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	records := make([]boardcopy.Record, 0, 23)
	for record, recordErr := range repository.readCopyRecords(
		t.Context(),
		"board-source",
		configuration.Overrides{},
		1,
	) {
		require.NoError(t, recordErr)
		records = append(records, record)
		if len(records) != 2 {
			continue
		}
		assert.Equal(t, int64(3), records[0].(boardcopy.RecordHeader).SourceRevision)
		assert.Equal(
			t,
			uint64(3),
			records[0].(boardcopy.RecordHeader).Configuration.Board.Pins.MaxCount.Uint64(),
		)
		assert.Equal(t, "src-1", records[1].(boardcopy.CopyIssue).ID)

		change, err := persistence.Change(t.Context())
		require.NoError(t, err)
		_, err = change.ExecContext(t.Context(), `
UPDATE issues
SET title = 'Changed after page one', revision = 4
WHERE id = 'src-2';
INSERT INTO issues (
    id, board_id, title, kind, lifecycle, priority, created_at, updated_at,
    revision
) VALUES (
    'src-5', 'board-source', 'Added after page one', 'task', 'open', 2,
    1003, 1003, 4
);
UPDATE boards SET revision = 4 WHERE id = 'board-source';
UPDATE store_state
SET current_revision = 4
WHERE singleton = 1`)
		require.NoError(t, err)
		require.NoError(t, change.Commit())
		require.NoError(t, change.Done())
	}

	assert.Equal(t, []boardcopy.RecordType{
		boardcopy.RecordTypeHeader,
		boardcopy.RecordTypeIssue,
		boardcopy.RecordTypeIssue,
		boardcopy.RecordTypeIssue,
		boardcopy.RecordTypeIssue,
		boardcopy.RecordTypeLabel,
		boardcopy.RecordTypeLabel,
		boardcopy.RecordTypeLabel,
		boardcopy.RecordTypeDependency,
		boardcopy.RecordTypeDependency,
		boardcopy.RecordTypeContainment,
		boardcopy.RecordTypeContainment,
		boardcopy.RecordTypeExternalKey,
		boardcopy.RecordTypeExternalKey,
		boardcopy.RecordTypeLogEntry,
		boardcopy.RecordTypeLogEntry,
		boardcopy.RecordTypeState,
		boardcopy.RecordTypeResult,
		boardcopy.RecordTypeCheckpoint,
		boardcopy.RecordTypeAttachment,
		boardcopy.RecordTypePin,
		boardcopy.RecordTypePin,
		boardcopy.RecordTypeTrailer,
	}, recordTypes(records))

	issues := make([]boardcopy.CopyIssue, 0, 4)
	for _, record := range records {
		if value, ok := record.(boardcopy.CopyIssue); ok {
			issues = append(issues, value)
		}
	}
	require.Len(t, issues, 4)
	assert.Equal(t, "Reserved source identity", issues[1].Title)
	assert.Equal(t, "src-4", issues[3].ID)
	labels := make([]boardcopy.CopyLabel, 0, 3)
	for _, record := range records {
		if value, ok := record.(boardcopy.CopyLabel); ok {
			labels = append(labels, value)
		}
	}
	assert.Equal(t, []boardcopy.CopyLabel{
		{IssueID: "src-1", Label: "area:copy"},
		{IssueID: "src-1", Label: "area:z"},
		{IssueID: "src-2", Label: "area:a"},
	}, labels)
	var pins []boardcopy.CopyPin
	for _, record := range records {
		if value, ok := record.(boardcopy.CopyPin); ok {
			pins = append(pins, value)
		}
	}
	assert.Equal(t, []boardcopy.CopyPin{
		{Order: 0, IssueID: "src-2"},
		{Order: 1, IssueID: "src-1"},
	}, pins)

	assert.Equal(t, boardcopy.RecordTrailer{Counts: boardcopy.RecordCounts{
		Issues: 4, Labels: 3, Dependencies: 2, Containment: 2,
		ExternalKeys: 2, LogEntries: 2, States: 1, Results: 1,
		Checkpoints: 1, Attachments: 1, Pins: 2,
	}}, records[len(records)-1])
}

func TestRepository_RecordSequenceClosesOwnedViewAfterEarlyStop(t *testing.T) {
	persistence := openCopyTestStore(t, t.TempDir())
	seedCopySource(t, persistence, copyTestBlobDescriptor(t))
	repository, err := New(persistence, Config{BoardID: "board-source"})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)

	for range 8 {
		for _, recordErr := range repository.ReadCopyRecords(
			ctx,
			"board-source",
			configuration.Overrides{},
		) {
			require.NoError(t, recordErr)
			break
		}
	}
	assert.NoError(t, ctx.Err())
}

func TestRepository_RecordSequenceClosesOwnedViewAfterPinStop(t *testing.T) {
	persistence := openCopyTestStore(t, t.TempDir())
	seedCopySource(t, persistence, copyTestBlobDescriptor(t))
	repository, err := New(persistence, Config{BoardID: "board-source"})
	require.NoError(t, err)

	for record, recordErr := range repository.readCopyRecords(
		t.Context(),
		"board-source",
		configuration.Overrides{},
		1,
	) {
		require.NoError(t, recordErr)
		if boardcopy.RecordTypeOf(record) == boardcopy.RecordTypePin {
			break
		}
	}

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	assert.NoError(t, view.Done())
}

func TestBackupReader_RecordSequenceBorrowsView(t *testing.T) {
	persistence := openCopyTestStore(t, t.TempDir())
	seedCopySource(t, persistence, copyTestBlobDescriptor(t))
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = view.Done() })
	lineageID, err := view.LineageID(t.Context())
	require.NoError(t, err)

	for _, recordErr := range (&BackupReader{}).ReadBackupRecords(
		t.Context(),
		view,
		"board-source",
		configuration.Overrides{},
		BackupSource{LineageID: lineageID, Revision: 2},
	) {
		require.NoError(t, recordErr)
		break
	}

	revision, err := view.CanonicalRevision(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(2), revision)
	assert.NoError(t, view.Done())
}

func recordTypes(records []boardcopy.Record) []boardcopy.RecordType {
	types := make([]boardcopy.RecordType, 0, len(records))
	for _, record := range records {
		types = append(types, boardcopy.RecordTypeOf(record))
	}
	return types
}
