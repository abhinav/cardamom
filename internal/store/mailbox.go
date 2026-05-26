package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Mailbox is one row in the inter-agent messaging table — a single
// fire-and-forget "ping" from sender to recipient. Distinct from
// issue comments: ephemeral (TTL'd), addressed to an agent not a
// work item, doesn't accumulate in the work log.
//
// `read_at` is NULL until the recipient consumes the ping via
// `clu inbox` (or explicitly with PingMarkRead). Expired rows are
// purged opportunistically on every read/write path via PingGC so
// the table stays bounded without a cron.
type Mailbox struct {
	bun.BaseModel `bun:"table:mailbox" json:"-"`

	ID        int64  `bun:"id,pk,autoincrement" json:"id"`
	Sender    string `bun:"sender,notnull" json:"sender"`
	Recipient string `bun:"recipient,notnull" json:"recipient"`
	Body      string `bun:"body,notnull" json:"body"`
	Created   int64  `bun:"created,notnull" json:"created"`
	Expires   int64  `bun:"expires,notnull" json:"expires"`
	ReadAt    *int64 `bun:"read_at" json:"read_at,omitempty"`
}

// PingMaxTTL bounds how long an unread ping can sit in the mailbox.
// Beyond this, the row is purged regardless of read state — protects
// against a never-checking agent inflating the table indefinitely.
const PingMaxTTL = 30 * 24 * time.Hour

// PingSend writes a new mailbox row and returns it. Sender and
// recipient must be non-empty. ttl is capped at PingMaxTTL; ttl <= 0
// uses a 7-day default.
func (s *Store) PingSend(ctx context.Context, sender, recipient, body string, ttl time.Duration) (Mailbox, error) {
	if sender == "" {
		return Mailbox{}, fmt.Errorf("%w: sender required", ErrInvalid)
	}
	if recipient == "" {
		return Mailbox{}, fmt.Errorf("%w: recipient required", ErrInvalid)
	}
	if body == "" {
		return Mailbox{}, fmt.Errorf("%w: body required", ErrInvalid)
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	if ttl > PingMaxTTL {
		ttl = PingMaxTTL
	}
	t := now()
	expires := t + int64(ttl.Seconds())
	m := Mailbox{Sender: sender, Recipient: recipient, Body: body, Created: t, Expires: expires}
	if _, err := s.db.NewInsert().Model(&m).Returning("*").Exec(ctx); err != nil {
		return Mailbox{}, err
	}
	// Opportunistic GC. Errors here are non-fatal; the send already
	// happened.
	_, _ = s.PingGC(ctx)
	return m, nil
}

// Inbox returns mailbox rows for `recipient`, newest first. Filters:
//
//	includeRead  false = unread only (read_at IS NULL); true = all
//	since        only rows with created >= since (0 = no floor)
//	limit        cap (0 = no cap; recommended >0 for human display)
//
// Expired rows are excluded regardless of includeRead. Opportunistic
// GC runs on entry.
func (s *Store) Inbox(ctx context.Context, recipient string, includeRead bool, since int64, limit int) ([]Mailbox, error) {
	if recipient == "" {
		return nil, fmt.Errorf("%w: recipient required", ErrInvalid)
	}
	_, _ = s.PingGC(ctx)
	t := now()
	q := s.db.NewSelect().Model((*Mailbox)(nil)).
		Where("recipient = ?", recipient).
		Where("expires > ?", t).
		OrderExpr("created DESC")
	if !includeRead {
		q = q.Where("read_at IS NULL")
	}
	if since > 0 {
		q = q.Where("created >= ?", since)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []Mailbox
	if err := q.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// InboxUnreadCount returns the number of unread, non-expired pings for
// `recipient`. Used by `clu brief` to surface "you have N waiting" at
// session start without listing the bodies.
func (s *Store) InboxUnreadCount(ctx context.Context, recipient string) (int, error) {
	if recipient == "" {
		return 0, nil
	}
	return s.db.NewSelect().Model((*Mailbox)(nil)).
		Where("recipient = ?", recipient).
		Where("read_at IS NULL").
		Where("expires > ?", now()).
		Count(ctx)
}

// PingMarkRead stamps read_at on the given mailbox IDs (only those
// addressed to `recipient` — guards against marking someone else's
// mail read by ID guess). Returns the number actually marked.
// Idempotent: already-read rows are left alone.
func (s *Store) PingMarkRead(ctx context.Context, recipient string, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	t := now()
	res, err := s.db.NewUpdate().
		Model((*Mailbox)(nil)).
		Set("read_at = ?", t).
		Where("id IN (?)", bun.In(ids)).
		Where("recipient = ?", recipient).
		Where("read_at IS NULL").
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// InboxClear marks every unread, non-expired ping for `recipient` as
// read. Used by `clu inbox clear` to dismiss without listing. Returns
// the number marked.
func (s *Store) InboxClear(ctx context.Context, recipient string) (int, error) {
	if recipient == "" {
		return 0, errors.New("recipient required")
	}
	t := now()
	res, err := s.db.NewUpdate().
		Model((*Mailbox)(nil)).
		Set("read_at = ?", t).
		Where("recipient = ?", recipient).
		Where("read_at IS NULL").
		Where("expires > ?", t).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PingGC deletes expired mailbox rows. Returns the number removed.
// Called opportunistically from PingSend, Inbox, and InboxUnreadCount
// so the table stays bounded without a separate cron job.
func (s *Store) PingGC(ctx context.Context) (int, error) {
	res, err := s.db.NewDelete().
		Model((*Mailbox)(nil)).
		Where("expires <= ?", now()).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
