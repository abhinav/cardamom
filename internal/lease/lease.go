package lease

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound reports that no active lease exists for a resource name.
var ErrNotFound = errors.New("lease not found")

// Lease records active temporary ownership of a named external resource.
// Timestamps have whole-second precision and use UTC.
type Lease struct {
	// Name identifies the external resource.
	Name string `json:"name"`

	// Owner identifies the actor that owns the lease.
	Owner string `json:"owner"`

	// AcquiredAt is the original acquisition time.
	AcquiredAt time.Time `json:"acquired_at"`

	// ExpiresAt is the exclusive expiration boundary.
	ExpiresAt time.Time `json:"expires_at"`
}

// AcquireRequest identifies a resource, requested owner, and lease lifetime.
type AcquireRequest struct {
	// Name identifies the external resource.
	Name string // required

	// Owner identifies the actor that will own the lease.
	Owner string // required

	// TTL is the positive requested lifetime.
	TTL time.Duration // required
}

// Validate reports whether the acquisition request has all required values.
func (r AcquireRequest) Validate() error {
	return validateMutation(r.Name, r.Owner, r.TTL)
}

// RenewRequest identifies an owner-held lease and its new lifetime.
type RenewRequest struct {
	// Name identifies the external resource.
	Name string // required

	// Owner identifies the actor that currently owns the lease.
	Owner string // required

	// TTL is the positive requested lifetime from renewal.
	TTL time.Duration // required
}

// Validate reports whether the renewal request has all required values.
func (r RenewRequest) Validate() error {
	return validateMutation(r.Name, r.Owner, r.TTL)
}

// ReleaseRequest identifies an owner-held lease to remove.
type ReleaseRequest struct {
	// Name identifies the external resource.
	Name string // required

	// Owner identifies the actor that currently owns the lease.
	Owner string // required
}

// Validate reports whether the release request has all required values.
func (r ReleaseRequest) Validate() error {
	if err := validateName(r.Name); err != nil {
		return err
	}
	return validateOwner(r.Owner)
}

// RevokeRequest identifies an active lease by its expected owner and records
// the coordinator-attributed recovery context.
type RevokeRequest struct {
	// Name identifies the external resource.
	Name string // required

	// Owner identifies the expected active lease owner.
	Owner string // required

	// RevokedBy identifies the actor performing the revocation.
	RevokedBy string // required

	// Reason records why coordinator recovery requires revocation.
	Reason string // required
}

// Validate reports whether the revocation request has all required values.
func (r RevokeRequest) Validate() error {
	if err := validateName(r.Name); err != nil {
		return err
	}
	if err := validateOwner(r.Owner); err != nil {
		return err
	}
	if strings.TrimSpace(r.RevokedBy) == "" {
		return errors.New("revoking actor required")
	}
	if strings.TrimSpace(r.Reason) == "" {
		return errors.New("reason required")
	}
	return nil
}

// Revocation reports the removed lease and transient operation context.
// Revocation does not represent external resource cleanup or persisted history.
type Revocation struct {
	// Lease is the active ownership record removed by the operation.
	Lease Lease `json:"lease"`

	// RevokedBy identifies the actor that performed the revocation.
	RevokedBy string `json:"revoked_by"`

	// Reason records why coordinator recovery required revocation.
	Reason string `json:"reason"`

	// RevokedAt is the time Cardamom removed the lease.
	RevokedAt time.Time `json:"revoked_at"`
}

// GetRequest identifies one active resource lease.
type GetRequest struct {
	// Name identifies the external resource.
	Name string // required
}

// Validate reports whether the read request identifies a resource.
func (r GetRequest) Validate() error {
	return validateName(r.Name)
}

// HeldError reports the active lease that prevented an acquisition or
// owner-only operation.
type HeldError struct {
	// Current is the active lease that prevented the operation.
	Current Lease // required
}

// Error reports the current owner and expiration boundary.
func (e *HeldError) Error() string {
	return fmt.Sprintf(
		"lease %q is held by %q until %s",
		e.Current.Name,
		e.Current.Owner,
		e.Current.ExpiresAt.Format(time.RFC3339),
	)
}

func validateMutation(name, owner string, ttl time.Duration) error {
	if err := validateName(name); err != nil {
		return err
	}
	if err := validateOwner(owner); err != nil {
		return err
	}
	if ttl <= 0 {
		return errors.New("ttl must be greater than zero")
	}
	return nil
}

func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("lease name required")
	}
	return nil
}

func validateOwner(owner string) error {
	if strings.TrimSpace(owner) == "" {
		return errors.New("owner required")
	}
	return nil
}
