# Cancel, deny, or reopen work

Use cancellation for authorized abandonment, invalidation, or supersession.
Preserve useful partial outcomes and the reason the work ended.

## Check cancellation impact

`cancel` affects the requested issues and their non-terminal transitive
dependency dependents.
`checkpoint deny` records denial and performs the same dependent cancellation
atomically.
Containment is a different graph.

Inspect the affected issues before acting:

1. Start with each requested root and read `show --json`.
2. Follow its `blocks` IDs, inspecting each unseen issue until none remain.
3. For the non-terminal affected set, establish authority and coordinate any
    active execution before invalidating its work.

Cardamom does not stop processes.
Uncertain ownership, impact, or authority must be resolved before cancellation.

```bash
card --actor <actor> cancel <id>
```

For checkpoint denial, record the actual authority's reason on the operation:

```bash
card --actor <actor> checkpoint deny <id> \
  --reason 'The policy owner rejected the proposed data mapping.'
```

## Reopen for further work

Record what invalidated the terminal decision and inspect whether the old
goal, prerequisites, and evidence still apply.

```bash
card --actor <actor> reopen <id>
```

Reopening leaves the issue open and unclaimed.
It does not reopen prerequisites, restore the prior claim, or establish current
progress.
Update changed context and let the intended executor claim the work.
