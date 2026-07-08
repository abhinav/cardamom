package attachment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

func newUpload(row query.AttachmentUpload) (domainattachment.Upload, *string, error) {
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
	return newAttachment(row)
}

func newAttachment(row query.Attachment) (domainattachment.Attachment, error) {
	boardID, err := board.NewID(row.BoardID)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	var attachment domainattachment.Attachment
	attachment.ID, err = domainattachment.NewID(row.ID)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	if row.OriginIssueID != nil {
		originID, err := issue.NewID(*row.OriginIssueID)
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
	attachment.Blob.Digest, err = domainattachment.NewDigest(row.BlobDigest)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.Blob.SizeBytes = uint64(row.BlobSizeBytes)
	attachment.Filename, err = domainattachment.NewFilename(row.Filename)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.MediaType, err = domainattachment.NewMediaType(row.MediaType)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.Lifecycle, err = domainattachment.NewLifecycle(row.Lifecycle)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	attachment.Created = domainattachment.Attribution{
		Actor: row.CreatedActor, At: row.CreatedAt,
		Revision: board.Revision(row.CreatedRevision),
	}
	if row.RemovedActor != nil || row.RemovedAt != nil || row.RemovedRevision != nil {
		if row.RemovedActor == nil || row.RemovedAt == nil || row.RemovedRevision == nil {
			return domainattachment.Attachment{},
				errors.New("attachment removal attribution is incomplete")
		}
		attachment.Removed = &domainattachment.Attribution{
			Actor: *row.RemovedActor, At: *row.RemovedAt,
			Revision: board.Revision(*row.RemovedRevision),
		}
	}
	return attachment, nil
}
