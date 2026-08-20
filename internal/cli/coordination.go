package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/lease"
	"go.abhg.dev/cardamom/internal/mail"
)

// MailOperations supplies the store-scoped mail and subscription operations
// used by coordination commands.
type MailOperations interface {
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

	// Tail polls one mailbox selection until cancellation.
	Tail(context.Context, mail.TailRequest, mail.Sink) error

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

var _ MailOperations = (*mail.Service)(nil)

// LeaseOperations supplies the store-scoped resource lease operations used by
// coordination commands.
type LeaseOperations interface {
	// Acquire acquires an absent or expired resource lease.
	Acquire(context.Context, lease.AcquireRequest) (lease.Lease, error)

	// Renew extends an active owner-held resource lease.
	Renew(context.Context, lease.RenewRequest) (lease.Lease, error)

	// Release removes an active owner-held resource lease.
	Release(context.Context, lease.ReleaseRequest) (lease.Lease, error)

	// Revoke removes an active resource lease held by the expected owner.
	Revoke(context.Context, lease.RevokeRequest) (lease.Revocation, error)

	// Get returns one active resource lease.
	Get(context.Context, lease.GetRequest) (lease.Lease, error)

	// List returns every active resource lease in domain order.
	List(context.Context) ([]lease.Lease, error)
}

type mailCommand struct {
	Send          mailSendCommand          `cmd:"" help:"Send an expiring message."`
	Publish       mailPublishCommand       `cmd:"" help:"Publish an expiring topic message."`
	Recv          mailRecvCommand          `cmd:"" help:"Receive messages."`
	Subscribe     mailSubscribeCommand     `cmd:"" help:"Subscribe to a topic pattern."`
	Subscriptions mailSubscriptionsCommand `cmd:"" help:"List active subscriptions."`
	Unsubscribe   mailUnsubscribeCommand   `cmd:"" help:"Remove a topic subscription."`
}

// Help explains the mail command family independently of the coordination
// skill.
func (*mailCommand) Help() string {
	return "Send actor mail, publish topics, and receive ephemeral deliveries."
}

type mailSendCommand struct {
	Recipient string        `arg:"" predictor:"actor" help:"Actor mailbox receiving the message."`
	Body      *string       `arg:"" optional:"" help:"Message body. Use - or omit with piped input to read standard input."`
	TTL       time.Duration `name:"ttl" placeholder:"DURATION" help:"Message lifetime in Go duration syntax, such as 30m or 24h. Defaults to 7d and is capped at 30d."`
}

// Run sends one attributed message to an actor mailbox.
func (cmd *mailSendCommand) Run(
	invocation *Invocation,
	operations MailOperations,
) error {
	body, provided, err := invocation.Markdown.Read(cmd.Body)
	if err != nil {
		return err
	}
	if !provided {
		return UsageErrorf("message body required")
	}

	message, err := operations.Send(invocation.Context, mail.SendRequest{
		Sender: invocation.Actor, Recipient: cmd.Recipient,
		Body: body, TTL: cmd.TTL,
	})
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return fmt.Errorf("send mail: %w", err)
	}
	return renderMailMessages(invocation.Output, []mail.Message{message})
}

type mailPublishCommand struct {
	Topic string        `arg:"" help:"Literal topic matched against active subscriptions."`
	Body  *string       `arg:"" optional:"" help:"Message body. Use - or omit with piped input to read standard input."`
	TTL   time.Duration `name:"ttl" placeholder:"DURATION" help:"Message lifetime in Go duration syntax, such as 30m or 24h. Defaults to 7d and is capped at 30d."`
}

// Run publishes one attributed message to active matching subscribers.
func (cmd *mailPublishCommand) Run(
	invocation *Invocation,
	operations MailOperations,
) error {
	body, provided, err := invocation.Markdown.Read(cmd.Body)
	if err != nil {
		return err
	}
	if !provided {
		return UsageErrorf("message body required")
	}

	messages, err := operations.Publish(invocation.Context, mail.PublishRequest{
		Sender: invocation.Actor, Topic: cmd.Topic, Body: body, TTL: cmd.TTL,
	})
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return fmt.Errorf("publish mail: %w", err)
	}
	return renderMailMessages(invocation.Output, messages)
}

type mailRecvCommand struct {
	Peek     bool          `help:"Read messages without marking them read."`
	All      bool          `help:"Include messages that were already read."`
	Clear    bool          `help:"Mark unread messages read without printing them."`
	Global   bool          `help:"Observe messages across all recipients without changing read state."`
	Tail     bool          `help:"Poll until canceled and emit each delivery once per session."`
	Age      time.Duration `placeholder:"DURATION" help:"Include messages no older than this Go duration, such as 30m or 24h. Zero has no age limit."`
	Limit    int           `placeholder:"COUNT" help:"Return at most this many messages per read. Zero has no count limit."`
	Interval time.Duration `placeholder:"DURATION" help:"Delay between tail polls in Go duration syntax. Values below 1s use 1s."`
}

// Help explains mailbox consumption and global observation semantics.
func (*mailRecvCommand) Help() string {
	return "Read unread actor mail by default. Global observation is always read-only."
}

// Run selects one finite mailbox operation or delegates a tail session to the
// domain poller.
func (cmd *mailRecvCommand) Run(
	invocation *Invocation,
	operations MailOperations,
) error {
	if err := cmd.validate(); err != nil {
		return err
	}

	recipient := invocation.Actor
	if cmd.Global {
		recipient = ""
	}
	request := mail.MailboxRequest{
		Recipient: recipient, AllRecipients: cmd.Global,
		IncludeRead: cmd.All, MaxAge: cmd.Age, Limit: cmd.Limit,
	}

	if cmd.Tail {
		return operations.Tail(
			invocation.Context,
			mail.TailRequest{
				Mailbox: request, Interval: cmd.Interval,
			},
			mailOutputSink{output: invocation.Output},
		)
	}
	if cmd.Clear {
		result, err := operations.Clear(invocation.Context, request)
		if err != nil {
			if errkind.Of(err) == errkind.InvalidInput {
				return UsageErrorf("%s", err)
			}
			return fmt.Errorf("clear mailbox: %w", err)
		}
		return renderMailCleared(invocation.Output, result.Cleared)
	}
	if cmd.Peek || cmd.Global {
		messages, err := operations.Peek(invocation.Context, request)
		if err != nil {
			if errkind.Of(err) == errkind.InvalidInput {
				return UsageErrorf("%s", err)
			}
			return fmt.Errorf("read mailbox: %w", err)
		}
		return renderMailMessages(invocation.Output, messages)
	}

	messages, err := operations.Receive(invocation.Context, request)
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return fmt.Errorf("receive mail: %w", err)
	}
	return renderMailMessages(invocation.Output, messages)
}

func (cmd *mailRecvCommand) validate() error {
	if cmd.Interval < 0 {
		return UsageErrorf("--interval must be greater than or equal to zero")
	}
	if cmd.Clear && (cmd.Peek || cmd.All || cmd.Global || cmd.Tail) {
		return UsageErrorf("--clear cannot be combined with --peek, --all, --global, or --tail")
	}
	if cmd.Tail && (cmd.Peek || cmd.All) {
		return UsageErrorf("--tail cannot be combined with --peek or --all")
	}
	if !cmd.Tail && cmd.Interval != 0 {
		return UsageErrorf("--interval requires --tail")
	}
	return nil
}

// mailOutputSink renders each completed polling batch after the repository
// operation has released its persistence scope.
type mailOutputSink struct {
	output *Output
}

func (s mailOutputSink) DeliverMail(_ context.Context, batch mail.Batch) error {
	return renderMailMessages(s.output, batch.Messages)
}

type mailSubscribeCommand struct {
	Pattern string        `arg:"" help:"Filepath-style topic glob to create or refresh."`
	TTL     time.Duration `name:"ttl" placeholder:"DURATION" default:"15m" help:"Subscription lifetime in Go duration syntax, such as 30m or 24h. Defaults to 15m and is capped at 7d."`
}

// Help explains that subscribing refreshes rather than consumes state.
func (*mailSubscribeCommand) Help() string {
	return "Create or refresh the current actor's expiring topic subscription."
}

// Run creates or refreshes one subscription owned by the invocation actor.
func (cmd *mailSubscribeCommand) Run(
	invocation *Invocation,
	operations MailOperations,
) error {
	result, err := operations.Subscribe(invocation.Context, mail.SubscriptionRequest{
		Listener: invocation.Actor, Pattern: cmd.Pattern, TTL: cmd.TTL,
	})
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return fmt.Errorf("update subscription: %w", err)
	}
	return renderMailSubscription(invocation.Output, result)
}

type mailSubscriptionsCommand struct{}

// Help explains the global active-subscription listing.
func (*mailSubscriptionsCommand) Help() string {
	return "List active subscriptions for every listener."
}

// Run lists all active subscriptions in repository order.
func (*mailSubscriptionsCommand) Run(
	invocation *Invocation,
	operations MailOperations,
) error {
	subscriptions, err := operations.ListSubscriptions(invocation.Context)
	if err != nil {
		return fmt.Errorf("list subscriptions: %w", err)
	}
	return renderMailSubscriptions(invocation.Output, subscriptions)
}

type mailUnsubscribeCommand struct {
	Pattern string `arg:"" predictor:"subscription" help:"Exact topic pattern owned by the current actor."`
}

// Help explains the actor ownership boundary for subscription removal.
func (*mailUnsubscribeCommand) Help() string {
	return "Remove only the current actor's matching topic subscription."
}

// Run removes one subscription owned by the invocation actor.
func (cmd *mailUnsubscribeCommand) Run(
	invocation *Invocation,
	operations MailOperations,
) error {
	result, err := operations.RemoveSubscription(
		invocation.Context,
		mail.SubscriptionRemovalRequest{
			Listener: invocation.Actor, Pattern: cmd.Pattern,
		},
	)
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return fmt.Errorf("remove subscription: %w", err)
	}
	return renderMailRemoval(invocation.Output, result)
}

type mailMessageRecord struct {
	ID          mail.ID    `json:"id"`
	Sender      string     `json:"sender"`
	Recipient   string     `json:"recipient"`
	SourceTopic *string    `json:"source_topic"`
	Body        string     `json:"body"`
	Created     time.Time  `json:"created"`
	Expires     time.Time  `json:"expires"`
	ReadAt      *time.Time `json:"read_at"`
}

func renderMailMessages(output *Output, messages []mail.Message) error {
	records := make([]mailMessageRecord, len(messages))
	for index, message := range messages {
		records[index] = mailMessageRecord{
			ID: message.ID, Sender: message.Sender, Recipient: message.Recipient,
			SourceTopic: message.SourceTopic, Body: message.Body,
			Created: message.Created, Expires: message.Expires,
			ReadAt: message.ReadAt,
		}
	}
	if output.JSON() {
		return WriteJSONLines(output, records)
	}
	for _, record := range records {
		sourceTopic := ""
		if record.SourceTopic != nil {
			sourceTopic = *record.SourceTopic
		}
		header := fmt.Sprintf(
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			record.ID,
			record.Sender,
			record.Recipient,
			sourceTopic,
			record.Created.Format(time.RFC3339),
			record.Expires.Format(time.RFC3339),
		)
		if err := output.WriteString(header + record.Body); err != nil {
			return err
		}
		if !strings.HasSuffix(record.Body, "\n") {
			if err := output.WriteString("\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

type mailClearedRecord struct {
	Cleared int `json:"cleared"`
}

func renderMailCleared(output *Output, cleared int) error {
	if output.JSON() {
		return output.WriteJSON(mailClearedRecord{Cleared: cleared})
	}
	return output.Noticef("cleared %d unread messages", cleared)
}

type mailSubscriptionRecord struct {
	Listener string    `json:"listener"`
	Pattern  string    `json:"pattern"`
	Created  time.Time `json:"created"`
	Expires  time.Time `json:"expires"`
}

func renderMailSubscription(output *Output, subscription mail.Subscription) error {
	record := newMailSubscriptionRecord(subscription)
	if output.JSON() {
		return output.WriteJSON(record)
	}
	return output.WriteString(formatMailSubscription(record))
}

func renderMailSubscriptions(output *Output, subscriptions []mail.Subscription) error {
	records := make([]mailSubscriptionRecord, len(subscriptions))
	for index, subscription := range subscriptions {
		records[index] = newMailSubscriptionRecord(subscription)
	}
	if output.JSON() {
		return WriteJSONLines(output, records)
	}
	for _, record := range records {
		if err := output.WriteString(formatMailSubscription(record)); err != nil {
			return err
		}
	}
	return nil
}

func newMailSubscriptionRecord(subscription mail.Subscription) mailSubscriptionRecord {
	return mailSubscriptionRecord{
		Listener: subscription.Listener, Pattern: subscription.Pattern,
		Created: subscription.Created, Expires: subscription.Expires,
	}
}

func formatMailSubscription(record mailSubscriptionRecord) string {
	return fmt.Sprintf(
		"%s\t%s\t%s\t%s\n",
		record.Listener,
		record.Pattern,
		record.Created.Format(time.RFC3339),
		record.Expires.Format(time.RFC3339),
	)
}

type mailRemovalRecord struct {
	Pattern string `json:"pattern"`
	Removed bool   `json:"removed"`
}

func renderMailRemoval(output *Output, removal mail.SubscriptionRemoval) error {
	record := mailRemovalRecord{Pattern: removal.Pattern, Removed: removal.Removed}
	if output.JSON() {
		return output.WriteJSON(record)
	}
	if removal.Removed {
		return output.Noticef("removed subscription %q", removal.Pattern)
	}
	return output.Noticef("no active subscription %q", removal.Pattern)
}

type leaseCommand struct {
	Acquire leaseAcquireCommand `cmd:"" help:"Acquire a resource lease."`
	Renew   leaseRenewCommand   `cmd:"" help:"Renew a resource lease."`
	Release leaseReleaseCommand `cmd:"" help:"Release a resource lease."`
	Revoke  leaseRevokeCommand  `cmd:"" help:"Revoke a resource lease."`
	Show    leaseShowCommand    `cmd:"" help:"Show an active resource lease."`
	List    leaseListCommand    `cmd:"" help:"List active resource leases."`
}

// Help distinguishes resource leases from issue custody.
func (*leaseCommand) Help() string {
	return "Coordinate time-limited ownership of named external resources."
}

type leaseAcquireCommand struct {
	Name string        `arg:"" predictor:"lease" help:"External resource name."`
	TTL  time.Duration `name:"ttl" placeholder:"DURATION" required:"" help:"Positive lifetime in Go duration syntax, such as 30m or 24h."`
}

// Help explains lease acquisition eligibility and attribution.
func (*leaseAcquireCommand) Help() string {
	return "Acquire an absent or expired resource lease for the current actor."
}

// Run requests lease acquisition for the invocation actor.
func (cmd *leaseAcquireCommand) Run(
	invocation *Invocation,
	operations LeaseOperations,
) error {
	result, err := operations.Acquire(invocation.Context, lease.AcquireRequest{
		Name: cmd.Name, Owner: invocation.Actor, TTL: cmd.TTL,
	})
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return presentLeaseError("acquire", cmd.Name, err)
	}
	return renderLease(invocation.Output, result)
}

type leaseRenewCommand struct {
	Name string        `arg:"" predictor:"lease" help:"Active resource lease owned by the current actor."`
	TTL  time.Duration `name:"ttl" placeholder:"DURATION" required:"" help:"Positive lifetime from renewal in Go duration syntax, such as 30m or 24h."`
}

// Help explains lease renewal ownership and expiry semantics.
func (*leaseRenewCommand) Help() string {
	return "Extend the current actor's active lease from the current time."
}

// Run requests lease renewal for the invocation actor.
func (cmd *leaseRenewCommand) Run(
	invocation *Invocation,
	operations LeaseOperations,
) error {
	result, err := operations.Renew(invocation.Context, lease.RenewRequest{
		Name: cmd.Name, Owner: invocation.Actor, TTL: cmd.TTL,
	})
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return presentLeaseError("renew", cmd.Name, err)
	}
	return renderLease(invocation.Output, result)
}

type leaseReleaseCommand struct {
	Name string `arg:"" predictor:"lease" help:"Active resource lease owned by the current actor."`
}

// Help explains lease release ownership.
func (*leaseReleaseCommand) Help() string {
	return "Release the current actor's active resource lease."
}

// Run requests lease release for the invocation actor.
func (cmd *leaseReleaseCommand) Run(
	invocation *Invocation,
	operations LeaseOperations,
) error {
	result, err := operations.Release(invocation.Context, lease.ReleaseRequest{
		Name: cmd.Name, Owner: invocation.Actor,
	})
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return presentLeaseError("release", cmd.Name, err)
	}
	return renderLease(invocation.Output, result)
}

type leaseRevokeCommand struct {
	Name   string `arg:"" predictor:"lease" help:"Active resource lease to revoke."`
	Owner  string `name:"owner" placeholder:"ACTOR" predictor:"actor" required:"" help:"Expected active lease owner."`
	Reason string `name:"reason" placeholder:"REASON" required:"" help:"Required operation context for coordinator recovery."`
}

// Help explains the conditional coordinator recovery operation.
func (*leaseRevokeCommand) Help() string {
	return "Revoke an active lease only when its owner matches --owner. " +
		"This removes only Cardamom coordination state; " +
		"it does not establish external resource cleanup. " +
		"The returned reason and revocation context are transient; " +
		"Cardamom does not persist them as audit history."
}

// Run requests lease revocation attributed to the invocation actor.
func (cmd *leaseRevokeCommand) Run(
	invocation *Invocation,
	operations LeaseOperations,
) error {
	result, err := operations.Revoke(invocation.Context, lease.RevokeRequest{
		Name: cmd.Name, Owner: cmd.Owner,
		RevokedBy: invocation.Actor, Reason: cmd.Reason,
	})
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return presentLeaseError("revoke", cmd.Name, err)
	}
	return renderLeaseRevocation(invocation.Output, result)
}

type leaseShowCommand struct {
	Name string `arg:"" predictor:"lease" help:"Active resource lease to inspect."`
}

// Help explains the active lease projection.
func (*leaseShowCommand) Help() string {
	return "Show one active lease, including its owner and expiration time."
}

// Run reads and renders one active lease.
func (cmd *leaseShowCommand) Run(
	invocation *Invocation,
	operations LeaseOperations,
) error {
	result, err := operations.Get(invocation.Context, lease.GetRequest{Name: cmd.Name})
	if err != nil {
		if errkind.Of(err) == errkind.InvalidInput {
			return UsageErrorf("%s", err)
		}
		return fmt.Errorf("read lease %q: %w", cmd.Name, err)
	}
	return renderLease(invocation.Output, result)
}

type leaseListCommand struct{}

// Help explains active-only lease listing and ordering.
func (*leaseListCommand) Help() string {
	return "List active leases in resource-name order."
}

// Run lists and renders active leases in repository order.
func (*leaseListCommand) Run(
	invocation *Invocation,
	operations LeaseOperations,
) error {
	leases, err := operations.List(invocation.Context)
	if err != nil {
		return fmt.Errorf("list leases: %w", err)
	}
	return renderLeases(invocation.Output, leases)
}

func presentLeaseError(
	operation string,
	name string,
	err error,
) error {
	if _, ok := errors.AsType[*lease.HeldError](err); ok {
		return err
	}
	return fmt.Errorf("%s lease %q: %w", operation, name, err)
}

func renderLease(output *Output, value lease.Lease) error {
	if output.JSON() {
		return output.WriteJSON(value)
	}
	return output.WriteString(formatLease(value))
}

func renderLeaseRevocation(output *Output, value lease.Revocation) error {
	if output.JSON() {
		return output.WriteJSON(value)
	}
	return output.WriteString(fmt.Sprintf(
		"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
		value.Lease.Name,
		value.Lease.Owner,
		value.Lease.AcquiredAt.Format(time.RFC3339),
		value.Lease.ExpiresAt.Format(time.RFC3339),
		value.RevokedBy,
		value.RevokedAt.Format(time.RFC3339),
		value.Reason,
	))
}

func renderLeases(output *Output, values []lease.Lease) error {
	if output.JSON() {
		return WriteJSONLines(output, values)
	}
	for _, value := range values {
		if err := output.WriteString(formatLease(value)); err != nil {
			return err
		}
	}
	return nil
}

func formatLease(value lease.Lease) string {
	return fmt.Sprintf(
		"%s\t%s\t%s\t%s\n",
		value.Name,
		value.Owner,
		value.AcquiredAt.Format(time.RFC3339),
		value.ExpiresAt.Format(time.RFC3339),
	)
}
