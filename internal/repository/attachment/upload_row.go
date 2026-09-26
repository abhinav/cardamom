package attachment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func (r *Repository) loadUpload(
	ctx context.Context,
	scope query.DBTX,
	uploadID domainattachment.UploadID,
) (domainattachment.Upload, error) {
	row, err := query.New(scope).AttachmentGetUpload(ctx, uploadID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domainattachment.Upload{}, domainattachment.ErrUploadNotFound
		}
		return domainattachment.Upload{}, fmt.Errorf("select attachment upload: %w", err)
	}
	upload, attachmentID, err := newUpload(row)
	if err != nil {
		return domainattachment.Upload{}, err
	}
	if upload.State != domainattachment.UploadStateCommitted {
		return upload, nil
	}
	if attachmentID == nil {
		return domainattachment.Upload{}, errors.New("committed attachment upload has no attachment")
	}
	id, err := domainattachment.NewID(*attachmentID)
	if err != nil {
		return domainattachment.Upload{}, err
	}
	attachment, err := r.loadAttachment(ctx, scope, upload.Association.BoardID(), id)
	if err != nil {
		return domainattachment.Upload{}, err
	}
	upload.Attachment = &attachment
	return upload, nil
}

func newUpload(row query.AttachmentGetUploadRow) (domainattachment.Upload, *string, error) {
	parsedBoardID, err := board.NewID(row.BoardID)
	if err != nil {
		return domainattachment.Upload{}, nil, err
	}
	var upload domainattachment.Upload
	upload.ID, err = domainattachment.NewUploadID(row.ID)
	if err != nil {
		return domainattachment.Upload{}, nil, err
	}
	if row.OriginIssueID != nil {
		parsedIssueID, parseErr := issue.NewID(*row.OriginIssueID)
		if parseErr != nil {
			return domainattachment.Upload{}, nil, parseErr
		}
		upload.Association, err = domainattachment.NewIssueAssociation(
			parsedBoardID,
			parsedIssueID,
		)
	} else {
		upload.Association, err = domainattachment.NewBoardAssociation(parsedBoardID)
	}
	if err != nil {
		return domainattachment.Upload{}, nil, err
	}
	upload.Filename, err = domainattachment.NewFilename(row.Filename)
	if err != nil {
		return domainattachment.Upload{}, nil, err
	}
	upload.State, err = domainattachment.NewUploadState(row.State)
	if err != nil {
		return domainattachment.Upload{}, nil, err
	}
	if row.ExpectedSizeBytes != nil {
		value := uint64(*row.ExpectedSizeBytes)
		upload.ExpectedSizeBytes = &value
	}
	if row.ExpectedDigest != nil {
		value, err := domainattachment.NewDigest(*row.ExpectedDigest)
		if err != nil {
			return domainattachment.Upload{}, nil, err
		}
		upload.ExpectedDigest = &value
	}
	upload.Actor = row.Actor
	upload.AcceptedOffset = uint64(row.AcceptedOffset)
	upload.ExpiresAt = row.ExpiresAt
	upload.MaximumSizeBytes, err = configuration.NewByteLimit(uint64(row.AdmittedMaxBytes))
	if err != nil {
		return domainattachment.Upload{}, nil, err
	}
	return upload, row.AttachmentID, nil
}

func (r *Repository) loadAttachment(
	ctx context.Context,
	scope query.DBTX,
	boardID board.ID,
	attachmentID domainattachment.ID,
) (domainattachment.Attachment, error) {
	attachment, err := selectAttachment(ctx, scope, boardID, attachmentID)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.Availability, err = r.blobs.inspect(attachment.Blob)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	return attachment, nil
}

func selectAttachment(
	ctx context.Context,
	scope query.DBTX,
	boardID board.ID,
	attachmentID domainattachment.ID,
) (domainattachment.Attachment, error) {
	row, err := query.New(scope).AttachmentGetMetadata(
		ctx,
		query.AttachmentGetMetadataParams{
			BoardID: boardID.String(),
			ID:      attachmentID.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domainattachment.Attachment{}, domainattachment.ErrAttachmentNotFound
	}
	if err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("select committed attachment: %w", err)
	}
	return newAttachment(attachmentRowFromGet(row))
}

// attachmentRow normalizes sqlc read shapes after they join a private origin
// UID back to the public issue ID required by the attachment domain.
type attachmentRow struct {
	boardID         string
	id              string
	originIssueID   *string
	blobDigest      string
	blobSizeBytes   int64
	filename        string
	mediaType       string
	lifecycle       string
	createdActor    string
	createdAt       time.Time
	createdRevision int64
	removedActor    *string
	removedAt       *time.Time
	removedRevision *int64
}

func attachmentRowFromGet(row query.AttachmentGetMetadataRow) attachmentRow {
	return attachmentRow{
		boardID: row.BoardID, id: row.ID, originIssueID: row.OriginIssueID,
		blobDigest: row.BlobDigest, blobSizeBytes: row.BlobSizeBytes,
		filename: row.Filename, mediaType: row.MediaType, lifecycle: row.Lifecycle,
		createdActor: row.CreatedActor, createdAt: row.CreatedAt,
		createdRevision: row.CreatedRevision, removedActor: row.RemovedActor,
		removedAt: row.RemovedAt, removedRevision: row.RemovedRevision,
	}
}

func attachmentRowFromList(row query.AttachmentListMetadataRow) attachmentRow {
	return attachmentRow{
		boardID: row.BoardID, id: row.ID, originIssueID: row.OriginIssueID,
		blobDigest: row.BlobDigest, blobSizeBytes: row.BlobSizeBytes,
		filename: row.Filename, mediaType: row.MediaType, lifecycle: row.Lifecycle,
		createdActor: row.CreatedActor, createdAt: row.CreatedAt,
		createdRevision: row.CreatedRevision, removedActor: row.RemovedActor,
		removedAt: row.RemovedAt, removedRevision: row.RemovedRevision,
	}
}

func attachmentRowFromResolution(row query.AttachmentResolveMetadataRow) attachmentRow {
	return attachmentRow{
		boardID: row.BoardID, id: row.ID, originIssueID: row.OriginIssueID,
		blobDigest: row.BlobDigest, blobSizeBytes: row.BlobSizeBytes,
		filename: row.Filename, mediaType: row.MediaType, lifecycle: row.Lifecycle,
		createdActor: row.CreatedActor, createdAt: row.CreatedAt,
		createdRevision: row.CreatedRevision, removedActor: row.RemovedActor,
		removedAt: row.RemovedAt, removedRevision: row.RemovedRevision,
	}
}

func newAttachment(row attachmentRow) (domainattachment.Attachment, error) {
	boardID, err := board.NewID(row.boardID)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	var attachment domainattachment.Attachment
	attachment.ID, err = domainattachment.NewID(row.id)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	if row.originIssueID != nil {
		originID, err := issue.NewID(*row.originIssueID)
		if err != nil {
			return domainattachment.Attachment{}, err
		}
		attachment.Association, err = domainattachment.NewIssueAssociation(boardID, originID)
		if err != nil {
			return domainattachment.Attachment{}, err
		}
	} else {
		attachment.Association, err = domainattachment.NewBoardAssociation(boardID)
		if err != nil {
			return domainattachment.Attachment{}, err
		}
	}
	attachment.Blob.Digest, err = domainattachment.NewDigest(row.blobDigest)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.Blob.SizeBytes = uint64(row.blobSizeBytes)
	attachment.Filename, err = domainattachment.NewFilename(row.filename)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.MediaType, err = domainattachment.NewMediaType(row.mediaType)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.Lifecycle, err = domainattachment.NewLifecycle(row.lifecycle)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.Created = domainattachment.Attribution{
		Actor: row.createdActor, At: row.createdAt,
		Revision: board.Revision(row.createdRevision),
	}
	if row.removedActor != nil || row.removedAt != nil || row.removedRevision != nil {
		if row.removedActor == nil || row.removedAt == nil || row.removedRevision == nil {
			return domainattachment.Attachment{},
				errors.New("attachment removal attribution is incomplete")
		}
		attachment.Removed = &domainattachment.Attribution{
			Actor: *row.removedActor, At: *row.removedAt,
			Revision: board.Revision(*row.removedRevision),
		}
	}
	return attachment, nil
}
