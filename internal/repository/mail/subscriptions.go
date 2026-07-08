package mail

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/mail"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ListSubscriptions returns every active subscription in listener and pattern
// order.
func (r *Repository) ListSubscriptions(ctx context.Context) (_ []mail.Subscription, err error) {
	view, err := r.persistence.View(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subscription view: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	rows, err := query.New(view).MailListActiveSubscriptions(
		ctx,
		wholeSecond(r.clock.Now()),
	)
	if err != nil {
		return nil, fmt.Errorf("select active subscriptions: %w", err)
	}
	return loadSubscriptions(rows), nil
}

// UpdateSubscriptions atomically removes and refreshes one listener's
// subscriptions.
func (r *Repository) UpdateSubscriptions(ctx context.Context, update mail.SubscriptionUpdate) (_ mail.SubscriptionsUpdated, err error) {
	change, err := r.persistence.Change(ctx)
	if err != nil {
		return mail.SubscriptionsUpdated{}, fmt.Errorf("begin subscription update: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()

	now := wholeSecond(r.clock.Now())
	if err := cleanupExpired(ctx, change, now); err != nil {
		return mail.SubscriptionsUpdated{}, err
	}
	queries := query.New(change)
	rows, err := queries.MailListListenerSubscriptions(
		ctx,
		query.MailListListenerSubscriptionsParams{
			Listener: update.Listener(),
			Now:      now,
		},
	)
	if err != nil {
		return mail.SubscriptionsUpdated{}, fmt.Errorf("select listener subscriptions: %w", err)
	}
	current := loadSubscriptions(rows)
	changes := update.Changes(current, now)

	for _, removal := range changes.Removals {
		if !removal.Removed {
			continue
		}
		result, err := queries.MailRemoveSubscription(
			ctx,
			query.MailRemoveSubscriptionParams{
				Listener: update.Listener(),
				Pattern:  removal.Pattern,
			},
		)
		if err != nil {
			return mail.SubscriptionsUpdated{}, fmt.Errorf("remove subscription %q: %w", removal.Pattern, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return mail.SubscriptionsUpdated{}, fmt.Errorf("inspect removed subscription %q: %w", removal.Pattern, err)
		}
		if changed != 1 {
			return mail.SubscriptionsUpdated{}, fmt.Errorf("remove subscription %q: changed %d rows", removal.Pattern, changed)
		}
	}

	updated := make([]mail.Subscription, 0, len(changes.Upserts))
	for _, subscription := range changes.Upserts {
		createdAt, err := queries.MailRefreshSubscription(
			ctx,
			query.MailRefreshSubscriptionParams{
				Listener:  subscription.Listener,
				Pattern:   subscription.Pattern,
				CreatedAt: subscription.Created,
				ExpiresAt: subscription.Expires,
			},
		)
		if err != nil {
			return mail.SubscriptionsUpdated{}, fmt.Errorf("refresh subscription %q: %w", subscription.Pattern, err)
		}
		subscription.Created = createdAt
		updated = append(updated, subscription)
	}

	if err := change.Commit(); err != nil {
		return mail.SubscriptionsUpdated{}, fmt.Errorf("commit subscription update: %w", err)
	}
	return mail.SubscriptionsUpdated{
		Subscriptions: updated,
		Removals:      changes.Removals,
	}, nil
}

func loadSubscriptions(rows []query.Subscription) []mail.Subscription {
	subscriptions := make([]mail.Subscription, 0, len(rows))
	for _, row := range rows {
		subscriptions = append(subscriptions, mail.Subscription{
			Listener: row.Listener,
			Pattern:  row.Pattern,
			Created:  row.CreatedAt,
			Expires:  row.ExpiresAt,
		})
	}
	return subscriptions
}
