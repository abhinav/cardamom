package attachmentconnect

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestServiceMapsMetadataPaginationAndRemoval(t *testing.T) {
	association, err := attachment.NewIssueAssociation(
		board.ID("board-one"),
		issue.ID("an-origin"),
	)
	require.NoError(t, err)
	filename, err := attachment.NewFilename("report.txt")
	require.NoError(t, err)
	mediaType, err := attachment.NewMediaType("text/plain")
	require.NoError(t, err)
	digest, err := attachment.NewDigest("sha256:0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	active := attachment.Attachment{
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
	removed := active
	removed.Lifecycle = attachment.LifecycleRemoved
	removed.Removed = &attachment.Attribution{
		Actor: "remover", At: now.Add(time.Minute), Revision: board.Revision(8),
	}
	repository := &recordingRepository{
		attachment: active,
		page: attachment.Page{
			Attachments:   []attachment.Attachment{active, removed},
			NextPageToken: "next-page",
		},
	}
	client := newDomainTestClient(t, repository)

	got, err := client.GetAttachment(t.Context(), connect.NewRequest(
		&privatev1.GetAttachmentRequest{
			BoardId: "board-one", AttachmentId: active.ID.String(),
		},
	))
	require.NoError(t, err)
	listed, err := client.ListAttachments(t.Context(), connect.NewRequest(
		&privatev1.ListAttachmentsRequest{
			BoardId: "board-one", IssueId: new("an-origin"),
			IncludeRemoved: true, PageSize: 25, PageToken: new("opaque-page"),
		},
	))
	require.NoError(t, err)
	repository.attachment = removed
	deleted, err := client.RemoveAttachment(t.Context(), connect.NewRequest(
		&privatev1.RemoveAttachmentRequest{
			BoardId: "board-one", AttachmentId: active.ID.String(),
			Mutation: &privatev1.MutationContext{Actor: new(" remover ")},
		},
	))

	require.NoError(t, err)
	assert.Equal(t, active.ID.String(), got.Msg.GetAttachment().GetId())
	assert.Equal(t, "an-origin", got.Msg.GetAttachment().GetIssueId())
	assert.Equal(t, privatev1.BlobAvailability_BLOB_AVAILABILITY_VERIFIED, got.Msg.GetAttachment().GetAvailability())
	assert.Len(t, listed.Msg.GetAttachments(), 2)
	assert.Equal(t, "next-page", listed.Msg.GetNextPageToken())
	assert.Equal(t, "opaque-page", repository.listRequest.PageToken)
	assert.Equal(t, uint32(25), repository.listRequest.PageSize)
	assert.True(t, repository.listRequest.IncludeRemoved)
	require.NotNil(t, repository.listRequest.OriginIssueID)
	assert.Equal(t, "an-origin", repository.listRequest.OriginIssueID.String())
	assert.Equal(t, privatev1.AttachmentLifecycle_ATTACHMENT_LIFECYCLE_REMOVED, deleted.Msg.GetAttachment().GetLifecycle())
	assert.Equal(t, "remover", repository.removeRequest.Invocation.Actor())
	assert.Equal(t, "remover", deleted.Msg.GetAttachment().GetRemoved().GetActor())
}

func TestServiceMapsTypedDomainErrors(t *testing.T) {
	attachmentID := "att_aaaaaaaaaaaaaaaaaaaaaaaaaa"

	t.Run("UploadConflict", func(t *testing.T) {
		repository := &recordingRepository{beginErr: attachment.ErrUploadActorConflict}
		client := newDomainTestClient(t, repository)
		_, err := client.BeginAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.BeginAttachmentUploadRequest{
				BoardId: "board-one", Filename: "report.txt",
				Mutation: &privatev1.MutationContext{Actor: new("uploader")},
			},
		))
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	})

	t.Run("AttachmentNotFound", func(t *testing.T) {
		repository := &recordingRepository{metadataGetErr: attachment.ErrAttachmentNotFound}
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachment(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentRequest{
				BoardId: "board-one", AttachmentId: attachmentID,
			},
		))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	t.Run("InvalidPageToken", func(t *testing.T) {
		repository := &recordingRepository{listErr: attachment.ErrAttachmentPageTokenInvalid}
		client := newDomainTestClient(t, repository)
		_, err := client.ListAttachments(t.Context(), connect.NewRequest(
			&privatev1.ListAttachmentsRequest{
				BoardId: "board-one", PageToken: new("invalid"),
			},
		))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})
}

func TestServiceRejectsInvalidWireIdentities(t *testing.T) {
	t.Run("BoardID", func(t *testing.T) {
		repository := new(recordingRepository)
		client := newDomainTestClient(t, repository)
		_, err := client.ListAttachments(t.Context(), connect.NewRequest(
			&privatev1.ListAttachmentsRequest{BoardId: "not valid"},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Empty(t, repository.listRequest.BoardID)
	})

	t.Run("IssueID", func(t *testing.T) {
		repository := new(recordingRepository)
		client := newDomainTestClient(t, repository)
		_, err := client.ListAttachments(t.Context(), connect.NewRequest(
			&privatev1.ListAttachmentsRequest{
				BoardId: "board-one", IssueId: new("not valid"),
			},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Nil(t, repository.listRequest.OriginIssueID)
	})

	t.Run("AttachmentID", func(t *testing.T) {
		repository := new(recordingRepository)
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachment(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentRequest{
				BoardId: "board-one", AttachmentId: "invalid",
			},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Empty(t, repository.metadataGet.AttachmentID)
	})

	t.Run("UploadID", func(t *testing.T) {
		repository := new(recordingRepository)
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentUploadRequest{UploadId: "not valid"},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Empty(t, repository.getRequest.UploadID)
	})

	t.Run("Filename", func(t *testing.T) {
		repository := new(recordingRepository)
		client := newDomainTestClient(t, repository)
		_, err := client.BeginAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.BeginAttachmentUploadRequest{
				BoardId: "board-one", Filename: "../report.txt",
				Mutation: &privatev1.MutationContext{Actor: new("uploader")},
			},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Empty(t, repository.beginRequest.Filename)
	})

	t.Run("Digest", func(t *testing.T) {
		repository := new(recordingRepository)
		client := newDomainTestClient(t, repository)
		_, err := client.BeginAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.BeginAttachmentUploadRequest{
				BoardId: "board-one", Filename: "report.txt",
				ExpectedDigest: new("sha256:not-valid"),
				Mutation:       &privatev1.MutationContext{Actor: new("uploader")},
			},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Nil(t, repository.beginRequest.ExpectedDigest)
	})
}

func TestServiceRejectsImpossibleDomainEnums(t *testing.T) {
	t.Run("UploadState", func(t *testing.T) {
		upload := validUpload(t)
		upload.State = 0
		repository := &recordingRepository{upload: upload}
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentUploadRequest{UploadId: "upload-one"},
		))

		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})

	t.Run("AttachmentLifecycle", func(t *testing.T) {
		value := validAttachment(t)
		value.Lifecycle = 0
		repository := &recordingRepository{attachment: value}
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachment(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentRequest{
				BoardId:      "board-one",
				AttachmentId: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		))

		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})

	t.Run("BlobAvailability", func(t *testing.T) {
		verification := validVerification(t)
		verification.Availability = 0
		repository := &recordingRepository{verify: verification}
		client := newDomainTestClient(t, repository)
		_, err := client.VerifyAttachment(t.Context(), connect.NewRequest(
			&privatev1.VerifyAttachmentRequest{
				BoardId:      "board-one",
				AttachmentId: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		))

		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})
}
