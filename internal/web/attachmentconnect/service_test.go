package attachmentconnect

import (
	"context"
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

type recordingRepository struct {
	attachment.Repository

	beginRequest   attachment.BeginUploadRequest
	writeRequests  []attachment.WriteChunkRequest
	getRequest     attachment.GetUploadRequest
	commitRequests []attachment.CommitUploadRequest
	abortRequest   attachment.AbortUploadRequest
	metadataGet    attachment.GetRequest
	listRequest    attachment.ListRequest
	removeRequest  attachment.RemoveRequest
	verifyRequest  attachment.VerifyRequest
	collectRequest attachment.CollectRequest

	upload     attachment.Upload
	attachment attachment.Attachment
	page       attachment.Page
	verify     attachment.Verification
	collection attachment.CollectionResult

	beginErr       error
	writeErr       error
	getErr         error
	commitErr      error
	abortErr       error
	metadataGetErr error
	listErr        error
	removeErr      error
	verifyErr      error
	collectErr     error
}

func (r *recordingRepository) BeginUpload(
	_ context.Context,
	admission attachment.BeginUploadAdmission,
) (attachment.Upload, error) {
	r.beginRequest = admission.Request
	r.upload.MaximumSizeBytes = admission.MaximumSizeBytes
	return r.upload, r.beginErr
}

func (r *recordingRepository) WriteChunk(
	_ context.Context,
	request attachment.WriteChunkRequest,
) (attachment.Upload, error) {
	r.writeRequests = append(r.writeRequests, request)
	return r.upload, r.writeErr
}

func (r *recordingRepository) GetUpload(
	_ context.Context,
	request attachment.GetUploadRequest,
) (attachment.Upload, error) {
	r.getRequest = request
	return r.upload, r.getErr
}

func (r *recordingRepository) CommitUpload(
	_ context.Context,
	request attachment.CommitUploadRequest,
) (attachment.Attachment, error) {
	r.commitRequests = append(r.commitRequests, request)
	return r.attachment, r.commitErr
}

func (r *recordingRepository) AbortUpload(
	_ context.Context,
	request attachment.AbortUploadRequest,
) (attachment.Upload, error) {
	r.abortRequest = request
	return r.upload, r.abortErr
}

func (r *recordingRepository) GetAttachment(
	_ context.Context,
	request attachment.GetRequest,
) (attachment.Attachment, error) {
	r.metadataGet = request
	return r.attachment, r.metadataGetErr
}

func (r *recordingRepository) ListAttachments(
	_ context.Context,
	request attachment.ListRequest,
) (attachment.Page, error) {
	r.listRequest = request
	return r.page, r.listErr
}

func (r *recordingRepository) RemoveAttachment(
	_ context.Context,
	request attachment.RemoveRequest,
) (attachment.Attachment, error) {
	r.removeRequest = request
	return r.attachment, r.removeErr
}

func (r *recordingRepository) VerifyAttachment(
	_ context.Context,
	request attachment.VerifyRequest,
) (attachment.Verification, error) {
	r.verifyRequest = request
	return r.verify, r.verifyErr
}

func (r *recordingRepository) CollectAttachments(
	_ context.Context,
	request attachment.CollectRequest,
) (attachment.CollectionResult, error) {
	r.collectRequest = request
	return r.collection, r.collectErr
}

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
	repository *recordingRepository,
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
