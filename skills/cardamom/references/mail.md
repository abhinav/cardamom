# Ephemeral mail

Mail is expiring communication addressed to an actor name or topic.
Mail is not attached to an issue and must never be the only record of a
material decision, recovery state, or outcome.
Copy material information into the relevant issue record.

Mail is store-scoped.
Resolve the store but do not select a board solely to send or receive mail.
Use stable actor and topic names that are unambiguous throughout the store.
Separate stores do not exchange mail.

## Send and receive

Send a short message to an actor mailbox:

```bash
card --actor coordinator mail send worker-a \
  "Issue cm-abcd is ready for execution."
```

Receive unread mail for the current actor and mark it read:

```bash
card --actor worker-a mail recv
```

Inspect without marking messages read:

```bash
card --actor worker-a mail recv --peek
```

Use a single-quoted heredoc for a Markdown-rich message:

```bash
card --actor coordinator mail send worker-a - <<'MAIL'
Issue `cm-abcd` is ready.

Read the issue context before claiming it.
MAIL
```

## Topics and long-lived receivers

Subscribe an actor to one topic or matching pattern without reading mail:

```bash
card --actor release-watcher mail subscribe 'release.*' --ttl 30m
```

Running `mail subscribe` again refreshes the subscription's TTL.

Publish to a topic:

```bash
card --actor scheduler mail publish release.ready \
  "A release checkpoint is ready."
```

Use `--tail` to watch continuously for new messages after subscribing:

```bash
card --actor release-watcher mail recv --tail
```

`mail recv --tail` does not refresh subscriptions.
For a receiver that runs longer than the subscription TTL,
run `mail subscribe` again from another process before the TTL expires.
Use `card --actor release-watcher mail subscriptions` to inspect active
subscriptions,
and use `card --actor release-watcher mail unsubscribe 'release.*'` to remove
one.
