// Package mailconnect exposes store-scoped mail and subscriptions through
// Connect.
package mailconnect

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/mail"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:generate go tool mockgen -destination mocks_test.go -package mailconnect -typed -write_package_comment=false . Operations

// Operations supplies the store-scoped mail and subscription behavior exposed
// by MailService.
type Operations interface {
	// Send persists one direct actor delivery.
	Send(context.Context, mail.SendRequest) (mail.Message, error)

	// Publish persists one delivery per matching topic subscriber.
	Publish(context.Context, mail.PublishRequest) ([]mail.Message, error)

	// Receive consumes selected messages in one actor mailbox.
	Receive(context.Context, mail.MailboxRequest) ([]mail.Message, error)

	// Peek reads selected messages without changing read state.
	Peek(context.Context, mail.MailboxRequest) ([]mail.Message, error)

	// Clear marks selected unread messages read without returning them.
	Clear(context.Context, mail.MailboxRequest) (mail.ClearResult, error)

	// Subscribe creates or refreshes one listener registration.
	Subscribe(context.Context, mail.SubscriptionRequest) (mail.Subscription, error)

	// ListSubscriptions returns every active subscription in domain order.
	ListSubscriptions(context.Context) ([]mail.Subscription, error)

	// RemoveSubscription removes one listener-owned registration.
	RemoveSubscription(
		context.Context,
		mail.SubscriptionRemovalRequest,
	) (mail.SubscriptionRemoval, error)
}

var _ Operations = (*mail.Service)(nil)

// Service adapts store-scoped mail operations to generated MailService RPCs.
type Service struct {
	privatev1connect.UnimplementedMailServiceHandler
	operations Operations
}

var _ privatev1connect.MailServiceHandler = (*Service)(nil)

// New constructs a MailService handler from store-scoped domain operations.
func New(operations Operations) *Service {
	must.NotBeNilf(operations, "mailconnect: mail operations are required")
	return &Service{operations: operations}
}

// SendMail validates transport values and returns the direct delivery.
func (s *Service) SendMail(
	ctx context.Context,
	request *connect.Request[privatev1.SendMailRequest],
) (*connect.Response[privatev1.SendMailResponse], error) {
	ttl, err := duration("ttl", request.Msg.GetTtl())
	if err != nil {
		return nil, web.FromError(err)
	}
	message, err := s.operations.Send(ctx, mail.SendRequest{
		Sender: request.Msg.GetActor(), Recipient: request.Msg.GetRecipient(),
		Body: request.Msg.GetBody(), TTL: ttl,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.SendMailResponse{
		Message: mailMessage(message),
	}), nil
}

// PublishMail validates transport values and returns subscriber deliveries.
func (s *Service) PublishMail(
	ctx context.Context,
	request *connect.Request[privatev1.PublishMailRequest],
) (*connect.Response[privatev1.PublishMailResponse], error) {
	ttl, err := duration("ttl", request.Msg.GetTtl())
	if err != nil {
		return nil, web.FromError(err)
	}
	messages, err := s.operations.Publish(ctx, mail.PublishRequest{
		Sender: request.Msg.GetActor(), Topic: request.Msg.GetTopic(),
		Body: request.Msg.GetBody(), TTL: ttl,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.PublishMailResponse{
		Messages: mailMessages(messages),
	}), nil
}

// ReceiveMail consumes selected messages in one actor mailbox.
func (s *Service) ReceiveMail(
	ctx context.Context,
	request *connect.Request[privatev1.ReceiveMailRequest],
) (*connect.Response[privatev1.ReceiveMailResponse], error) {
	maxAge, err := duration("max_age", request.Msg.GetMaxAge())
	if err != nil {
		return nil, web.FromError(err)
	}
	messages, err := s.operations.Receive(ctx, mail.MailboxRequest{
		Recipient: request.Msg.GetActor(), IncludeRead: request.Msg.GetIncludeRead(),
		MaxAge: maxAge, Limit: int(request.Msg.GetLimit()),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ReceiveMailResponse{
		Messages: mailMessages(messages),
	}), nil
}

// PeekMail reads selected messages without changing read state.
func (s *Service) PeekMail(
	ctx context.Context,
	request *connect.Request[privatev1.PeekMailRequest],
) (*connect.Response[privatev1.PeekMailResponse], error) {
	maxAge, err := duration("max_age", request.Msg.GetMaxAge())
	if err != nil {
		return nil, web.FromError(err)
	}
	messages, err := s.operations.Peek(ctx, mail.MailboxRequest{
		Recipient:     request.Msg.GetActor(),
		AllRecipients: request.Msg.GetAllRecipients(),
		IncludeRead:   request.Msg.GetIncludeRead(),
		MaxAge:        maxAge, Limit: int(request.Msg.GetLimit()),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.PeekMailResponse{
		Messages: mailMessages(messages),
	}), nil
}

// ClearMail marks selected unread messages read without returning them.
func (s *Service) ClearMail(
	ctx context.Context,
	request *connect.Request[privatev1.ClearMailRequest],
) (*connect.Response[privatev1.ClearMailResponse], error) {
	maxAge, err := duration("max_age", request.Msg.GetMaxAge())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := s.operations.Clear(ctx, mail.MailboxRequest{
		Recipient: request.Msg.GetActor(), MaxAge: maxAge,
		Limit: int(request.Msg.GetLimit()),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ClearMailResponse{
		Cleared: int64(result.Cleared),
	}), nil
}

// Subscribe creates or refreshes one actor-owned registration.
func (s *Service) Subscribe(
	ctx context.Context,
	request *connect.Request[privatev1.SubscribeRequest],
) (*connect.Response[privatev1.SubscribeResponse], error) {
	ttl, err := duration("ttl", request.Msg.GetTtl())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := s.operations.Subscribe(ctx, mail.SubscriptionRequest{
		Listener: request.Msg.GetActor(), Pattern: request.Msg.GetPattern(), TTL: ttl,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.SubscribeResponse{
		Subscription: subscription(result),
	}), nil
}

// ListSubscriptions returns every active store subscription.
func (s *Service) ListSubscriptions(
	ctx context.Context,
	_ *connect.Request[privatev1.ListSubscriptionsRequest],
) (*connect.Response[privatev1.ListSubscriptionsResponse], error) {
	values, err := s.operations.ListSubscriptions(ctx)
	if err != nil {
		return nil, web.FromError(err)
	}
	result := make([]*privatev1.Subscription, len(values))
	for index, value := range values {
		result[index] = subscription(value)
	}
	return connect.NewResponse(&privatev1.ListSubscriptionsResponse{
		Subscriptions: result,
	}), nil
}

// RemoveSubscription removes one actor-owned registration.
func (s *Service) RemoveSubscription(
	ctx context.Context,
	request *connect.Request[privatev1.RemoveSubscriptionRequest],
) (*connect.Response[privatev1.RemoveSubscriptionResponse], error) {
	result, err := s.operations.RemoveSubscription(
		ctx,
		mail.SubscriptionRemovalRequest{
			Listener: request.Msg.GetActor(), Pattern: request.Msg.GetPattern(),
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.RemoveSubscriptionResponse{
		Pattern: result.Pattern, Removed: result.Removed,
	}), nil
}

func duration(name string, value *durationpb.Duration) (time.Duration, error) {
	if value == nil {
		return 0, nil
	}
	if err := value.CheckValid(); err != nil {
		return 0, errkind.Errorf(errkind.InvalidInput, "invalid input: %s: %v", name, err)
	}
	return value.AsDuration(), nil
}

func mailMessages(values []mail.Message) []*privatev1.MailMessage {
	result := make([]*privatev1.MailMessage, len(values))
	for index, value := range values {
		result[index] = mailMessage(value)
	}
	return result
}

func mailMessage(value mail.Message) *privatev1.MailMessage {
	result := &privatev1.MailMessage{
		Id: value.ID.String(), Sender: value.Sender, Recipient: value.Recipient,
		Body: value.Body, CreatedAt: timestamppb.New(value.Created),
		ExpiresAt: timestamppb.New(value.Expires),
	}
	if value.ReadAt != nil {
		result.ReadAt = timestamppb.New(*value.ReadAt)
	}
	result.SourceTopic = value.SourceTopic
	return result
}

func subscription(value mail.Subscription) *privatev1.Subscription {
	return &privatev1.Subscription{
		Listener: value.Listener, Pattern: value.Pattern,
		CreatedAt: timestamppb.New(value.Created),
		ExpiresAt: timestamppb.New(value.Expires),
	}
}
