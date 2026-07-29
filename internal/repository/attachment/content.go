package attachment

import (
	"context"
	"errors"
	"fmt"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ResolveAttachments returns input-aligned board-scoped reference results from
// one bounded metadata query.
func (r *Repository) ResolveAttachments(
	ctx context.Context,
	request domainattachment.ResolveRequest,
) (_ []domainattachment.Resolution, err error) {
	if len(request.AttachmentIDs) == 0 {
		return []domainattachment.Resolution{}, nil
	}

	view, err := r.store.View(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin attachment resolution: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	attachmentIDs := make([]string, len(request.AttachmentIDs))
	for index, id := range request.AttachmentIDs {
		attachmentIDs[index] = id.String()
	}
	rows, err := query.New(view).AttachmentResolveMetadata(
		ctx,
		query.AttachmentResolveMetadataParams{
			BoardID:       request.BoardID.String(),
			AttachmentIDs: attachmentIDs,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("select attachment resolutions: %w", err)
	}

	attachments := make(map[domainattachment.ID]domainattachment.Attachment)
	for _, row := range rows {
		value, err := newAttachment(row)
		if err != nil {
			return nil, err
		}
		value.Availability, err = r.blobs.inspect(value.Blob)
		if err != nil {
			return nil, err
		}
		attachments[value.ID] = value
	}

	resolutions := make([]domainattachment.Resolution, 0, len(request.AttachmentIDs))
	for _, id := range request.AttachmentIDs {
		resolution := domainattachment.Resolution{
			AttachmentID: id,
			State:        domainattachment.ResolutionUnknown,
		}
		value, found := attachments[id]
		if found {
			resolution.Attachment = &value
			resolution.State = domainattachment.ResolutionActive
			if value.Lifecycle == domainattachment.LifecycleRemoved {
				resolution.State = domainattachment.ResolutionRemoved
			}
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

// OpenAttachmentContent returns active attachment metadata and a fully
// verified, rewound content handle. The caller owns closing the handle.
func (r *Repository) OpenAttachmentContent(
	ctx context.Context,
	request domainattachment.OpenContentRequest,
) (domainattachment.OpenedContent, error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return domainattachment.OpenedContent{}, fmt.Errorf(
			"begin attachment content read: %w",
			err,
		)
	}
	value, selectErr := selectAttachment(
		ctx,
		view,
		request.BoardID,
		request.AttachmentID,
	)
	err = errors.Join(selectErr, view.Done())
	if err != nil {
		return domainattachment.OpenedContent{}, err
	}
	if value.Lifecycle == domainattachment.LifecycleRemoved {
		return domainattachment.OpenedContent{}, fmt.Errorf(
			"%w: %s",
			domainattachment.ErrAttachmentRemoved,
			value.ID,
		)
	}

	handle, availability, err := r.blobs.openVerified(value.Blob)
	if err != nil {
		return domainattachment.OpenedContent{}, err
	}
	switch availability {
	case domainattachment.BlobAvailabilityMissing:
		return domainattachment.OpenedContent{}, fmt.Errorf(
			"%w: %s",
			domainattachment.ErrAttachmentContentMissing,
			value.ID,
		)
	case domainattachment.BlobAvailabilitySizeMismatch:
		return domainattachment.OpenedContent{}, fmt.Errorf(
			"%w: %s",
			domainattachment.ErrAttachmentContentSizeMismatch,
			value.ID,
		)
	case domainattachment.BlobAvailabilityDigestMismatch:
		return domainattachment.OpenedContent{}, fmt.Errorf(
			"%w: %s",
			domainattachment.ErrAttachmentContentDigestMismatch,
			value.ID,
		)
	case domainattachment.BlobAvailabilityVerified:
		value.Availability = availability
		return domainattachment.OpenedContent{
			Attachment: value,
			Handle:     handle,
		}, nil
	default:
		return domainattachment.OpenedContent{}, fmt.Errorf(
			"open attachment content returned availability %q",
			availability,
		)
	}
}
