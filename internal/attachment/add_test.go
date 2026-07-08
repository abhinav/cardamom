package attachment

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
)

func TestService_AddAttachment_streamsMaximumSizeInBoundedChunks(t *testing.T) {
	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	association, err := NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := NewFilename("maximum.bin")
	require.NoError(t, err)
	repository := new(addRecordingRepository)
	service := NewService(ServiceConfig{Repository: repository})
	reader := &boundedAttachmentReader{remaining: MaxAttachmentSizeBytes}
	expectedSize := MaxAttachmentSizeBytes

	created, err := service.AddAttachment(t.Context(), AddRequest{
		Invocation: NewInvocation("tester"), Association: association,
		Filename: filename, ExpectedSizeBytes: &expectedSize, Content: reader,
	})

	require.NoError(t, err)
	assert.Equal(t, "att_aaaaaaaaaaaaaaaaaaaaaaaaaa", created.ID.String())
	assert.Equal(t, MaxChunkSizeBytes, reader.largestRequest)
	assert.Len(t, repository.chunkSizes, 25)
	for _, size := range repository.chunkSizes {
		assert.Equal(t, MaxChunkSizeBytes, size)
	}
	assert.Equal(t, MaxAttachmentSizeBytes, repository.acceptedOffset)
}

func TestService_AddAttachment_abortsActiveUploadAfterChunkFailure(t *testing.T) {
	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	association, err := NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := NewFilename("failure.bin")
	require.NoError(t, err)
	repository := &addRecordingRepository{writeErr: errors.New("write rejected")}
	service := NewService(ServiceConfig{Repository: repository})

	_, err = service.AddAttachment(t.Context(), AddRequest{
		Invocation: NewInvocation("tester"), Association: association,
		Filename: filename, Content: &boundedAttachmentReader{remaining: 1},
	})

	assert.ErrorContains(t, err, "write rejected")
	assert.Equal(t, "upload-test", repository.abortedID.String())
}

func TestService_AddAttachment_abortsWithUsableContextAfterCancellation(t *testing.T) {
	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	association, err := NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := NewFilename("cancelled.bin")
	require.NoError(t, err)
	repository := &addRecordingRepository{writeErr: context.Canceled}
	service := NewService(ServiceConfig{Repository: repository})
	ctx := context.WithValue(t.Context(), addContextKey{}, "request-value")
	ctx, cancel := context.WithCancel(ctx)

	_, err = service.AddAttachment(ctx, AddRequest{
		Invocation: NewInvocation("tester"), Association: association,
		Filename: filename, Content: &cancelingAttachmentReader{cancel: cancel},
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.NoError(t, repository.abortContextErr)
	assert.Equal(t, "request-value", repository.abortContextValue)
	assert.Equal(t, "upload-test", repository.abortedID.String())
}

type addContextKey struct{}

type cancelingAttachmentReader struct {
	cancel context.CancelFunc
}

func (r *cancelingAttachmentReader) Read(buffer []byte) (int, error) {
	r.cancel()
	buffer[0] = 'x'
	return 1, io.EOF
}

// boundedAttachmentReader supplies a virtual attachment without allocating its
// complete content and records the largest buffer requested by the service.
type boundedAttachmentReader struct {
	remaining      uint64
	largestRequest int
}

func (r *boundedAttachmentReader) Read(buffer []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if len(buffer) > r.largestRequest {
		r.largestRequest = len(buffer)
	}
	bytesRead := len(buffer)
	if uint64(bytesRead) > r.remaining {
		bytesRead = int(r.remaining)
	}
	r.remaining -= uint64(bytesRead)
	return bytesRead, nil
}

// addRecordingRepository records upload boundaries while leaving unrelated
// attachment repository operations outside this test path.
type addRecordingRepository struct {
	Repository
	upload            Upload
	chunkSizes        []int
	acceptedOffset    uint64
	writeErr          error
	abortedID         UploadID
	abortContextErr   error
	abortContextValue any
}

func (r *addRecordingRepository) BeginUpload(
	_ context.Context,
	admission BeginUploadAdmission,
) (Upload, error) {
	request := admission.Request
	uploadID, err := NewUploadID("upload-test")
	if err != nil {
		return Upload{}, err
	}
	r.upload = Upload{
		ID: uploadID, Association: request.Association, Filename: request.Filename,
		Actor: request.Invocation.Actor(), State: UploadStateActive,
		AcceptedOffset: 0, MaximumSizeBytes: admission.MaximumSizeBytes,
		ExpiresAt: time.Unix(1, 0).UTC(),
	}
	return r.upload, nil
}

func (r *addRecordingRepository) WriteChunk(
	_ context.Context,
	request WriteChunkRequest,
) (Upload, error) {
	if r.writeErr != nil {
		return Upload{}, r.writeErr
	}
	r.chunkSizes = append(r.chunkSizes, len(request.Content))
	r.acceptedOffset += uint64(len(request.Content))
	r.upload.AcceptedOffset = r.acceptedOffset
	return r.upload, nil
}

func (r *addRecordingRepository) CommitUpload(
	context.Context,
	CommitUploadRequest,
) (Attachment, error) {
	id, err := NewID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		return Attachment{}, err
	}
	digest, err := NewDigest(
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)
	if err != nil {
		return Attachment{}, err
	}
	mediaType, err := NewMediaType("application/octet-stream")
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{
		ID: id, Association: r.upload.Association,
		Blob:     BlobDescriptor{Digest: digest, SizeBytes: r.acceptedOffset},
		Filename: r.upload.Filename, MediaType: mediaType,
		Lifecycle: LifecycleActive, Availability: BlobAvailabilityVerified,
		Created: Attribution{
			Actor: "tester", At: time.Unix(1, 0).UTC(), Revision: 1,
		},
	}, nil
}

func (r *addRecordingRepository) AbortUpload(
	ctx context.Context,
	request AbortUploadRequest,
) (Upload, error) {
	r.abortContextErr = ctx.Err()
	r.abortContextValue = ctx.Value(addContextKey{})
	if r.abortContextErr != nil {
		return Upload{}, r.abortContextErr
	}
	r.abortedID = request.UploadID
	return r.upload, nil
}
