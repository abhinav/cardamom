package attachment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// CommitUpload publishes one attachment or returns its existing receipt.
func (r *Repository) CommitUpload(
	ctx context.Context,
	request domainattachment.CommitUploadRequest,
) (_ domainattachment.Attachment, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("begin attachment commit: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	upload, err := r.loadUpload(ctx, change, request.UploadID)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	if err := requireUploadActor(upload, request.Invocation.Actor()); err != nil {
		return domainattachment.Attachment{}, err
	}
	if upload.State == domainattachment.UploadStateCommitted {
		return *upload.Attachment, r.blobs.removeStaging(upload.ID)
	}
	now := wholeSecond(r.clock.Now())
	if uploadExpired(upload, now) {
		return domainattachment.Attachment{}, r.commitExpiredConflict(ctx, change, &upload, now)
	}
	if upload.State != domainattachment.UploadStateActive {
		return domainattachment.Attachment{}, domainattachment.ErrUploadStateConflict
	}
	descriptor, err := r.blobs.publishForCommit(
		upload.ID,
		upload.MaximumSizeBytes.Uint64(),
		upload.ExpectedSizeBytes,
		upload.ExpectedDigest,
	)
	if errors.Is(err, errDescriptorMismatch) {
		return domainattachment.Attachment{}, errors.Join(
			domainattachment.ErrUploadDescriptorMismatch,
			err,
		)
	}
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	mediaType, err := r.detectMediaType(descriptor)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachmentID, err := r.newAttachmentID()
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	revision, err := r.reserveAttachmentRevision(ctx, change, upload.Association)
	if err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("reserve attachment revision: %w", err)
	}
	attachment := domainattachment.Attachment{
		ID: attachmentID, Association: upload.Association, Blob: descriptor,
		Filename: upload.Filename, MediaType: mediaType,
		Lifecycle:    domainattachment.LifecycleActive,
		Availability: domainattachment.BlobAvailabilityVerified,
		Created: domainattachment.Attribution{
			Actor: upload.Actor, At: now,
			Revision: board.Revision(revision.reservation.Revision()),
		},
	}
	if err := insertAttachment(ctx, change, attachment); err != nil {
		return domainattachment.Attachment{}, err
	}
	upload.State = domainattachment.UploadStateCommitted
	upload.ExpiresAt = now.Add(domainattachment.TerminalReceiptRetention)
	upload.Attachment = &attachment
	attachmentIDValue := attachment.ID.String()
	err = query.New(change).AttachmentCommitUploadReceipt(
		ctx,
		query.AttachmentCommitUploadReceiptParams{
			ExpiresAt:    upload.ExpiresAt,
			AttachmentID: &attachmentIDValue,
			ID:           upload.ID.String(),
		},
	)
	if err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("commit attachment upload receipt: %w", err)
	}
	if err := r.commitAttachmentRevision(ctx, change, revision); err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("commit attachment metadata: %w", err)
	}
	return attachment, r.blobs.removeStaging(upload.ID)
}

// AbortUpload abandons an active upload or returns its existing receipt.
func (r *Repository) AbortUpload(
	ctx context.Context,
	request domainattachment.AbortUploadRequest,
) (_ domainattachment.Upload, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return domainattachment.Upload{}, fmt.Errorf("begin attachment abort: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	upload, err := r.loadUpload(ctx, change, request.UploadID)
	if err != nil {
		return domainattachment.Upload{}, err
	}
	if err := requireUploadActor(upload, request.Invocation.Actor()); err != nil {
		return domainattachment.Upload{}, err
	}
	if upload.State == domainattachment.UploadStateAborted {
		return upload, r.blobs.removeStaging(upload.ID)
	}
	now := wholeSecond(r.clock.Now())
	if uploadExpired(upload, now) {
		return domainattachment.Upload{}, r.commitExpiredConflict(ctx, change, &upload, now)
	}
	if upload.State != domainattachment.UploadStateActive {
		return domainattachment.Upload{}, domainattachment.ErrUploadStateConflict
	}
	upload.State = domainattachment.UploadStateAborted
	upload.ExpiresAt = now.Add(domainattachment.TerminalReceiptRetention)
	err = query.New(change).AttachmentAbortUploadReceipt(
		ctx,
		query.AttachmentAbortUploadReceiptParams{
			ExpiresAt: upload.ExpiresAt,
			ID:        upload.ID.String(),
		},
	)
	if err != nil {
		return domainattachment.Upload{}, fmt.Errorf("abort attachment upload: %w", err)
	}
	if err := change.Commit(); err != nil {
		return domainattachment.Upload{}, fmt.Errorf("commit attachment abort: %w", err)
	}
	return upload, r.blobs.removeStaging(upload.ID)
}

func (r *Repository) detectMediaType(
	descriptor domainattachment.BlobDescriptor,
) (_ domainattachment.MediaType, err error) {
	reader, availability, err := r.blobs.openVerified(descriptor)
	if err != nil {
		return "", err
	}
	if availability != domainattachment.BlobAvailabilityVerified || reader == nil {
		return "", fmt.Errorf("published attachment blob is %s", availability)
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	buffer := make([]byte, 512)
	count, err := reader.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read attachment media type sample: %w", err)
	}
	return domainattachment.NewMediaType(http.DetectContentType(buffer[:count]))
}

func insertAttachment(
	ctx context.Context,
	change *store.Change,
	attachment domainattachment.Attachment,
) error {
	queries := query.New(change)
	if err := queries.AttachmentRetainBlob(
		ctx,
		query.AttachmentRetainBlobParams{
			Digest:    attachment.Blob.Digest.String(),
			SizeBytes: int64(attachment.Blob.SizeBytes),
		},
	); err != nil {
		return fmt.Errorf("retain attachment blob descriptor: %w", err)
	}
	originIssueID, hasOrigin := attachment.Association.OriginIssueID()
	if err := queries.AttachmentInsertMetadata(
		ctx,
		query.AttachmentInsertMetadataParams{
			BoardID:         attachment.Association.BoardID().String(),
			ID:              attachment.ID.String(),
			OriginIssueID:   nullableOrigin(originIssueID.String(), hasOrigin),
			BlobDigest:      attachment.Blob.Digest.String(),
			BlobSizeBytes:   int64(attachment.Blob.SizeBytes),
			Filename:        attachment.Filename.String(),
			MediaType:       attachment.MediaType.String(),
			CreatedActor:    attachment.Created.Actor,
			CreatedAt:       attachment.Created.At,
			CreatedRevision: int64(attachment.Created.Revision),
		},
	); err != nil {
		return fmt.Errorf("insert attachment metadata: %w", err)
	}
	return nil
}
