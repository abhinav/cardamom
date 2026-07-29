package attachment

import (
	"context"
	"fmt"
	"io"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
)

// OpenCopyBlob returns a verified reader for one retained blob descriptor.
func (r *Repository) OpenCopyBlob(
	ctx context.Context,
	descriptor domainattachment.BlobDescriptor,
) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, availability, err := r.blobs.openVerified(descriptor)
	if err != nil {
		return nil, err
	}
	if availability != domainattachment.BlobAvailabilityVerified {
		return nil, fmt.Errorf(
			"attachment blob %s is %s",
			descriptor.Digest,
			availability,
		)
	}
	return reader, nil
}

// PublishCopyBlob verifies and idempotently publishes copied blob content.
func (r *Repository) PublishCopyBlob(
	ctx context.Context,
	descriptor domainattachment.BlobDescriptor,
	reader io.Reader,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.blobs.publishReader(descriptor, reader)
}
