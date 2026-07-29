package mail

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.abhg.dev/cardamom/internal/mail"
)

func TestRepositoryDirectSendDoesNotPublishToSubscribers(t *testing.T) {
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	repository, _, _ := openRepository(t, now)
	subscription, err := mail.NewSubscriptionUpdate(
		"carol", []string{"bob"}, nil, time.Hour,
	)
	require.NoError(t, err)
	_, err = repository.UpdateSubscriptions(t.Context(), subscription)
	require.NoError(t, err)
	send, err := mail.NewDirectSend("alice", "bob", "status", time.Hour)
	require.NoError(t, err)

	delivery, err := repository.SendMail(t.Context(), send)

	require.NoError(t, err)
	assert.Equal(t, "bob", delivery.Recipient)
	selection, err := mail.NewSelection(mail.SelectionRequest{Recipient: "carol"}, now)
	require.NoError(t, err)
	carol, err := repository.PeekMailbox(t.Context(), selection)
	require.NoError(t, err)
	assert.Empty(t, carol)
}

func TestRepositoryPublishSnapshotsMatchingSubscribers(t *testing.T) {
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	repository, _, _ := openRepository(t, now)
	for _, update := range []struct {
		listener string
		patterns []string
	}{
		{listener: "bob", patterns: []string{"release/*", "release/urgent"}},
		{listener: "carol", patterns: []string{"release/*"}},
		{listener: "alice", patterns: []string{"release/*"}},
	} {
		subscription, err := mail.NewSubscriptionUpdate(
			update.listener, update.patterns, nil, time.Hour,
		)
		require.NoError(t, err)
		_, err = repository.UpdateSubscriptions(t.Context(), subscription)
		require.NoError(t, err)
	}
	publish, err := mail.NewTopicPublication(
		"alice", "release/urgent", "deploy", time.Hour,
	)
	require.NoError(t, err)

	deliveries, err := repository.PublishMail(t.Context(), publish)

	require.NoError(t, err)
	assert.Equal(t, []string{"bob", "carol"}, messageRecipients(deliveries))
	for _, delivery := range deliveries {
		require.NotNil(t, delivery.SourceTopic)
		assert.Equal(t, "release/urgent", *delivery.SourceTopic)
	}
}
