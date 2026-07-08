package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/lease"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// Clock supplies current time for lease expiration boundaries.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// Repository owns finite resource lease operations for one store.
type Repository struct {
	// store supplies one explicit scope for each persistence operation.
	store *store.Store // required

	// clock supplies acquisition, renewal, revocation, and expiration timestamps.
	clock Clock // required
}

// New binds lease operations to one Store and process clock.
func New(persistence *store.Store, clock Clock) *Repository {
	must.NotBeNilf(persistence, "lease Store is required")
	must.NotBeNilf(clock, "lease Clock is required")
	return &Repository{store: persistence, clock: clock}
}

// Acquire creates a lease when the named resource has no active lease.
// HeldError reports the current active lease when acquisition is unavailable.
func (r *Repository) Acquire(
	ctx context.Context,
	request lease.AcquireRequest,
) (out lease.Lease, err error) {
	if err := request.Validate(); err != nil {
		return out, errkind.Wrap(errkind.InvalidInput, err)
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	now, expiresAt := leaseWindow(r.clock.Now(), request.TTL)
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, fmt.Errorf("begin lease acquisition: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	queries := query.New(change)
	row, err := queries.LeaseAcquire(ctx, query.LeaseAcquireParams{
		Name:       request.Name,
		Owner:      request.Owner,
		AcquiredAt: now,
		ExpiresAt:  expiresAt,
	})
	if errors.Is(err, sql.ErrNoRows) {
		current, found, readErr := activeLease(
			ctx,
			queries,
			request.Name,
			now,
		)
		if readErr != nil {
			return lease.Lease{}, fmt.Errorf(
				"read conflicting lease %q: %w",
				request.Name,
				readErr,
			)
		}
		if !found {
			return lease.Lease{}, fmt.Errorf(
				"acquire lease %q: conditional write returned no lease",
				request.Name,
			)
		}
		return lease.Lease{}, held(current)
	}
	if err != nil {
		return lease.Lease{}, fmt.Errorf("write lease %q: %w", request.Name, err)
	}
	out = loadLease(row)
	if err := change.Commit(); err != nil {
		return lease.Lease{}, fmt.Errorf("commit lease acquisition: %w", err)
	}
	return out, nil
}

// Renew extends an active lease held by the requested owner.
// HeldError reports an active lease held by a different owner.
func (r *Repository) Renew(
	ctx context.Context,
	request lease.RenewRequest,
) (out lease.Lease, err error) {
	if err := request.Validate(); err != nil {
		return out, errkind.Wrap(errkind.InvalidInput, err)
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	now, expiresAt := leaseWindow(r.clock.Now(), request.TTL)
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, fmt.Errorf("begin lease renewal: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	queries := query.New(change)
	row, err := queries.LeaseRenew(ctx, query.LeaseRenewParams{
		ExpiresAt: expiresAt,
		Name:      request.Name,
		Owner:     request.Owner,
		Now:       now,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return lease.Lease{}, ownerGuardFailure(
			ctx,
			queries,
			request.Name,
			request.Owner,
			now,
		)
	}
	if err != nil {
		return lease.Lease{}, fmt.Errorf("renew lease %q: %w", request.Name, err)
	}
	out = loadLease(row)
	if err := change.Commit(); err != nil {
		return lease.Lease{}, fmt.Errorf("commit lease renewal: %w", err)
	}
	return out, nil
}

// Release removes an active lease held by the requested owner.
// HeldError reports an active lease held by a different owner.
func (r *Repository) Release(
	ctx context.Context,
	request lease.ReleaseRequest,
) (out lease.Lease, err error) {
	if err := request.Validate(); err != nil {
		return out, errkind.Wrap(errkind.InvalidInput, err)
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	now := wholeSecond(r.clock.Now())
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, fmt.Errorf("begin lease release: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	queries := query.New(change)
	row, err := queries.LeaseRelease(ctx, query.LeaseReleaseParams{
		Name:  request.Name,
		Owner: request.Owner,
		Now:   now,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return lease.Lease{}, ownerGuardFailure(
			ctx,
			queries,
			request.Name,
			request.Owner,
			now,
		)
	}
	if err != nil {
		return lease.Lease{}, fmt.Errorf("release lease %q: %w", request.Name, err)
	}
	out = loadLease(row)
	if err := change.Commit(); err != nil {
		return lease.Lease{}, fmt.Errorf("commit lease release: %w", err)
	}
	return out, nil
}

// Revoke removes an active lease held by the expected owner and reports the
// transient coordinator recovery context.
// HeldError reports an active lease held by a different owner.
func (r *Repository) Revoke(
	ctx context.Context,
	request lease.RevokeRequest,
) (out lease.Revocation, err error) {
	if err := request.Validate(); err != nil {
		return out, errkind.Wrap(errkind.InvalidInput, err)
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	now := wholeSecond(r.clock.Now())
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, fmt.Errorf("begin lease revocation: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	queries := query.New(change)
	row, err := queries.LeaseRevoke(ctx, query.LeaseRevokeParams{
		Name: request.Name, Owner: request.Owner, Now: now,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return lease.Revocation{}, ownerGuardFailure(
			ctx,
			queries,
			request.Name,
			request.Owner,
			now,
		)
	}
	if err != nil {
		return lease.Revocation{}, fmt.Errorf("revoke lease %q: %w", request.Name, err)
	}
	out = lease.Revocation{
		Lease: loadLease(row), RevokedBy: request.RevokedBy,
		Reason: request.Reason, RevokedAt: now,
	}
	if err := change.Commit(); err != nil {
		return lease.Revocation{}, fmt.Errorf("commit lease revocation: %w", err)
	}
	return out, nil
}

// Get returns one active resource lease.
func (r *Repository) Get(
	ctx context.Context,
	request lease.GetRequest,
) (out lease.Lease, err error) {
	if err := request.Validate(); err != nil {
		return out, errkind.Wrap(errkind.InvalidInput, err)
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	view, err := r.store.View(ctx)
	if err != nil {
		return out, fmt.Errorf("begin lease read: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	out, found, err := activeLease(
		ctx,
		query.New(view),
		request.Name,
		wholeSecond(r.clock.Now()),
	)
	if err != nil {
		return lease.Lease{}, fmt.Errorf("read lease %q: %w", request.Name, err)
	}
	if !found {
		return lease.Lease{}, errkind.Wrap(errkind.NotFound, lease.ErrNotFound)
	}
	return out, nil
}

// List returns every active resource lease ordered by resource name.
func (r *Repository) List(
	ctx context.Context,
) (out []lease.Lease, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin lease list: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	rows, err := query.New(view).LeaseListActive(
		ctx,
		wholeSecond(r.clock.Now()),
	)
	if err != nil {
		return nil, fmt.Errorf("query active leases: %w", err)
	}

	out = make([]lease.Lease, 0, len(rows))
	for _, row := range rows {
		out = append(out, loadLease(row))
	}
	return out, nil
}

func activeLease(
	ctx context.Context,
	queries *query.Queries,
	name string,
	now time.Time,
) (lease.Lease, bool, error) {
	row, err := queries.LeaseGetActive(ctx, query.LeaseGetActiveParams{
		Name: name,
		Now:  now,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return lease.Lease{}, false, nil
	}
	if err != nil {
		return lease.Lease{}, false, err
	}
	return loadLease(row), true, nil
}

func ownerGuardFailure(
	ctx context.Context,
	queries *query.Queries,
	name string,
	owner string,
	now time.Time,
) error {
	current, found, err := activeLease(ctx, queries, name, now)
	if err != nil {
		return fmt.Errorf("read active lease %q: %w", name, err)
	}
	if !found {
		return errkind.Wrap(errkind.NotFound, lease.ErrNotFound)
	}
	if current.Owner != owner {
		return held(current)
	}
	return fmt.Errorf("update active lease %q: conditional write returned no lease", name)
}

func held(current lease.Lease) error {
	return errkind.Wrap(errkind.Conflict, &lease.HeldError{Current: current})
}

func loadLease(row query.Lease) lease.Lease {
	return lease.Lease{
		Name:       row.Name,
		Owner:      row.Owner,
		AcquiredAt: row.AcquiredAt,
		ExpiresAt:  row.ExpiresAt,
	}
}

func leaseWindow(now time.Time, ttl time.Duration) (time.Time, time.Time) {
	now = wholeSecond(now)
	expiresAt := now.Add(ttl)
	if !expiresAt.After(now) || expiresAt.Unix() <= now.Unix() {
		return now, now.Add(time.Second)
	}
	return now, wholeSecond(expiresAt)
}

func wholeSecond(value time.Time) time.Time {
	return time.Unix(value.Unix(), 0).UTC()
}
