package attachmentconnect

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.uber.org/mock/gomock"
)

func TestServiceMapsUploadLifecycleAndReplay(t *testing.T) {
	association, err := attachment.NewIssueAssociation(
		board.ID("board-one"),
		issue.ID("an-origin"),
	)
	require.NoError(t, err)
	filename, err := attachment.NewFilename("report.txt")
	require.NoError(t, err)
	digest, err := attachment.NewDigest("sha256:0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	expectedSize := uint64(12)
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	mediaType, err := attachment.NewMediaType("text/plain")
	require.NoError(t, err)
	committedAttachment := attachment.Attachment{
		ID:           attachment.ID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Association:  association,
		Blob:         attachment.BlobDescriptor{Digest: digest, SizeBytes: 12},
		Filename:     filename,
		MediaType:    mediaType,
		Lifecycle:    attachment.LifecycleActive,
		Availability: attachment.BlobAvailabilityVerified,
		Created: attachment.Attribution{
			Actor: "uploader", At: now, Revision: board.Revision(7),
		},
	}
	upload := attachment.Upload{
		ID:                attachment.UploadID("upload-one"),
		Association:       association,
		Filename:          filename,
		ExpectedSizeBytes: &expectedSize,
		ExpectedDigest:    &digest,
		Actor:             "uploader",
		State:             attachment.UploadStateActive,
		AcceptedOffset:    6,
		MaximumSizeBytes:  configuration.ByteLimit(configuration.DefaultAttachmentMaxBytes),
		ExpiresAt:         now.Add(time.Hour),
	}
	repository := NewMockRepository(gomock.NewController(t))
	repository.EXPECT().BeginUpload(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		admission attachment.BeginUploadAdmission,
	) (attachment.Upload, error) {
		assert.Equal(t, "uploader", admission.Request.Invocation.Actor())
		assert.Equal(t, board.ID("board-one"), admission.Request.Association.BoardID())
		assert.Equal(t, filename, admission.Request.Filename)
		assert.Equal(t, upload.MaximumSizeBytes, admission.MaximumSizeBytes)
		return upload, nil
	})
	repository.EXPECT().WriteChunk(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		request attachment.WriteChunkRequest,
	) (attachment.Upload, error) {
		assert.Equal(t, "uploader", request.Invocation.Actor())
		assert.Equal(t, attachment.UploadID("upload-one"), request.UploadID)
		assert.Equal(t, []byte("repeat"), request.Content)
		return upload, nil
	}).Times(2)
	repository.EXPECT().GetUpload(
		gomock.Any(), attachment.GetUploadRequest{UploadID: "upload-one"},
	).Return(upload, nil)
	repository.EXPECT().CommitUpload(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		request attachment.CommitUploadRequest,
	) (attachment.Attachment, error) {
		assert.Equal(t, "uploader", request.Invocation.Actor())
		assert.Equal(t, attachment.UploadID("upload-one"), request.UploadID)
		return committedAttachment, nil
	}).Times(2)
	abortedUpload := upload
	abortedUpload.State = attachment.UploadStateAborted
	repository.EXPECT().AbortUpload(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		request attachment.AbortUploadRequest,
	) (attachment.Upload, error) {
		assert.Equal(t, "uploader", request.Invocation.Actor())
		assert.Equal(t, attachment.UploadID("upload-one"), request.UploadID)
		return abortedUpload, nil
	})
	client := newDomainTestClient(t, repository)
	mutation := &privatev1.MutationContext{Actor: new(" uploader ")}

	begun, err := client.BeginAttachmentUpload(t.Context(), connect.NewRequest(
		&privatev1.BeginAttachmentUploadRequest{
			BoardId: "board-one", IssueId: new("an-origin"),
			Filename: "report.txt", ExpectedSizeBytes: &expectedSize,
			ExpectedDigest: new(digest.String()), Mutation: mutation,
		},
	))
	require.NoError(t, err)
	for range 2 {
		written, writeErr := client.WriteAttachmentChunk(t.Context(), connect.NewRequest(
			&privatev1.WriteAttachmentChunkRequest{
				UploadId: "upload-one", ExpectedOffset: 0,
				Content: []byte("repeat"), Mutation: mutation,
			},
		))
		require.NoError(t, writeErr)
		assert.Equal(t, uint64(6), written.Msg.GetUpload().GetAcceptedOffset())
	}
	status, err := client.GetAttachmentUpload(t.Context(), connect.NewRequest(
		&privatev1.GetAttachmentUploadRequest{UploadId: "upload-one"},
	))
	require.NoError(t, err)
	for range 2 {
		committed, commitErr := client.CommitAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.CommitAttachmentUploadRequest{UploadId: "upload-one", Mutation: mutation},
		))
		require.NoError(t, commitErr)
		assert.Equal(t, committedAttachment.ID.String(), committed.Msg.GetAttachment().GetId())
	}
	aborted, err := client.AbortAttachmentUpload(t.Context(), connect.NewRequest(
		&privatev1.AbortAttachmentUploadRequest{UploadId: "upload-one", Mutation: mutation},
	))

	require.NoError(t, err)
	assert.Equal(t, "upload-one", begun.Msg.GetUpload().GetId())
	assert.Equal(t, uint64(attachment.MaxChunkSizeBytes), begun.Msg.GetMaxChunkSizeBytes())
	assert.Equal(t, attachment.MaxAttachmentSizeBytes, begun.Msg.GetMaxAttachmentSizeBytes())
	assert.Equal(t, privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_ACTIVE, status.Msg.GetUpload().GetState())
	assert.Equal(t, privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_ABORTED, aborted.Msg.GetUpload().GetState())
}
