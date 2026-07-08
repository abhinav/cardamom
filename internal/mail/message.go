package mail

import (
	"errors"
	"time"
)

const (
	// MessageDefaultTTL is the lifetime used when a sender omits a TTL.
	MessageDefaultTTL = 7 * 24 * time.Hour

	// MessageMaxTTL is the longest lifetime accepted for ephemeral mail.
	MessageMaxTTL = 30 * 24 * time.Hour
)

// Message is one persisted delivery in a recipient's ephemeral mailbox.
type Message struct {
	// ID identifies the persisted delivery.
	ID ID

	// Sender identifies who sent the message.
	Sender string

	// Recipient identifies the mailbox that received this delivery.
	Recipient string

	// SourceTopic identifies the publication that created this delivery.
	// Nil identifies direct actor mail.
	SourceTopic *string

	// Body is the delivered message text.
	Body string

	// Created is the delivery time.
	Created time.Time

	// Expires is the exclusive lifetime boundary.
	Expires time.Time

	// ReadAt is the consuming read time. Nil means the message remains unread.
	ReadAt *time.Time
}

type messageDraft struct {
	sender string
	body   string
	ttl    time.Duration
}

func newMessageDraft(sender, body string, ttl time.Duration) (messageDraft, error) {
	switch {
	case sender == "":
		return messageDraft{}, errors.New("sender required")
	case body == "":
		return messageDraft{}, errors.New("body required")
	case ttl < 0:
		return messageDraft{}, errors.New("ttl must be greater than or equal to zero")
	case ttl == 0:
		ttl = MessageDefaultTTL
	default:
		ttl = min(ttl, MessageMaxTTL)
	}
	return messageDraft{sender: sender, body: body, ttl: ttl}, nil
}

// DirectSend is a validated message addressed to one actor mailbox.
type DirectSend struct {
	draft     messageDraft
	recipient string
}

// NewDirectSend validates one direct actor message.
func NewDirectSend(sender, recipient, body string, ttl time.Duration) (DirectSend, error) {
	if recipient == "" {
		return DirectSend{}, errors.New("recipient required")
	}
	draft, err := newMessageDraft(sender, body, ttl)
	if err != nil {
		return DirectSend{}, err
	}
	return DirectSend{draft: draft, recipient: recipient}, nil
}

// Delivery returns the direct mailbox delivery at the supplied time.
func (s DirectSend) Delivery(now time.Time) Delivery {
	return s.draft.delivery(s.recipient, nil, now)
}

// TopicPublication is a validated message published to a literal topic.
type TopicPublication struct {
	draft messageDraft
	topic string
}

// NewTopicPublication validates one topic publication.
func NewTopicPublication(sender, topic, body string, ttl time.Duration) (TopicPublication, error) {
	if topic == "" {
		return TopicPublication{}, errors.New("topic required")
	}
	draft, err := newMessageDraft(sender, body, ttl)
	if err != nil {
		return TopicPublication{}, err
	}
	return TopicPublication{draft: draft, topic: topic}, nil
}

// Topic returns the literal topic matched against active subscriptions.
func (p TopicPublication) Topic() string { return p.topic }

// Delivery is one mailbox row in a message fanout.
type Delivery struct {
	// Sender identifies who sent the message.
	Sender string

	// Recipient identifies the mailbox receiving this delivery.
	Recipient string

	// SourceTopic identifies the publication that produced this delivery.
	// Nil identifies direct actor mail.
	SourceTopic *string

	// Body is the delivered message text.
	Body string

	// Created is the delivery time at whole-second precision.
	Created time.Time

	// Expires is the exclusive lifetime boundary.
	Expires time.Time
}

// Deliveries returns one ordered delivery per matching actor.
// Repeated matching subscriptions are deduplicated and the sender is excluded.
func (p TopicPublication) Deliveries(matched []Subscription, now time.Time) []Delivery {
	now = wholeSecond(now)
	deliveries := make([]Delivery, 0, len(matched))
	seen := map[string]struct{}{p.draft.sender: {}}
	for _, subscription := range matched {
		if _, exists := seen[subscription.Listener]; exists {
			continue
		}
		seen[subscription.Listener] = struct{}{}
		deliveries = append(
			deliveries,
			p.draft.delivery(subscription.Listener, &p.topic, now),
		)
	}
	return deliveries
}

func (d messageDraft) delivery(recipient string, sourceTopic *string, now time.Time) Delivery {
	now = wholeSecond(now)
	return Delivery{
		Sender: d.sender, Recipient: recipient, SourceTopic: sourceTopic,
		Body: d.body, Created: now, Expires: now.Add(d.ttl),
	}
}

func wholeSecond(value time.Time) time.Time {
	return time.Unix(value.Unix(), 0).UTC()
}
