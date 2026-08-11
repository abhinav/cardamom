# Recover interrupted issue work

Recovery continues the existing outcome after re-establishing context and
custody.

## Establish custody from known state

When current custody is not established,
inspect before changing it:

```bash
card --actor <actor> --json show <issue-id> --context
```

When a trusted handoff or runtime observation already establishes that the
known issue is waiting or unclaimed,
claim it directly by ID with context instead of adding a separate inspection.

Choose from the observed owner and status:

| Observation | Recovery action |
| --- | --- |
| Current actor owns the claim | Inspect and continue without another claim |
| Issue is unclaimed or waiting | Claim directly by ID with context |
| Another actor may still be running | Do not take custody; coordinate with that actor or runtime |
| Prior actor stopped; reassignment is authorized | Use the procedure below |

Do not select unrelated work from an automatic pool while recovering a known
issue.
Direct claim clears waiting,
but the waiting reason remains coordination context rather than an
authorization grant.

## Recover only the context needed for safe continuation

Start from the current Summary, Details, and State;
ancestor Summaries and States;
and completed direct-dependency Results.
These records should establish the contract, inherited constraints,
operative position, and prerequisite outcomes without chat.

Open Log when a material choice is unexplained,
evidence may be stale,
or the path to the current position affects safe continuation:

```bash
card --actor <actor> --json log show <issue-id> --limit 20
```

Log is newest-first by default.
Increase the bounded window when the needed decision is older;
use `--oldest-first` only when chronological replay from the beginning answers
the recovery question.

Inspect ancestor Details, terminal descendant outcomes, attachments,
or earlier Results only when the recovery question needs them.
Do not replay complete history merely because a new actor or session arrived.

Before primary work,
replace Details if recovered knowledge changes the stable remaining contract.
Replace State if it changes the active position, uncertainty, or next action.
Use Log only for distinct reasoning or evidence useful later.
Then continue with [execution.md](execution.md).

## Reassign a stopped executor

The runtime must first confirm that the prior executor has stopped.
Cardamom does not stop processes and has no force-release operation.

After explicit reassignment authority is established:

1. The coordinator records the evidence and decision under its own actor.
2. The coordinator issues one owner-attributed release under the stopped
   actor solely to end that actor's claim.
3. The replacement actor claims by ID and re-establishes context before work.

```bash
card --actor <coordinator-actor> log post <issue-id> \
  'The runtime confirmed the prior executor stopped; reassignment is authorized.'
card --actor <prior-actor> release <issue-id> \
  --waiting 'stopped executor reassignment'
card --actor <replacement-actor> --json claim <issue-id> --context
```

This exception neither proves authority nor transfers custody directly.
If process termination or reassignment authority is uncertain,
leave custody unchanged and report the gap.
