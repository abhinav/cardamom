package attachment

import (
	"context"
	"errors"
	"fmt"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
)

// VerifyAttachment returns one complete local content observation.
func (r *Repository) VerifyAttachment(
	ctx context.Context,
	request domainattachment.VerifyRequest,
) (_ domainattachment.Verification, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return domainattachment.Verification{}, fmt.Errorf("begin attachment verification: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	value, err := r.loadAttachment(ctx, view, request.BoardID, request.AttachmentID)
	if err != nil {
		return domainattachment.Verification{}, err
	}
	availability, err := r.verifyBlob(value.Blob)
	if err != nil {
		return domainattachment.Verification{}, err
	}
	return domainattachment.Verification{
		AttachmentID: value.ID,
		Blob:         value.Blob,
		Availability: availability,
		ObservedAt:   wholeSecond(r.clock.Now()),
	}, nil
}

func (r *Repository) verifyBlob(
	descriptor domainattachment.BlobDescriptor,
) (domainattachment.BlobAvailability, error) {
	reader, availability, err := r.blobs.openVerified(descriptor)
	if err != nil {
		return 0, err
	}
	if reader != nil {
		if err := reader.Close(); err != nil {
			return 0, fmt.Errorf("close verified attachment blob: %w", err)
		}
	}
	return availability, nil
}
