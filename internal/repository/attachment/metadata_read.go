package attachment

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

const (
	defaultAttachmentPageSize = 100
	maxAttachmentPageSize     = 1000
)

// GetAttachment returns one board-scoped attachment with local availability.
func (r *Repository) GetAttachment(
	ctx context.Context,
	request domainattachment.GetRequest,
) (_ domainattachment.Attachment, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return domainattachment.Attachment{}, fmt.Errorf("begin attachment read: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	return r.loadAttachment(ctx, view, request.BoardID, request.AttachmentID)
}

// ListAttachments returns one ID-ordered board-scoped attachment page.
func (r *Repository) ListAttachments(
	ctx context.Context,
	request domainattachment.ListRequest,
) (_ domainattachment.Page, err error) {
	afterID, err := decodeAttachmentPageToken(request.PageToken)
	if err != nil {
		return domainattachment.Page{}, err
	}
	pageSize := request.PageSize
	if pageSize == 0 {
		pageSize = defaultAttachmentPageSize
	}
	if pageSize > maxAttachmentPageSize {
		pageSize = maxAttachmentPageSize
	}

	view, err := r.store.View(ctx)
	if err != nil {
		return domainattachment.Page{}, fmt.Errorf("begin attachment list: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	originIssueID, hasOrigin := request.OriginIssueID, request.OriginIssueID != nil
	rows, err := query.New(view).AttachmentListMetadata(
		ctx,
		query.AttachmentListMetadataParams{
			BoardID:        request.BoardID.String(),
			AfterID:        afterID.String(),
			IncludeRemoved: request.IncludeRemoved,
			HasOriginIssue: hasOrigin,
			OriginIssueID:  nullableListOrigin(originIssueID),
			ResultLimit:    int64(pageSize) + 1,
		},
	)
	if err != nil {
		return domainattachment.Page{}, fmt.Errorf("select attachment page: %w", err)
	}

	attachments := make([]domainattachment.Attachment, 0, pageSize)
	for _, row := range rows {
		value, err := newAttachment(row)
		if err != nil {
			return domainattachment.Page{}, err
		}
		value.Availability, err = r.blobs.inspect(value.Blob)
		if err != nil {
			return domainattachment.Page{}, err
		}
		attachments = append(attachments, value)
	}

	var nextPageToken string
	if len(attachments) > int(pageSize) {
		nextPageToken = encodeAttachmentPageToken(attachments[pageSize-1].ID)
		attachments = attachments[:pageSize]
	}
	return domainattachment.Page{
		Attachments: attachments, NextPageToken: nextPageToken,
	}, nil
}

func decodeAttachmentPageToken(token string) (domainattachment.ID, error) {
	if token == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domainattachment.ErrAttachmentPageTokenInvalid, err)
	}
	id, err := domainattachment.NewID(string(decoded))
	if err != nil {
		return "", fmt.Errorf("%w: %w", domainattachment.ErrAttachmentPageTokenInvalid, err)
	}
	return id, nil
}

func encodeAttachmentPageToken(id domainattachment.ID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id.String()))
}

func nullableListOrigin(originIssueID *issue.ID) *string {
	if originIssueID == nil {
		return nil
	}
	value := originIssueID.String()
	return &value
}
