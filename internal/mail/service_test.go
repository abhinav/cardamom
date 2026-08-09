package mail

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.uber.org/mock/gomock"
)

func TestService_Send(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	want := Message{
		ID: "mail_00000000000000000000000000000001", Sender: "alice", Recipient: "bob", Body: "status",
		Created: now, Expires: now.Add(MessageDefaultTTL),
	}
	repository := NewMockRepository(gomock.NewController(t))
	var send DirectSend
	repository.EXPECT().SendMail(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request DirectSend) (Message, error) {
			send = request
			return want, nil
		},
	)
	service := NewService(repository, fixedServiceClock{now: now})

	result, err := service.Send(t.Context(), SendRequest{
		Sender: "alice", Recipient: "bob", Body: "status",
	})

	require.NoError(t, err)
	assert.Equal(t, want, result)
	delivery := send.Delivery(now)
	assert.Equal(t, "alice", delivery.Sender)
	assert.Equal(t, "bob", delivery.Recipient)
	assert.Equal(t, "status", delivery.Body)
	assert.Equal(t, now.Add(MessageDefaultTTL), delivery.Expires)
}

func TestService_Send_rejectsInvalidInput(t *testing.T) {
	service := NewService(NewMockRepository(gomock.NewController(t)), fixedServiceClock{})

	_, err := service.Send(t.Context(), SendRequest{Sender: "alice"})

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.ErrorContains(t, err, "recipient required")
}

func TestService_Tail_rejectsNegativeInterval(t *testing.T) {
	service := NewService(NewMockRepository(gomock.NewController(t)), fixedServiceClock{})

	err := service.Tail(t.Context(), TailRequest{
		Mailbox:  MailboxRequest{Recipient: "alice"},
		Interval: -time.Second,
	}, nil)

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.ErrorContains(t, err, "interval must be greater than or equal to zero")
}

func TestServiceMailboxOperations(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

	t.Run("Receive", func(t *testing.T) {
		want := Consumed{Messages: []Message{{ID: "mail_00000000000000000000000000000003"}}}
		repository := NewMockRepository(gomock.NewController(t))
		var consumption Consumption
		repository.EXPECT().ConsumeMailbox(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, request Consumption) (Consumed, error) {
				consumption = request
				return want, nil
			},
		)
		service := NewService(repository, fixedServiceClock{now: now})

		messages, err := service.Receive(t.Context(), MailboxRequest{
			Recipient: "alice", IncludeRead: true,
			MaxAge: time.Hour, Limit: 4,
		})

		require.NoError(t, err)
		assert.Equal(t, []Message{{ID: "mail_00000000000000000000000000000003"}}, messages)
		selection := consumption.Selection()
		assert.Equal(t, "alice", selection.Recipient())
		assert.True(t, selection.IncludeRead())
		assert.Equal(t, now.Add(-time.Hour), selection.Since())
		assert.Equal(t, 4, selection.Limit())
		assert.False(t, consumption.ClearOnly())
	})

	t.Run("PeekAllRecipients", func(t *testing.T) {
		want := []Message{{ID: "mail_00000000000000000000000000000005"}}
		repository := NewMockRepository(gomock.NewController(t))
		var selection Selection
		repository.EXPECT().PeekMailbox(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, request Selection) ([]Message, error) {
				selection = request
				return want, nil
			},
		)
		service := NewService(repository, fixedServiceClock{now: now})

		messages, err := service.Peek(t.Context(), MailboxRequest{
			AllRecipients: true,
			IncludeRead:   true,
		})

		require.NoError(t, err)
		assert.Equal(t, []Message{{ID: "mail_00000000000000000000000000000005"}}, messages)
		assert.True(t, selection.AllRecipients())
		assert.Empty(t, selection.Recipient())
	})

	t.Run("Clear", func(t *testing.T) {
		repository := NewMockRepository(gomock.NewController(t))
		var consumption Consumption
		repository.EXPECT().ConsumeMailbox(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, request Consumption) (Consumed, error) {
				consumption = request
				return Consumed{Cleared: 2}, nil
			},
		)
		service := NewService(repository, fixedServiceClock{now: now})

		result, err := service.Clear(t.Context(), MailboxRequest{Recipient: "alice"})

		require.NoError(t, err)
		assert.Equal(t, ClearResult{Cleared: 2}, result)
		assert.True(t, consumption.ClearOnly())
	})
}

func TestServiceSubscriptions(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

	t.Run("Create", func(t *testing.T) {
		want := SubscriptionsUpdated{Subscriptions: []Subscription{{
			Listener: "alice", Pattern: "release.*",
			Created: now, Expires: now.Add(time.Hour),
		}}}
		repository := NewMockRepository(gomock.NewController(t))
		var update SubscriptionUpdate
		repository.EXPECT().UpdateSubscriptions(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, request SubscriptionUpdate) (SubscriptionsUpdated, error) {
				update = request
				return want, nil
			},
		)
		service := NewService(repository, fixedServiceClock{now: now})

		result, err := service.Subscribe(t.Context(), SubscriptionRequest{
			Listener: "alice", Pattern: "release.*", TTL: time.Hour,
		})

		require.NoError(t, err)
		assert.Equal(t, want.Subscriptions[0], result)
		changes := update.Changes(nil, now)
		require.Len(t, changes.Upserts, 1)
		assert.Equal(t, "release.*", changes.Upserts[0].Pattern)
	})

	t.Run("Refresh", func(t *testing.T) {
		want := SubscriptionsUpdated{Subscriptions: []Subscription{{
			Listener: "alice", Pattern: "release.*",
			Created: now.Add(-time.Hour), Expires: now.Add(2 * time.Hour),
		}}}
		repository := NewMockRepository(gomock.NewController(t))
		repository.EXPECT().UpdateSubscriptions(gomock.Any(), gomock.Any()).Return(want, nil)
		service := NewService(repository, fixedServiceClock{now: now})

		result, err := service.Subscribe(t.Context(), SubscriptionRequest{
			Listener: "alice", Pattern: "release.*", TTL: 2 * time.Hour,
		})

		require.NoError(t, err)
		assert.Equal(t, "alice", result.Listener)
	})

	t.Run("Remove", func(t *testing.T) {
		want := SubscriptionsUpdated{Removals: []SubscriptionRemoval{{
			Pattern: "release.*", Removed: true,
		}}}
		repository := NewMockRepository(gomock.NewController(t))
		repository.EXPECT().UpdateSubscriptions(gomock.Any(), gomock.Any()).Return(want, nil)
		service := NewService(repository, fixedServiceClock{now: now})

		result, err := service.RemoveSubscription(t.Context(), SubscriptionRemovalRequest{
			Listener: "alice", Pattern: "release.*",
		})

		require.NoError(t, err)
		assert.Equal(t, SubscriptionRemoval{Pattern: "release.*", Removed: true}, result)
	})
}

type fixedServiceClock struct{ now time.Time }

func (c fixedServiceClock) Now() time.Time { return c.now }
