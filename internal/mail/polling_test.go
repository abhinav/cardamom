package mail

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_Tail_consumesOutsideRepositoryCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		repository := newFakeRepository()
		repository.pages = []TailPage{
			{Messages: []Message{{ID: "mail_00000000000000000000000000000001", Recipient: "bob"}}, Cursor: "one"},
			{Messages: []Message{{ID: "mail_00000000000000000000000000000002", Recipient: "bob"}}, Cursor: "two"},
		}
		sink := &fakeSink{repository: repository, cancelAfter: 2, cancel: cancel}
		err := NewService(repository, fixedServiceClock{}).Tail(ctx, TailRequest{
			Mailbox:  MailboxRequest{Recipient: "bob"},
			Interval: time.Minute,
		}, sink)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, []string{"tail", "tail"}, repository.calls)
		assert.Equal(t, []string{
			"mail_00000000000000000000000000000001",
			"mail_00000000000000000000000000000002",
		}, sink.messageIDs())
		assert.False(t, sink.calledInsideRepository)
	})
}

func TestService_Tail_passesOpaqueCursorBackToRepository(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		repository := newFakeRepository()
		repository.pages = []TailPage{
			{
				Messages: []Message{
					{ID: "mail_00000000000000000000000000000003"},
					{ID: "mail_00000000000000000000000000000002"},
				},
				Cursor: "repository-page-one",
			},
			{
				Messages: []Message{{ID: "mail_00000000000000000000000000000004"}},
				Cursor:   "repository-page-two",
			},
		}
		sink := &fakeSink{repository: repository, cancelAfter: 2, cancel: cancel}
		err := NewService(repository, fixedServiceClock{}).Tail(ctx, TailRequest{
			Mailbox:  MailboxRequest{AllRecipients: true},
			Interval: time.Minute,
		}, sink)
		assert.ErrorIs(t, err, context.Canceled)
		require.Len(t, sink.batches, 2)
		assert.Equal(t, []string{
			"mail_00000000000000000000000000000003",
			"mail_00000000000000000000000000000002",
		}, messageIDs(sink.batches[0].Messages))
		assert.Equal(t, []string{"mail_00000000000000000000000000000004"}, messageIDs(sink.batches[1].Messages))
		require.Len(t, repository.tailRequests, 2)
		assert.True(t, repository.tailRequests[0].Selection.IncludeRead())
		assert.Empty(t, repository.tailRequests[0].Cursor)
		assert.Equal(t, TailCursor("repository-page-one"), repository.tailRequests[1].Cursor)
	})
}

func TestService_Tail_localSelectionIsUnreadOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		repository := newFakeRepository()
		sink := &fakeSink{repository: repository, cancelAfter: 1, cancel: cancel}
		err := NewService(repository, fixedServiceClock{}).Tail(ctx, TailRequest{
			Mailbox: MailboxRequest{Recipient: "bob", IncludeRead: true},
		}, sink)
		assert.ErrorIs(t, err, context.Canceled)
		require.Len(t, repository.tailRequests, 1)
		assert.False(t, repository.tailRequests[0].Selection.IncludeRead())
	})
}

func TestService_Tail_requiresSink(t *testing.T) {
	err := NewService(newFakeRepository(), fixedServiceClock{}).Tail(t.Context(), TailRequest{
		Mailbox: MailboxRequest{Recipient: "bob"},
	}, nil)
	assert.ErrorContains(t, err, "mail sink required")
}

func TestService_Tail_propagatesCollaboratorFailures(t *testing.T) {
	t.Run("Repository", func(t *testing.T) {
		readErr := errors.New("read failed")
		repository := newFakeRepository()
		repository.err = readErr
		sink := new(fakeSink)

		err := NewService(repository, fixedServiceClock{}).Tail(t.Context(), TailRequest{
			Mailbox: MailboxRequest{Recipient: "bob"},
		}, sink)
		assert.ErrorIs(t, err, readErr)
		assert.Equal(t, []string{"tail"}, repository.calls)
		assert.Empty(t, sink.batches)
	})

	t.Run("Sink", func(t *testing.T) {
		deliverErr := errors.New("deliver failed")
		repository := newFakeRepository()
		sink := &fakeSink{err: deliverErr}

		err := NewService(repository, fixedServiceClock{}).Tail(t.Context(), TailRequest{
			Mailbox: MailboxRequest{Recipient: "bob"},
		}, sink)
		assert.ErrorIs(t, err, deliverErr)
		assert.Equal(t, []string{"tail"}, repository.calls)
		assert.Len(t, sink.batches, 1)
	})
}

type fakeRepository struct {
	calls        []string
	inCall       bool
	err          error
	pages        []TailPage
	tailRequests []TailPageRequest
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{pages: []TailPage{{}}}
}

func (f *fakeRepository) TailMailbox(_ context.Context, request TailPageRequest) (TailPage, error) {
	f.record("tail")
	f.tailRequests = append(f.tailRequests, request)
	if f.err != nil {
		return TailPage{}, f.err
	}
	result := f.pages[0]
	if len(f.pages) > 1 {
		f.pages = f.pages[1:]
	}
	return result, nil
}

func (f *fakeRepository) ConsumeMailbox(context.Context, Consumption) (Consumed, error) {
	return Consumed{}, nil
}

func (f *fakeRepository) PeekMailbox(context.Context, Selection) ([]Message, error) {
	return nil, nil
}

func (f *fakeRepository) SendMail(context.Context, DirectSend) (Message, error) {
	return Message{}, nil
}

func (f *fakeRepository) PublishMail(context.Context, TopicPublication) ([]Message, error) {
	return nil, nil
}

func (f *fakeRepository) ListSubscriptions(context.Context) ([]Subscription, error) {
	return nil, nil
}

func (f *fakeRepository) UpdateSubscriptions(context.Context, SubscriptionUpdate) (SubscriptionsUpdated, error) {
	return SubscriptionsUpdated{}, nil
}

func (f *fakeRepository) record(call string) {
	f.inCall = true
	f.calls = append(f.calls, call)
	f.inCall = false
}

type fakeSink struct {
	repository             *fakeRepository
	batches                []Batch
	err                    error
	cancelAfter            int
	cancel                 context.CancelFunc
	calledInsideRepository bool
}

func (s *fakeSink) DeliverMail(_ context.Context, batch Batch) error {
	if s.repository != nil {
		s.calledInsideRepository = s.calledInsideRepository || s.repository.inCall
	}
	s.batches = append(s.batches, batch)
	if len(s.batches) == s.cancelAfter {
		s.cancel()
	}
	return s.err
}

func (s *fakeSink) messageIDs() []string {
	var ids []string
	for _, batch := range s.batches {
		ids = append(ids, messageIDs(batch.Messages)...)
	}
	return ids
}

func messageIDs(messages []Message) []string {
	ids := make([]string, len(messages))
	for i, message := range messages {
		ids[i] = message.ID.String()
	}
	return ids
}
