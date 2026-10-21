package attachment

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryRetainsAttachmentMutationHistory(t *testing.T) {
	body := []byte("retained evidence")
	fixture := openUploadFixture(t, maintenanceEntropy())
	view, err := fixture.persistence.View(t.Context())
	require.NoError(t, err)
	var foreignKeysEnabled bool
	require.NoError(t, view.QueryRowContext(t.Context(), `PRAGMA foreign_keys`).Scan(&foreignKeysEnabled))
	require.NoError(t, view.Done())
	assert.True(t, foreignKeysEnabled)
	upload := fixture.begin(t, "captain", nil, nil)
	_, err = fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 0, Content: body,
	})
	require.NoError(t, err)
	added, err := fixture.service.CommitUpload(
		t.Context(),
		domainattachment.CommitUploadRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		},
	)
	require.NoError(t, err)

	mutations := readAttachmentHistory(t, fixture.persistence)
	require.Len(t, mutations, 2)
	require.Len(t, mutations[1], 1)
	assert.Equal(t, added.Created.At, mutations[1][0].OccurredAt)
	addition, ok := mutations[1][0].Payload.(*boardhistory.AttachmentAdd)
	require.True(t, ok)
	assert.Equal(t, added.ID.String(), addition.ID)
	assert.Nil(t, addition.OriginIssueUID)
	assert.Equal(t, added.Blob.Digest.String(), addition.Digest)
	assert.Equal(t, int64(added.Blob.SizeBytes), addition.SizeBytes)
	assert.Equal(t, added.Filename.String(), addition.Filename)
	assert.Equal(t, added.MediaType.String(), addition.MediaType)
	assert.Equal(t, added.Created.Actor, addition.Actor)
	assert.Equal(t, added.Created.At, addition.At)
	assert.Equal(t, 1, countRows(t, fixture.persistence, "board_mutation_blob_refs"))

	_, err = fixture.service.CommitUpload(
		t.Context(),
		domainattachment.CommitUploadRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		},
	)
	require.NoError(t, err)
	assert.Len(t, readAttachmentHistory(t, fixture.persistence), 2)

	fixture.clock.now = fixture.clock.now.Add(domainattachment.TerminalReceiptRetention)
	removed, err := fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation: domainattachment.NewInvocation("commander"),
			BoardID:    fixture.association.BoardID(), AttachmentID: added.ID,
		},
	)
	require.NoError(t, err)
	mutations = readAttachmentHistory(t, fixture.persistence)
	require.Len(t, mutations, 3)
	require.Len(t, mutations[2], 1)
	require.NotNil(t, removed.Removed)
	assert.Equal(t, removed.Removed.At, mutations[2][0].OccurredAt)
	removal, ok := mutations[2][0].Payload.(*boardhistory.AttachmentRemove)
	require.True(t, ok)
	assert.Equal(t, removed.ID.String(), removal.ID)
	assert.Equal(t, removed.Removed.Actor, removal.Actor)
	assert.Equal(t, removed.Removed.At, removal.At)
	assert.Equal(t, 1, countRows(t, fixture.persistence, "board_mutation_blob_refs"))

	_, err = fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation: domainattachment.NewInvocation("another actor"),
			BoardID:    fixture.association.BoardID(), AttachmentID: added.ID,
		},
	)
	require.NoError(t, err)
	assert.Len(t, readAttachmentHistory(t, fixture.persistence), 3)
}

func TestRepositoryAttachmentHistoryFailureRollsBackProjection(t *testing.T) {
	t.Run("Addition", func(t *testing.T) {
		fixture := openUploadFixture(t, maintenanceEntropy())
		upload := fixture.begin(t, "captain", nil, nil)
		_, err := fixture.service.WriteChunk(
			t.Context(),
			domainattachment.WriteChunkRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
				ExpectedOffset: 0, Content: []byte("addition"),
			},
		)
		require.NoError(t, err)
		installAttachmentHistoryFailure(t, fixture.persistence)

		_, err = fixture.service.CommitUpload(
			t.Context(),
			domainattachment.CommitUploadRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			},
		)
		assert.ErrorContains(t, err, "history failure")
		assert.Zero(t, attachmentCount(t, fixture.persistence))
		assert.Equal(t, "active", uploadState(t, fixture, upload.ID))
		assert.Zero(t, uploadRevision(t, fixture.persistence))
		assert.Len(t, readAttachmentHistory(t, fixture.persistence), 1)
	})

	t.Run("BlobReference", func(t *testing.T) {
		fixture := openUploadFixture(t, maintenanceEntropy())
		upload := fixture.begin(t, "captain", nil, nil)
		_, err := fixture.service.WriteChunk(
			t.Context(),
			domainattachment.WriteChunkRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
				ExpectedOffset: 0, Content: []byte("blob reference"),
			},
		)
		require.NoError(t, err)
		installAttachmentBlobReferenceFailure(t, fixture.persistence)

		_, err = fixture.service.CommitUpload(
			t.Context(),
			domainattachment.CommitUploadRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			},
		)
		assert.ErrorContains(t, err, "blob reference failure")
		assert.Zero(t, attachmentCount(t, fixture.persistence))
		assert.Equal(t, "active", uploadState(t, fixture, upload.ID))
		assert.Zero(t, uploadRevision(t, fixture.persistence))
		assert.Len(t, readAttachmentHistory(t, fixture.persistence), 1)
		assert.Zero(t, countRows(t, fixture.persistence, "board_mutation_blob_refs"))
	})

	t.Run("Removal", func(t *testing.T) {
		fixture := openUploadFixture(t, maintenanceEntropy())
		added := fixture.commit(t, fixture.association, "removal")
		installAttachmentHistoryFailure(t, fixture.persistence)

		_, err := fixture.service.RemoveAttachment(
			t.Context(),
			domainattachment.RemoveRequest{
				Invocation: domainattachment.NewInvocation("captain"),
				BoardID:    fixture.association.BoardID(), AttachmentID: added.ID,
			},
		)
		assert.ErrorContains(t, err, "history failure")
		retained, readErr := fixture.service.GetAttachment(
			t.Context(),
			domainattachment.GetRequest{
				BoardID: fixture.association.BoardID(), AttachmentID: added.ID,
			},
		)
		require.NoError(t, readErr)
		assert.Equal(t, domainattachment.LifecycleActive, retained.Lifecycle)
		assert.Equal(t, int64(1), uploadRevision(t, fixture.persistence))
		assert.Len(t, readAttachmentHistory(t, fixture.persistence), 2)
	})
}

func TestRepositoryCollectAttachmentsRetainsHistoryOnlyBlob(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	added := fixture.commit(t, fixture.association, "history only")
	orphan := publishToStore(t, fixture.repository.blobs, "orphan", []byte("discard"))

	change, err := fixture.persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(
		t.Context(),
		`DELETE FROM attachment_uploads WHERE attachment_id = ?`,
		added.ID.String(),
	)
	require.NoError(t, err)
	_, err = change.ExecContext(
		t.Context(),
		`DELETE FROM attachments WHERE board_id = ? AND id = ?`,
		fixture.association.BoardID().String(),
		added.ID.String(),
	)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	assert.Zero(t, attachmentCount(t, fixture.persistence))

	collected, err := fixture.service.CollectAttachments(
		t.Context(),
		domainattachment.CollectRequest{},
	)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 1, Bytes: 7}, collected.OrphanBlobs)
	assertFileAvailability(
		t, fixture, added.Blob,
		domainattachment.BlobAvailabilityPresentUnverified,
	)
	assertFileAvailability(t, fixture, orphan, domainattachment.BlobAvailabilityMissing)
}

func readAttachmentHistory(
	t *testing.T,
	persistence *store.Store,
) [][]boardhistory.Event {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	rows, err := view.QueryContext(t.Context(), `
SELECT marker_bytes
FROM board_mutation_commits
WHERE board_id = 'board-test'
ORDER BY writer_uid, sequence
`)
	require.NoError(t, err)
	var markers [][]byte
	for rows.Next() {
		var marker []byte
		require.NoError(t, rows.Scan(&marker))
		markers = append(markers, bytes.Clone(marker))
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	mutations := make([][]boardhistory.Event, 0, len(markers))
	for _, encoded := range markers {
		marker, err := boardhistory.DecodeMarker(encoded)
		require.NoError(t, err)
		var events []boardhistory.Event
		for retained, err := range boardhistory.MutationEvents(
			marker,
			func(index int64) ([]byte, error) {
				header := marker.Header()
				var part []byte
				err := view.QueryRowContext(t.Context(), `
SELECT bytes
FROM board_mutation_parts
WHERE board_id = 'board-test'
    AND writer_uid = ? AND sequence = ? AND part_index = ?
`, header.Writer.Bytes(), header.Sequence, index).Scan(&part)
				return part, err
			},
		) {
			require.NoError(t, err)
			events = append(events, retained.Event)
		}
		mutations = append(mutations, events)
	}
	return mutations
}

func installAttachmentHistoryFailure(t *testing.T, persistence *store.Store) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
CREATE TRIGGER fail_attachment_history_append
BEFORE INSERT ON board_mutation_parts
WHEN NEW.sequence > 1
BEGIN
    SELECT RAISE(ABORT, 'history failure');
END
`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

func installAttachmentBlobReferenceFailure(t *testing.T, persistence *store.Store) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
CREATE TRIGGER fail_attachment_blob_reference
BEFORE INSERT ON board_mutation_blob_refs
BEGIN
    SELECT RAISE(ABORT, 'blob reference failure');
END
`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}
