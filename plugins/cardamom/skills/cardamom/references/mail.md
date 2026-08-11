# Send ephemeral coordination mail

Cardamom mail is a store-scoped, expiring attention channel.
It creates no issue custody, assignment, readiness, dependency, waiting,
or durable recovery context.
Use the collaboration runtime instead when it already supplies the needed
dispatch or notification.

Use mail to notify a known actor or publish to interested observers:

```bash
card --actor <actor> mail send <recipient-actor> \
  'Issue cm-abcd is ready for directed continuation.'
card --actor <recipient-actor> mail recv
```

Use a topic when the producer should not know every observer:

```bash
card --actor <observer-actor> mail subscribe 'release.*' --ttl 30m
card --actor <publisher-actor> mail publish release.ready \
  'A release checkpoint is ready for inspection.'
card --actor <observer-actor> mail recv
```

Subscriptions and messages expire independently of issue history.
The receiving actor owns subscription renewal.
`mail recv --tail` receives deliveries but does not renew a subscription;
a longer-lived observer must maintain its lifetime separately.

Resolve a store but do not select a board solely for mail.
Separate stores do not exchange messages.
Copy every material decision, active position, and outcome into the issue that
owns it;
mail only draws attention to durable work.
