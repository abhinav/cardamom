package mail

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"time"

	"go.abhg.dev/cardamom/internal/mail"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// Clock supplies the current time for expiry and consumption boundaries.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// Config defines repository-owned time and identity sources.
type Config struct {
	// Clock supplies operation timestamps. Nil uses the system clock.
	Clock Clock

	// Entropy supplies mailbox delivery identities. Nil uses crypto/rand.Reader.
	Entropy io.Reader
}

// Repository owns finite mailbox and subscription persistence operations.
type Repository struct {
	persistence *store.Store // required
	clock       Clock        // required
	entropy     io.Reader    // required
}

// New constructs a Repository from its process-lifetime store and identity
// sources.
func New(persistence *store.Store, cfg Config) *Repository {
	must.NotBeNilf(persistence, "mail Store is required")
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	entropy := cfg.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	return &Repository{persistence: persistence, clock: clock, entropy: entropy}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// SendMail atomically persists one direct actor delivery.
func (r *Repository) SendMail(ctx context.Context, request mail.DirectSend) (_ mail.Message, err error) {
	change, err := r.persistence.Change(ctx)
	if err != nil {
		return mail.Message{}, fmt.Errorf("begin mail delivery: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	now := wholeSecond(r.clock.Now())
	if err := cleanupExpired(ctx, change, now); err != nil {
		return mail.Message{}, err
	}
	messages, err := r.insertDeliveries(ctx, change, []mail.Delivery{request.Delivery(now)})
	if err != nil {
		return mail.Message{}, err
	}
	if err := change.Commit(); err != nil {
		return mail.Message{}, fmt.Errorf("commit mail delivery: %w", err)
	}
	return messages[0], nil
}

// PublishMail atomically snapshots active matching subscriptions and persists
// one delivery per matching actor.
func (r *Repository) PublishMail(ctx context.Context, request mail.TopicPublication) (_ []mail.Message, err error) {
	change, err := r.persistence.Change(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin mail publication: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	now := wholeSecond(r.clock.Now())
	if err := cleanupExpired(ctx, change, now); err != nil {
		return nil, err
	}

	queries := query.New(change)
	rows, err := queries.MailSelectPublicationSubscriptions(
		ctx,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("select active subscriptions: %w", err)
	}
	subscriptions := loadSubscriptions(rows)

	matched := make([]mail.Subscription, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		matches, err := path.Match(subscription.Pattern, request.Topic())
		if err != nil {
			return nil, fmt.Errorf("match subscription pattern %q: %w", subscription.Pattern, err)
		}
		if matches {
			matched = append(matched, subscription)
		}
	}

	messages, err := r.insertDeliveries(ctx, change, request.Deliveries(matched, now))
	if err != nil {
		return nil, err
	}
	if err := change.Commit(); err != nil {
		return nil, fmt.Errorf("commit mail publication: %w", err)
	}
	return messages, nil
}

func (r *Repository) insertDeliveries(
	ctx context.Context,
	change *store.Change,
	deliveries []mail.Delivery,
) ([]mail.Message, error) {
	queries := query.New(change)
	messages := make([]mail.Message, 0, len(deliveries))
	for _, delivery := range deliveries {
		id, err := newMessageID(r.entropy)
		if err != nil {
			return nil, fmt.Errorf("generate delivery for %q: %w", delivery.Recipient, err)
		}
		err = queries.MailInsertDelivery(ctx, query.MailInsertDeliveryParams{
			ID:          id.String(),
			Sender:      delivery.Sender,
			Recipient:   delivery.Recipient,
			SourceTopic: delivery.SourceTopic,
			Body:        delivery.Body,
			CreatedAt:   delivery.Created,
			ExpiresAt:   delivery.Expires,
		})
		if err != nil {
			return nil, fmt.Errorf("insert delivery for %q: %w", delivery.Recipient, err)
		}
		messages = append(messages, mail.Message{
			ID:          id,
			Sender:      delivery.Sender,
			Recipient:   delivery.Recipient,
			SourceTopic: delivery.SourceTopic,
			Body:        delivery.Body,
			Created:     delivery.Created,
			Expires:     delivery.Expires,
		})
	}

	return messages, nil
}

// PeekMailbox selects active mailbox deliveries without changing read state.
func (r *Repository) PeekMailbox(ctx context.Context, selection mail.Selection) (_ []mail.Message, err error) {
	view, err := r.persistence.View(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin mailbox view: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	query, args := mailboxSelection(selection, wholeSecond(r.clock.Now()), 0)
	rows, err := view.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select mailbox: %w", err)
	}
	persisted, err := scanMessages(rows)
	return messageProjections(persisted), err
}

// TailMailbox reads one finite page after an opaque repository cursor.
// Actor pages consume returned unread deliveries; all-recipient pages only
// observe them.
func (r *Repository) TailMailbox(
	ctx context.Context,
	request mail.TailPageRequest,
) (_ mail.TailPage, err error) {
	change, err := r.persistence.Change(ctx)
	if err != nil {
		return mail.TailPage{}, fmt.Errorf("begin mailbox tail page: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	now := wholeSecond(r.clock.Now())
	if err := cleanupExpired(ctx, change, now); err != nil {
		return mail.TailPage{}, err
	}
	sequence, err := decodeTailCursor(request.Cursor)
	if err != nil {
		return mail.TailPage{}, err
	}
	query, args := mailboxSelection(request.Selection, now, sequence)
	rows, err := change.QueryContext(ctx, query, args...)
	if err != nil {
		return mail.TailPage{}, fmt.Errorf("select mailbox tail page: %w", err)
	}
	persisted, err := scanMessages(rows)
	if err != nil {
		return mail.TailPage{}, err
	}
	if !request.Selection.AllRecipients() {
		if _, err := consumeMessages(ctx, change, persisted, now); err != nil {
			return mail.TailPage{}, err
		}
	}
	if len(persisted) > 0 {
		sequence = persisted[len(persisted)-1].localSequence
	}
	if err := change.Commit(); err != nil {
		return mail.TailPage{}, fmt.Errorf("commit mailbox tail page: %w", err)
	}
	return mail.TailPage{
		Messages: messageProjections(persisted),
		Cursor:   mail.TailCursor(strconv.FormatInt(sequence, 10)),
	}, nil
}

// ConsumeMailbox atomically selects active deliveries and marks each selected
// unread delivery as read.
func (r *Repository) ConsumeMailbox(ctx context.Context, operation mail.Consumption) (_ mail.Consumed, err error) {
	change, err := r.persistence.Change(ctx)
	if err != nil {
		return mail.Consumed{}, fmt.Errorf("begin mailbox consumption: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	now := wholeSecond(r.clock.Now())
	if err := cleanupExpired(ctx, change, now); err != nil {
		return mail.Consumed{}, err
	}
	query, args := mailboxSelection(operation.Selection(), now, 0)
	rows, err := change.QueryContext(ctx, query, args...)
	if err != nil {
		return mail.Consumed{}, fmt.Errorf("select mailbox for consumption: %w", err)
	}
	persisted, err := scanMessages(rows)
	if err != nil {
		return mail.Consumed{}, err
	}
	consumed, err := consumeMessages(ctx, change, persisted, now)
	if err != nil {
		return mail.Consumed{}, err
	}

	if err := change.Commit(); err != nil {
		return mail.Consumed{}, fmt.Errorf("commit mailbox consumption: %w", err)
	}
	if operation.ClearOnly() {
		return mail.Consumed{Cleared: consumed}, nil
	}
	return mail.Consumed{Messages: messageProjections(persisted)}, nil
}

func mailboxSelection(
	selection mail.Selection,
	now time.Time,
	afterSequence int64,
) (string, []any) {
	query := `
		SELECT local_sequence, id, sender, recipient, source_topic, body, created_at, expires_at, read_at
		FROM mailbox
		WHERE local_sequence > ? AND expires_at > ? AND created_at >= ?
	`
	args := []any{afterSequence, now, inclusiveSecond(selection.Since())}
	if !selection.AllRecipients() {
		query += ` AND recipient = ?`
		args = append(args, selection.Recipient())
	}
	if !selection.IncludeRead() {
		query += ` AND read_at IS NULL`
	}
	query += ` ORDER BY local_sequence`
	if selection.Limit() > 0 {
		query += ` LIMIT ?`
		args = append(args, selection.Limit())
	}
	return query, args
}

// inclusiveSecond returns the earliest stored whole-second timestamp that is
// not before value.
func inclusiveSecond(value time.Time) time.Time {
	second := value.Unix()
	if value.Nanosecond() != 0 {
		second++
	}
	return time.Unix(second, 0).UTC()
}

// persistedMessage associates one stable delivery with its repository-local
// ordering and cursor position.
type persistedMessage struct {
	// localSequence is the repository-owned ordering and cursor position.
	localSequence int64

	// message is the stable delivery projection returned to domain callers.
	message mail.Message
}

func scanMessages(rows *sql.Rows) (messages []persistedMessage, err error) {
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var (
			persisted   persistedMessage
			id          string
			createdAt   time.Time
			expiresAt   time.Time
			readAt      *time.Time
			sourceTopic sql.NullString
		)
		if err := rows.Scan(
			&persisted.localSequence,
			&id,
			&persisted.message.Sender,
			&persisted.message.Recipient,
			&sourceTopic,
			&persisted.message.Body,
			&createdAt,
			&expiresAt,
			&readAt,
		); err != nil {
			return nil, fmt.Errorf("scan mailbox delivery: %w", err)
		}
		persisted.message.ID, err = mail.NewID(id)
		if err != nil {
			return nil, err
		}
		persisted.message.Created = createdAt
		if sourceTopic.Valid {
			persisted.message.SourceTopic = &sourceTopic.String
		}
		persisted.message.Expires = expiresAt
		persisted.message.ReadAt = readAt
		messages = append(messages, persisted)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select mailbox deliveries: %w", err)
	}
	return messages, nil
}

func consumeMessages(
	ctx context.Context,
	change *store.Change,
	messages []persistedMessage,
	now time.Time,
) (int, error) {
	queries := query.New(change)
	consumed := 0
	for index := range messages {
		if messages[index].message.ReadAt != nil {
			continue
		}
		persistedReadAt := now
		result, err := queries.MailAcknowledgeDelivery(
			ctx,
			query.MailAcknowledgeDeliveryParams{
				ReadAt:        &persistedReadAt,
				LocalSequence: messages[index].localSequence,
			},
		)
		if err != nil {
			return 0, fmt.Errorf("consume delivery %q: %w", messages[index].message.ID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("inspect consumed delivery %q: %w", messages[index].message.ID, err)
		}
		if changed != 1 {
			return 0, fmt.Errorf("consume delivery %q: changed %d rows", messages[index].message.ID, changed)
		}
		consumed++
		readAt := now
		messages[index].message.ReadAt = &readAt
	}
	return consumed, nil
}

func messageProjections(messages []persistedMessage) []mail.Message {
	out := make([]mail.Message, len(messages))
	for index := range messages {
		out[index] = messages[index].message
	}
	return out
}

func decodeTailCursor(cursor mail.TailCursor) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	sequence, err := strconv.ParseInt(string(cursor), 10, 64)
	if err != nil || sequence < 0 {
		return 0, fmt.Errorf("invalid mailbox tail cursor %q", cursor)
	}
	return sequence, nil
}

// cleanupExpired removes rows at or beyond their exclusive expiry boundary
// inside the caller's transaction.
func cleanupExpired(ctx context.Context, change *store.Change, now time.Time) error {
	queries := query.New(change)
	if err := queries.MailDeleteExpiredMailbox(ctx, now); err != nil {
		return fmt.Errorf("delete expired mail: %w", err)
	}
	if err := queries.MailDeleteExpiredSubscriptions(
		ctx,
		now,
	); err != nil {
		return fmt.Errorf("delete expired subscriptions: %w", err)
	}
	return nil
}

func wholeSecond(value time.Time) time.Time {
	return time.Unix(value.Unix(), 0).UTC()
}
