package attachment

import (
	"context"
	"errors"
	"fmt"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// RemoveAttachment creates one permanent tombstone or returns the existing one.
func (r *Repository) RemoveAttachment(
	ctx context.Context,
	request domainattachment.RemoveRequest,
) (_ domainattachment.Attachment, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("begin attachment removal: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	value, err := r.loadAttachment(ctx, change, request.BoardID, request.AttachmentID)
	if err != nil {
		return domainattachment.Attachment{}, err
	}
	if value.Lifecycle == domainattachment.LifecycleRemoved {
		return value, nil
	}

	now := wholeSecond(r.clock.Now())
	revision, err := r.reserveAttachmentRevision(ctx, change, value.Association)
	if err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("reserve attachment removal revision: %w", err)
	}
	value.Lifecycle = domainattachment.LifecycleRemoved
	value.Removed = &domainattachment.Attribution{
		Actor: request.Invocation.Actor(), At: now,
		Revision: board.Revision(revision.reservation.Revision()),
	}
	removedActor := value.Removed.Actor
	removedAt := value.Removed.At
	removedRevision := int64(value.Removed.Revision)
	if err := query.New(change).AttachmentTombstoneMetadata(
		ctx,
		query.AttachmentTombstoneMetadataParams{
			RemovedActor:    &removedActor,
			RemovedAt:       &removedAt,
			RemovedRevision: &removedRevision,
			BoardID:         request.BoardID.String(),
			ID:              request.AttachmentID.String(),
		},
	); err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("tombstone attachment: %w", err)
	}
	if err := r.commitAttachmentRevision(ctx, change, revision); err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("commit attachment removal: %w", err)
	}
	return value, nil
}
