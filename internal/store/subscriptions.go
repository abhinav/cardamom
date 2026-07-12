package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// SubscriptionMaxTTL caps how long any single subscription lives without
// being refreshed. Mirrors the lock TTL cap — keeps the table from
// accumulating zombie rows when a listener crashes without cleanup.
const SubscriptionMaxTTL = 7 * 24 * time.Hour

// SubscriptionUpsert registers (or refreshes) one listener's interest
// in `pattern`. The UNIQUE (listener, pattern) constraint makes this
// idempotent — repeated calls bump `expires` instead of inserting
// duplicate rows, so watch/tail loops can refresh every tick cheaply.
//
// `ttl` is required and capped at SubscriptionMaxTTL.
func (s *Store) SubscriptionUpsert(ctx context.Context, listener, pattern string, pid int, host string, ttl time.Duration) (Subscription, error) {
	if listener == "" {
		return Subscription{}, fmt.Errorf("%w: listener required", ErrInvalid)
	}
	if pattern == "" {
		return Subscription{}, fmt.Errorf("%w: pattern required", ErrInvalid)
	}
	if ttl <= 0 {
		return Subscription{}, fmt.Errorf("%w: ttl must be > 0", ErrInvalid)
	}
	if ttl > SubscriptionMaxTTL {
		ttl = SubscriptionMaxTTL
	}
	t := now()
	exp := t + int64(ttl.Seconds())
	row := Subscription{
		Listener: listener, Pattern: pattern,
		PID: pid, Host: host,
		Created: t, Expires: exp,
	}
	_, err := s.db.NewInsert().
		Model(&row).
		On("CONFLICT (listener, pattern) DO UPDATE").
		Set("expires = EXCLUDED.expires").
		Set("pid = EXCLUDED.pid").
		Set("host = EXCLUDED.host").
		Exec(ctx)
	if err != nil {
		return Subscription{}, err
	}
	// On upsert-update we don't get the existing id back from RETURNING
	// portably; re-select so callers get the canonical row.
	var out Subscription
	if err := s.db.NewSelect().
		Model(&out).
		Where("listener = ? AND pattern = ?", listener, pattern).
		Scan(ctx); err != nil {
		return Subscription{}, err
	}
	return out, nil
}

// SubscriptionRemove drops the (listener, pattern) row if present.
// Returns ErrSubscriptionNotFound if no such subscription exists, so
// `clu inbox --no-topic` can distinguish "removed" from "wasn't
// subscribed."
func (s *Store) SubscriptionRemove(ctx context.Context, listener, pattern string) error {
	res, err := s.db.NewDelete().
		Model((*Subscription)(nil)).
		Where("listener = ? AND pattern = ?", listener, pattern).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

// SubscriptionList returns every non-expired subscription, optionally
// scoped to one listener. Ordered by listener then pattern for stable
// output. Runs opportunistic GC on entry.
func (s *Store) SubscriptionList(ctx context.Context, listener string) ([]Subscription, error) {
	_, _ = s.SubscriptionGC(ctx)
	q := s.db.NewSelect().Model((*Subscription)(nil)).
		Where("expires > ?", now()).
		OrderExpr("listener ASC, pattern ASC")
	if listener != "" {
		q = q.Where("listener = ?", listener)
	}
	var out []Subscription
	if err := q.Scan(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SubscriptionMatching returns every non-expired subscription whose
// pattern matches `recipient`. Used by PingSend to fan out delivery.
// Pattern matching uses filepath.Match (same glob engine as the rest
// of clu) — `*` matches any sequence of non-separator characters and
// `.` is literal, so `release.*` matches `release.urgent` but not
// `release.urgent.deploy`.
func (s *Store) SubscriptionMatching(ctx context.Context, recipient string) ([]Subscription, error) {
	subs, err := s.SubscriptionList(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []Subscription
	for _, sub := range subs {
		ok, _ := filepath.Match(sub.Pattern, recipient)
		if ok {
			out = append(out, sub)
		}
	}
	return out, nil
}

// SubscriptionGC deletes expired subscription rows. Called
// opportunistically from List/Matching so the table self-cleans
// without a background job.
func (s *Store) SubscriptionGC(ctx context.Context) (int, error) {
	res, err := s.db.NewDelete().
		Model((*Subscription)(nil)).
		Where("expires <= ?", now()).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ErrSubscriptionNotFound is returned by SubscriptionRemove when the
// (listener, pattern) tuple isn't in the table.
var ErrSubscriptionNotFound = errors.New("subscription not found")
