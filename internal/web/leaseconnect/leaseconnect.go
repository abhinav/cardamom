// Package leaseconnect exposes store-scoped resource leases through Connect.
package leaseconnect

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/lease"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:generate go tool mockgen -destination mocks_test.go -package leaseconnect -typed -write_package_comment=false . Operations

// Operations supplies the store-scoped resource lease behavior exposed by
// LeaseService.
type Operations interface {
	// Acquire acquires an absent or expired resource lease.
	Acquire(context.Context, lease.AcquireRequest) (lease.Lease, error)

	// Renew extends an active owner-held resource lease.
	Renew(context.Context, lease.RenewRequest) (lease.Lease, error)

	// Release removes an active owner-held resource lease.
	Release(context.Context, lease.ReleaseRequest) (lease.Lease, error)

	// Revoke removes an active resource lease held by the expected owner.
	Revoke(context.Context, lease.RevokeRequest) (lease.Revocation, error)

	// Get returns one active resource lease.
	Get(context.Context, lease.GetRequest) (lease.Lease, error)

	// List returns every active resource lease in domain order.
	List(context.Context) ([]lease.Lease, error)
}

// Service adapts store-scoped resource leases to generated LeaseService RPCs.
type Service struct {
	privatev1connect.UnimplementedLeaseServiceHandler
	operations Operations
}

var _ privatev1connect.LeaseServiceHandler = (*Service)(nil)

// New constructs a LeaseService handler from store-scoped domain operations.
func New(operations Operations) *Service {
	must.NotBeNilf(operations, "leaseconnect: lease operations are required")
	return &Service{operations: operations}
}

// AcquireLease validates transport values and acquires one resource lease.
func (s *Service) AcquireLease(
	ctx context.Context,
	request *connect.Request[privatev1.AcquireLeaseRequest],
) (*connect.Response[privatev1.AcquireLeaseResponse], error) {
	ttl, err := duration("ttl", request.Msg.GetTtl())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := s.operations.Acquire(ctx, lease.AcquireRequest{
		Name: request.Msg.GetName(), Owner: request.Msg.GetOwner(), TTL: ttl,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.AcquireLeaseResponse{
		Lease: resourceLease(result),
	}), nil
}

// RenewLease validates transport values and renews one owner-held lease.
func (s *Service) RenewLease(
	ctx context.Context,
	request *connect.Request[privatev1.RenewLeaseRequest],
) (*connect.Response[privatev1.RenewLeaseResponse], error) {
	ttl, err := duration("ttl", request.Msg.GetTtl())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := s.operations.Renew(ctx, lease.RenewRequest{
		Name: request.Msg.GetName(), Owner: request.Msg.GetOwner(), TTL: ttl,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.RenewLeaseResponse{
		Lease: resourceLease(result),
	}), nil
}

// ReleaseLease removes one owner-held lease.
func (s *Service) ReleaseLease(
	ctx context.Context,
	request *connect.Request[privatev1.ReleaseLeaseRequest],
) (*connect.Response[privatev1.ReleaseLeaseResponse], error) {
	result, err := s.operations.Release(ctx, lease.ReleaseRequest{
		Name: request.Msg.GetName(), Owner: request.Msg.GetOwner(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ReleaseLeaseResponse{
		Lease: resourceLease(result),
	}), nil
}

// RevokeLease removes one active lease held by the expected owner.
func (s *Service) RevokeLease(
	ctx context.Context,
	request *connect.Request[privatev1.RevokeLeaseRequest],
) (*connect.Response[privatev1.RevokeLeaseResponse], error) {
	result, err := s.operations.Revoke(ctx, lease.RevokeRequest{
		Name: request.Msg.GetName(), Owner: request.Msg.GetOwner(),
		RevokedBy: request.Msg.GetRevokedBy(), Reason: request.Msg.GetReason(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.RevokeLeaseResponse{
		Revocation: resourceLeaseRevocation(result),
	}), nil
}

// GetLease returns one active resource lease.
func (s *Service) GetLease(
	ctx context.Context,
	request *connect.Request[privatev1.GetLeaseRequest],
) (*connect.Response[privatev1.GetLeaseResponse], error) {
	result, err := s.operations.Get(ctx, lease.GetRequest{Name: request.Msg.GetName()})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.GetLeaseResponse{
		Lease: resourceLease(result),
	}), nil
}

// ListLeases returns every active resource lease in domain order.
func (s *Service) ListLeases(
	ctx context.Context,
	_ *connect.Request[privatev1.ListLeasesRequest],
) (*connect.Response[privatev1.ListLeasesResponse], error) {
	values, err := s.operations.List(ctx)
	if err != nil {
		return nil, web.FromError(err)
	}
	result := make([]*privatev1.ResourceLease, len(values))
	for index, value := range values {
		result[index] = resourceLease(value)
	}
	return connect.NewResponse(&privatev1.ListLeasesResponse{Leases: result}), nil
}

func duration(name string, value *durationpb.Duration) (time.Duration, error) {
	if value == nil {
		return 0, nil
	}
	if err := value.CheckValid(); err != nil {
		return 0, errkind.Errorf(errkind.InvalidInput, "invalid input: %s: %v", name, err)
	}
	return value.AsDuration(), nil
}

func resourceLease(value lease.Lease) *privatev1.ResourceLease {
	return &privatev1.ResourceLease{
		Name: value.Name, Owner: value.Owner,
		AcquiredAt: timestamppb.New(value.AcquiredAt),
		ExpiresAt:  timestamppb.New(value.ExpiresAt),
	}
}

func resourceLeaseRevocation(
	value lease.Revocation,
) *privatev1.ResourceLeaseRevocation {
	return &privatev1.ResourceLeaseRevocation{
		Lease: resourceLease(value.Lease), RevokedBy: value.RevokedBy,
		Reason: value.Reason, RevokedAt: timestamppb.New(value.RevokedAt),
	}
}
