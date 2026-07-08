package attachment

import (
	"context"
	"errors"
	"fmt"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// attachmentRevision binds a store successor to the projection rows that must
// publish it atomically.
type attachmentRevision struct {
	// reservation is the global store successor reserved for the mutation.
	reservation store.RevisionReservation
	// association identifies the board and optional issue to advance.
	association domainattachment.Association
	// board is the board revision observed before publication.
	board int64
}

func (r *Repository) reserveAttachmentRevision(
	ctx context.Context,
	change *store.Change,
	association domainattachment.Association,
) (attachmentRevision, error) {
	currentBoard, err := query.New(change).AttachmentGetBoardRevision(
		ctx,
		association.BoardID().String(),
	)
	if err != nil {
		return attachmentRevision{}, fmt.Errorf("read board revision: %w", err)
	}
	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return attachmentRevision{}, err
	}
	return attachmentRevision{
		reservation: reservation, association: association, board: currentBoard,
	}, nil
}

func (r *Repository) commitAttachmentRevision(
	ctx context.Context,
	change *store.Change,
	revision attachmentRevision,
) error {
	next := revision.reservation.Revision()
	queries := query.New(change)
	if originIssueID, ok := revision.association.OriginIssueID(); ok {
		result, err := queries.AttachmentPublishIssueRevision(
			ctx,
			query.AttachmentPublishIssueRevisionParams{
				Revision: next,
				BoardID:  revision.association.BoardID().String(),
				IssueID:  originIssueID.String(),
			},
		)
		if err != nil {
			return fmt.Errorf("publish attachment issue revision: %w", err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return errors.New("publish attachment issue revision: row not found")
		}
	}
	result, err := queries.AttachmentPublishBoardRevision(
		ctx,
		query.AttachmentPublishBoardRevisionParams{
			Revision:         next,
			BoardID:          revision.association.BoardID().String(),
			PreviousRevision: revision.board,
		},
	)
	if err != nil {
		return fmt.Errorf("publish attachment board revision: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("attachment board revision changed")
	}
	if err := change.PublishRevision(ctx, revision.reservation); err != nil {
		return fmt.Errorf("publish attachment store revision: %w", err)
	}
	return change.Commit()
}
