package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

// Lock is one named lock row. Locks are an ad-hoc coordination
// primitive for cross-cutting work that isn't shaped like an issue
// (deploy slots, build directories, shared test databases). Per-issue
// serialization should still use the dep graph + status transitions.
//
// `expires` is a wall-clock TTL — required at acquire time so that a
// crashed holder can't wedge the lock forever. The atomic acquire path
// (LockAcquire) overwrites a row whose expires has already passed.
type Lock struct {
	bun.BaseModel `bun:"table:locks" json:"-"`

	Name     string `bun:"name,pk" json:"name"`
	Holder   string `bun:"holder,notnull" json:"holder"`
	PID      int    `bun:"pid,notnull" json:"pid"`
	Host     string `bun:"host,notnull" json:"host"`
	Acquired int64  `bun:"acquired,notnull" json:"acquired"`
	Expires  int64  `bun:"expires,notnull" json:"expires"`
}

// LockAcquire tries to take the named lock for `holder` with the given
// TTL. Returns the freshly-acquired Lock on success.
//
// If the lock is currently held (`expires > now`) by anyone else,
// returns ErrLockHeld with the current holder's record. If it's held
// by `holder` already (the same agent re-acquiring), the TTL is
// refreshed and the call returns the updated row — this is intentional
// so a long-running operation can renew without releasing.
//
// The acquire is atomic: the WHERE clause on the ON CONFLICT branch
// only overwrites rows whose `expires` has passed. Two racing callers
// will produce exactly one winner — the loser's UPDATE matches zero
// rows and re-reads the row to learn the winner's identity.
func (s *Store) LockAcquire(ctx context.Context, name, holder string, pid int, host string, ttl time.Duration) (Lock, error) {
	if name == "" {
		return Lock{}, errors.New("lock name required")
	}
	if holder == "" {
		return Lock{}, errors.New("holder required")
	}
	if ttl <= 0 {
		return Lock{}, errors.New("ttl must be > 0 — locks always have a finite TTL so crashed holders don't wedge them")
	}
	t := now()
	expires := t + int64(ttl.Seconds())
	if expires <= t { // sub-second TTL — round up
		expires = t + 1
	}

	// Single statement: INSERT, or UPDATE if the existing row is either
	// stale OR held by us already. The WHERE guards against stealing a
	// lock from a live holder.
	res, err := s.db.ExecContext(ctx, `
        INSERT INTO locks (name, holder, pid, host, acquired, expires)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT (name) DO UPDATE SET
            holder   = excluded.holder,
            pid      = excluded.pid,
            host     = excluded.host,
            acquired = excluded.acquired,
            expires  = excluded.expires
        WHERE locks.expires <= ? OR locks.holder = ?
    `, name, holder, pid, host, t, expires, t, holder)
	if err != nil {
		return Lock{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Held by someone else; re-read for the error path.
		cur, gerr := s.LockGet(ctx, name)
		if gerr != nil {
			return Lock{}, gerr
		}
		return cur, ErrLockHeld
	}
	return s.LockGet(ctx, name)
}

// LockRelease drops the named lock if `holder` is the current holder.
// Returns ErrLockNotFound if no row exists; ErrLockNotHolder if the
// lock exists but is held by someone else. A stale (expired) lock is
// treated as not-held and can be released by anyone — there's nothing
// to steal at that point.
func (s *Store) LockRelease(ctx context.Context, name, holder string) error {
	cur, err := s.LockGet(ctx, name)
	if err != nil {
		return err
	}
	if cur.Expires > now() && cur.Holder != holder {
		return ErrLockNotHolder
	}
	_, err = s.db.NewDelete().
		Model((*Lock)(nil)).
		Where("name = ?", name).
		Exec(ctx)
	return err
}

// LockGet returns the row for `name`, or ErrLockNotFound if absent.
// Returns the row regardless of whether it's expired — callers compare
// expires against now() to decide if it's still live.
func (s *Store) LockGet(ctx context.Context, name string) (Lock, error) {
	var l Lock
	err := s.db.NewSelect().Model(&l).Where("name = ?", name).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Lock{}, ErrLockNotFound
	}
	return l, err
}

// LockList returns every lock row, name-ordered. Stale (expired) rows
// are included — the CLI annotates them so the operator can see
// what's stuck. Call LockGC separately to actually remove them.
func (s *Store) LockList(ctx context.Context) ([]Lock, error) {
	var locks []Lock
	err := s.db.NewSelect().Model(&locks).OrderExpr("name ASC").Scan(ctx)
	return locks, err
}

// LockGC deletes all expired lock rows. Returns the number removed.
// Idempotent; safe to call from anywhere.
func (s *Store) LockGC(ctx context.Context) (int, error) {
	res, err := s.db.NewDelete().
		Model((*Lock)(nil)).
		Where("expires <= ?", now()).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
