package attachment

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// CollectAttachments expires inactive staging and removes true orphan blobs.
func (r *Repository) CollectAttachments(
	ctx context.Context,
	request domainattachment.CollectRequest,
) (domainattachment.CollectionResult, error) {
	result := domainattachment.CollectionResult{
		DryRun:            request.DryRun,
		ExpiredStaging:    domainattachment.CollectionSummary{Count: 0, Bytes: 0},
		OrphanBlobs:       domainattachment.CollectionSummary{Count: 0, Bytes: 0},
		IntegrityProblems: nil,
	}
	var err error
	result.ExpiredStaging, err = r.collectUploadStaging(ctx, request.DryRun)
	if err != nil {
		return domainattachment.CollectionResult{}, err
	}

	result.IntegrityProblems, result.OrphanBlobs, err = r.collectRetainedContent(ctx, request.DryRun)
	if err != nil {
		return domainattachment.CollectionResult{}, err
	}
	return result, nil
}

func (r *Repository) collectUploadStaging(
	ctx context.Context,
	dryRun bool,
) (_ domainattachment.CollectionSummary, err error) {
	now := wholeSecond(r.clock.Now())
	if !dryRun {
		if err := r.expireActiveUploads(ctx, now); err != nil {
			return domainattachment.CollectionSummary{}, err
		}
	}

	change, err := r.store.Change(ctx)
	if err != nil {
		return domainattachment.CollectionSummary{},
			fmt.Errorf("begin attachment staging collection: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	rows, err := query.New(change).AttachmentListCollectibleUploads(
		ctx,
		query.AttachmentListCollectibleUploadsParams{
			IncludeExpiredActive: dryRun,
			Now:                  now,
		},
	)
	if err != nil {
		return domainattachment.CollectionSummary{},
			fmt.Errorf("select collectible attachment staging: %w", err)
	}
	uploadIDs := make([]domainattachment.UploadID, 0, len(rows))
	for _, row := range rows {
		uploadID, err := domainattachment.NewUploadID(row)
		if err != nil {
			return domainattachment.CollectionSummary{}, err
		}
		uploadIDs = append(uploadIDs, uploadID)
	}
	summary, err := r.blobs.collectExpiredStaging(uploadIDs, dryRun)
	if err != nil || dryRun {
		return summary, err
	}
	if err := query.New(change).AttachmentDeleteExpiredUploadReceipts(
		ctx,
		now,
	); err != nil {
		return domainattachment.CollectionSummary{},
			fmt.Errorf("delete expired attachment upload receipts: %w", err)
	}
	if err := change.Commit(); err != nil {
		return domainattachment.CollectionSummary{},
			fmt.Errorf("commit attachment staging collection: %w", err)
	}
	return summary, nil
}

func (r *Repository) expireActiveUploads(ctx context.Context, now time.Time) (err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return fmt.Errorf("begin attachment upload expiry collection: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	if err := query.New(change).AttachmentExpireActiveUploads(
		ctx,
		query.AttachmentExpireActiveUploadsParams{
			ReceiptExpiresAt: now.Add(domainattachment.TerminalReceiptRetention),
			Now:              now,
		},
	); err != nil {
		return fmt.Errorf("expire collected attachment uploads: %w", err)
	}
	if err := change.Commit(); err != nil {
		return fmt.Errorf("commit attachment upload expiry collection: %w", err)
	}
	return nil
}

func (r *Repository) collectRetainedContent(
	ctx context.Context,
	dryRun bool,
) (_ []domainattachment.IntegrityProblem, _ domainattachment.CollectionSummary, err error) {
	// A writer scope prevents upload publication from racing the retained set
	// used to select orphan bytes.
	change, err := r.store.Change(ctx)
	if err != nil {
		return nil, domainattachment.CollectionSummary{},
			fmt.Errorf("begin attachment blob collection: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	retained, err := loadRetainedAttachments(ctx, change)
	if err != nil {
		return nil, domainattachment.CollectionSummary{}, err
	}
	descriptors := make([]domainattachment.BlobDescriptor, 0, len(retained))
	observations := make(map[domainattachment.BlobDescriptor]domainattachment.BlobAvailability)
	var problems []domainattachment.IntegrityProblem
	for _, value := range retained {
		descriptors = append(descriptors, value.blob)
		availability, ok := observations[value.blob]
		if !ok {
			availability, err = r.verifyBlob(value.blob)
			if err != nil {
				return nil, domainattachment.CollectionSummary{}, err
			}
			observations[value.blob] = availability
		}
		if availability != domainattachment.BlobAvailabilityVerified {
			problems = append(problems, domainattachment.IntegrityProblem{
				BoardID: value.boardID, AttachmentID: value.attachmentID,
				Blob: value.blob, Availability: availability,
			})
		}
	}
	orphans, err := r.blobs.collectOrphanBlobs(descriptors, dryRun)
	if err != nil {
		return nil, domainattachment.CollectionSummary{}, err
	}
	return problems, orphans, nil
}

// retainedAttachment is the metadata needed to protect and inspect one blob.
type retainedAttachment struct {
	// boardID identifies the board retaining the blob.
	boardID board.ID

	// attachmentID identifies the metadata reporting an integrity problem.
	attachmentID domainattachment.ID

	// blob identifies content retained by active metadata or a tombstone.
	blob domainattachment.BlobDescriptor
}

func loadRetainedAttachments(
	ctx context.Context,
	scope query.DBTX,
) (_ []retainedAttachment, err error) {
	rows, err := query.New(scope).AttachmentListRetainedBlobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("select retained attachment blobs: %w", err)
	}
	var retained []retainedAttachment
	for _, row := range rows {
		value, err := newRetainedAttachment(
			row.BoardID,
			row.ID,
			row.BlobDigest,
			row.BlobSizeBytes,
		)
		if err != nil {
			return nil, err
		}
		retained = append(retained, value)
	}
	return retained, nil
}

func newRetainedAttachment(
	boardValue string,
	attachmentValue string,
	digestValue string,
	sizeBytes int64,
) (retainedAttachment, error) {
	boardID, err := board.NewID(boardValue)
	if err != nil {
		return retainedAttachment{}, err
	}
	attachmentID, err := domainattachment.NewID(attachmentValue)
	if err != nil {
		return retainedAttachment{}, err
	}
	digest, err := domainattachment.NewDigest(digestValue)
	if err != nil {
		return retainedAttachment{}, err
	}
	if sizeBytes < 0 {
		return retainedAttachment{}, errors.New("retained attachment blob has negative size")
	}
	blob := domainattachment.BlobDescriptor{Digest: digest, SizeBytes: uint64(sizeBytes)}
	if err := blob.Validate(); err != nil {
		return retainedAttachment{}, err
	}
	return retainedAttachment{
		boardID: boardID, attachmentID: attachmentID, blob: blob,
	}, nil
}
