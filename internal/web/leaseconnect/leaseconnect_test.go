package leaseconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/lease"
	repositorylease "go.abhg.dev/cardamom/internal/repository/lease"
	"go.abhg.dev/cardamom/internal/repository/store"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestServiceLeaseOperations(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	operations := &recordingOperations{
		value: lease.Lease{
			Name: "staging-db", Owner: "alice", AcquiredAt: now,
			ExpiresAt: now.Add(time.Hour),
		},
		values: []lease.Lease{{Name: "device-a"}, {Name: "staging-db"}},
	}
	client := newTestClient(t, operations)

	acquired, err := client.AcquireLease(t.Context(), connect.NewRequest(&privatev1.AcquireLeaseRequest{
		Name: "staging-db", Owner: "alice", Ttl: durationpb.New(time.Hour),
	}))
	require.NoError(t, err)
	renewed, err := client.RenewLease(t.Context(), connect.NewRequest(&privatev1.RenewLeaseRequest{
		Name: "staging-db", Owner: "alice", Ttl: durationpb.New(2 * time.Hour),
	}))
	require.NoError(t, err)
	released, err := client.ReleaseLease(t.Context(), connect.NewRequest(&privatev1.ReleaseLeaseRequest{
		Name: "staging-db", Owner: "alice",
	}))
	require.NoError(t, err)
	revoked, err := client.RevokeLease(t.Context(), connect.NewRequest(&privatev1.RevokeLeaseRequest{
		Name: "staging-db", Owner: "alice",
		RevokedBy: "coordinator", Reason: "owner cannot continue",
	}))
	require.NoError(t, err)
	read, err := client.GetLease(t.Context(), connect.NewRequest(&privatev1.GetLeaseRequest{Name: "staging-db"}))
	require.NoError(t, err)
	listed, err := client.ListLeases(t.Context(), connect.NewRequest(&privatev1.ListLeasesRequest{}))

	require.NoError(t, err)
	assert.Equal(t, lease.AcquireRequest{
		Name: "staging-db", Owner: "alice", TTL: time.Hour,
	}, operations.acquisitions[0])
	assert.Equal(t, 2*time.Hour, operations.renewals[0].TTL)
	assert.Equal(t, "alice", operations.releases[0].Owner)
	assert.Equal(t, lease.RevokeRequest{
		Name: "staging-db", Owner: "alice",
		RevokedBy: "coordinator", Reason: "owner cannot continue",
	}, operations.revocations[0])
	assert.Equal(t, "staging-db", operations.reads[0].Name)
	assert.Equal(t, "staging-db", acquired.Msg.GetLease().GetName())
	assert.Equal(t, "staging-db", renewed.Msg.GetLease().GetName())
	assert.Equal(t, "staging-db", released.Msg.GetLease().GetName())
	assert.Equal(t, "staging-db", revoked.Msg.GetRevocation().GetLease().GetName())
	assert.Equal(t, "coordinator", revoked.Msg.GetRevocation().GetRevokedBy())
	assert.Equal(t, "owner cannot continue", revoked.Msg.GetRevocation().GetReason())
	assert.Equal(t, "staging-db", read.Msg.GetLease().GetName())
	assert.Len(t, listed.Msg.GetLeases(), 2)
}

func TestServiceLeaseOperations_translateDomainErrors(t *testing.T) {
	operations := &recordingOperations{
		err: errkind.Wrap(errkind.Conflict, &lease.HeldError{
			Current: lease.Lease{
				Name: "staging-db", Owner: "bob",
				ExpiresAt: time.Date(2026, time.July, 20, 13, 0, 0, 0, time.UTC),
			},
		}),
	}
	client := newTestClient(t, operations)

	_, err := client.AcquireLease(t.Context(), connect.NewRequest(&privatev1.AcquireLeaseRequest{
		Name: "staging-db", Owner: "alice", Ttl: durationpb.New(time.Hour),
	}))

	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.ErrorContains(t, err, `held by "bob" until 2026-07-20T13:00:00Z`)
}

func TestServiceLeaseOperations_preservePersistedDomainOutcomes(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{now: now}
	persistence, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(t.TempDir(), "cardamom.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	operations := repositorylease.New(persistence, clock)
	client := newTestClient(t, operations)

	acquired, err := client.AcquireLease(t.Context(), connect.NewRequest(&privatev1.AcquireLeaseRequest{
		Name: "staging-db", Owner: "alice", Ttl: durationpb.New(time.Hour),
	}))
	require.NoError(t, err)
	persisted, err := operations.Get(t.Context(), lease.GetRequest{Name: "staging-db"})

	require.NoError(t, err)
	assert.Equal(t, acquired.Msg.GetLease().GetName(), persisted.Name)
	assert.Equal(t, acquired.Msg.GetLease().GetOwner(), persisted.Owner)
	assert.Equal(t, acquired.Msg.GetLease().GetAcquiredAt().AsTime(), persisted.AcquiredAt)
	assert.Equal(t, acquired.Msg.GetLease().GetExpiresAt().AsTime(), persisted.ExpiresAt)
}

type recordingOperations struct {
	acquisitions []lease.AcquireRequest
	renewals     []lease.RenewRequest
	releases     []lease.ReleaseRequest
	revocations  []lease.RevokeRequest
	reads        []lease.GetRequest

	value  lease.Lease
	values []lease.Lease
	err    error
}

func (o *recordingOperations) Acquire(_ context.Context, request lease.AcquireRequest) (lease.Lease, error) {
	o.acquisitions = append(o.acquisitions, request)
	return o.value, o.err
}

func (o *recordingOperations) Renew(_ context.Context, request lease.RenewRequest) (lease.Lease, error) {
	o.renewals = append(o.renewals, request)
	return o.value, o.err
}

func (o *recordingOperations) Release(_ context.Context, request lease.ReleaseRequest) (lease.Lease, error) {
	o.releases = append(o.releases, request)
	return o.value, o.err
}

func (o *recordingOperations) Revoke(_ context.Context, request lease.RevokeRequest) (lease.Revocation, error) {
	o.revocations = append(o.revocations, request)
	return lease.Revocation{
		Lease: o.value, RevokedBy: request.RevokedBy,
		Reason: request.Reason, RevokedAt: o.value.AcquiredAt,
	}, o.err
}

func (o *recordingOperations) Get(_ context.Context, request lease.GetRequest) (lease.Lease, error) {
	o.reads = append(o.reads, request)
	return o.value, o.err
}

func (o *recordingOperations) List(context.Context) ([]lease.Lease, error) {
	return o.values, o.err
}

func newTestClient(t *testing.T, operations Operations) privatev1connect.LeaseServiceClient {
	t.Helper()
	_, handler := privatev1connect.NewLeaseServiceHandler(New(operations))
	client := &http.Client{Transport: &testHandlerTransport{handler: handler}}
	return privatev1connect.NewLeaseServiceClient(client, "http://cardamom.test")
}

type testHandlerTransport struct{ handler http.Handler }

func (t *testHandlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
