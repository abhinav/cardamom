# Recover interrupted issue work

Recovery continues the existing issue when its outcome remains the same.
It establishes the current durable position and custody before execution
continues.

## Re-establish context and custody

When status or custody is uncertain, inspect the issue before changing it:

```bash
card --actor <actor> --json show <issue-id> --context
```

Claim available work by ID with context.
Waiting work is available through direct claim rather than automatic pools.
Its waiting reason explains the directed continuation, acceptance,
or external condition that was pending.
Claim it when taking up the intended continuation:

```bash
card --actor <actor> --json claim <issue-id> --context
```

The claim establishes custody and clears waiting.
The waiting reason is coordination context,
not an authorization gate or reservation enforced by Cardamom.

When no issue ID is supplied,
constrain an automatic claim by containing outcome and action labels before
opening deeper history.
This establishes custody before recovery work changes the issue.

## Expand only the context the recovery question needs

Most recovery starts from the current issue's Details and State,
inherited Summaries and States, and dependency Results.
These records should establish the operative contract and current position
without requiring chat.

Open Log when current records do not explain a material choice,
evidence may have gone stale, or the path to the current position
affects safe continuation:

```bash
card --actor <actor> --json log show <issue-id> --limit 20
```

Log output is newest-first by default.
Use `--oldest-first` when a complete chronological replay is useful for finite
work.
Inspect ancestor Details, terminal descendant outcomes,
or earlier Results when their deeper context affects continuation.

After recovering a stable conclusion from history,
promote it into Details when future execution should not need the same replay.
Replace State when recovered evidence changes the active position or next
action.
Treat an unset Result as an observed absence;
other command or store errors stop recovery.

After context and custody are established,
follow [execution.md](execution.md) for ordinary work and record transitions.

## Reassign a stopped executor

When the prior executor may still run,
stop that executor before reassignment.
Cardamom has no force-release operation.

After the runtime confirms the prior executor is stopped
and the coordinator is authorized to reassign the work,
record the reassignment under the coordinator actor,
issue one release under the claim owner's actor,
and claim under the replacement actor:

```bash
card --actor <coordinator-actor> log post <issue-id> \
  'The prior executor was stopped; this issue is reassigned for recovery.'
card --actor <prior-actor> release <issue-id> \
  --waiting 'worker reassignment'
card --actor <replacement-actor> --json claim <issue-id> --context
```

The owner-attributed release is limited to ending the stopped owner's claim.
It does not establish authority,
reserve the waiting issue, or transfer custody directly.
