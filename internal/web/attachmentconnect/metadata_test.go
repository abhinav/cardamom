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
	"go.uber.org/mock/gomock"
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
	repository := NewMockRepository(gomock.NewController(t))
	repository.EXPECT().GetAttachment(gomock.Any(), attachment.GetRequest{
		BoardID: "board-one", AttachmentID: active.ID,
	}).Return(active, nil)
	originIssueID := issue.ID("an-origin")
	repository.EXPECT().ListAttachments(gomock.Any(), attachment.ListRequest{
		BoardID: "board-one", OriginIssueID: &originIssueID,
		IncludeRemoved: true, PageSize: 25, PageToken: "opaque-page",
	}).Return(attachment.Page{
		Attachments:   []attachment.Attachment{active, removed},
		NextPageToken: "next-page",
	}, nil)
	repository.EXPECT().RemoveAttachment(gomock.Any(), attachment.RemoveRequest{
		Invocation: attachment.NewInvocation("remover"),
		BoardID:    "board-one", AttachmentID: active.ID,
	}).Return(removed, nil)
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
	assert.Equal(t, privatev1.AttachmentLifecycle_ATTACHMENT_LIFECYCLE_REMOVED, deleted.Msg.GetAttachment().GetLifecycle())
	assert.Equal(t, "remover", deleted.Msg.GetAttachment().GetRemoved().GetActor())
}

func TestServiceMapsTypedDomainErrors(t *testing.T) {
	attachmentID := "att_aaaaaaaaaaaaaaaaaaaaaaaaaa"

	t.Run("UploadConflict", func(t *testing.T) {
		repository := NewMockRepository(gomock.NewController(t))
		repository.EXPECT().BeginUpload(gomock.Any(), gomock.Any()).
			Return(validUpload(t), attachment.ErrUploadActorConflict)
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
		repository := NewMockRepository(gomock.NewController(t))
		repository.EXPECT().GetAttachment(gomock.Any(), gomock.Any()).
			Return(validAttachment(t), attachment.ErrAttachmentNotFound)
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachment(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentRequest{
				BoardId: "board-one", AttachmentId: attachmentID,
			},
		))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	t.Run("InvalidPageToken", func(t *testing.T) {
		repository := NewMockRepository(gomock.NewController(t))
		repository.EXPECT().ListAttachments(gomock.Any(), gomock.Any()).
			Return(attachment.Page{Attachments: []attachment.Attachment{}}, attachment.ErrAttachmentPageTokenInvalid)
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
		repository := NewMockRepository(gomock.NewController(t))
		client := newDomainTestClient(t, repository)
		_, err := client.ListAttachments(t.Context(), connect.NewRequest(
			&privatev1.ListAttachmentsRequest{BoardId: "not valid"},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("IssueID", func(t *testing.T) {
		repository := NewMockRepository(gomock.NewController(t))
		client := newDomainTestClient(t, repository)
		_, err := client.ListAttachments(t.Context(), connect.NewRequest(
			&privatev1.ListAttachmentsRequest{
				BoardId: "board-one", IssueId: new("not valid"),
			},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("AttachmentID", func(t *testing.T) {
		repository := NewMockRepository(gomock.NewController(t))
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachment(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentRequest{
				BoardId: "board-one", AttachmentId: "invalid",
			},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("UploadID", func(t *testing.T) {
		repository := NewMockRepository(gomock.NewController(t))
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentUploadRequest{UploadId: "not valid"},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("Filename", func(t *testing.T) {
		repository := NewMockRepository(gomock.NewController(t))
		client := newDomainTestClient(t, repository)
		_, err := client.BeginAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.BeginAttachmentUploadRequest{
				BoardId: "board-one", Filename: "../report.txt",
				Mutation: &privatev1.MutationContext{Actor: new("uploader")},
			},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("Digest", func(t *testing.T) {
		repository := NewMockRepository(gomock.NewController(t))
		client := newDomainTestClient(t, repository)
		_, err := client.BeginAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.BeginAttachmentUploadRequest{
				BoardId: "board-one", Filename: "report.txt",
				ExpectedDigest: new("sha256:not-valid"),
				Mutation:       &privatev1.MutationContext{Actor: new("uploader")},
			},
		))

		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})
}

func TestServiceRejectsImpossibleDomainEnums(t *testing.T) {
	t.Run("UploadState", func(t *testing.T) {
		upload := validUpload(t)
		upload.State = 0
		repository := NewMockRepository(gomock.NewController(t))
		repository.EXPECT().GetUpload(gomock.Any(), attachment.GetUploadRequest{
			UploadID: "upload-one",
		}).Return(upload, nil)
		client := newDomainTestClient(t, repository)
		_, err := client.GetAttachmentUpload(t.Context(), connect.NewRequest(
			&privatev1.GetAttachmentUploadRequest{UploadId: "upload-one"},
		))

		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})

	t.Run("AttachmentLifecycle", func(t *testing.T) {
		value := validAttachment(t)
		value.Lifecycle = 0
		repository := NewMockRepository(gomock.NewController(t))
		repository.EXPECT().GetAttachment(gomock.Any(), attachment.GetRequest{
			BoardID: "board-one", AttachmentID: value.ID,
		}).Return(value, nil)
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
		repository := NewMockRepository(gomock.NewController(t))
		repository.EXPECT().VerifyAttachment(gomock.Any(), attachment.VerifyRequest{
			BoardID: "board-one", AttachmentID: verification.AttachmentID,
		}).Return(verification, nil)
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
