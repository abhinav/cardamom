package mail

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectSendCreatesOneActorDelivery(t *testing.T) {
	now := time.Date(2026, time.July, 22, 12, 0, 0, 500, time.UTC)
	send, err := NewDirectSend("alice", "bob", "status", 0)
	require.NoError(t, err)

	delivery := send.Delivery(now)

	assert.Equal(t, "alice", delivery.Sender)
	assert.Equal(t, "bob", delivery.Recipient)
	assert.Nil(t, delivery.SourceTopic)
	assert.Equal(t, "status", delivery.Body)
	assert.Equal(t, wholeSecond(now), delivery.Created)
	assert.Equal(t, wholeSecond(now).Add(MessageDefaultTTL), delivery.Expires)
}

func TestTopicPublicationDeduplicatesSubscribersAndExcludesSender(t *testing.T) {
	now := time.Date(2026, time.July, 22, 12, 0, 0, 500, time.UTC)
	publish, err := NewTopicPublication("alice", "release/urgent", "deploy", time.Hour)
	require.NoError(t, err)

	deliveries := publish.Deliveries([]Subscription{
		{Listener: "carol", Pattern: "release/*"},
		{Listener: "alice", Pattern: "release/*"},
		{Listener: "bob", Pattern: "release/*"},
		{Listener: "carol", Pattern: "release/urgent"},
	}, now)

	require.Len(t, deliveries, 2)
	assert.Equal(t, []string{"carol", "bob"}, []string{
		deliveries[0].Recipient,
		deliveries[1].Recipient,
	})
	for _, delivery := range deliveries {
		require.NotNil(t, delivery.SourceTopic)
		assert.Equal(t, "release/urgent", *delivery.SourceTopic)
	}
}

func TestNewSubscriptionUpdateRejectsInvalidPathPattern(t *testing.T) {
	_, err := NewSubscriptionUpdate("alice", []string{"release/["}, nil, time.Hour)

	assert.ErrorContains(t, err, "invalid subscription pattern")
}
