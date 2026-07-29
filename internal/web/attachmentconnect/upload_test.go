package attachmentconnect

import (
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
	repository := &recordingRepository{
		upload: attachment.Upload{
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
		},
		attachment: committedAttachment,
	}
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
		assert.Equal(t, repository.attachment.ID.String(), committed.Msg.GetAttachment().GetId())
	}
	repository.upload.State = attachment.UploadStateAborted
	aborted, err := client.AbortAttachmentUpload(t.Context(), connect.NewRequest(
		&privatev1.AbortAttachmentUploadRequest{UploadId: "upload-one", Mutation: mutation},
	))

	require.NoError(t, err)
	assert.Equal(t, "upload-one", begun.Msg.GetUpload().GetId())
	assert.Equal(t, uint64(attachment.MaxChunkSizeBytes), begun.Msg.GetMaxChunkSizeBytes())
	assert.Equal(t, attachment.MaxAttachmentSizeBytes, begun.Msg.GetMaxAttachmentSizeBytes())
	assert.Equal(t, privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_ACTIVE, status.Msg.GetUpload().GetState())
	assert.Equal(t, privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_ABORTED, aborted.Msg.GetUpload().GetState())
	assert.Equal(t, "uploader", repository.beginRequest.Invocation.Actor())
	assert.Equal(t, board.ID("board-one"), repository.beginRequest.Association.BoardID())
	assert.Equal(t, filename, repository.beginRequest.Filename)
	require.Len(t, repository.writeRequests, 2)
	assert.Equal(t, "uploader", repository.writeRequests[0].Invocation.Actor())
	assert.Equal(t, []byte("repeat"), repository.writeRequests[0].Content)
	assert.Equal(t, attachment.UploadID("upload-one"), repository.getRequest.UploadID)
	require.Len(t, repository.commitRequests, 2)
	assert.Equal(t, "uploader", repository.commitRequests[0].Invocation.Actor())
	assert.Equal(t, "uploader", repository.abortRequest.Invocation.Actor())
}
