package attachment

import (
	"context"
	"errors"
	"fmt"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// BeginUpload establishes durable staging before publishing upload metadata.
// It returns board.ErrArchived without creating staging when the target board
// is archived.
func (r *Repository) BeginUpload(
	ctx context.Context,
	admission domainattachment.BeginUploadAdmission,
) (_ domainattachment.Upload, err error) {
	request := admission.Request
	change, err := r.store.Change(ctx)
	if err != nil {
		return domainattachment.Upload{}, fmt.Errorf("begin attachment upload change: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	queries := query.New(change)
	if err := requireMutableBoard(ctx, queries, request.Association.BoardID()); err != nil {
		return domainattachment.Upload{}, err
	}
	if err := validateUploadTarget(ctx, queries, request.Association); err != nil {
		return domainattachment.Upload{}, err
	}
	uploadID, err := r.newUploadID()
	if err != nil {
		return domainattachment.Upload{}, err
	}
	if err := r.blobs.beginStaging(uploadID); err != nil {
		return domainattachment.Upload{}, err
	}
	now := wholeSecond(r.clock.Now())
	upload := domainattachment.Upload{
		ID:                uploadID,
		Association:       request.Association,
		Filename:          request.Filename,
		ExpectedSizeBytes: request.ExpectedSizeBytes,
		ExpectedDigest:    request.ExpectedDigest,
		Actor:             request.Invocation.Actor(),
		State:             domainattachment.UploadStateActive,
		AcceptedOffset:    0,
		MaximumSizeBytes:  admission.MaximumSizeBytes,
		ExpiresAt:         now.Add(domainattachment.StagingExpiry),
	}
	originIssueID, hasOrigin := upload.Association.OriginIssueID()
	err = queries.AttachmentInsertUpload(ctx, query.AttachmentInsertUploadParams{
		ID:                upload.ID.String(),
		BoardID:           upload.Association.BoardID().String(),
		OriginIssueID:     nullableOrigin(originIssueID.String(), hasOrigin),
		Filename:          upload.Filename.String(),
		ExpectedSizeBytes: nullableUint64(upload.ExpectedSizeBytes),
		ExpectedDigest:    nullableDigest(upload.ExpectedDigest),
		Actor:             upload.Actor,
		State:             upload.State.String(),
		AcceptedOffset:    int64(upload.AcceptedOffset),
		ExpiresAt:         upload.ExpiresAt,
		AdmittedMaxBytes:  int64(upload.MaximumSizeBytes.Uint64()),
	})
	if err != nil {
		return domainattachment.Upload{}, fmt.Errorf("insert attachment upload: %w", err)
	}
	if err := change.Commit(); err != nil {
		return domainattachment.Upload{}, fmt.Errorf("commit attachment upload begin: %w", err)
	}
	return upload, nil
}

func validateUploadTarget(
	ctx context.Context,
	queries *query.Queries,
	association domainattachment.Association,
) error {
	exists, err := queries.AttachmentTargetBoardExists(
		ctx,
		association.BoardID().String(),
	)
	if err != nil {
		return fmt.Errorf("select attachment board: %w", err)
	}
	if !exists {
		return fmt.Errorf(
			"%w: board %s",
			domainattachment.ErrAttachmentTargetNotFound,
			association.BoardID(),
		)
	}
	originIssueID, hasOrigin := association.OriginIssueID()
	if !hasOrigin {
		return nil
	}
	exists, err = queries.AttachmentTargetIssueExists(
		ctx,
		query.AttachmentTargetIssueExistsParams{
			BoardID: association.BoardID().String(),
			IssueID: originIssueID.String(),
		},
	)
	if err != nil {
		return fmt.Errorf("select attachment origin issue: %w", err)
	}
	if !exists {
		return fmt.Errorf(
			"%w: issue %s",
			domainattachment.ErrAttachmentTargetNotFound,
			originIssueID,
		)
	}
	return nil
}

func nullableOrigin(value string, present bool) *string {
	if !present {
		return nil
	}
	return &value
}

func nullableUint64(value *uint64) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	return &converted
}

func nullableDigest(value *domainattachment.Digest) *string {
	if value == nil {
		return nil
	}
	converted := value.String()
	return &converted
}
