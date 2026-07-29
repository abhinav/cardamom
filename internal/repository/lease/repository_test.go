package lease

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	domainlease "go.abhg.dev/cardamom/internal/lease"
)

func TestRepositoryFiniteCustodyLifecycle(t *testing.T) {
	now := time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)
	clock := newTestClock(now)
	repository, _ := openTestRepository(
		t,
		filepath.Join(t.TempDir(), "cardamom.db"),
		clock,
	)

	acquired, err := repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "alice", TTL: time.Hour,
	})
	require.NoError(t, err)
	assert.Equal(t, domainlease.Lease{
		Name: "deploy-slot", Owner: "alice", AcquiredAt: now,
		ExpiresAt: now.Add(time.Hour),
	}, acquired)

	_, err = repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "alice", TTL: time.Hour,
	})
	assertHeldBy(t, err, acquired)

	_, err = repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "bob", TTL: time.Hour,
	})
	assertHeldBy(t, err, acquired)

	_, err = repository.Renew(t.Context(), domainlease.RenewRequest{
		Name: "deploy-slot", Owner: "bob", TTL: 2 * time.Hour,
	})
	assertHeldBy(t, err, acquired)

	clock.Set(now.Add(15 * time.Minute))
	renewed, err := repository.Renew(t.Context(), domainlease.RenewRequest{
		Name: "deploy-slot", Owner: "alice", TTL: 2 * time.Hour,
	})
	require.NoError(t, err)
	assert.Equal(t, acquired.AcquiredAt, renewed.AcquiredAt)
	assert.Equal(t, now.Add(2*time.Hour+15*time.Minute), renewed.ExpiresAt)

	listed, err := repository.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []domainlease.Lease{renewed}, listed)

	_, err = repository.Release(t.Context(), domainlease.ReleaseRequest{
		Name: "deploy-slot", Owner: "bob",
	})
	assertHeldBy(t, err, renewed)

	released, err := repository.Release(t.Context(), domainlease.ReleaseRequest{
		Name: "deploy-slot", Owner: "alice",
	})
	require.NoError(t, err)
	assert.Equal(t, renewed, released)

	_, err = repository.Get(t.Context(), domainlease.GetRequest{Name: "deploy-slot"})
	assert.ErrorIs(t, err, domainlease.ErrNotFound)
}

func TestRepositoryReplacesLeaseAtExpiry(t *testing.T) {
	now := time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)
	clock := newTestClock(now)
	repository, _ := openTestRepository(
		t,
		filepath.Join(t.TempDir(), "cardamom.db"),
		clock,
	)

	acquired, err := repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "alice", TTL: 500 * time.Millisecond,
	})
	require.NoError(t, err)
	assert.Equal(t, now.Add(time.Second), acquired.ExpiresAt)

	clock.Set(acquired.ExpiresAt)
	_, err = repository.Get(t.Context(), domainlease.GetRequest{Name: "deploy-slot"})
	assert.ErrorIs(t, err, domainlease.ErrNotFound)

	replacement, err := repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "bob", TTL: 2 * time.Minute,
	})
	require.NoError(t, err)
	assert.Equal(t, domainlease.Lease{
		Name: "deploy-slot", Owner: "bob",
		AcquiredAt: acquired.ExpiresAt,
		ExpiresAt:  acquired.ExpiresAt.Add(2 * time.Minute),
	}, replacement)
}

func TestRepositoryRevokeLease(t *testing.T) {
	now := time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)
	clock := newTestClock(now)
	repository, _ := openTestRepository(
		t,
		filepath.Join(t.TempDir(), "cardamom.db"),
		clock,
	)
	acquired, err := repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "alice", TTL: time.Hour,
	})
	require.NoError(t, err)

	clock.Set(now.Add(15 * time.Minute))
	revoked, err := repository.Revoke(t.Context(), domainlease.RevokeRequest{
		Name: "deploy-slot", Owner: "alice",
		RevokedBy: "coordinator", Reason: "owner cannot continue",
	})

	require.NoError(t, err)
	assert.Equal(t, domainlease.Revocation{
		Lease: acquired, RevokedBy: "coordinator",
		Reason: "owner cannot continue", RevokedAt: now.Add(15 * time.Minute),
	}, revoked)
	_, err = repository.Get(t.Context(), domainlease.GetRequest{Name: "deploy-slot"})
	assert.ErrorIs(t, err, domainlease.ErrNotFound)
}

func TestRepositoryRevokeLease_preservesUnexpectedOwner(t *testing.T) {
	now := time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)
	repository, _ := openTestRepository(
		t,
		filepath.Join(t.TempDir(), "cardamom.db"),
		newTestClock(now),
	)
	acquired, err := repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "alice", TTL: time.Hour,
	})
	require.NoError(t, err)

	_, err = repository.Revoke(t.Context(), domainlease.RevokeRequest{
		Name: "deploy-slot", Owner: "bob",
		RevokedBy: "coordinator", Reason: "owner cannot continue",
	})

	assertHeldBy(t, err, acquired)
	stored, err := repository.Get(t.Context(), domainlease.GetRequest{Name: "deploy-slot"})
	require.NoError(t, err)
	assert.Equal(t, acquired, stored)
}

func TestRepositoryRevokeLease_rejectsMissingAndExpiredLeases(t *testing.T) {
	now := time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)
	clock := newTestClock(now)
	repository, _ := openTestRepository(
		t,
		filepath.Join(t.TempDir(), "cardamom.db"),
		clock,
	)

	_, err := repository.Revoke(t.Context(), domainlease.RevokeRequest{
		Name: "deploy-slot", Owner: "alice",
		RevokedBy: "coordinator", Reason: "owner cannot continue",
	})
	assert.ErrorIs(t, err, domainlease.ErrNotFound)

	_, err = repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "alice", TTL: time.Minute,
	})
	require.NoError(t, err)
	clock.Set(now.Add(time.Minute))
	_, err = repository.Revoke(t.Context(), domainlease.RevokeRequest{
		Name: "deploy-slot", Owner: "alice",
		RevokedBy: "coordinator", Reason: "owner cannot continue",
	})
	assert.ErrorIs(t, err, domainlease.ErrNotFound)
}

func TestRepositorySerializesCompetingAcquisitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cardamom.db")
	clock := newTestClock(time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC))
	first, _ := openTestRepository(t, path, clock)
	second, _ := openTestRepository(t, path, clock)

	type result struct {
		lease domainlease.Lease
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var group sync.WaitGroup
	for owner, repository := range map[string]*Repository{
		"alice": first,
		"bob":   second,
	} {
		group.Go(func() {
			<-start
			acquired, err := repository.Acquire(t.Context(), domainlease.AcquireRequest{
				Name: "deploy-slot", Owner: owner, TTL: time.Hour,
			})
			results <- result{lease: acquired, err: err}
		})
	}
	close(start)
	group.Wait()
	close(results)

	var acquired domainlease.Lease
	var held *domainlease.HeldError
	for result := range results {
		if result.err == nil {
			assert.Empty(t, acquired)
			acquired = result.lease
			continue
		}
		var conflict *domainlease.HeldError
		require.ErrorAs(t, result.err, &conflict)
		held = conflict
	}
	require.NotEmpty(t, acquired)
	require.NotNil(t, held)
	assert.Equal(t, acquired, held.Current)
}

func TestRepositoryRejectsInvalidInput(t *testing.T) {
	repository, _ := openTestRepository(
		t,
		filepath.Join(t.TempDir(), "cardamom.db"),
		newTestClock(time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)),
	)

	_, err := repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: " ", Owner: " ", TTL: -time.Second,
	})

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	listed, listErr := repository.List(t.Context())
	assert.NoError(t, listErr)
	assert.Empty(t, listed)
}

func TestRepositoryRollsBackReleaseOnStorageFailure(t *testing.T) {
	now := time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)
	repository, persistence := openTestRepository(
		t,
		filepath.Join(t.TempDir(), "cardamom.db"),
		newTestClock(now),
	)
	acquired, err := repository.Acquire(t.Context(), domainlease.AcquireRequest{
		Name: "deploy-slot", Owner: "alice", TTL: time.Hour,
	})
	require.NoError(t, err)
	testExec(t, persistence, `
		CREATE TRIGGER reject_lease_release
		AFTER DELETE ON leases
		BEGIN
			SELECT RAISE(ABORT, 'induced release failure');
		END
	`)

	_, err = repository.Release(t.Context(), domainlease.ReleaseRequest{
		Name: "deploy-slot", Owner: "alice",
	})
	assert.ErrorContains(t, err, "induced release failure")
	testExec(t, persistence, `DROP TRIGGER reject_lease_release`)
	stored, err := repository.Get(t.Context(), domainlease.GetRequest{Name: "deploy-slot"})
	require.NoError(t, err)
	assert.Equal(t, acquired, stored)
}

func assertHeldBy(t *testing.T, err error, current domainlease.Lease) {
	t.Helper()
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	var held *domainlease.HeldError
	require.True(t, errors.As(err, &held))
	assert.Equal(t, current, held.Current)
}
