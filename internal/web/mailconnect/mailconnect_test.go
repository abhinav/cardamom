package mailconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/mail"
	repositorymail "go.abhg.dev/cardamom/internal/repository/mail"
	"go.abhg.dev/cardamom/internal/repository/store"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestServiceMailOperations(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	operations := &recordingOperations{
		message: mail.Message{
			ID:     "mail_33333333333333333333333333333333",
			Sender: "alice", Recipient: "bob", Body: "status",
			Created: now, Expires: now.Add(time.Hour),
		},
		messages: []mail.Message{{
			ID:     "mail_33333333333333333333333333333333",
			Sender: "alice", Recipient: "bob", Body: "status",
			Created: now, Expires: now.Add(time.Hour),
		}},
		cleared: mail.ClearResult{Cleared: 2},
	}
	client := newTestClient(t, operations)

	sent, err := client.SendMail(t.Context(), connect.NewRequest(&privatev1.SendMailRequest{
		Actor: "alice", Recipient: "bob", Body: "status",
		Ttl: durationpb.New(time.Hour),
	}))
	require.NoError(t, err)
	received, err := client.ReceiveMail(t.Context(), connect.NewRequest(&privatev1.ReceiveMailRequest{
		Actor: "bob", IncludeRead: true,
		MaxAge: durationpb.New(30 * time.Minute), Limit: 4,
	}))
	require.NoError(t, err)
	peeked, err := client.PeekMail(t.Context(), connect.NewRequest(&privatev1.PeekMailRequest{
		AllRecipients: true, IncludeRead: true,
	}))
	require.NoError(t, err)
	cleared, err := client.ClearMail(t.Context(), connect.NewRequest(&privatev1.ClearMailRequest{
		Actor: "bob", Limit: 2,
	}))

	require.NoError(t, err)
	require.Len(t, operations.sends, 1)
	assert.Equal(t, mail.SendRequest{
		Sender: "alice", Recipient: "bob", Body: "status", TTL: time.Hour,
	}, operations.sends[0])
	assert.Equal(t, mail.MailboxRequest{
		Recipient: "bob", IncludeRead: true,
		MaxAge: 30 * time.Minute, Limit: 4,
	}, operations.receives[0])
	assert.Equal(t, mail.MailboxRequest{
		AllRecipients: true, IncludeRead: true,
	}, operations.peeks[0])
	assert.Equal(t, mail.MailboxRequest{Recipient: "bob", Limit: 2}, operations.clears[0])
	assert.Equal(t, "mail_33333333333333333333333333333333", sent.Msg.GetMessage().GetId())
	assert.Equal(t, "mail_33333333333333333333333333333333", received.Msg.GetMessages()[0].GetId())
	assert.Equal(t, "mail_33333333333333333333333333333333", peeked.Msg.GetMessages()[0].GetId())
	assert.Equal(t, int64(2), cleared.Msg.GetCleared())
}

func TestServiceSubscriptionOperations(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	operations := &recordingOperations{
		subscription: mail.Subscription{
			Listener: "alice", Pattern: "release.*",
			Created: now, Expires: now.Add(time.Hour),
		},
		subscriptions: []mail.Subscription{{
			Listener: "alice", Pattern: "release.*",
			Created: now, Expires: now.Add(time.Hour),
		}},
		removal: mail.SubscriptionRemoval{Pattern: "release.*", Removed: true},
	}
	client := newTestClient(t, operations)

	created, err := client.Subscribe(t.Context(), connect.NewRequest(&privatev1.SubscribeRequest{
		Actor: "alice", Pattern: "release.*", Ttl: durationpb.New(time.Hour),
	}))
	require.NoError(t, err)
	listed, err := client.ListSubscriptions(t.Context(), connect.NewRequest(&privatev1.ListSubscriptionsRequest{}))
	require.NoError(t, err)
	removed, err := client.RemoveSubscription(t.Context(), connect.NewRequest(&privatev1.RemoveSubscriptionRequest{
		Actor: "alice", Pattern: "release.*",
	}))

	require.NoError(t, err)
	assert.Equal(t, time.Hour, operations.creates[0].TTL)
	assert.Equal(t, mail.SubscriptionRemovalRequest{
		Listener: "alice", Pattern: "release.*",
	}, operations.removals[0])
	assert.Equal(t, "alice", created.Msg.GetSubscription().GetListener())
	assert.Equal(t, "release.*", listed.Msg.GetSubscriptions()[0].GetPattern())
	assert.True(t, removed.Msg.GetRemoved())
}

func TestServiceMailOperations_preservePersistedDomainOutcomes(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{now: now}
	persistence, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(t.TempDir(), "cardamom.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	operations := mail.NewService(repositorymail.New(persistence, repositorymail.Config{
		Clock: clock,
	}), clock)
	client := newTestClient(t, operations)

	_, err = client.Subscribe(t.Context(), connect.NewRequest(&privatev1.SubscribeRequest{
		Actor: "carol", Pattern: "release/*", Ttl: durationpb.New(time.Hour),
	}))
	require.NoError(t, err)
	published, err := client.PublishMail(t.Context(), connect.NewRequest(&privatev1.PublishMailRequest{
		Actor: "alice", Topic: "release/ready", Body: "deploy",
	}))
	require.NoError(t, err)
	received, err := client.ReceiveMail(t.Context(), connect.NewRequest(&privatev1.ReceiveMailRequest{
		Actor: "carol",
	}))
	require.NoError(t, err)
	unread, err := operations.Peek(t.Context(), mail.MailboxRequest{Recipient: "carol"})

	require.NoError(t, err)
	assert.Len(t, published.Msg.GetMessages(), 1)
	require.Len(t, received.Msg.GetMessages(), 1)
	assert.Equal(t, "carol", received.Msg.GetMessages()[0].GetRecipient())
	assert.Equal(t, "release/ready", received.Msg.GetMessages()[0].GetSourceTopic())
	assert.Empty(t, unread)
}

type recordingOperations struct {
	sends     []mail.SendRequest
	publishes []mail.PublishRequest
	receives  []mail.MailboxRequest
	peeks     []mail.MailboxRequest
	clears    []mail.MailboxRequest
	creates   []mail.SubscriptionRequest
	removals  []mail.SubscriptionRemovalRequest

	message       mail.Message
	messages      []mail.Message
	cleared       mail.ClearResult
	subscription  mail.Subscription
	subscriptions []mail.Subscription
	removal       mail.SubscriptionRemoval
}

func (o *recordingOperations) Send(_ context.Context, request mail.SendRequest) (mail.Message, error) {
	o.sends = append(o.sends, request)
	return o.message, nil
}

func (o *recordingOperations) Publish(_ context.Context, request mail.PublishRequest) ([]mail.Message, error) {
	o.publishes = append(o.publishes, request)
	return o.messages, nil
}

func (o *recordingOperations) Receive(_ context.Context, request mail.MailboxRequest) ([]mail.Message, error) {
	o.receives = append(o.receives, request)
	return o.messages, nil
}

func (o *recordingOperations) Peek(_ context.Context, request mail.MailboxRequest) ([]mail.Message, error) {
	o.peeks = append(o.peeks, request)
	return o.messages, nil
}

func (o *recordingOperations) Clear(_ context.Context, request mail.MailboxRequest) (mail.ClearResult, error) {
	o.clears = append(o.clears, request)
	return o.cleared, nil
}

func (o *recordingOperations) Subscribe(_ context.Context, request mail.SubscriptionRequest) (mail.Subscription, error) {
	o.creates = append(o.creates, request)
	return o.subscription, nil
}

func (o *recordingOperations) ListSubscriptions(context.Context) ([]mail.Subscription, error) {
	return o.subscriptions, nil
}

func (o *recordingOperations) RemoveSubscription(_ context.Context, request mail.SubscriptionRemovalRequest) (mail.SubscriptionRemoval, error) {
	o.removals = append(o.removals, request)
	return o.removal, nil
}

func newTestClient(t *testing.T, operations Operations) privatev1connect.MailServiceClient {
	t.Helper()
	_, handler := privatev1connect.NewMailServiceHandler(New(operations))
	client := &http.Client{Transport: &testHandlerTransport{handler: handler}}
	return privatev1connect.NewMailServiceClient(client, "http://cardamom.test")
}

type testHandlerTransport struct{ handler http.Handler }

func (t *testHandlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
