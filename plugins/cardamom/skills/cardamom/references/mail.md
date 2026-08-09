# Send ephemeral coordination mail

Cardamom mail is an expiring attention channel for asynchronous coordination
within one store.
It does not create issue custody,
assignment, readiness, dependency, waiting,
or durable recovery context.

Use mail when a Cardamom workflow needs to notify a known actor,
publish an event to interested subscribers, or operate a store-wide observer
across boards.
The collaboration runtime may already provide dispatch and notification,
so delegation alone does not require Cardamom mail.

Mailboxes and topic subscriptions are store-scoped.
Resolve the store but do not select a board solely for mail.
Separate stores do not exchange messages.
Copy every material decision, active position, and outcome into the issue record
that owns it.

## Notify one actor

Send a short expiring message to an actor mailbox:

```bash
card --actor <actor> mail send <recipient-actor> \
  'Issue cm-abcd is ready for directed continuation.'
```

Receive unread messages and mark them read:

```bash
card --actor <recipient-actor> mail recv
```

Use `mail recv --peek` to inspect without changing read state.
For a multiline message, pass `-` as the body and use a single-quoted heredoc
so shell metacharacters remain literal.

## Publish and observe topics

Use a topic when producers should not need to know every interested actor.
Subscribe the receiving actor before publishing:

```bash
card --actor <observer-actor> mail subscribe 'release.*' --ttl 30m
card --actor <publisher-actor> mail publish release.ready \
  'A release checkpoint is ready for inspection.'
card --actor <observer-actor> mail recv
```

Subscription patterns and lifetimes belong to each actor.
Running `mail subscribe` again refreshes the subscription TTL.
Inspect or remove subscriptions with `mail subscriptions` and
`mail unsubscribe`.

Use `mail recv --tail` for a continuously running receiver.
Tailing receives deliveries but does not renew topic subscriptions.
A receiver that outlives its subscription TTL must refresh the subscription
from another process before expiry.

Messages expire independently of issue history.
Use mail to draw attention to durable work,
not to carry the only copy of information another executor must recover.
