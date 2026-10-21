package attachment

import (
	"context"
	"errors"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func appendAttachmentAddition(
	ctx context.Context,
	change *store.Change,
	value domainattachment.Attachment,
	originIssueUID []byte,
) error {
	var origin *issue.UID
	if originIssueUID != nil {
		parsed, err := issue.ParseUID(originIssueUID)
		if err != nil {
			return err
		}
		origin = &parsed
	}
	appender := repositoryhistory.NewAppender(
		ctx,
		change,
		value.Association.BoardID(),
	)
	if err := appender.Append(boardhistory.Event{
		OccurredAt: value.Created.At,
		Payload: &boardhistory.AttachmentAdd{
			ID: value.ID.String(), OriginIssueUID: origin,
			Digest:    value.Blob.Digest.String(),
			SizeBytes: int64(value.Blob.SizeBytes),
			Filename:  value.Filename.String(), MediaType: value.MediaType.String(),
			Actor: value.Created.Actor, At: value.Created.At,
		},
	}); err != nil {
		return err
	}
	return appender.Finish()
}

func appendAttachmentRemoval(
	ctx context.Context,
	change *store.Change,
	value domainattachment.Attachment,
) error {
	if value.Removed == nil {
		return errors.New("attachment removal attribution is required")
	}
	appender := repositoryhistory.NewAppender(
		ctx,
		change,
		value.Association.BoardID(),
	)
	if err := appender.Append(boardhistory.Event{
		OccurredAt: value.Removed.At,
		Payload: &boardhistory.AttachmentRemove{
			ID: value.ID.String(), Actor: value.Removed.Actor, At: value.Removed.At,
		},
	}); err != nil {
		return err
	}
	return appender.Finish()
}
