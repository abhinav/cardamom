package mail

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionUpdateChanges(t *testing.T) {
	now := time.Date(2026, time.July, 13, 19, 0, 0, 0, time.UTC)
	update, err := NewSubscriptionUpdate(
		"bob",
		[]string{"release.*", "build.*"},
		[]string{"old.*", "missing.*", "old.*"},
		30*time.Minute,
	)
	require.NoError(t, err)

	changes := update.Changes([]Subscription{
		{Listener: "bob", Pattern: "release.*", Created: now.Add(-time.Hour), Expires: now.Add(time.Minute)},
		{Listener: "bob", Pattern: "old.*", Created: now.Add(-time.Hour), Expires: now.Add(time.Minute)},
	}, now)

	assert.Equal(t, []SubscriptionRemoval{
		{Pattern: "old.*", Removed: true},
		{Pattern: "missing.*"},
		{Pattern: "old.*"},
	}, changes.Removals)
	assert.Equal(t, []Subscription{
		{Listener: "bob", Pattern: "release.*", Created: now.Add(-time.Hour), Expires: now.Add(30 * time.Minute)},
		{Listener: "bob", Pattern: "build.*", Created: now, Expires: now.Add(30 * time.Minute)},
	}, changes.Upserts)
}

func TestNewSubscriptionUpdateCapsTTL(t *testing.T) {
	now := time.Date(2026, time.July, 13, 19, 0, 0, 0, time.UTC)
	update, err := NewSubscriptionUpdate(
		"bob",
		[]string{"release.*"},
		nil,
		30*24*time.Hour,
	)
	require.NoError(t, err)

	changes := update.Changes(nil, now)
	require.Len(t, changes.Upserts, 1)
	assert.Equal(t, now.Add(SubscriptionMaxTTL), changes.Upserts[0].Expires)
}

func TestNewSubscriptionUpdateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		listener string
		refresh  []string
		remove   []string
		ttl      time.Duration
		want     string
	}{
		{name: "MissingListener", remove: []string{"old.*"}, want: "listener required"},
		{name: "MissingRefreshPattern", listener: "bob", refresh: []string{""}, ttl: time.Minute, want: "refresh pattern required"},
		{name: "MissingRemovalPattern", listener: "bob", remove: []string{""}, want: "removal pattern required"},
		{name: "MissingTTL", listener: "bob", refresh: []string{"release.*"}, want: "ttl must be greater than zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSubscriptionUpdate(
				tt.listener,
				tt.refresh,
				tt.remove,
				tt.ttl,
			)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}
