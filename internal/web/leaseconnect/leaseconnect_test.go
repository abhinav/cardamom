package leaseconnect

import (
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
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestServiceLeaseOperations(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	value := lease.Lease{
		Name: "staging-db", Owner: "alice", AcquiredAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
	operations := NewMockOperations(gomock.NewController(t))
	operations.EXPECT().Acquire(gomock.Any(), lease.AcquireRequest{
		Name: "staging-db", Owner: "alice", TTL: time.Hour,
	}).Return(value, nil)
	operations.EXPECT().Renew(gomock.Any(), lease.RenewRequest{
		Name: "staging-db", Owner: "alice", TTL: 2 * time.Hour,
	}).Return(value, nil)
	operations.EXPECT().Release(gomock.Any(), lease.ReleaseRequest{
		Name: "staging-db", Owner: "alice",
	}).Return(value, nil)
	operations.EXPECT().Revoke(gomock.Any(), lease.RevokeRequest{
		Name: "staging-db", Owner: "alice",
		RevokedBy: "coordinator", Reason: "owner cannot continue",
	}).Return(lease.Revocation{
		Lease: value, RevokedBy: "coordinator",
		Reason: "owner cannot continue", RevokedAt: now,
	}, nil)
	operations.EXPECT().Get(gomock.Any(), lease.GetRequest{Name: "staging-db"}).Return(value, nil)
	operations.EXPECT().List(gomock.Any()).Return(
		[]lease.Lease{{Name: "device-a"}, {Name: "staging-db"}}, nil,
	)
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
	operations := NewMockOperations(gomock.NewController(t))
	operations.EXPECT().Acquire(gomock.Any(), lease.AcquireRequest{
		Name: "staging-db", Owner: "alice", TTL: time.Hour,
	}).Return(lease.Lease{}, errkind.Wrap(errkind.Conflict, &lease.HeldError{
		Current: lease.Lease{
			Name: "staging-db", Owner: "bob",
			ExpiresAt: time.Date(2026, time.July, 20, 13, 0, 0, 0, time.UTC),
		},
	}))
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
