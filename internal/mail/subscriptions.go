package mail

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"time"
)

// SubscriptionMaxTTL is the longest lifetime assigned without a refresh.
const SubscriptionMaxTTL = 7 * 24 * time.Hour

// Subscription records one listener's temporary interest in a topic pattern.
type Subscription struct {
	// Listener receives messages whose topic matches Pattern.
	Listener string

	// Pattern is the filepath-style topic glob selected by the listener.
	Pattern string

	// Created is the initial registration time.
	Created time.Time

	// Expires is the exclusive lifetime boundary.
	Expires time.Time
}

// SubscriptionUpdate is a validated atomic change to one listener's patterns.
type SubscriptionUpdate struct {
	listener string
	refresh  []string
	remove   []string
	ttl      time.Duration
}

// NewSubscriptionUpdate parses one listener's subscription changes.
// Removal-only updates may use a zero TTL. Refreshed lifetimes are capped at
// SubscriptionMaxTTL.
func NewSubscriptionUpdate(
	listener string,
	refresh, remove []string,
	ttl time.Duration,
) (SubscriptionUpdate, error) {
	switch {
	case listener == "":
		return SubscriptionUpdate{}, errors.New("listener required")
	case slices.Contains(refresh, ""):
		return SubscriptionUpdate{}, errors.New("refresh pattern required")
	case slices.Contains(remove, ""):
		return SubscriptionUpdate{}, errors.New("removal pattern required")
	case len(refresh) > 0 && ttl <= 0:
		return SubscriptionUpdate{}, errors.New("ttl must be greater than zero")
	}
	for _, pattern := range refresh {
		if _, err := path.Match(pattern, ""); err != nil {
			return SubscriptionUpdate{}, fmt.Errorf(
				"invalid subscription pattern %q: %w",
				pattern,
				err,
			)
		}
	}
	return SubscriptionUpdate{
		listener: listener, refresh: slices.Clone(refresh), remove: slices.Clone(remove),
		ttl: min(ttl, SubscriptionMaxTTL),
	}, nil
}

// Listener returns the owner of the updated pattern set.
func (u SubscriptionUpdate) Listener() string { return u.listener }

// HasChanges reports whether the update contains a refresh or removal.
func (u SubscriptionUpdate) HasChanges() bool {
	return len(u.refresh) > 0 || len(u.remove) > 0
}

// SubscriptionRemoval reports whether one requested pattern was active and
// removed.
type SubscriptionRemoval struct {
	// Pattern is the requested topic pattern.
	Pattern string

	// Removed reports whether an active subscription existed.
	Removed bool
}

// SubscriptionChanges contains the normalized persistence changes for one
// listener.
type SubscriptionChanges struct {
	// Upserts contains subscriptions to create or refresh.
	Upserts []Subscription

	// Removals reports requested patterns in request order.
	Removals []SubscriptionRemoval
}

// Changes derives the normalized subscription changes from the active state.
// Removals precede refreshes, so a pattern present in both collections remains
// active.
func (u SubscriptionUpdate) Changes(
	current []Subscription,
	now time.Time,
) SubscriptionChanges {
	now = wholeSecond(now)
	active := make(map[string]Subscription, len(current))
	for _, subscription := range current {
		if subscription.Listener == u.listener && subscription.Expires.After(now) {
			active[subscription.Pattern] = subscription
		}
	}

	removals := make([]SubscriptionRemoval, 0, len(u.remove))
	for _, pattern := range u.remove {
		_, removed := active[pattern]
		delete(active, pattern)
		removals = append(removals, SubscriptionRemoval{Pattern: pattern, Removed: removed})
	}

	upserts := make([]Subscription, 0, len(u.refresh))
	expires := now.Add(u.ttl)
	for _, pattern := range u.refresh {
		subscription, exists := active[pattern]
		if !exists {
			subscription = Subscription{
				Listener: u.listener, Pattern: pattern, Created: now,
			}
		}
		subscription.Expires = expires
		active[pattern] = subscription
		upserts = append(upserts, subscription)
	}
	return SubscriptionChanges{Upserts: upserts, Removals: removals}
}

// SubscriptionsUpdated is the committed result of a subscription update.
type SubscriptionsUpdated struct {
	// Subscriptions contains created or refreshed subscriptions.
	Subscriptions []Subscription

	// Removals contains the outcome for each requested removal.
	Removals []SubscriptionRemoval
}
