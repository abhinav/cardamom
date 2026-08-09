package attachmentconnect

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
)

func newTestClient(
	t *testing.T,
	operations Operations,
) privatev1connect.AttachmentServiceClient {
	t.Helper()
	_, handler := privatev1connect.NewAttachmentServiceHandler(
		New(operations),
	)
	client := &http.Client{Transport: attachmentHandlerTransport{handler: handler}}
	return privatev1connect.NewAttachmentServiceClient(client, "http://cardamom.test")
}

func newDomainTestClient(
	t *testing.T,
	repository attachment.Repository,
) privatev1connect.AttachmentServiceClient {
	t.Helper()
	return newTestClient(t, attachment.NewService(attachment.ServiceConfig{
		Repository: repository,
	}))
}

type attachmentHandlerTransport struct{ handler http.Handler }

func (t attachmentHandlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

func validUpload(t *testing.T) attachment.Upload {
	t.Helper()
	association, err := attachment.NewBoardAssociation(board.ID("board-one"))
	require.NoError(t, err)
	filename, err := attachment.NewFilename("report.txt")
	require.NoError(t, err)
	return attachment.Upload{
		ID: attachment.UploadID("upload-one"), Association: association,
		Filename: filename, Actor: "uploader", State: attachment.UploadStateActive,
		AcceptedOffset:   0,
		MaximumSizeBytes: configuration.ByteLimit(configuration.DefaultAttachmentMaxBytes),
		ExpiresAt:        time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC),
	}
}

func validAttachment(t *testing.T) attachment.Attachment {
	t.Helper()
	association, err := attachment.NewBoardAssociation(board.ID("board-one"))
	require.NoError(t, err)
	filename, err := attachment.NewFilename("report.txt")
	require.NoError(t, err)
	mediaType, err := attachment.NewMediaType("text/plain")
	require.NoError(t, err)
	digest, err := attachment.NewDigest("sha256:0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	return attachment.Attachment{
		ID:          attachment.ID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Association: association,
		Blob:        attachment.BlobDescriptor{Digest: digest, SizeBytes: 12},
		Filename:    filename, MediaType: mediaType,
		Lifecycle:    attachment.LifecycleActive,
		Availability: attachment.BlobAvailabilityVerified,
		Created: attachment.Attribution{
			Actor:    "uploader",
			At:       time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC),
			Revision: 7,
		},
	}
}

func validVerification(t *testing.T) attachment.Verification {
	t.Helper()
	value := validAttachment(t)
	return attachment.Verification{
		AttachmentID: value.ID, Blob: value.Blob,
		Availability: attachment.BlobAvailabilityVerified,
		ObservedAt:   value.Created.At.Add(time.Minute),
	}
}
