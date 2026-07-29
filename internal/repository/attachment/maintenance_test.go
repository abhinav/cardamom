package attachment

import (
	"errors"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestRepositoryAttachmentReadsAreBoardScopedAndPaginated(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	attachments := []domainattachment.Attachment{
		fixture.commit(t, fixture.association, "one"),
		fixture.commit(t, fixture.association, "two"),
		fixture.commit(t, fixture.association, "three"),
	}
	otherBoardID := addAttachmentBoard(t, fixture, "board-other")
	otherAssociation, err := domainattachment.NewBoardAssociation(otherBoardID)
	require.NoError(t, err)
	other := fixture.commit(t, otherAssociation, "other")

	got, err := fixture.service.GetAttachment(t.Context(), domainattachment.GetRequest{
		BoardID: fixture.association.BoardID(), AttachmentID: attachments[0].ID,
	})
	require.NoError(t, err)
	assert.Equal(t, domainattachment.BlobAvailabilityPresentUnverified, got.Availability)

	_, err = fixture.service.GetAttachment(t.Context(), domainattachment.GetRequest{
		BoardID: otherBoardID, AttachmentID: attachments[0].ID,
	})
	assert.ErrorIs(t, err, domainattachment.ErrAttachmentNotFound)
	assert.Equal(t, errkind.NotFound, errkind.Of(err))

	var listed []domainattachment.Attachment
	pageToken := ""
	for {
		page, err := fixture.service.ListAttachments(t.Context(), domainattachment.ListRequest{
			BoardID: fixture.association.BoardID(), PageSize: 1, PageToken: pageToken,
		})
		require.NoError(t, err)
		listed = append(listed, page.Attachments...)
		pageToken = page.NextPageToken
		if pageToken == "" {
			break
		}
	}
	sort.Slice(attachments, func(i, j int) bool {
		return attachments[i].ID.String() < attachments[j].ID.String()
	})
	for i := range attachments {
		attachments[i].Availability = domainattachment.BlobAvailabilityPresentUnverified
	}
	assert.Equal(t, attachments, listed)
	assert.NotContains(t, attachmentIDs(listed), other.ID)

	_, err = fixture.service.ListAttachments(t.Context(), domainattachment.ListRequest{
		BoardID: fixture.association.BoardID(), PageToken: "not-a-page-token",
	})
	assert.ErrorIs(t, err, domainattachment.ErrAttachmentPageTokenInvalid)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestRepositoryAttachmentAvailabilityAndVerification(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	missing := fixture.commit(t, fixture.association, "missing")
	sizeMismatch := fixture.commit(t, fixture.association, "expected-size")
	digestMismatch := fixture.commit(t, fixture.association, "expected")
	verified := fixture.commit(t, fixture.association, "verified")

	require.NoError(t, os.Remove(fixture.repository.blobs.contentPath(missing.Blob.Digest)))
	require.NoError(t, os.Truncate(
		fixture.repository.blobs.contentPath(sizeMismatch.Blob.Digest),
		1,
	))
	require.NoError(t, os.WriteFile(
		fixture.repository.blobs.contentPath(digestMismatch.Blob.Digest),
		[]byte("damaged!"),
		0o600,
	))

	tests := []struct {
		name             string
		attachment       domainattachment.Attachment
		wantRead         domainattachment.BlobAvailability
		wantVerification domainattachment.BlobAvailability
	}{
		{
			name: "Missing", attachment: missing,
			wantRead:         domainattachment.BlobAvailabilityMissing,
			wantVerification: domainattachment.BlobAvailabilityMissing,
		},
		{
			name: "SizeMismatch", attachment: sizeMismatch,
			wantRead:         domainattachment.BlobAvailabilitySizeMismatch,
			wantVerification: domainattachment.BlobAvailabilitySizeMismatch,
		},
		{
			name: "DigestMismatch", attachment: digestMismatch,
			wantRead:         domainattachment.BlobAvailabilityPresentUnverified,
			wantVerification: domainattachment.BlobAvailabilityDigestMismatch,
		},
		{
			name: "Verified", attachment: verified,
			wantRead:         domainattachment.BlobAvailabilityPresentUnverified,
			wantVerification: domainattachment.BlobAvailabilityVerified,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fixture.service.GetAttachment(t.Context(), domainattachment.GetRequest{
				BoardID: fixture.association.BoardID(), AttachmentID: tt.attachment.ID,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantRead, got.Availability)

			observation, err := fixture.service.VerifyAttachment(
				t.Context(),
				domainattachment.VerifyRequest{
					BoardID:      fixture.association.BoardID(),
					AttachmentID: tt.attachment.ID,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, tt.attachment.ID, observation.AttachmentID)
			assert.Equal(t, tt.attachment.Blob, observation.Blob)
			assert.Equal(t, tt.wantVerification, observation.Availability)
			assert.Equal(t, fixture.clock.now, observation.ObservedAt)
		})
	}
}

func TestRepositoryRemoveAttachmentCreatesOneTombstone(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	created := fixture.commit(t, fixture.association, "remove me")
	fixture.clock.now = fixture.clock.now.Add(time.Minute)

	removed, err := fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation: domainattachment.NewInvocation("captain"),
			BoardID:    fixture.association.BoardID(), AttachmentID: created.ID,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.LifecycleRemoved, removed.Lifecycle)
	require.NotNil(t, removed.Removed)
	assert.Equal(t, "captain", removed.Removed.Actor)
	assert.Equal(t, fixture.clock.now, removed.Removed.At)
	assert.Equal(t, int64(2), uploadRevision(t, fixture.persistence))

	replayed, err := fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation: domainattachment.NewInvocation("commander"),
			BoardID:    fixture.association.BoardID(), AttachmentID: created.ID,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, removed, replayed)
	assert.Equal(t, int64(2), uploadRevision(t, fixture.persistence))

	page, err := fixture.service.ListAttachments(t.Context(), domainattachment.ListRequest{
		BoardID: fixture.association.BoardID(),
	})
	require.NoError(t, err)
	assert.Empty(t, page.Attachments)
	page, err = fixture.service.ListAttachments(t.Context(), domainattachment.ListRequest{
		BoardID: fixture.association.BoardID(), IncludeRemoved: true,
	})
	require.NoError(t, err)
	assert.Equal(t, []domainattachment.Attachment{removed}, page.Attachments)
}

func TestRepositoryRemoveAttachmentRollsBackRevisionFailure(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	created := fixture.commit(t, fixture.association, "atomic")
	fixture.clock.now = fixture.clock.now.Add(time.Minute)
	change, err := fixture.persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		CREATE TRIGGER reject_attachment_board_revision
		BEFORE UPDATE OF revision ON boards
		BEGIN
			SELECT RAISE(ABORT, 'revision publication failed');
		END
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	_, err = fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation: domainattachment.NewInvocation("captain"),
			BoardID:    fixture.association.BoardID(), AttachmentID: created.ID,
		},
	)
	assert.Error(t, err)
	got, getErr := fixture.service.GetAttachment(t.Context(), domainattachment.GetRequest{
		BoardID: fixture.association.BoardID(), AttachmentID: created.ID,
	})
	require.NoError(t, getErr)
	assert.Equal(t, domainattachment.LifecycleActive, got.Lifecycle)
	assert.Equal(t, int64(1), uploadRevision(t, fixture.persistence))

	change, err = fixture.persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `DROP TRIGGER reject_attachment_board_revision`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	_, err = fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation: domainattachment.NewInvocation("captain"),
			BoardID:    fixture.association.BoardID(), AttachmentID: created.ID,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(2), uploadRevision(t, fixture.persistence))
}

func TestRepositoryCollectAttachmentsIsConservative(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	active := fixture.commit(t, fixture.association, "keep active")
	removed := fixture.commit(t, fixture.association, "keep removed")
	_, err := fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation: domainattachment.NewInvocation("captain"),
			BoardID:    fixture.association.BoardID(), AttachmentID: removed.ID,
		},
	)
	require.NoError(t, err)
	corrupt := fixture.commit(t, fixture.association, "expected")
	require.NoError(t, os.WriteFile(
		fixture.repository.blobs.contentPath(corrupt.Blob.Digest),
		[]byte("damaged!"),
		0o600,
	))
	orphan := publishToStore(t, fixture.repository.blobs, "orphan", []byte("discard"))

	stale := fixture.begin(t, "captain", nil, nil)
	_, err = fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: stale.ID,
		ExpectedOffset: 0, Content: []byte("stale"),
	})
	require.NoError(t, err)
	fixture.clock.now = stale.ExpiresAt

	dryRun, err := fixture.service.CollectAttachments(
		t.Context(),
		domainattachment.CollectRequest{DryRun: true},
	)
	require.NoError(t, err)
	assert.True(t, dryRun.DryRun)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 1, Bytes: 5}, dryRun.ExpiredStaging)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 1, Bytes: 7}, dryRun.OrphanBlobs)
	require.Len(t, dryRun.IntegrityProblems, 1)
	assert.Equal(t, corrupt.ID, dryRun.IntegrityProblems[0].AttachmentID)
	assert.Equal(t, domainattachment.BlobAvailabilityDigestMismatch, dryRun.IntegrityProblems[0].Availability)
	assert.Equal(t, "active", uploadState(t, fixture, stale.ID))
	assertFileAvailability(t, fixture, orphan, domainattachment.BlobAvailabilityPresentUnverified)

	collected, err := fixture.service.CollectAttachments(
		t.Context(),
		domainattachment.CollectRequest{},
	)
	require.NoError(t, err)
	assert.False(t, collected.DryRun)
	dryRun.DryRun = false
	assert.Equal(t, dryRun, collected)
	assert.Equal(t, "expired", uploadState(t, fixture, stale.ID))
	assertFileAvailability(t, fixture, orphan, domainattachment.BlobAvailabilityMissing)
	assertFileAvailability(t, fixture, active.Blob, domainattachment.BlobAvailabilityPresentUnverified)
	assertFileAvailability(t, fixture, removed.Blob, domainattachment.BlobAvailabilityPresentUnverified)

	repeated, err := fixture.service.CollectAttachments(
		t.Context(),
		domainattachment.CollectRequest{},
	)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 0, Bytes: 0}, repeated.ExpiredStaging)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 0, Bytes: 0}, repeated.OrphanBlobs)
	require.Len(t, repeated.IntegrityProblems, 1)
	assert.Equal(t, corrupt.ID, repeated.IntegrityProblems[0].AttachmentID)
}

func TestRepositoryCollectAttachmentsRetainsTerminalReceiptsUntilExpiry(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	upload := fixture.begin(t, "captain", nil, nil)
	aborted, err := fixture.service.AbortUpload(
		t.Context(),
		domainattachment.AbortUploadRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		},
	)
	require.NoError(t, err)
	_, err = fixture.repository.blobs.writeChunk(upload.ID, 0, []byte("stale"))
	require.NoError(t, err)

	collected, err := fixture.service.CollectAttachments(
		t.Context(),
		domainattachment.CollectRequest{},
	)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 1, Bytes: 5}, collected.ExpiredStaging)
	assert.Equal(t, "aborted", uploadState(t, fixture, upload.ID))
	assert.False(t, stagingExists(t, fixture, upload.ID))

	fixture.clock.now = aborted.ExpiresAt
	_, err = fixture.service.CollectAttachments(
		t.Context(),
		domainattachment.CollectRequest{},
	)
	require.NoError(t, err)
	assert.False(t, uploadExists(t, fixture, upload.ID))
}

func (f *uploadFixture) commit(
	t *testing.T,
	association domainattachment.Association,
	body string,
) domainattachment.Attachment {
	t.Helper()
	upload, err := f.service.BeginUpload(t.Context(), domainattachment.BeginUploadRequest{
		Invocation:  domainattachment.NewInvocation("captain"),
		Association: association,
		Filename:    f.filename,
	})
	require.NoError(t, err)
	_, err = f.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 0, Content: []byte(body),
	})
	require.NoError(t, err)
	value, err := f.service.CommitUpload(t.Context(), domainattachment.CommitUploadRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
	})
	require.NoError(t, err)
	return value
}

func maintenanceEntropy() []byte {
	value := make([]byte, 16*128)
	for chunk := range 128 {
		for offset := range 16 {
			value[chunk*16+offset] = byte(chunk + 1)
		}
	}
	return value
}

func addAttachmentBoard(t *testing.T, fixture *uploadFixture, value string) board.ID {
	t.Helper()
	boardID, err := board.NewID(value)
	require.NoError(t, err)
	change, err := fixture.persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES (?, 'project-test', ?, 1700000000)
	`, boardID, value)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	return boardID
}

func attachmentIDs(values []domainattachment.Attachment) []domainattachment.ID {
	ids := make([]domainattachment.ID, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.ID)
	}
	return ids
}

func uploadState(
	t *testing.T,
	fixture *uploadFixture,
	uploadID domainattachment.UploadID,
) string {
	t.Helper()
	view, err := fixture.persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var state string
	err = view.QueryRowContext(t.Context(), `
		SELECT state FROM attachment_uploads WHERE id = ?
	`, uploadID).Scan(&state)
	require.NoError(t, err)
	return state
}

func uploadExists(
	t *testing.T,
	fixture *uploadFixture,
	uploadID domainattachment.UploadID,
) bool {
	t.Helper()
	view, err := fixture.persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var exists bool
	err = view.QueryRowContext(t.Context(), `
		SELECT EXISTS (SELECT 1 FROM attachment_uploads WHERE id = ?)
	`, uploadID).Scan(&exists)
	require.NoError(t, err)
	return exists
}

func stagingExists(
	t *testing.T,
	fixture *uploadFixture,
	uploadID domainattachment.UploadID,
) bool {
	t.Helper()
	_, err := os.Stat(fixture.repository.blobs.stagingPath(uploadID))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	require.NoError(t, err)
	return true
}

func assertFileAvailability(
	t *testing.T,
	fixture *uploadFixture,
	descriptor domainattachment.BlobDescriptor,
	want domainattachment.BlobAvailability,
) {
	t.Helper()
	got, err := fixture.repository.blobs.inspect(descriptor)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
