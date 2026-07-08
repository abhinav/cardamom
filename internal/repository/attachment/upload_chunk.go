package attachment

import (
	"context"
	"errors"
	"fmt"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// WriteChunk appends or replays bounded sequential content.
func (r *Repository) WriteChunk(
	ctx context.Context,
	request domainattachment.WriteChunkRequest,
) (_ domainattachment.Upload, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return domainattachment.Upload{}, fmt.Errorf("begin attachment chunk change: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	upload, err := r.loadUpload(ctx, change, request.UploadID)
	if err != nil {
		return domainattachment.Upload{}, err
	}
	if err := requireUploadActor(upload, request.Invocation.Actor()); err != nil {
		return domainattachment.Upload{}, err
	}
	now := wholeSecond(r.clock.Now())
	if uploadExpired(upload, now) {
		return domainattachment.Upload{}, r.commitExpiredConflict(ctx, change, &upload, now)
	}
	if upload.State != domainattachment.UploadStateActive {
		return domainattachment.Upload{}, domainattachment.ErrUploadStateConflict
	}
	maximumSizeBytes := upload.MaximumSizeBytes.Uint64()
	contentSize := uint64(len(request.Content))
	if request.ExpectedOffset > maximumSizeBytes ||
		contentSize > maximumSizeBytes-request.ExpectedOffset {
		return domainattachment.Upload{}, domainattachment.ErrUploadDescriptorMismatch
	}
	if upload.ExpectedSizeBytes != nil {
		if request.ExpectedOffset > *upload.ExpectedSizeBytes ||
			contentSize > *upload.ExpectedSizeBytes-request.ExpectedOffset {
			return domainattachment.Upload{}, domainattachment.ErrUploadDescriptorMismatch
		}
	}
	offset, err := r.blobs.writeChunk(
		upload.ID,
		request.ExpectedOffset,
		request.Content,
	)
	if err != nil {
		return domainattachment.Upload{}, classifyBlobChunkError(err)
	}
	upload.AcceptedOffset = offset
	upload.ExpiresAt = now.Add(domainattachment.StagingExpiry)
	err = query.New(change).AttachmentUpdateUploadProgress(
		ctx,
		query.AttachmentUpdateUploadProgressParams{
			AcceptedOffset: int64(upload.AcceptedOffset),
			ExpiresAt:      upload.ExpiresAt,
			ID:             upload.ID.String(),
		},
	)
	if err != nil {
		return domainattachment.Upload{}, fmt.Errorf("update attachment upload progress: %w", err)
	}
	if err := change.Commit(); err != nil {
		return domainattachment.Upload{}, fmt.Errorf("commit attachment chunk: %w", err)
	}
	return upload, nil
}

func classifyBlobChunkError(err error) error {
	switch {
	case errors.Is(err, errStagingOffsetConflict):
		return errors.Join(domainattachment.ErrUploadOffsetConflict, err)
	case errors.Is(err, errStagingChunkConflict):
		return errors.Join(domainattachment.ErrUploadChunkConflict, err)
	default:
		return err
	}
}
