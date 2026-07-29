package lease

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLeaseRequestsValidate(t *testing.T) {
	t.Run("Acquire", func(t *testing.T) {
		assert.NoError(t, (AcquireRequest{
			Name: "staging-db", Owner: "alice", TTL: time.Minute,
		}).Validate())
		assert.EqualError(t, (AcquireRequest{
			Name: " ", Owner: "alice", TTL: time.Minute,
		}).Validate(), "lease name required")
		assert.EqualError(t, (AcquireRequest{
			Name: "staging-db", Owner: " ", TTL: time.Minute,
		}).Validate(), "owner required")
		assert.EqualError(t, (AcquireRequest{
			Name: "staging-db", Owner: "alice", TTL: -time.Second,
		}).Validate(), "ttl must be greater than zero")
	})

	t.Run("Renew", func(t *testing.T) {
		assert.NoError(t, (RenewRequest{
			Name: "staging-db", Owner: "alice", TTL: time.Minute,
		}).Validate())
		assert.EqualError(t, (RenewRequest{
			Name: "staging-db", Owner: "alice", TTL: -time.Second,
		}).Validate(), "ttl must be greater than zero")
	})

	t.Run("Release", func(t *testing.T) {
		assert.NoError(t, (ReleaseRequest{
			Name: "staging-db", Owner: "alice",
		}).Validate())
		assert.EqualError(t, (ReleaseRequest{
			Name: "staging-db", Owner: " ",
		}).Validate(), "owner required")
	})

	t.Run("Revoke", func(t *testing.T) {
		assert.NoError(t, (RevokeRequest{
			Name: "staging-db", Owner: "alice",
			RevokedBy: "coordinator", Reason: "owner cannot continue",
		}).Validate())
		assert.EqualError(t, (RevokeRequest{
			Name: "staging-db", Owner: "alice",
			RevokedBy: " ", Reason: "owner cannot continue",
		}).Validate(), "revoking actor required")
		assert.EqualError(t, (RevokeRequest{
			Name: "staging-db", Owner: "alice",
			RevokedBy: "coordinator", Reason: " ",
		}).Validate(), "reason required")
	})

	t.Run("Get", func(t *testing.T) {
		assert.NoError(t, (GetRequest{Name: "staging-db"}).Validate())
		assert.EqualError(t, (GetRequest{Name: " "}).Validate(), "lease name required")
	})
}

func TestHeldErrorCarriesCurrentLease(t *testing.T) {
	current := Lease{
		Name: "staging-db", Owner: "bob",
		ExpiresAt: time.Date(2026, time.July, 18, 12, 10, 0, 0, time.UTC),
	}
	err := &HeldError{Current: current}

	assert.Equal(t, current, err.Current)
	assert.Equal(
		t,
		`lease "staging-db" is held by "bob" until 2026-07-18T12:10:00Z`,
		err.Error(),
	)
}
