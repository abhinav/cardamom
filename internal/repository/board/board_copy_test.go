package board

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	repositoryattachment "go.abhg.dev/cardamom/internal/repository/attachment"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestCopyRepositoryCopiesCompleteBoardAndRemapsCollisions(t *testing.T) {
	root := t.TempDir()
	sourceDirectory := filepath.Join(root, "source")
	destinationDirectory := filepath.Join(root, "destination")
	require.NoError(t, os.MkdirAll(sourceDirectory, 0o700))
	require.NoError(t, os.MkdirAll(destinationDirectory, 0o700))

	sourceStore := openCopyTestStore(t, sourceDirectory)
	destinationStore := openCopyTestStore(t, destinationDirectory)
	descriptor := copyTestBlobDescriptor(t)
	sourceAttachments := openCopyAttachmentRepository(
		t,
		sourceStore,
		sourceDirectory,
	)
	destinationAttachments := openCopyAttachmentRepository(
		t,
		destinationStore,
		destinationDirectory,
	)
	require.NoError(t, sourceAttachments.PublishCopyBlob(
		t.Context(),
		descriptor,
		strings.NewReader("data"),
	))
	seedCopySource(t, sourceStore, descriptor)
	seedCopyDestinationCollisions(t, destinationStore)

	sourceRepository, err := New(sourceStore, Config{
		BoardID: "board-source",
	})
	require.NoError(t, err)
	destinationRepository, err := NewCopyRepository(
		destinationStore,
		CopyRepositoryConfig{Entropy: copyTestEntropy()},
	)
	require.NoError(t, err)
	service := boardcopy.NewCopyService(boardcopy.CopyServiceConfig{
		Source:           sourceRepository,
		Destination:      destinationRepository,
		SourceBlobs:      sourceAttachments,
		DestinationBlobs: destinationAttachments,
		Configuration:    copyTestConfiguration{},
	})

	outcome, err := service.Copy(t.Context(), boardcopy.CopyRequest{
		SourceBoardID: "board-source",
		Options: boardcopy.CopyOptions{
			ProjectID: "project-destination",
		},
	})
	require.NoError(t, err)

	assert.NotEqual(t, "board-source", outcome.DestinationBoardID)
	require.Len(t, outcome.Mappings.Issues, 4)
	assert.Equal(t, boardcopy.CopyIdentityMap{
		Source: "src-1", Destination: "src-5",
	}, outcome.Mappings.Issues[0])
	assert.Equal(t, boardcopy.CopyIdentityMap{
		Source: "src-2", Destination: "src-2",
	}, outcome.Mappings.Issues[1])
	assert.Equal(t, boardcopy.CopyIdentityMap{
		Source: "src-3", Destination: "src-3",
	}, outcome.Mappings.Issues[2])
	assert.Equal(t, boardcopy.CopyIdentityMap{
		Source: "src-4", Destination: "src-4",
	}, outcome.Mappings.Issues[3])
	require.Len(t, outcome.Mappings.LogEntries, 2)
	assert.Equal(t, boardcopy.CopyIdentityMap{
		Source: copyTestLogID, Destination: copyTestRemappedLogID,
	}, outcome.Mappings.LogEntries[0])
	assert.Equal(t, boardcopy.CopyIdentityMap{
		Source: copyTestLaterLogID, Destination: copyTestLaterLogID,
	}, outcome.Mappings.LogEntries[1])
	assert.Equal(t, []boardcopy.CopyIdentityMap{{
		Source: copyTestAttachmentID, Destination: copyTestAttachmentID,
	}}, outcome.Mappings.Attachments)
	assert.Equal(t, boardcopy.CopyCounts{
		Issues: 4, LogEntries: 2, Attachments: 1, Blobs: 1,
	}, outcome.Counts)

	view, err := destinationStore.View(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, view.Done()) })
	var (
		prefix, strategy                  string
		summaryMax, attachmentMax, pinMax int64
		boardRevision                     int64
		boardDescription                  string
	)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT
    issue_id_prefix, issue_id_strategy, issue_summary_max_bytes,
    attachment_max_bytes, board_pins_max_count, revision, description
FROM boards
WHERE id = ?`,
		outcome.DestinationBoardID,
	).Scan(
		&prefix,
		&strategy,
		&summaryMax,
		&attachmentMax,
		&pinMax,
		&boardRevision,
		&boardDescription,
	))
	assert.Equal(t, "src-", prefix)
	assert.Equal(t, "sequential", strategy)
	assert.Equal(t, int64(4096), summaryMax)
	assert.Equal(t, int64(8192), attachmentMax)
	assert.Equal(t, int64(3), pinMax)
	assert.Equal(t, outcome.DestinationRevision, boardRevision)
	assert.Equal(t, "Board %src-5", boardDescription)

	rows, err := view.QueryContext(t.Context(), `
SELECT issue_id
FROM board_pins
WHERE board_id = ?
ORDER BY position`, outcome.DestinationBoardID)
	require.NoError(t, err)
	var pinIDs []string
	for rows.Next() {
		var pinID string
		require.NoError(t, rows.Scan(&pinID))
		pinIDs = append(pinIDs, pinID)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	assert.Equal(t, []string{"src-2", "src-5"}, pinIDs)

	var summary string
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT summary
FROM issues
WHERE board_id = ? AND id = 'src-5'`,
		outcome.DestinationBoardID,
	).Scan(&summary))
	mappedLogID := outcome.Mappings.LogEntries[0].Destination
	assert.Equal(t,
		"See %src-5 and %"+mappedLogID+" and %"+copyTestAttachmentID+
			"; keep \\%src-1 and `%src-1`.",
		summary,
	)

	var (
		stateIssueID, stateLogID        string
		dependencyIssue, prerequisiteID string
		containmentChild, parentID      string
		checkpointOutcome               string
		checkpointReason                string
		attachmentLifecycle             string
		attachmentCreatedRevision       int64
		attachmentRemovedRevision       int64
		receiptCount                    int64
		nextIssueNumber                 int64
	)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT issue_id, snapshot_log_entry_id
FROM issue_states
WHERE board_id = ?`,
		outcome.DestinationBoardID,
	).Scan(&stateIssueID, &stateLogID))
	assert.Equal(t, "src-5", stateIssueID)
	assert.Equal(t, mappedLogID, stateLogID)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT issue_id, prerequisite_id
FROM dependencies
WHERE board_id = ?`,
		outcome.DestinationBoardID,
	).Scan(&dependencyIssue, &prerequisiteID))
	assert.Equal(t, "src-5", dependencyIssue)
	assert.Equal(t, "src-4", prerequisiteID)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT child_id, parent_id
FROM containment
WHERE board_id = ?`,
		outcome.DestinationBoardID,
	).Scan(&containmentChild, &parentID))
	assert.Equal(t, "src-5", containmentChild)
	assert.Equal(t, "src-3", parentID)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT outcome, reason
FROM checkpoint_decisions
WHERE board_id = ? AND issue_id = 'src-4'`,
		outcome.DestinationBoardID,
	).Scan(&checkpointOutcome, &checkpointReason))
	assert.Equal(t, "approved", checkpointOutcome)
	assert.Equal(t, "Ready after %src-5", checkpointReason)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT lifecycle, created_revision, removed_revision
FROM attachments
WHERE board_id = ? AND id = ?`,
		outcome.DestinationBoardID,
		copyTestAttachmentID,
	).Scan(
		&attachmentLifecycle,
		&attachmentCreatedRevision,
		&attachmentRemovedRevision,
	))
	assert.Equal(t, "removed", attachmentLifecycle)
	assert.Less(t, attachmentCreatedRevision, attachmentRemovedRevision)
	assert.Equal(t, outcome.DestinationRevision, attachmentRemovedRevision)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT count(*)
FROM board_copy_receipts
WHERE destination_board_id = ?`,
		outcome.DestinationBoardID,
	).Scan(&receiptCount))
	assert.Equal(t, int64(1), receiptCount)
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT next_issue_number
FROM store_state
WHERE singleton = 1`,
	).Scan(&nextIssueNumber))
	assert.Equal(t, int64(6), nextIssueNumber)
	require.NoError(t, view.Done())

	reader, err := destinationAttachments.OpenCopyBlob(t.Context(), descriptor)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, []byte("data"), body)

	retry, err := service.Copy(t.Context(), boardcopy.CopyRequest{
		SourceBoardID: "board-source",
		Options: boardcopy.CopyOptions{
			ProjectID: "project-destination",
		},
	})
	require.NoError(t, err)
	assert.True(t, retry.AlreadyCompleted)
	assert.Equal(t, outcome.DestinationBoardID, retry.DestinationBoardID)
	assert.Equal(t, int64(2), retry.SourceRevision)

	index, err := boardcopy.IndexRecords(
		sourceRepository.ReadCopyRecords(
			t.Context(),
			"board-source",
			configuration.Overrides{},
		),
	)
	require.NoError(t, err)
	assert.Equal(t, outcome.SnapshotDigest, index.Digest)
	options := boardcopy.CopyOptions{ProjectID: "project-destination"}
	importResult, err := destinationRepository.ImportCopyRecords(
		t.Context(),
		index,
		sourceRepository.ReadCopyRecords(
			t.Context(),
			"board-source",
			configuration.Overrides{},
		),
		options,
	)
	require.NoError(t, err)
	concurrent, err := boardcopy.EvaluateCopyImport(index, options, importResult)
	require.NoError(t, err)
	assert.True(t, concurrent.AlreadyCompleted)
	assert.Equal(t, outcome.SnapshotDigest, concurrent.SnapshotDigest)
	assert.Equal(t, int64(2), concurrent.SourceRevision)
	assert.Equal(t, "Source", concurrent.DestinationName)

	change, err := sourceStore.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
UPDATE store_state
SET current_revision = 3
WHERE singleton = 1`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	retry, err = service.Copy(t.Context(), boardcopy.CopyRequest{
		SourceBoardID: "board-source",
		Options: boardcopy.CopyOptions{
			ProjectID: "project-destination",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), retry.SourceRevision)

	change, err = sourceStore.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
UPDATE boards
SET description = 'changed', revision = 4
WHERE id = 'board-source';
UPDATE store_state
SET current_revision = 4
WHERE singleton = 1`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	_, err = service.Copy(t.Context(), boardcopy.CopyRequest{
		SourceBoardID: "board-source",
		Options: boardcopy.CopyOptions{
			ProjectID: "project-destination",
		},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "incremental synchronization is not supported")
}

func TestCopyRepositoryRollsBackChangedSecondRecordPass(t *testing.T) {
	root := t.TempDir()
	sourceDirectory := filepath.Join(root, "source")
	destinationDirectory := filepath.Join(root, "destination")
	require.NoError(t, os.MkdirAll(sourceDirectory, 0o700))
	require.NoError(t, os.MkdirAll(destinationDirectory, 0o700))
	sourceStore := openCopyTestStore(t, sourceDirectory)
	destinationStore := openCopyTestStore(t, destinationDirectory)
	seedCopySource(t, sourceStore, copyTestBlobDescriptor(t))
	seedCopyDestinationCollisions(t, destinationStore)

	source, err := New(sourceStore, Config{BoardID: "board-source"})
	require.NoError(t, err)
	destination, err := NewCopyRepository(
		destinationStore,
		CopyRepositoryConfig{Entropy: copyTestEntropy()},
	)
	require.NoError(t, err)
	index, err := boardcopy.IndexRecords(
		source.ReadCopyRecords(
			t.Context(),
			"board-source",
			configuration.Overrides{},
		),
	)
	require.NoError(t, err)

	change, err := sourceStore.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
UPDATE issues
SET title = 'Changed between passes'
WHERE board_id = 'board-source' AND id = 'src-1';
UPDATE store_state
SET current_revision = current_revision + 1
WHERE singleton = 1`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	_, err = destination.ImportCopyRecords(
		t.Context(),
		index,
		source.ReadCopyRecords(
			t.Context(),
			"board-source",
			configuration.Overrides{},
		),
		boardcopy.CopyOptions{ProjectID: "project-destination"},
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "source board changed while copying records")

	view, err := destinationStore.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var boardCount, receiptCount int64
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT count(*)
FROM boards
WHERE project_id = 'project-destination' AND name = 'Source'`).Scan(&boardCount))
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT count(*)
FROM board_copy_receipts`).Scan(&receiptCount))
	assert.Zero(t, boardCount)
	assert.Zero(t, receiptCount)
}

func TestRepository_ReadCopyRecordsRejectsOperationalState(t *testing.T) {
	t.Run("ActiveClaim", func(t *testing.T) {
		directory := t.TempDir()
		persistence := openCopyTestStore(t, directory)
		seedCopySource(t, persistence, copyTestBlobDescriptor(t))
		change, err := persistence.Change(t.Context())
		require.NoError(t, err)
		_, err = change.ExecContext(t.Context(), `
INSERT INTO active_claims (
    issue_id, board_id, actor, started_at, started_revision
) VALUES ('src-1', 'board-source', 'worker', 1002, 2)`)
		require.NoError(t, err)
		require.NoError(t, change.Commit())
		repository, err := New(persistence, Config{BoardID: "board-source"})
		require.NoError(t, err)

		_, err = boardcopy.IndexRecords(
			repository.ReadCopyRecords(
				t.Context(),
				"board-source",
				configuration.Overrides{},
			),
		)
		require.Error(t, err)
		assert.ErrorContains(t, err, "active claims")
	})

	t.Run("ActiveUpload", func(t *testing.T) {
		directory := t.TempDir()
		persistence := openCopyTestStore(t, directory)
		seedCopySource(t, persistence, copyTestBlobDescriptor(t))
		change, err := persistence.Change(t.Context())
		require.NoError(t, err)
		_, err = change.ExecContext(t.Context(), `
INSERT INTO attachment_uploads (
    id, board_id, filename, actor, state, accepted_offset, expires_at
) VALUES ('upload-active', 'board-source', 'pending.txt', 'worker', 'active', 0, 2000)`)
		require.NoError(t, err)
		require.NoError(t, change.Commit())
		repository, err := New(persistence, Config{BoardID: "board-source"})
		require.NoError(t, err)

		_, err = boardcopy.IndexRecords(
			repository.ReadCopyRecords(
				t.Context(),
				"board-source",
				configuration.Overrides{},
			),
		)
		require.Error(t, err)
		assert.ErrorContains(t, err, "active attachment uploads")
	})
}

const (
	copyTestLogID         = "log_0123456789abcdef0123456789abcdef"
	copyTestLaterLogID    = "log_11111111111111111111111111111111"
	copyTestRemappedLogID = "log_22222222222222222222222222222222"
	copyTestAttachmentID  = "att_aaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func copyTestEntropy() io.Reader {
	return bytes.NewReader(bytes.Join([][]byte{
		bytes.Repeat([]byte{0x1f}, 16),
		bytes.Repeat([]byte{0x11}, 16),
		bytes.Repeat([]byte{0x22}, 16),
	}, nil))
}

type copyTestConfiguration struct{}

func (copyTestConfiguration) ReadStoreConfiguration(
	context.Context,
) (configuration.Overrides, error) {
	return configuration.Overrides{}, nil
}

func openCopyTestStore(t *testing.T, directory string) *store.Store {
	t.Helper()
	persistence, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(directory, "board.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	return persistence
}

func openCopyAttachmentRepository(
	t *testing.T,
	persistence *store.Store,
	directory string,
) *repositoryattachment.Repository {
	t.Helper()
	repository, err := repositoryattachment.New(
		persistence,
		repositoryattachment.Config{StoreDirectory: directory},
	)
	require.NoError(t, err)
	return repository
}

func seedCopySource(
	t *testing.T,
	persistence *store.Store,
	descriptor attachment.BlobDescriptor,
) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, change.Done()) })
	_, err = change.ExecContext(t.Context(), `
INSERT INTO projects (
    id, name, created_at, issue_id_prefix, issue_id_strategy,
    issue_summary_max_bytes, attachment_max_bytes, board_pins_max_count
) VALUES (
    'project-source', 'Source project', 1000, 'src-', 'sequential', 4096, 8192, 3
);
INSERT INTO boards (
    id, project_id, name, description, created_at, revision
) VALUES (
    'board-source', 'project-source', 'Source', 'Board %src-1', 1000, 2
);
INSERT INTO issues (
    id, board_id, title, kind, lifecycle, priority, created_at, updated_at,
    closed_at, waiting_reason, waiting_since, summary, details, revision
) VALUES (
    'src-1', 'board-source', 'Issue', 'task', 'open', 2, 1000, 1001,
    NULL, 'root acceptance', 1001, ?, 'Details %src-1', 2
), (
    'src-2', 'board-source', 'Reserved source identity', 'task', 'open', 2,
    1000, 1001, NULL, NULL, NULL, NULL, NULL, 2
), (
    'src-3', 'board-source', 'Parent', 'workstream', 'open', 2, 1000, 1001,
    NULL, NULL, NULL, NULL, NULL, 2
), (
    'src-4', 'board-source', 'Checkpoint', 'checkpoint', 'closed', 2, 1000, 1001,
    1001, NULL, NULL, NULL, NULL, 2
);`,
		"See %src-1 and %"+copyTestLogID+" and %"+copyTestAttachmentID+
			"; keep \\%src-1 and `%src-1`.",
	)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO issue_labels (board_id, issue_id, label)
VALUES ('board-source', 'src-1', 'area:copy');
INSERT INTO issue_external_keys (board_id, external_key, issue_id)
VALUES ('board-source', 'external-1', 'src-1');
INSERT INTO dependencies (board_id, issue_id, prerequisite_id)
VALUES ('board-source', 'src-1', 'src-4');
INSERT INTO containment (board_id, child_id, parent_id)
VALUES ('board-source', 'src-1', 'src-3');
INSERT INTO board_pins (board_id, issue_id, position)
VALUES
    ('board-source', 'src-2', 1),
    ('board-source', 'src-1', 2);
INSERT INTO issue_log_entries (
    id, board_id, issue_id, kind, author, committer, body, created_at,
    next_action
) VALUES (
    'log_0123456789abcdef0123456789abcdef',
    'board-source', 'src-1', 'state_snapshot', 'worker', 'worker',
    'State %src-1', 1001, 'Continue %src-1'
), (
    ?, 'board-source', 'src-2', 'post', 'worker', 'worker',
    'Later source Log', 1002, NULL
);
INSERT INTO issue_states (
    issue_id, board_id, body, author, updated_at, snapshot_log_entry_id,
    next_action
) VALUES (
    'src-1', 'board-source', 'State %src-1', 'worker', 1001,
    'log_0123456789abcdef0123456789abcdef',
    'Continue %src-1'
);
INSERT INTO issue_results (issue_id, board_id, body)
VALUES ('src-1', 'board-source', 'Result %src-1');
INSERT INTO checkpoint_decisions (
    issue_id, board_id, outcome, reason, decided_at, revision
) VALUES (
    'src-4', 'board-source', 'approved', 'Ready after %src-1', 1001, 2
)`,
		copyTestLaterLogID,
	)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO attachment_blobs (digest, size_bytes)
VALUES (?, ?)`,
		descriptor.Digest.String(),
		int64(descriptor.SizeBytes),
	)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO attachments (
    board_id, id, origin_issue_id, blob_digest, blob_size_bytes, filename,
    media_type, lifecycle, created_actor, created_at, created_revision,
    removed_actor, removed_at, removed_revision
) VALUES (
    'board-source', ?, 'src-1', ?, ?, 'evidence.txt', 'text/plain',
    'removed', 'worker', 1000, 1, 'worker', 1001, 2
);`,
		copyTestAttachmentID,
		descriptor.Digest.String(),
		int64(descriptor.SizeBytes),
	)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
UPDATE store_state
SET current_revision = 2, next_issue_number = 5
WHERE singleton = 1`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

func seedCopyDestinationCollisions(
	t *testing.T,
	persistence *store.Store,
) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, change.Done()) })
	_, err = change.ExecContext(t.Context(), `
INSERT INTO projects (id, name, created_at)
VALUES ('project-destination', 'Destination project', 1000);
INSERT INTO boards (id, project_id, name, created_at, revision)
VALUES ('board-source', 'project-destination', 'Existing', 1000, 1);
INSERT INTO issues (
    id, board_id, title, kind, lifecycle, priority, created_at, updated_at,
    revision
) VALUES (
    'src-1', 'board-source', 'Existing issue', 'task', 'open', 2, 1000, 1000, 1
);
INSERT INTO issue_log_entries (
    id, board_id, issue_id, kind, author, committer, body, created_at
) VALUES (
    'log_0123456789abcdef0123456789abcdef',
    'board-source', 'src-1', 'post', 'worker', 'worker', 'Existing', 1000
);
UPDATE store_state
SET current_revision = 1, next_issue_number = 2
WHERE singleton = 1`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

func copyTestBlobDescriptor(t *testing.T) attachment.BlobDescriptor {
	t.Helper()
	digest, err := attachment.NewDigest(
		"sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
	)
	require.NoError(t, err)
	return attachment.BlobDescriptor{Digest: digest, SizeBytes: 4}
}
