package mail

import (
	"context"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
)

// Repository supplies the atomic persistence operations required by Service.
type Repository interface {
	// SendMail atomically persists one direct actor delivery.
	SendMail(context.Context, DirectSend) (Message, error)

	// PublishMail atomically snapshots subscribers and persists their deliveries.
	PublishMail(context.Context, TopicPublication) ([]Message, error)

	// PeekMailbox reads selected mailbox messages without changing read state.
	PeekMailbox(context.Context, Selection) ([]Message, error)

	// ConsumeMailbox atomically reads selected messages and marks them read.
	ConsumeMailbox(context.Context, Consumption) (Consumed, error)

	// TailMailbox reads one finite page and advances an opaque cursor.
	TailMailbox(context.Context, TailPageRequest) (TailPage, error)

	// ListSubscriptions returns every active subscription in persistence order.
	ListSubscriptions(context.Context) ([]Subscription, error)

	// UpdateSubscriptions atomically applies subscription changes.
	UpdateSubscriptions(context.Context, SubscriptionUpdate) (SubscriptionsUpdated, error)
}

// Clock supplies current time for relative mailbox selections.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// Service owns store-scoped mail, mailbox, and subscription operations.
type Service struct {
	repository Repository // required
	clock      Clock      // required
}

// NewService binds mail operations to one store-scoped persistence boundary.
func NewService(repository Repository, clock Clock) *Service {
	must.NotBeNilf(repository, "mail persistence is required")
	must.NotBeNilf(clock, "mail clock is required")
	return &Service{repository: repository, clock: clock}
}

// SendRequest describes one attributed message and its requested lifetime.
type SendRequest struct {
	// Sender identifies the actor sending the message.
	Sender string

	// Recipient identifies one actor mailbox or topic.
	Recipient string

	// Body is the message text.
	Body string

	// TTL is the requested lifetime. Zero selects MessageDefaultTTL.
	TTL time.Duration
}

// Send validates and persists one direct actor delivery.
func (s *Service) Send(ctx context.Context, request SendRequest) (Message, error) {
	message, err := NewDirectSend(request.Sender, request.Recipient, request.Body, request.TTL)
	if err != nil {
		return Message{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	return s.repository.SendMail(ctx, message)
}

// PublishRequest describes one attributed topic publication.
type PublishRequest struct {
	// Sender identifies the actor publishing the message.
	Sender string

	// Topic is the literal topic matched against active subscriptions.
	Topic string

	// Body is the published message text.
	Body string

	// TTL is the requested lifetime. Zero selects MessageDefaultTTL.
	TTL time.Duration
}

// Publish snapshots matching subscribers and persists one delivery per actor.
func (s *Service) Publish(ctx context.Context, request PublishRequest) ([]Message, error) {
	publication, err := NewTopicPublication(
		request.Sender,
		request.Topic,
		request.Body,
		request.TTL,
	)
	if err != nil {
		return nil, errkind.Wrap(errkind.InvalidInput, err)
	}
	return s.repository.PublishMail(ctx, publication)
}

// MailboxRequest selects messages relative to the service clock.
type MailboxRequest struct {
	// Recipient identifies one mailbox. It must be empty when AllRecipients is
	// true and non-empty otherwise.
	Recipient string

	// AllRecipients selects every mailbox without changing read state.
	AllRecipients bool

	// IncludeRead includes messages already consumed by their recipient.
	IncludeRead bool

	// MaxAge limits messages to those created within this duration. Zero has no
	// age limit.
	MaxAge time.Duration

	// Limit caps the selected message count. Zero has no count limit.
	Limit int
}

// Receive returns selected messages and atomically marks unread results read.
func (s *Service) Receive(ctx context.Context, request MailboxRequest) ([]Message, error) {
	selection, err := s.selection(request)
	if err != nil {
		return nil, err
	}
	consumption, err := Read(selection)
	if err != nil {
		return nil, errkind.Wrap(errkind.InvalidInput, err)
	}
	result, err := s.repository.ConsumeMailbox(ctx, consumption)
	return result.Messages, err
}

// Peek returns selected messages without changing read state.
func (s *Service) Peek(ctx context.Context, request MailboxRequest) ([]Message, error) {
	selection, err := s.selection(request)
	if err != nil {
		return nil, err
	}
	return s.repository.PeekMailbox(ctx, selection)
}

// ClearResult reports how many unread messages were marked read.
type ClearResult struct {
	// Cleared is the number of messages whose read state changed.
	Cleared int
}

// Clear marks selected unread messages read without returning them.
func (s *Service) Clear(ctx context.Context, request MailboxRequest) (ClearResult, error) {
	selection, err := s.selection(request)
	if err != nil {
		return ClearResult{}, err
	}
	consumption, err := Clear(selection)
	if err != nil {
		return ClearResult{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	result, err := s.repository.ConsumeMailbox(ctx, consumption)
	return ClearResult{Cleared: result.Cleared}, err
}

// TailRequest configures an append-oriented mailbox polling session.
type TailRequest struct {
	// Mailbox selects the messages read on every poll.
	Mailbox MailboxRequest

	// Interval is the requested delay between reads. Values below one second are
	// raised to one second.
	Interval time.Duration
}

// Tail reads immediately and emits each global delivery ID at most once per
// session. Actor mailbox tailing consumes unread messages.
func (s *Service) Tail(ctx context.Context, request TailRequest, sink Sink) error {
	if request.Interval < 0 {
		return errkind.Errorf(
			errkind.InvalidInput,
			"interval must be greater than or equal to zero",
		)
	}
	selection, err := s.selection(request.Mailbox)
	if err != nil {
		return err
	}
	selection.includeRead = selection.allRecipients
	return newPoller(s.repository).poll(ctx, pollRequest{
		selection: selection,
		interval:  request.Interval,
	}, sink)
}

func (s *Service) selection(request MailboxRequest) (Selection, error) {
	selection, err := NewSelection(SelectionRequest(request), s.clock.Now())
	if err != nil {
		return Selection{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	return selection, nil
}

// SubscriptionRequest identifies one listener pattern and requested lifetime.
type SubscriptionRequest struct {
	// Listener identifies the actor receiving matching topic messages.
	Listener string

	// Pattern is the filepath-style topic glob selected by the listener.
	Pattern string

	// TTL is the positive requested lifetime.
	TTL time.Duration
}

// Subscribe creates or refreshes one actor-pattern subscription.
func (s *Service) Subscribe(
	ctx context.Context,
	request SubscriptionRequest,
) (Subscription, error) {
	return s.saveSubscription(ctx, request)
}

func (s *Service) saveSubscription(
	ctx context.Context,
	request SubscriptionRequest,
) (Subscription, error) {
	update, err := NewSubscriptionUpdate(
		request.Listener,
		[]string{request.Pattern},
		nil,
		request.TTL,
	)
	if err != nil {
		return Subscription{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	result, err := s.repository.UpdateSubscriptions(ctx, update)
	if err != nil {
		return Subscription{}, err
	}
	if len(result.Subscriptions) != 1 {
		return Subscription{}, fmt.Errorf(
			"update subscription: expected one result, got %d",
			len(result.Subscriptions),
		)
	}
	return result.Subscriptions[0], nil
}

// ListSubscriptions returns every active subscription in persistence order.
func (s *Service) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	return s.repository.ListSubscriptions(ctx)
}

// SubscriptionRemovalRequest identifies one listener-owned pattern to remove.
type SubscriptionRemovalRequest struct {
	// Listener identifies the actor that owns the subscription.
	Listener string

	// Pattern identifies the exact topic pattern to remove.
	Pattern string
}

// RemoveSubscription removes one active listener-pattern subscription.
func (s *Service) RemoveSubscription(
	ctx context.Context,
	request SubscriptionRemovalRequest,
) (SubscriptionRemoval, error) {
	update, err := NewSubscriptionUpdate(
		request.Listener,
		nil,
		[]string{request.Pattern},
		0,
	)
	if err != nil {
		return SubscriptionRemoval{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	result, err := s.repository.UpdateSubscriptions(ctx, update)
	if err != nil {
		return SubscriptionRemoval{}, err
	}
	if len(result.Removals) != 1 {
		return SubscriptionRemoval{}, fmt.Errorf(
			"remove subscription: expected one result, got %d",
			len(result.Removals),
		)
	}
	return result.Removals[0], nil
}
