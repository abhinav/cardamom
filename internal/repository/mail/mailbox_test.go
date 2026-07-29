package mail

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.abhg.dev/cardamom/internal/mail"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepository_PeekMailboxSupportsUnreadAllAndGlobalSelections(t *testing.T) {
	now := time.Date(2026, time.July, 18, 13, 0, 0, 0, time.UTC)
	repository, _, _ := openRepository(t, now)

	for _, recipient := range []string{"bob", "carol", "bob"} {
		request, err := mail.NewDirectSend("alice", recipient, "status", time.Hour)
		require.NoError(t, err)
		_, err = repository.SendMail(t.Context(), request)
		require.NoError(t, err)
	}

	firstOnly, err := mail.NewSelection(mail.SelectionRequest{Recipient: "bob", Limit: 1}, now)
	require.NoError(t, err)
	read, err := mail.Read(firstOnly)
	require.NoError(t, err)
	consumed, err := repository.ConsumeMailbox(t.Context(), read)
	require.NoError(t, err)
	assert.Equal(t, []string{testMessageID(0)}, messageIDs(consumed.Messages))
	require.NotNil(t, consumed.Messages[0].ReadAt)
	assert.Equal(t, now, *consumed.Messages[0].ReadAt)

	unread, err := mail.NewSelection(mail.SelectionRequest{Recipient: "bob"}, now)
	require.NoError(t, err)
	unreadMessages, err := repository.PeekMailbox(t.Context(), unread)
	require.NoError(t, err)
	assert.Equal(t, []string{testMessageID(2)}, messageIDs(unreadMessages))

	all, err := mail.NewSelection(mail.SelectionRequest{Recipient: "bob", IncludeRead: true}, now)
	require.NoError(t, err)
	allMessages, err := repository.PeekMailbox(t.Context(), all)
	require.NoError(t, err)
	assert.Equal(t, []string{testMessageID(0), testMessageID(2)}, messageIDs(allMessages))

	global, err := mail.NewSelection(mail.SelectionRequest{AllRecipients: true, IncludeRead: true, Limit: 2}, now)
	require.NoError(t, err)
	globalMessages, err := repository.PeekMailbox(t.Context(), global)
	require.NoError(t, err)
	assert.Equal(t, []string{testMessageID(0), testMessageID(1)}, messageIDs(globalMessages))

	afterCreation, err := mail.NewSelection(mail.SelectionRequest{
		Recipient: "bob", IncludeRead: true, MaxAge: time.Nanosecond,
	}, now.Add(2*time.Nanosecond))
	require.NoError(t, err)
	afterCreationMessages, err := repository.PeekMailbox(t.Context(), afterCreation)
	require.NoError(t, err)
	assert.Empty(t, afterCreationMessages)
}

func TestRepository_ConsumeMailboxClaimsEachUnreadDeliveryOnce(t *testing.T) {
	now := time.Date(2026, time.July, 18, 14, 0, 0, 0, time.UTC)
	repository, _, _ := openRepository(t, now)

	for range 2 {
		request, err := mail.NewDirectSend("alice", "bob", "status", time.Hour)
		require.NoError(t, err)
		_, err = repository.SendMail(t.Context(), request)
		require.NoError(t, err)
	}

	selection, err := mail.NewSelection(mail.SelectionRequest{Recipient: "bob", Limit: 1}, now)
	require.NoError(t, err)
	read, err := mail.Read(selection)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan mail.Consumed, 2)
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			result, err := repository.ConsumeMailbox(t.Context(), read)
			results <- result
			errs <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	close(errs)

	for err := range errs {
		assert.NoError(t, err)
	}
	var consumedIDs []string
	for result := range results {
		consumedIDs = append(consumedIDs, messageIDs(result.Messages)...)
	}
	slices.Sort(consumedIDs)
	assert.Equal(t, []string{testMessageID(0), testMessageID(1)}, consumedIDs)
}

func TestRepository_ConsumeMailboxClearSuppressesMessages(t *testing.T) {
	now := time.Date(2026, time.July, 18, 15, 0, 0, 0, time.UTC)
	repository, _, _ := openRepository(t, now)

	for range 2 {
		request, err := mail.NewDirectSend("alice", "bob", "status", time.Hour)
		require.NoError(t, err)
		_, err = repository.SendMail(t.Context(), request)
		require.NoError(t, err)
	}

	selection, err := mail.NewSelection(mail.SelectionRequest{Recipient: "bob", Limit: 1}, now)
	require.NoError(t, err)
	clearRequest, err := mail.Clear(selection)
	require.NoError(t, err)
	result, err := repository.ConsumeMailbox(t.Context(), clearRequest)
	require.NoError(t, err)
	assert.Empty(t, result.Messages)
	assert.Equal(t, 1, result.Cleared)

	unread, err := repository.PeekMailbox(t.Context(), selection)
	require.NoError(t, err)
	assert.Equal(t, []string{testMessageID(1)}, messageIDs(unread))
}

func TestRepository_MailExpiryBoundaryCleansRowsWithoutReusingIDs(t *testing.T) {
	now := time.Date(2026, time.July, 18, 16, 0, 0, 0, time.UTC)
	repository, persistence, clock := openRepository(t, now)

	subscribe, err := mail.NewSubscriptionUpdate(
		"bob",
		[]string{"release.*"},
		nil,
		time.Minute,
	)
	require.NoError(t, err)
	_, err = repository.UpdateSubscriptions(t.Context(), subscribe)
	require.NoError(t, err)

	first, err := mail.NewDirectSend("alice", "release.urgent", "first", time.Minute)
	require.NoError(t, err)
	firstDelivery, err := repository.SendMail(t.Context(), first)
	require.NoError(t, err)
	assert.Equal(t, testMessageID(0), firstDelivery.ID.String())

	clock.now = now.Add(time.Minute)
	second, err := mail.NewDirectSend("alice", "release.urgent", "second", time.Hour)
	require.NoError(t, err)
	secondDelivery, err := repository.SendMail(t.Context(), second)
	require.NoError(t, err)
	assert.Equal(t, testMessageID(1), secondDelivery.ID.String())

	global, err := mail.NewSelection(mail.SelectionRequest{AllRecipients: true, IncludeRead: true}, now)
	require.NoError(t, err)
	messages, err := repository.PeekMailbox(t.Context(), global)
	require.NoError(t, err)
	assert.Equal(t, []string{testMessageID(1)}, messageIDs(messages))

	subscriptions, err := repository.ListSubscriptions(t.Context())
	require.NoError(t, err)
	assert.Empty(t, subscriptions)

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	var mailboxRows, subscriptionRows int
	err = view.QueryRowContext(t.Context(), `SELECT count(*) FROM mailbox`).Scan(&mailboxRows)
	require.NoError(t, err)
	err = view.QueryRowContext(t.Context(), `SELECT count(*) FROM subscriptions`).Scan(&subscriptionRows)
	require.NoError(t, err)
	assert.Equal(t, 1, mailboxRows)
	assert.Zero(t, subscriptionRows)
	require.NoError(t, view.Done())
}

func TestRepository_TailMailboxContinuesAcrossBoundedPagesAndCleanup(t *testing.T) {
	now := time.Unix(50, 0).UTC()
	repository, persistence, clock := openRepository(t, now)
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO mailbox (
			local_sequence, id, sender, recipient, body,
			created_at, expires_at
		) VALUES
			(1, 'mail_00000000000000000000000000000001', 'captain', 'alpha', 'First', 10, 100),
			(2, 'mail_00000000000000000000000000000002', 'captain', 'beta', 'Second', 10, 100),
			(3, 'mail_00000000000000000000000000000003', 'captain', 'gamma', 'Third', 10, 200)
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	selection, err := mail.NewSelection(mail.SelectionRequest{
		AllRecipients: true, IncludeRead: true, Limit: 2,
	}, now)
	require.NoError(t, err)
	first, err := repository.TailMailbox(t.Context(), mail.TailPageRequest{Selection: selection})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"mail_00000000000000000000000000000001",
		"mail_00000000000000000000000000000002",
	}, messageIDs(first.Messages))
	assert.NotEmpty(t, first.Cursor)

	clock.now = time.Unix(100, 0).UTC()
	second, err := repository.TailMailbox(t.Context(), mail.TailPageRequest{
		Selection: selection,
		Cursor:    first.Cursor,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"mail_00000000000000000000000000000003"}, messageIDs(second.Messages))
	assert.NotEqual(t, first.Cursor, second.Cursor)

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	var rows int
	require.NoError(t, view.QueryRowContext(t.Context(), `SELECT count(*) FROM mailbox`).Scan(&rows))
	assert.Equal(t, 1, rows)
	require.NoError(t, view.Done())
}

func TestRepository_TailMailboxConsumesActorPage(t *testing.T) {
	now := time.Date(2026, time.July, 18, 19, 0, 0, 0, time.UTC)
	repository, _, _ := openRepository(t, now)
	request, err := mail.NewDirectSend("captain", "engineer", "Report", time.Hour)
	require.NoError(t, err)
	_, err = repository.SendMail(t.Context(), request)
	require.NoError(t, err)

	selection, err := mail.NewSelection(mail.SelectionRequest{Recipient: "engineer"}, now)
	require.NoError(t, err)
	page, err := repository.TailMailbox(t.Context(), mail.TailPageRequest{Selection: selection})
	require.NoError(t, err)
	require.Len(t, page.Messages, 1)
	assert.Equal(t, testMessageID(0), page.Messages[0].ID.String())
	require.NotNil(t, page.Messages[0].ReadAt)
	assert.Equal(t, now, *page.Messages[0].ReadAt)

	unread, err := repository.PeekMailbox(t.Context(), selection)
	require.NoError(t, err)
	assert.Empty(t, unread)
}

func openRepository(t *testing.T, now time.Time) (*Repository, *store.Store, *testClock) {
	t.Helper()

	persistence, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(t.TempDir(), "cardamom.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	clock := &testClock{now: now}
	return New(persistence, Config{Clock: clock, Entropy: &incrementingEntropy{}}), persistence, clock
}

type testClock struct {
	now time.Time
}

func (c *testClock) Now() time.Time { return c.now }

// incrementingEntropy gives each generated test identity one repeated byte.
type incrementingEntropy struct {
	// next is repeated through one generated identity and advances per read.
	next byte
}

func (e *incrementingEntropy) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = e.next
	}
	e.next++
	return len(buffer), nil
}

func messageIDs(messages []mail.Message) []string {
	ids := make([]string, len(messages))
	for index, message := range messages {
		ids[index] = message.ID.String()
	}
	return ids
}

func testMessageID(value byte) string {
	return "mail_" + strings.Repeat(fmt.Sprintf("%02x", value), 16)
}

func messageRecipients(messages []mail.Message) []string {
	recipients := make([]string, len(messages))
	for index, message := range messages {
		recipients[index] = message.Recipient
	}
	return recipients
}
