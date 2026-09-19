package attachment

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestRepositoryCommitAndAbortReturnStableTerminalResults(t *testing.T) {
	t.Run("Commit", func(t *testing.T) {
		body := []byte("engineering report")
		expectedSize := uint64(len(body))
		expectedDigest := digestFor(t, body)
		fixture := openUploadFixture(t, bytes.Repeat([]byte{3}, 256))
		upload := fixture.begin(t, "captain", &expectedSize, &expectedDigest)
		_, err := fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			ExpectedOffset: 0, Content: body,
		})
		require.NoError(t, err)

		committed, err := fixture.service.CommitUpload(
			t.Context(),
			domainattachment.CommitUploadRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			},
		)
		require.NoError(t, err)
		assert.Equal(t, domainattachment.BlobDescriptor{
			Digest: expectedDigest, SizeBytes: expectedSize,
		}, committed.Blob)
		assert.Equal(t, domainattachment.BlobAvailabilityVerified, committed.Availability)
		assert.Equal(t, "text/plain; charset=utf-8", committed.MediaType.String())
		assert.Equal(t, int64(1), uploadRevision(t, fixture.persistence))

		replayed, err := fixture.service.CommitUpload(
			t.Context(),
			domainattachment.CommitUploadRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			},
		)
		require.NoError(t, err)
		assert.Equal(
			t,
			domainattachment.BlobAvailabilityPresentUnverified,
			replayed.Availability,
		)
		replayed.Availability = committed.Availability
		assert.Equal(t, committed, replayed)
		assert.Equal(t, int64(1), uploadRevision(t, fixture.persistence))

		status, err := fixture.service.GetUpload(
			t.Context(),
			domainattachment.GetUploadRequest{UploadID: upload.ID},
		)
		require.NoError(t, err)
		assert.Equal(t, domainattachment.UploadStateCommitted, status.State)
		require.NotNil(t, status.Attachment)
		assert.Equal(
			t,
			domainattachment.BlobAvailabilityPresentUnverified,
			status.Attachment.Availability,
		)
		status.Attachment.Availability = committed.Availability
		assert.Equal(t, committed, *status.Attachment)

		_, err = fixture.service.AbortUpload(
			t.Context(),
			domainattachment.AbortUploadRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			},
		)
		assert.ErrorIs(t, err, domainattachment.ErrUploadStateConflict)
	})

	t.Run("Abort", func(t *testing.T) {
		fixture := openUploadFixture(t, bytes.Repeat([]byte{4}, 128))
		upload := fixture.begin(t, "captain", nil, nil)
		aborted, err := fixture.service.AbortUpload(
			t.Context(),
			domainattachment.AbortUploadRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			},
		)
		require.NoError(t, err)
		assert.Equal(t, domainattachment.UploadStateAborted, aborted.State)

		replayed, err := fixture.service.AbortUpload(
			t.Context(),
			domainattachment.AbortUploadRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			},
		)
		require.NoError(t, err)
		assert.Equal(t, aborted, replayed)
		assert.Zero(t, uploadRevision(t, fixture.persistence))
	})
}

func TestRepositoryIssueAttachmentUsesPrivateIdentityAndPublishesRevision(t *testing.T) {
	body := []byte("issue evidence")
	fixture := openUploadFixture(t, bytes.Repeat([]byte{8}, 128))
	issueUID := bytes.Repeat([]byte{9}, 16)
	change, err := fixture.persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO issues (
			uid, id, board_id, title, kind, lifecycle, priority,
			created_at, updated_at
		) VALUES (?, 'issue-1', 'board-test', 'Evidence owner', 'task', 'open', 2, 1, 1)
	`, issueUID)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	issueID, err := issue.NewID("issue-1")
	require.NoError(t, err)
	association, err := domainattachment.NewIssueAssociation(
		fixture.association.BoardID(),
		issueID,
	)
	require.NoError(t, err)

	upload, err := fixture.service.BeginUpload(
		t.Context(),
		domainattachment.BeginUploadRequest{
			Invocation:  domainattachment.NewInvocation("captain"),
			Association: association,
			Filename:    fixture.filename,
		},
	)
	require.NoError(t, err)
	_, err = fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 0, Content: body,
	})
	require.NoError(t, err)
	committed, err := fixture.service.CommitUpload(
		t.Context(),
		domainattachment.CommitUploadRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, association, committed.Association)
	got, err := fixture.service.GetAttachment(t.Context(), domainattachment.GetRequest{
		BoardID: association.BoardID(), AttachmentID: committed.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, association, got.Association)
	page, err := fixture.service.ListAttachments(t.Context(), domainattachment.ListRequest{
		BoardID: association.BoardID(), OriginIssueID: &issueID,
	})
	require.NoError(t, err)
	require.Len(t, page.Attachments, 1)
	assert.Equal(t, association, page.Attachments[0].Association)

	view, err := fixture.persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var uploadOrigin, attachmentOrigin []byte
	var issueRevision, boardRevision int64
	require.NoError(t, view.QueryRowContext(t.Context(), `
		SELECT origin_issue_uid FROM attachment_uploads WHERE id = ?
	`, upload.ID.String()).Scan(&uploadOrigin))
	require.NoError(t, view.QueryRowContext(t.Context(), `
		SELECT origin_issue_uid FROM attachments WHERE board_id = 'board-test' AND id = ?
	`, committed.ID.String()).Scan(&attachmentOrigin))
	require.NoError(t, view.QueryRowContext(t.Context(), `
		SELECT issue.revision, board.revision
		FROM issues AS issue
		JOIN boards AS board ON board.id = issue.board_id
		WHERE issue.id = 'issue-1'
	`).Scan(&issueRevision, &boardRevision))
	assert.Equal(t, issueUID, uploadOrigin)
	assert.Equal(t, issueUID, attachmentOrigin)
	assert.Equal(t, int64(1), issueRevision)
	assert.Equal(t, issueRevision, boardRevision)
}

func TestRepositoryUploadExpiryAndDescriptorFailureRemainRecoverable(t *testing.T) {
	t.Run("Expiry", func(t *testing.T) {
		fixture := openUploadFixture(t, bytes.Repeat([]byte{5}, 128))
		upload := fixture.begin(t, "captain", nil, nil)
		fixture.clock.now = upload.ExpiresAt

		expired, err := fixture.service.GetUpload(
			t.Context(),
			domainattachment.GetUploadRequest{UploadID: upload.ID},
		)
		require.NoError(t, err)
		assert.Equal(t, domainattachment.UploadStateExpired, expired.State)
		staging, err := filepath.Glob(filepath.Join(
			fixture.directory,
			"blobs",
			"staging",
			"*",
		))
		require.NoError(t, err)
		assert.Empty(t, staging)

		_, err = fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			ExpectedOffset: 0, Content: []byte("late"),
		})
		assert.ErrorIs(t, err, domainattachment.ErrUploadStateConflict)
	})

	t.Run("Descriptor", func(t *testing.T) {
		body := []byte("payload")
		wrongSize := uint64(len(body) + 1)
		fixture := openUploadFixture(t, bytes.Repeat([]byte{6}, 128))
		upload := fixture.begin(t, "captain", &wrongSize, nil)
		_, err := fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			ExpectedOffset: 0, Content: body,
		})
		require.NoError(t, err)

		_, err = fixture.service.CommitUpload(
			t.Context(),
			domainattachment.CommitUploadRequest{
				Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
			},
		)
		assert.ErrorIs(t, err, domainattachment.ErrUploadDescriptorMismatch)

		status, err := fixture.service.GetUpload(
			t.Context(),
			domainattachment.GetUploadRequest{UploadID: upload.ID},
		)
		require.NoError(t, err)
		assert.Equal(t, domainattachment.UploadStateActive, status.State)
		assert.Equal(t, uint64(len(body)), status.AcceptedOffset)
	})
}

func TestRepositoryCommitRecoversAfterRevisionFailure(t *testing.T) {
	body := []byte("recoverable")
	fixture := openUploadFixture(t, bytes.Repeat([]byte{7}, 48))
	upload := fixture.begin(t, "captain", nil, nil)
	_, err := fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 0, Content: body,
	})
	require.NoError(t, err)
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

	_, err = fixture.service.CommitUpload(
		t.Context(),
		domainattachment.CommitUploadRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		},
	)
	assert.Error(t, err)
	assert.Zero(t, uploadRevision(t, fixture.persistence))
	assert.Zero(t, attachmentCount(t, fixture.persistence))
	staging, err := filepath.Glob(filepath.Join(
		fixture.directory,
		"blobs",
		"staging",
		"*",
	))
	require.NoError(t, err)
	assert.Len(t, staging, 1)

	change, err = fixture.persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `DROP TRIGGER reject_attachment_board_revision`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	committed, err := fixture.service.CommitUpload(
		t.Context(),
		domainattachment.CommitUploadRequest{
			Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, digestFor(t, body), committed.Blob.Digest)
	assert.Equal(t, int64(1), uploadRevision(t, fixture.persistence))
	staging, err = filepath.Glob(filepath.Join(
		fixture.directory,
		"blobs",
		"staging",
		"*",
	))
	require.NoError(t, err)
	assert.Empty(t, staging)
}
