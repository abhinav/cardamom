package attachment

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// GetUpload returns current upload progress or a terminal receipt.
func (r *Repository) GetUpload(
	ctx context.Context,
	request domainattachment.GetUploadRequest,
) (_ domainattachment.Upload, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return domainattachment.Upload{}, fmt.Errorf("begin attachment upload status: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	upload, err := r.loadUpload(ctx, change, request.UploadID)
	if err != nil {
		return domainattachment.Upload{}, err
	}
	now := wholeSecond(r.clock.Now())
	if !uploadExpired(upload, now) {
		return upload, nil
	}
	if err := expireUpload(ctx, change, &upload, now); err != nil {
		return domainattachment.Upload{}, err
	}
	if err := change.Commit(); err != nil {
		return domainattachment.Upload{}, fmt.Errorf("commit attachment upload expiry: %w", err)
	}
	return upload, r.blobs.removeStaging(upload.ID)
}

func (r *Repository) commitExpiredConflict(
	ctx context.Context,
	change *store.Change,
	upload *domainattachment.Upload,
	now time.Time,
) error {
	if err := expireUpload(ctx, change, upload, now); err != nil {
		return err
	}
	if err := change.Commit(); err != nil {
		return fmt.Errorf("commit attachment upload expiry: %w", err)
	}
	return errors.Join(
		domainattachment.ErrUploadStateConflict,
		r.blobs.removeStaging(upload.ID),
	)
}

func expireUpload(
	ctx context.Context,
	change *store.Change,
	upload *domainattachment.Upload,
	now time.Time,
) error {
	upload.State = domainattachment.UploadStateExpired
	upload.ExpiresAt = now.Add(domainattachment.TerminalReceiptRetention)
	err := query.New(change).AttachmentExpireUpload(
		ctx,
		query.AttachmentExpireUploadParams{
			ExpiresAt: upload.ExpiresAt,
			ID:        upload.ID.String(),
		},
	)
	if err != nil {
		return fmt.Errorf("expire attachment upload: %w", err)
	}
	return nil
}

func uploadExpired(upload domainattachment.Upload, now time.Time) bool {
	return upload.State == domainattachment.UploadStateActive &&
		!now.Before(upload.ExpiresAt)
}

func requireUploadActor(upload domainattachment.Upload, actor string) error {
	if upload.Actor != actor {
		return domainattachment.ErrUploadActorConflict
	}
	return nil
}

func wholeSecond(value time.Time) time.Time {
	return time.Unix(value.Unix(), 0).UTC()
}
