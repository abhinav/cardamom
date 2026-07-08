# Mail, subscriptions, leases, and routines

Issue claims and records are the durable coordination core.
Cardamom also provides store-scoped communication,
time-limited resource ownership,
and reusable operating contracts.
These features complement issue custody rather than replacing it.

## Choose the durable boundary

| Need | Use |
| --- | --- |
| Own a unit of work | Issue claim |
| Preserve a decision, recovery state, or outcome | Issue record |
| Notify one actor briefly | Mail |
| Notify current subscribers to a topic | Topic publication |
| Own an external resource for a bounded time | Lease |
| Repeat one stable operating contract | Routine |

## Ephemeral mail

Mail is addressed to an actor and expires.
It belongs to the store rather than one board.
Send a short notification,
then keep material decisions in the relevant issue:

```bash
card --actor coordinator mail send worker-a \
  "Issue cm-abcd is ready for execution."
card --actor worker-a mail recv
```

Use `mail recv --peek` to inspect unread messages without marking them read.
Mail must not be the only copy of a handoff state or accepted decision.

## Topic subscriptions

Subscriptions route ephemeral publications to matching actor mailboxes:

```bash
card --actor release-watcher mail subscribe 'release.*' --ttl 30m
card --actor scheduler mail publish release.ready \
  "A release checkpoint is ready."
card --actor release-watcher mail recv
```

Subscription patterns use filepath-style globs.
Subscriptions expire,
and `mail recv --tail` does not refresh their lifetime.

## Resource leases

A lease coordinates exclusive access to a named external resource,
such as a shared test account,
device,
or local port:

```bash
card --actor worker-a lease acquire staging-db --ttl 30m
card --actor worker-a lease renew staging-db --ttl 30m
card --actor worker-a lease release staging-db
```

Leases are store-scoped.
Use the same stable resource name wherever access must contend.
Lease expiry ends Cardamom ownership,
but it does not prove that the external resource is clean or idle.

## Routines

A routine stores one reusable operating contract.
Scheduling and wake policy stay outside Cardamom,
and each run claims the known routine ID:

```bash
card --actor routine-worker --json claim <routine-id> --context
card --actor routine-worker state set <routine-id> \
  "Inspecting pull requests after safe cursor 120."
```

The replacement keeps State aligned with the active run.
Omitting `--next` clears the planned transition consumed by this run.

At the end of the run,
append its outcome to State,
then commit that checkpoint while installing the next-run recovery truth:

```bash
card --actor routine-worker state append <routine-id> \
  "Reviewed through pull request 124; its test job is still failing."
card --actor routine-worker state commit <routine-id> \
  --replace "Pull request 124 is unresolved. Safe cursor: 124." \
  --next "Recheck pull request 124."
card --actor routine-worker release <routine-id>
```

The commit preserves the completed run as a State snapshot.
Release preserves the changed next-run State automatically.
Use `log add` only for additional replay-worthy material
that neither State snapshot represents.
Create child tasks only when part of the run needs independent ownership,
dependencies,
artifacts,
or acceptance.
