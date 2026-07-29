package mail

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestService_Send(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	repository := &serviceRepository{
		sendResult: Message{
			ID: "mail_00000000000000000000000000000001", Sender: "alice", Recipient: "bob", Body: "status",
			Created: now, Expires: now.Add(MessageDefaultTTL),
		},
	}
	service := NewService(repository, fixedServiceClock{now: now})

	result, err := service.Send(t.Context(), SendRequest{
		Sender: "alice", Recipient: "bob", Body: "status",
	})

	require.NoError(t, err)
	assert.Equal(t, repository.sendResult, result)
	require.Len(t, repository.sends, 1)
	delivery := repository.sends[0].Delivery(now)
	assert.Equal(t, "alice", delivery.Sender)
	assert.Equal(t, "bob", delivery.Recipient)
	assert.Equal(t, "status", delivery.Body)
	assert.Equal(t, now.Add(MessageDefaultTTL), delivery.Expires)
}

func TestService_Send_rejectsInvalidInput(t *testing.T) {
	service := NewService(&serviceRepository{}, fixedServiceClock{})

	_, err := service.Send(t.Context(), SendRequest{Sender: "alice"})

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.ErrorContains(t, err, "recipient required")
}

func TestService_Tail_rejectsNegativeInterval(t *testing.T) {
	service := NewService(&serviceRepository{}, fixedServiceClock{})

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
		repository := &serviceRepository{
			consumeResult: Consumed{Messages: []Message{{ID: "mail_00000000000000000000000000000003"}}},
		}
		service := NewService(repository, fixedServiceClock{now: now})

		messages, err := service.Receive(t.Context(), MailboxRequest{
			Recipient: "alice", IncludeRead: true,
			MaxAge: time.Hour, Limit: 4,
		})

		require.NoError(t, err)
		assert.Equal(t, []Message{{ID: "mail_00000000000000000000000000000003"}}, messages)
		require.Len(t, repository.consumptions, 1)
		selection := repository.consumptions[0].Selection()
		assert.Equal(t, "alice", selection.Recipient())
		assert.True(t, selection.IncludeRead())
		assert.Equal(t, now.Add(-time.Hour), selection.Since())
		assert.Equal(t, 4, selection.Limit())
		assert.False(t, repository.consumptions[0].ClearOnly())
	})

	t.Run("PeekAllRecipients", func(t *testing.T) {
		repository := &serviceRepository{peekResult: []Message{{ID: "mail_00000000000000000000000000000005"}}}
		service := NewService(repository, fixedServiceClock{now: now})

		messages, err := service.Peek(t.Context(), MailboxRequest{
			AllRecipients: true,
			IncludeRead:   true,
		})

		require.NoError(t, err)
		assert.Equal(t, []Message{{ID: "mail_00000000000000000000000000000005"}}, messages)
		require.Len(t, repository.selections, 1)
		assert.True(t, repository.selections[0].AllRecipients())
		assert.Empty(t, repository.selections[0].Recipient())
	})

	t.Run("Clear", func(t *testing.T) {
		repository := &serviceRepository{consumeResult: Consumed{Cleared: 2}}
		service := NewService(repository, fixedServiceClock{now: now})

		result, err := service.Clear(t.Context(), MailboxRequest{Recipient: "alice"})

		require.NoError(t, err)
		assert.Equal(t, ClearResult{Cleared: 2}, result)
		require.Len(t, repository.consumptions, 1)
		assert.True(t, repository.consumptions[0].ClearOnly())
	})
}

func TestServiceSubscriptions(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

	t.Run("Create", func(t *testing.T) {
		repository := &serviceRepository{
			updateResult: SubscriptionsUpdated{Subscriptions: []Subscription{{
				Listener: "alice", Pattern: "release.*",
				Created: now, Expires: now.Add(time.Hour),
			}}},
		}
		service := NewService(repository, fixedServiceClock{now: now})

		result, err := service.Subscribe(t.Context(), SubscriptionRequest{
			Listener: "alice", Pattern: "release.*", TTL: time.Hour,
		})

		require.NoError(t, err)
		assert.Equal(t, repository.updateResult.Subscriptions[0], result)
		require.Len(t, repository.updates, 1)
		changes := repository.updates[0].Changes(nil, now)
		require.Len(t, changes.Upserts, 1)
		assert.Equal(t, "release.*", changes.Upserts[0].Pattern)
	})

	t.Run("Refresh", func(t *testing.T) {
		repository := &serviceRepository{
			updateResult: SubscriptionsUpdated{Subscriptions: []Subscription{{
				Listener: "alice", Pattern: "release.*",
				Created: now.Add(-time.Hour), Expires: now.Add(2 * time.Hour),
			}}},
		}
		service := NewService(repository, fixedServiceClock{now: now})

		result, err := service.Subscribe(t.Context(), SubscriptionRequest{
			Listener: "alice", Pattern: "release.*", TTL: 2 * time.Hour,
		})

		require.NoError(t, err)
		assert.Equal(t, "alice", result.Listener)
		require.Len(t, repository.updates, 1)
	})

	t.Run("Remove", func(t *testing.T) {
		repository := &serviceRepository{
			updateResult: SubscriptionsUpdated{Removals: []SubscriptionRemoval{{
				Pattern: "release.*", Removed: true,
			}}},
		}
		service := NewService(repository, fixedServiceClock{now: now})

		result, err := service.RemoveSubscription(t.Context(), SubscriptionRemovalRequest{
			Listener: "alice", Pattern: "release.*",
		})

		require.NoError(t, err)
		assert.Equal(t, SubscriptionRemoval{Pattern: "release.*", Removed: true}, result)
		require.Len(t, repository.updates, 1)
	})
}

type serviceRepository struct {
	sends        []DirectSend
	publications []TopicPublication
	selections   []Selection
	consumptions []Consumption
	updates      []SubscriptionUpdate

	sendResult    Message
	publishResult []Message
	peekResult    []Message
	consumeResult Consumed
	subscriptions []Subscription
	updateResult  SubscriptionsUpdated
	updateErr     error
	afterConsume  func()
}

func (r *serviceRepository) SendMail(_ context.Context, request DirectSend) (Message, error) {
	r.sends = append(r.sends, request)
	return r.sendResult, nil
}

func (r *serviceRepository) PublishMail(_ context.Context, request TopicPublication) ([]Message, error) {
	r.publications = append(r.publications, request)
	return r.publishResult, nil
}

func (r *serviceRepository) PeekMailbox(_ context.Context, selection Selection) ([]Message, error) {
	r.selections = append(r.selections, selection)
	return r.peekResult, nil
}

func (r *serviceRepository) ConsumeMailbox(_ context.Context, consumption Consumption) (Consumed, error) {
	r.consumptions = append(r.consumptions, consumption)
	if r.afterConsume != nil {
		r.afterConsume()
	}
	return r.consumeResult, nil
}

func (r *serviceRepository) TailMailbox(context.Context, TailPageRequest) (TailPage, error) {
	return TailPage{}, nil
}

func (r *serviceRepository) ListSubscriptions(context.Context) ([]Subscription, error) {
	return r.subscriptions, nil
}

func (r *serviceRepository) UpdateSubscriptions(_ context.Context, update SubscriptionUpdate) (SubscriptionsUpdated, error) {
	r.updates = append(r.updates, update)
	return r.updateResult, r.updateErr
}

type fixedServiceClock struct{ now time.Time }

func (c fixedServiceClock) Now() time.Time { return c.now }
