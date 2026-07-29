package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/lease"
	"go.abhg.dev/cardamom/internal/mail"
)

func TestMailSendCommandAttributesAndRendersDirectDelivery(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	mailOperations := &recordingMailOperations{
		sendResult: mail.Message{
			ID:     "mail_11111111111111111111111111111111",
			Sender: "alice", Recipient: "bob",
			Body: "ship it", Created: now, Expires: now.Add(2 * time.Hour),
		},
	}

	stdout, stderr, exit := runCoordinationCommand(
		t,
		strings.NewReader(""),
		mailOperations,
		&recordingLeaseOperations{},
		"--actor", "alice", "--json",
		"mail", "send", "bob", "ship it", "--ttl", "2h",
	)

	assert.Equal(t, ExitSuccess, exit)
	require.Len(t, mailOperations.sends, 1)
	request := mailOperations.sends[0]
	assert.Equal(t, "alice", request.Sender)
	assert.Equal(t, "bob", request.Recipient)
	assert.Equal(t, "ship it", request.Body)
	assert.Equal(t, 2*time.Hour, request.TTL)
	assert.Equal(t,
		"{\"id\":\"mail_11111111111111111111111111111111\",\"sender\":\"alice\",\"recipient\":\"bob\",\"source_topic\":null,\"body\":\"ship it\",\"created\":\"2026-07-18T12:00:00Z\",\"expires\":\"2026-07-18T14:00:00Z\",\"read_at\":null}\n",
		stdout,
	)
	assert.Empty(t, stderr)
}

func TestMailSendCommand_readsBodyFromStandardInputAndUsesDomainTTLDefault(t *testing.T) {
	mailOperations := &recordingMailOperations{}

	_, stderr, exit := runCoordinationCommand(
		t,
		strings.NewReader("status from stdin\n"),
		mailOperations,
		&recordingLeaseOperations{},
		"--actor", "alice", "mail", "send", "bob", "-",
	)

	assert.Equal(t, ExitSuccess, exit)
	require.Len(t, mailOperations.sends, 1)
	assert.Equal(t, "status from stdin\n", mailOperations.sends[0].Body)
	assert.Zero(t, mailOperations.sends[0].TTL)
	assert.Empty(t, stderr)
}

func TestMailPublishCommandBuildsTopicPublication(t *testing.T) {
	topic := "release/ready"
	mailOperations := &recordingMailOperations{
		publishResult: []mail.Message{{
			ID:     "mail_22222222222222222222222222222222",
			Sender: "alice", Recipient: "bob", SourceTopic: &topic,
		}},
	}

	stdout, stderr, exit := runCoordinationCommand(
		t,
		strings.NewReader(""),
		mailOperations,
		&recordingLeaseOperations{},
		"--actor", "alice", "--json",
		"mail", "publish", topic, "deploy", "--ttl", "2h",
	)

	assert.Equal(t, ExitSuccess, exit)
	require.Len(t, mailOperations.publishes, 1)
	assert.Equal(t, mail.PublishRequest{
		Sender: "alice", Topic: topic, Body: "deploy", TTL: 2 * time.Hour,
	}, mailOperations.publishes[0])
	assert.Contains(t, stdout, `"source_topic":"release/ready"`)
	assert.Empty(t, stderr)
}

func TestMailRecvCommand_modesBuildDomainRequests(t *testing.T) {
	t.Run("DefaultConsumesActorMailbox", func(t *testing.T) {
		mailOperations := &recordingMailOperations{
			consumeResult: mail.Consumed{Messages: []mail.Message{{ID: "mail_33333333333333333333333333333333"}}},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			mailOperations,
			&recordingLeaseOperations{},
			"--actor", "alice", "--json", "mail", "recv", "--age", "1h", "--limit", "4",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, mailOperations.receives, 1)
		request := mailOperations.receives[0]
		assert.Equal(t, "alice", request.Recipient)
		assert.False(t, request.AllRecipients)
		assert.False(t, request.IncludeRead)
		assert.Equal(t, time.Hour, request.MaxAge)
		assert.Equal(t, 4, request.Limit)
		assert.Equal(t, "{\"id\":\"mail_33333333333333333333333333333333\",\"sender\":\"\",\"recipient\":\"\",\"source_topic\":null,\"body\":\"\",\"created\":\"0001-01-01T00:00:00Z\",\"expires\":\"0001-01-01T00:00:00Z\",\"read_at\":null}\n", stdout)
		assert.Empty(t, stderr)
	})

	t.Run("PeekIncludesReadWithoutConsumption", func(t *testing.T) {
		mailOperations := &recordingMailOperations{}

		_, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			mailOperations,
			&recordingLeaseOperations{},
			"--actor", "alice", "mail", "recv", "--peek", "--all",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, mailOperations.peeks, 1)
		assert.Equal(t, "alice", mailOperations.peeks[0].Recipient)
		assert.True(t, mailOperations.peeks[0].IncludeRead)
		assert.Empty(t, mailOperations.receives)
		assert.Empty(t, stderr)
	})

	t.Run("GlobalIsReadOnly", func(t *testing.T) {
		mailOperations := &recordingMailOperations{}

		_, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			mailOperations,
			&recordingLeaseOperations{},
			"--actor", "alice", "mail", "recv", "--global",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, mailOperations.peeks, 1)
		assert.True(t, mailOperations.peeks[0].AllRecipients)
		assert.Empty(t, mailOperations.peeks[0].Recipient)
		assert.Empty(t, mailOperations.receives)
		assert.Empty(t, stderr)
	})

	t.Run("ClearConsumesWithoutMessages", func(t *testing.T) {
		mailOperations := &recordingMailOperations{
			consumeResult: mail.Consumed{Cleared: 3},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			mailOperations,
			&recordingLeaseOperations{},
			"--actor", "alice", "--json", "mail", "recv", "--clear",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, mailOperations.clears, 1)
		assert.Equal(t, "{\"cleared\":3}\n", stdout)
		assert.Empty(t, stderr)
	})
}

func TestMailRecvCommand_rejectsUnsafeModeCombinations(t *testing.T) {
	tests := []struct {
		name string
		give []string
	}{
		{name: "ClearAndGlobal", give: []string{"--clear", "--global"}},
		{name: "ClearAndPeek", give: []string{"--clear", "--peek"}},
		{name: "ClearAndTail", give: []string{"--clear", "--tail"}},
		{name: "PeekAndTail", give: []string{"--peek", "--tail"}},
		{name: "AllAndTail", give: []string{"--all", "--tail"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"mail", "recv"}, tt.give...)
			stdout, stderr, exit := runCoordinationCommand(
				t,
				strings.NewReader(""),
				&recordingMailOperations{},
				&recordingLeaseOperations{},
				args...,
			)

			assert.Equal(t, ExitUsage, exit)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "error:")
		})
	}
}

func TestMailRecvCommand_tailUsesInvocationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	mailOperations := &recordingMailOperations{
		consumeResult: mail.Consumed{Messages: []mail.Message{{ID: "mail_77777777777777777777777777777777"}}},
		afterConsume:  cancel,
	}
	var stdout, stderr bytes.Buffer
	app, err := newCoordinationTestApplication(
		&stdout,
		&stderr,
		strings.NewReader(""),
		mailOperations,
		&recordingLeaseOperations{},
	)
	require.NoError(t, err)

	exit := app.Run(ctx, []string{"--actor", "alice", "--json", "mail", "recv", "--tail"})

	assert.Equal(t, ExitCanceled, exit)
	assert.Equal(t, "{\"id\":\"mail_77777777777777777777777777777777\",\"sender\":\"\",\"recipient\":\"\",\"source_topic\":null,\"body\":\"\",\"created\":\"0001-01-01T00:00:00Z\",\"expires\":\"0001-01-01T00:00:00Z\",\"read_at\":null}\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestMailSubscriptionCommands(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)

	t.Run("SubscribeUsesActorAndDefaultTTL", func(t *testing.T) {
		mailOperations := &recordingMailOperations{
			updateResult: mail.SubscriptionsUpdated{Subscriptions: []mail.Subscription{
				{Listener: "alice", Pattern: "release.*", Created: now, Expires: now.Add(15 * time.Minute)},
			}},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			mailOperations,
			&recordingLeaseOperations{},
			"--actor", "alice", "--json", "mail", "subscribe", "release.*",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, mailOperations.creates, 1)
		assert.Equal(t, "alice", mailOperations.creates[0].Listener)
		assert.Equal(t, "release.*", mailOperations.creates[0].Pattern)
		assert.Equal(t, 15*time.Minute, mailOperations.creates[0].TTL)
		assert.Equal(t, "{\"listener\":\"alice\",\"pattern\":\"release.*\",\"created\":\"2026-07-18T12:00:00Z\",\"expires\":\"2026-07-18T12:15:00Z\"}\n", stdout)
		assert.Empty(t, stderr)
	})

	t.Run("SubscriptionsAreJSONLines", func(t *testing.T) {
		mailOperations := &recordingMailOperations{
			subscriptions: []mail.Subscription{
				{Listener: "alice", Pattern: "release.*", Expires: now},
				{Listener: "bob", Pattern: "build.*", Expires: now.Add(time.Minute)},
			},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			mailOperations,
			&recordingLeaseOperations{},
			"--json", "mail", "subscriptions",
		)

		assert.Equal(t, ExitSuccess, exit)
		assert.Equal(t,
			"{\"listener\":\"alice\",\"pattern\":\"release.*\",\"created\":\"0001-01-01T00:00:00Z\",\"expires\":\"2026-07-18T12:00:00Z\"}\n"+
				"{\"listener\":\"bob\",\"pattern\":\"build.*\",\"created\":\"0001-01-01T00:00:00Z\",\"expires\":\"2026-07-18T12:01:00Z\"}\n",
			stdout,
		)
		assert.Empty(t, stderr)
	})

	t.Run("UnsubscribeReportsRemoval", func(t *testing.T) {
		mailOperations := &recordingMailOperations{
			updateResult: mail.SubscriptionsUpdated{Removals: []mail.SubscriptionRemoval{
				{Pattern: "release.*", Removed: true},
			}},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			mailOperations,
			&recordingLeaseOperations{},
			"--actor", "alice", "--json", "mail", "unsubscribe", "release.*",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, mailOperations.removals, 1)
		assert.Equal(t, "alice", mailOperations.removals[0].Listener)
		assert.Equal(t, "{\"pattern\":\"release.*\",\"removed\":true}\n", stdout)
		assert.Empty(t, stderr)
	})
}

func TestLeaseCommands_attributeRequestsAndFrameResults(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)

	t.Run("Acquire", func(t *testing.T) {
		leaseOperations := &recordingLeaseOperations{
			acquireResult: lease.Lease{
				Name: "staging-db", Owner: "alice", AcquiredAt: now,
				ExpiresAt: now.Add(30 * time.Minute),
			},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			&recordingMailOperations{},
			leaseOperations,
			"--actor", "alice", "--json", "lease", "acquire", "staging-db", "--ttl", "30m",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, leaseOperations.acquisitions, 1)
		assert.Equal(t, "staging-db", leaseOperations.acquisitions[0].Name)
		assert.Equal(t, "alice", leaseOperations.acquisitions[0].Owner)
		assert.Equal(t, 30*time.Minute, leaseOperations.acquisitions[0].TTL)
		assert.Equal(t, "{\"name\":\"staging-db\",\"owner\":\"alice\",\"acquired_at\":\"2026-07-18T12:00:00Z\",\"expires_at\":\"2026-07-18T12:30:00Z\"}\n", stdout)
		assert.Empty(t, stderr)
	})

	t.Run("ListUsesJSONLines", func(t *testing.T) {
		leaseOperations := &recordingLeaseOperations{listResult: []lease.Lease{
			{Name: "device-a", Owner: "alice", ExpiresAt: now},
			{Name: "staging-db", Owner: "bob", ExpiresAt: now.Add(time.Hour)},
		}}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			&recordingMailOperations{},
			leaseOperations,
			"--json", "lease", "list",
		)

		assert.Equal(t, ExitSuccess, exit)
		assert.Equal(t,
			"{\"name\":\"device-a\",\"owner\":\"alice\",\"acquired_at\":\"0001-01-01T00:00:00Z\",\"expires_at\":\"2026-07-18T12:00:00Z\"}\n"+
				"{\"name\":\"staging-db\",\"owner\":\"bob\",\"acquired_at\":\"0001-01-01T00:00:00Z\",\"expires_at\":\"2026-07-18T13:00:00Z\"}\n",
			stdout,
		)
		assert.Empty(t, stderr)
	})

	t.Run("Renew", func(t *testing.T) {
		leaseOperations := &recordingLeaseOperations{
			renewResult: lease.Lease{
				Name: "staging-db", Owner: "alice", AcquiredAt: now.Add(-time.Hour),
				ExpiresAt: now.Add(10 * time.Minute),
			},
		}

		_, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			&recordingMailOperations{},
			leaseOperations,
			"--actor", "alice", "lease", "renew", "staging-db", "--ttl", "10m",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, leaseOperations.renewals, 1)
		assert.Equal(t, "staging-db", leaseOperations.renewals[0].Name)
		assert.Equal(t, "alice", leaseOperations.renewals[0].Owner)
		assert.Equal(t, 10*time.Minute, leaseOperations.renewals[0].TTL)
		assert.Empty(t, stderr)
	})

	t.Run("Release", func(t *testing.T) {
		leaseOperations := &recordingLeaseOperations{
			releaseResult: lease.Lease{
				Name: "staging-db", Owner: "alice", AcquiredAt: now.Add(-time.Hour),
				ExpiresAt: now.Add(time.Minute),
			},
		}

		_, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			&recordingMailOperations{},
			leaseOperations,
			"--actor", "alice", "lease", "release", "staging-db",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, leaseOperations.releases, 1)
		assert.Equal(t, "staging-db", leaseOperations.releases[0].Name)
		assert.Equal(t, "alice", leaseOperations.releases[0].Owner)
		assert.Empty(t, stderr)
	})

	t.Run("Revoke", func(t *testing.T) {
		leaseOperations := &recordingLeaseOperations{
			revokeResult: lease.Revocation{
				Lease: lease.Lease{
					Name: "staging-db", Owner: "worker-a",
					AcquiredAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Minute),
				},
				RevokedBy: "coordinator", Reason: "owner cannot continue",
				RevokedAt: now,
			},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			&recordingMailOperations{},
			leaseOperations,
			"--actor", "coordinator", "--json", "lease", "revoke", "staging-db",
			"--owner", "worker-a", "--reason", "owner cannot continue",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, leaseOperations.revocations, 1)
		assert.Equal(t, lease.RevokeRequest{
			Name: "staging-db", Owner: "worker-a",
			RevokedBy: "coordinator", Reason: "owner cannot continue",
		}, leaseOperations.revocations[0])
		assert.Equal(
			t,
			"{\"lease\":{\"name\":\"staging-db\",\"owner\":\"worker-a\",\"acquired_at\":\"2026-07-18T11:00:00Z\",\"expires_at\":\"2026-07-18T12:01:00Z\"},\"revoked_by\":\"coordinator\",\"reason\":\"owner cannot continue\",\"revoked_at\":\"2026-07-18T12:00:00Z\"}\n",
			stdout,
		)
		assert.Empty(t, stderr)
	})

	t.Run("RevokePlain", func(t *testing.T) {
		leaseOperations := &recordingLeaseOperations{
			revokeResult: lease.Revocation{
				Lease: lease.Lease{
					Name: "staging-db", Owner: "worker-a",
					AcquiredAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Minute),
				},
				RevokedBy: "coordinator", Reason: "owner cannot continue",
				RevokedAt: now,
			},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			&recordingMailOperations{},
			leaseOperations,
			"--actor", "coordinator", "lease", "revoke", "staging-db",
			"--owner", "worker-a", "--reason", "owner cannot continue",
		)

		assert.Equal(t, ExitSuccess, exit)
		assert.Equal(
			t,
			"staging-db\tworker-a\t2026-07-18T11:00:00Z\t2026-07-18T12:01:00Z\tcoordinator\t2026-07-18T12:00:00Z\towner cannot continue\n",
			stdout,
		)
		assert.Empty(t, stderr)
	})

	t.Run("Show", func(t *testing.T) {
		leaseOperations := &recordingLeaseOperations{
			readResult: lease.Lease{
				Name: "staging-db", Owner: "alice", AcquiredAt: now,
				ExpiresAt: now.Add(time.Minute),
			},
		}

		stdout, stderr, exit := runCoordinationCommand(
			t,
			strings.NewReader(""),
			&recordingMailOperations{},
			leaseOperations,
			"--json", "lease", "show", "staging-db",
		)

		assert.Equal(t, ExitSuccess, exit)
		require.Len(t, leaseOperations.reads, 1)
		assert.Equal(t, "staging-db", leaseOperations.reads[0].Name)
		assert.Equal(t, "{\"name\":\"staging-db\",\"owner\":\"alice\",\"acquired_at\":\"2026-07-18T12:00:00Z\",\"expires_at\":\"2026-07-18T12:01:00Z\"}\n", stdout)
		assert.Empty(t, stderr)
	})
}

func TestLeaseCommand_conflictPresentsDomainHolder(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	leaseOperations := &recordingLeaseOperations{
		acquireErr: &lease.HeldError{
			Current: lease.Lease{
				Name: "staging-db", Owner: "bob",
				ExpiresAt: now.Add(10 * time.Minute),
			},
		},
	}

	stdout, stderr, exit := runCoordinationCommand(
		t,
		strings.NewReader(""),
		&recordingMailOperations{},
		leaseOperations,
		"--actor", "alice", "lease", "acquire", "staging-db", "--ttl", "30m",
	)

	assert.Equal(t, ExitOperation, exit)
	assert.Empty(t, stdout)
	assert.Equal(t, "error: lease \"staging-db\" is held by \"bob\" until 2026-07-18T12:10:00Z\n", stderr)
}

type recordingMailOperations struct {
	sends     []mail.SendRequest
	publishes []mail.PublishRequest
	receives  []mail.MailboxRequest
	peeks     []mail.MailboxRequest
	clears    []mail.MailboxRequest
	tails     []mail.TailRequest
	creates   []mail.SubscriptionRequest
	removals  []mail.SubscriptionRemovalRequest

	sendResult      mail.Message
	sendErr         error
	publishResult   []mail.Message
	publishErr      error
	peekResult      []mail.Message
	peekErr         error
	consumeResult   mail.Consumed
	consumeErr      error
	afterConsume    func()
	subscriptions   []mail.Subscription
	subscriptionErr error
	updateResult    mail.SubscriptionsUpdated
	updateErr       error
}

func (o *recordingMailOperations) Send(_ context.Context, request mail.SendRequest) (mail.Message, error) {
	o.sends = append(o.sends, request)
	return o.sendResult, o.sendErr
}

func (o *recordingMailOperations) Publish(_ context.Context, request mail.PublishRequest) ([]mail.Message, error) {
	o.publishes = append(o.publishes, request)
	return o.publishResult, o.publishErr
}

func (o *recordingMailOperations) Receive(_ context.Context, request mail.MailboxRequest) ([]mail.Message, error) {
	o.receives = append(o.receives, request)
	return o.consumeResult.Messages, o.consumeErr
}

func (o *recordingMailOperations) Peek(_ context.Context, request mail.MailboxRequest) ([]mail.Message, error) {
	o.peeks = append(o.peeks, request)
	return o.peekResult, o.peekErr
}

func (o *recordingMailOperations) Clear(_ context.Context, request mail.MailboxRequest) (mail.ClearResult, error) {
	o.clears = append(o.clears, request)
	return mail.ClearResult{Cleared: o.consumeResult.Cleared}, o.consumeErr
}

func (o *recordingMailOperations) Tail(ctx context.Context, request mail.TailRequest, sink mail.Sink) error {
	o.tails = append(o.tails, request)
	if err := sink.DeliverMail(ctx, mail.Batch{Messages: o.consumeResult.Messages}); err != nil {
		return err
	}
	if o.afterConsume != nil {
		o.afterConsume()
	}
	if o.consumeErr != nil {
		return o.consumeErr
	}
	return ctx.Err()
}

func (o *recordingMailOperations) Subscribe(_ context.Context, request mail.SubscriptionRequest) (mail.Subscription, error) {
	o.creates = append(o.creates, request)
	if len(o.updateResult.Subscriptions) == 0 {
		return mail.Subscription{}, o.updateErr
	}
	return o.updateResult.Subscriptions[0], o.updateErr
}

func (o *recordingMailOperations) ListSubscriptions(context.Context) ([]mail.Subscription, error) {
	return o.subscriptions, o.subscriptionErr
}

func (o *recordingMailOperations) RemoveSubscription(_ context.Context, request mail.SubscriptionRemovalRequest) (mail.SubscriptionRemoval, error) {
	o.removals = append(o.removals, request)
	if len(o.updateResult.Removals) == 0 {
		return mail.SubscriptionRemoval{}, o.updateErr
	}
	return o.updateResult.Removals[0], o.updateErr
}

type recordingLeaseOperations struct {
	acquisitions []lease.AcquireRequest
	renewals     []lease.RenewRequest
	releases     []lease.ReleaseRequest
	revocations  []lease.RevokeRequest
	reads        []lease.GetRequest

	acquireResult lease.Lease
	acquireErr    error
	renewResult   lease.Lease
	renewErr      error
	releaseResult lease.Lease
	releaseErr    error
	revokeResult  lease.Revocation
	revokeErr     error
	readResult    lease.Lease
	readErr       error
	listResult    []lease.Lease
	listErr       error
}

func (r *recordingLeaseOperations) Acquire(_ context.Context, request lease.AcquireRequest) (lease.Lease, error) {
	r.acquisitions = append(r.acquisitions, request)
	return r.acquireResult, r.acquireErr
}

func (r *recordingLeaseOperations) Renew(_ context.Context, request lease.RenewRequest) (lease.Lease, error) {
	r.renewals = append(r.renewals, request)
	return r.renewResult, r.renewErr
}

func (r *recordingLeaseOperations) Release(_ context.Context, request lease.ReleaseRequest) (lease.Lease, error) {
	r.releases = append(r.releases, request)
	return r.releaseResult, r.releaseErr
}

func (r *recordingLeaseOperations) Revoke(_ context.Context, request lease.RevokeRequest) (lease.Revocation, error) {
	r.revocations = append(r.revocations, request)
	return r.revokeResult, r.revokeErr
}

func (r *recordingLeaseOperations) Get(_ context.Context, request lease.GetRequest) (lease.Lease, error) {
	r.reads = append(r.reads, request)
	return r.readResult, r.readErr
}

func (r *recordingLeaseOperations) List(context.Context) ([]lease.Lease, error) {
	return r.listResult, r.listErr
}

func runCoordinationCommand(
	t *testing.T,
	stdin *strings.Reader,
	mailOperations MailOperations,
	leaseOperations LeaseOperations,
	args ...string,
) (stdout string, stderr string, exit int) {
	t.Helper()

	var stdoutBuffer, stderrBuffer bytes.Buffer
	app, err := newCoordinationTestApplication(
		&stdoutBuffer,
		&stderrBuffer,
		stdin,
		mailOperations,
		leaseOperations,
	)
	require.NoError(t, err)

	exit = app.Run(t.Context(), args)
	return stdoutBuffer.String(), stderrBuffer.String(), exit
}

func newCoordinationTestApplication(
	stdout, stderr *bytes.Buffer,
	stdin *strings.Reader,
	mailOperations MailOperations,
	leaseOperations LeaseOperations,
) (*Application, error) {
	return New(
		Config{
			Version:         "test",
			DefaultActor:    "tester",
			Stdin:           stdin,
			StdinIsTerminal: stdin.Len() == 0,
			Stdout:          stdout,
			Stderr:          stderr,
		},
		kong.BindTo(mailOperations, (*MailOperations)(nil)),
		kong.BindTo(leaseOperations, (*LeaseOperations)(nil)),
	)
}

var (
	_ MailOperations  = (*recordingMailOperations)(nil)
	_ LeaseOperations = (*recordingLeaseOperations)(nil)
)
