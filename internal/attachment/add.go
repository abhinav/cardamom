package attachment

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.abhg.dev/cardamom/internal/errkind"
)

// AddRequest supplies metadata and streamed content for one complete
// attachment upload.
type AddRequest struct {
	// Invocation identifies the actor that owns the upload.
	Invocation Invocation // required

	// Association identifies the target board and optional originating issue.
	Association Association // required

	// Filename is the portable presentation filename.
	Filename Filename // required

	// ExpectedSizeBytes is the optional client-declared complete size.
	ExpectedSizeBytes *uint64

	// ExpectedDigest is the optional client-computed SHA-256 identity.
	ExpectedDigest *Digest

	// Content supplies the attachment bytes. AddAttachment reads Content in
	// chunks no larger than MaxChunkSizeBytes.
	Content io.Reader // required
}

// Validate verifies attachment metadata, declared bounds, and streamed input.
func (r *AddRequest) Validate() error {
	if r.Content == nil {
		return errors.New("attachment content required")
	}
	return (&BeginUploadRequest{
		Invocation:        r.Invocation,
		Association:       r.Association,
		Filename:          r.Filename,
		ExpectedSizeBytes: r.ExpectedSizeBytes,
		ExpectedDigest:    r.ExpectedDigest,
	}).Validate()
}

// AddAttachment streams one complete attachment through the durable upload
// lifecycle. An unsuccessful active upload is aborted before the error is
// returned.
func (s *Service) AddAttachment(
	ctx context.Context,
	request AddRequest,
) (_ Attachment, err error) {
	if validateErr := request.Validate(); validateErr != nil {
		return Attachment{}, errkind.Wrap(errkind.InvalidInput, validateErr)
	}
	upload, err := s.BeginUpload(ctx, BeginUploadRequest{
		Invocation:        request.Invocation,
		Association:       request.Association,
		Filename:          request.Filename,
		ExpectedSizeBytes: request.ExpectedSizeBytes,
		ExpectedDigest:    request.ExpectedDigest,
	})
	if err != nil {
		return Attachment{}, err
	}
	abort := true
	defer func() {
		if !abort {
			return
		}
		// Cancellation stops the requested upload, not its terminal cleanup.
		cleanupContext := context.WithoutCancel(ctx)
		_, abortErr := s.AbortUpload(cleanupContext, AbortUploadRequest{
			Invocation: request.Invocation,
			UploadID:   upload.ID,
		})
		if abortErr != nil {
			err = errors.Join(err, fmt.Errorf("abort attachment upload: %w", abortErr))
		}
	}()

	buffer := make([]byte, MaxChunkSizeBytes)
	for {
		bytesRead, readErr := io.ReadFull(request.Content, buffer)
		if bytesRead > 0 {
			nextUpload, writeErr := s.WriteChunk(ctx, WriteChunkRequest{
				Invocation:     request.Invocation,
				UploadID:       upload.ID,
				ExpectedOffset: upload.AcceptedOffset,
				Content:        buffer[:bytesRead],
			})
			if writeErr != nil {
				return Attachment{}, writeErr
			}
			upload = nextUpload
		}
		switch {
		case readErr == nil:
			continue
		case errors.Is(readErr, io.EOF), errors.Is(readErr, io.ErrUnexpectedEOF):
		default:
			return Attachment{}, fmt.Errorf("read attachment content: %w", readErr)
		}
		break
	}

	value, err := s.CommitUpload(ctx, CommitUploadRequest{
		Invocation: request.Invocation,
		UploadID:   upload.ID,
	})
	if err != nil {
		return Attachment{}, err
	}
	abort = false
	return value, nil
}
