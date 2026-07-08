package mail

import (
	"context"
	"errors"
	"time"

	"go.abhg.dev/cardamom/internal/must"
)

// mailboxPersistence supplies the finite page operation used by the polling
// loop.
type mailboxPersistence interface {
	TailMailbox(context.Context, TailPageRequest) (TailPage, error)
}

// poller owns repeated mailbox reads and session-local delivery suppression.
type poller struct {
	repository mailboxPersistence
}

func newPoller(repository mailboxPersistence) *poller {
	must.NotBeNilf(repository, "mail persistence is required")
	return &poller{repository: repository}
}

// Batch is one completed polling read delivered to a Sink.
type Batch struct {
	// Messages contains the selected deliveries for this poll.
	Messages []Message
}

// TailCursor is an opaque repository-owned mailbox position.
// Callers may retain and return it but must not interpret its representation.
type TailCursor string

// TailPageRequest selects one finite tail page after Cursor.
type TailPageRequest struct {
	// Selection is the mailbox query applied to the page.
	Selection Selection

	// Cursor is empty for the first page and otherwise comes from a prior page.
	Cursor TailCursor
}

// TailPage is one finite mailbox page and its continuation position.
type TailPage struct {
	// Messages contains deliveries in repository order.
	Messages []Message

	// Cursor is the position to return on the next page request.
	Cursor TailCursor
}

// Sink receives one batch after its repository operation has completed.
type Sink interface {
	// DeliverMail receives the messages selected by one poll.
	DeliverMail(context.Context, Batch) error
}

type pollRequest struct {
	selection Selection
	interval  time.Duration
}

func (p *poller) poll(ctx context.Context, request pollRequest, sink Sink) error {
	if sink == nil {
		return errors.New("mail sink required")
	}
	ticker := time.NewTicker(max(request.interval, time.Second))
	defer ticker.Stop()

	var cursor TailCursor
	for {
		page, err := p.repository.TailMailbox(ctx, TailPageRequest{
			Selection: request.selection,
			Cursor:    cursor,
		})
		if err != nil {
			return err
		}
		cursor = page.Cursor
		if err := sink.DeliverMail(ctx, Batch{Messages: page.Messages}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
