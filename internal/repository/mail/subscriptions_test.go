package mail

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.abhg.dev/cardamom/internal/mail"
)

func TestRepository_UpdateSubscriptionsRefreshesRemovesAndOrders(t *testing.T) {
	now := time.Date(2026, time.July, 18, 17, 0, 0, 0, time.UTC)
	repository, _, clock := openRepository(t, now)

	first, err := mail.NewSubscriptionUpdate(
		"zoe",
		[]string{"release.*"},
		nil,
		15*time.Minute,
	)
	require.NoError(t, err)
	created, err := repository.UpdateSubscriptions(t.Context(), first)
	require.NoError(t, err)
	require.Len(t, created.Subscriptions, 1)
	original := created.Subscriptions[0]

	alice, err := mail.NewSubscriptionUpdate(
		"alice",
		[]string{"build.*", "alert.*"},
		nil,
		15*time.Minute,
	)
	require.NoError(t, err)
	_, err = repository.UpdateSubscriptions(t.Context(), alice)
	require.NoError(t, err)

	clock.now = now.Add(5 * time.Minute)
	refresh, err := mail.NewSubscriptionUpdate(
		"zoe",
		[]string{"release.*"},
		[]string{"missing.*"},
		30*time.Minute,
	)
	require.NoError(t, err)
	updated, err := repository.UpdateSubscriptions(t.Context(), refresh)
	require.NoError(t, err)
	require.Len(t, updated.Subscriptions, 1)
	assert.Equal(t, original.Created, updated.Subscriptions[0].Created)
	assert.Equal(t, clock.now.Add(30*time.Minute), updated.Subscriptions[0].Expires)
	assert.Equal(t, []mail.SubscriptionRemoval{
		{Pattern: "missing.*", Removed: false},
	}, updated.Removals)

	remove, err := mail.NewSubscriptionUpdate(
		"zoe",
		nil,
		[]string{"release.*", "release.*"},
		0,
	)
	require.NoError(t, err)
	removed, err := repository.UpdateSubscriptions(t.Context(), remove)
	require.NoError(t, err)
	assert.Empty(t, removed.Subscriptions)
	assert.Equal(t, []mail.SubscriptionRemoval{
		{Pattern: "release.*", Removed: true},
		{Pattern: "release.*", Removed: false},
	}, removed.Removals)

	active, err := repository.ListSubscriptions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"alice:alert.*", "alice:build.*"}, subscriptionKeys(active))
}

func subscriptionKeys(subscriptions []mail.Subscription) []string {
	keys := make([]string, len(subscriptions))
	for index, subscription := range subscriptions {
		keys[index] = subscription.Listener + ":" + subscription.Pattern
	}
	return keys
}
