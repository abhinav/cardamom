# Recovering work

Recovery continues the same issue.
Do not create a replacement issue merely because a different agent or session
is resuming the work.

## Reconstruct current State

When root selects an unclaimed task or workstream from `card ready`,
claim it by ID before reading further so another pool worker cannot take it:

```bash
card --actor recovery-worker --json claim <issue-id> --context
```

That successful claim establishes custody and returns the assembled context.
When no issue ID is supplied,
list only the relevant ready pool,
choose one returned task or workstream,
then claim it immediately:

```bash
card --actor recovery-worker --json list \
  --status ready --under <workstream-id> --label implementation --limit 0
card --actor recovery-worker --json claim <selected-issue-id> --context
```

If the issue is a routine,
follow `routines.md` and claim its known ID explicitly.
If it is a checkpoint,
follow `planning.md` and record an approve or deny decision rather than trying
to claim it.

When status or custody is uncertain,
or the issue is waiting or actively claimed,
inspect before changing custody:

```bash
card --actor recovery-worker --json show <issue-id> --context
```

The returned inherited summaries, current details,
State body, optional next action,
relationship summaries, log count,
and latest log identity should normally establish how to resume.
Treat the State body as mutable recovery facts
and the optional next action as the planned transition from them.
The Log contains committed State snapshots and any standalone replay material;
expand it only when current context does not answer the recovery decision.
`log list` returns newest entries first by default.
Expand only the evidence needed for the recovery decision:

```bash
card --actor recovery-worker --json show <ancestor-id>
card --actor recovery-worker --json log list <issue-id> --limit 20
card --actor recovery-worker --json list \
  --under <issue-id> --status closed,cancelled --limit 0
card --actor recovery-worker --json result show <issue-id>
```

Increase or remove the log entry limit only when earlier chronology is needed.
Use `--oldest-first` only when recovery requires replay from the beginning.
Read terminal descendants for material completed detail,
and the result only when an earlier outcome matters.
An unset result is an observed absence;
other command or store errors must stop recovery.
When a record cites `%log_...`,
inspect that chronology only when it helps the current recovery decision.
Recover materially important conclusions from summary, details, state,
or result rather than treating a log reference as essential context.

## Retrieve referenced artifacts

Load [attachments.md](attachments.md) when recovery depends on file bytes or
when issue context names a `%<attachment-id>` or `attachment:<id>` reference.

## Establish ownership

Root decides when recovery is required and may terminate a worker to reassign
its issue.
Worker termination, loss of response,
and planned reassignment are common recovery cases.
When an issue still has an active claim and the prior worker may still execute,
stop that worker before dispatching a replacement.

If the issue is waiting,
inspect its waiting reason and state before assigning an actor.
Leave it waiting until the directed continuation,
acceptance,
or external condition can proceed.
When an executor is selected,
claim the issue explicitly by ID;
that claim clears waiting status and establishes new custody:

```bash
card --actor recovery-worker --json claim <waiting-issue-id> --context
```

An acceptor may instead inspect and close waiting work without claiming it.

For an issue with an active claim,
record the reassignment reason,
release the existing claim,
and have the replacement claim the same issue.

After root has stopped the prior worker,
root may issue the one owner-attributed `release` command under the prior
worker's actor because Cardamom has no force-release operation.
Root-authored logs and every other root operation retain root's stable actor.

```bash
card --actor coordinator log add <issue-id> \
  "Root ended worker-old's assignment and reassigned this issue for recovery."
card --actor worker-old release <issue-id> \
  --waiting "worker reassignment"
card --actor recovery-worker --json claim <issue-id> --context
```

## Continue and promote context

Continue from the existing summary, details, and state.
Follow the canonical record roles in `SKILL.md`
and the recording order in `execution.md` as resumed work proceeds.
Summary and details replacement each replace the whole selected value;
inspect the current records first and retain every still-operative boundary,
acceptance criterion,
and artifact requirement.
Promote only the concise conclusion that future children materially need into
the containing issue's summary;
keep supporting reasoning and evidence in details or log entries.
Do not duplicate the full chronology into the summary, details, or state.
Preserve the evidence source when promoting a conclusion.
