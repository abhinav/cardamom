# Run recurring work

A routine is one persistent operating contract awakened by an external
scheduler or coordinator.
Cardamom preserves context and run history;
it does not schedule awakenings.

Use [leases.md](leases.md) before a run when the routine needs exclusive use of
an external resource.
Acquire and release the lease within each awakening.

## Establish the operating contract

Use one routine for one stable scope and success definition.

| Record | Routine content |
| --- | --- |
| Summary | Stable scope and contract facts every child needs |
| Details | Run and retirement conditions, procedure, evidence, resource needs |
| State | Cursor, targets, active-run facts, partial progress, and next transition |
| Log | Distinct policy choices or evidence not preserved by committed State |

Routines do not appear in automatic ready or blocked claim pools.
An external wake policy claims the known routine ID:

```bash
card --actor <actor> --json claim <routine-id> --context
```

Explicit selection still respects dependencies.
If an open dependency blocks the routine,
leave it unclaimed and let the wake policy retry.

## Run one awakening

Before external work,
install an active-run State that retains the last safe cursor and current
targets:

```bash
card --actor <actor> state set <routine-id> - \
  --next 'Assess the current release targets.' <<'STATE'
## Current targets

- release-121
- release-122

## Safe cursor

release-120

## Active run

- Starting cursor: release-120
- Evidence: review and validation status for every inspected release
STATE
```

Create a child only when part of the run needs independent custody,
dependencies, artifacts, or acceptance.
The routine owns coordination;
the child owns that bounded outcome.

Advance a cursor only through successfully processed input.
At the run boundary,
append the completed outcome to the active-run State before committing it.
`state commit` can preserve only the State that exists at that moment;
it cannot recover run evidence that was never published.

```bash
card --actor <actor> state append <routine-id> - <<'STATE'
## Run outcome

- Releases 121 through 123 are resolved.
- Release 124 remains active.
STATE
```

Then commit the completed run while installing the next recoverable State:

```bash
card --actor <actor> state commit <routine-id> \
  --set 'Release 124 remains active; safe cursor is release-123.' \
  --next 'Recheck release 124 before advancing the cursor.'
card --actor <actor> release <routine-id>
```

The committed State preserves the run boundary.
Add Log only for distinct policy or reasoning useful to later runs.
When a run establishes stable procedure or interpretation,
replace Details with the complete durable contract before release.
Do not accumulate run chronology there.

When an external event prevents the next run,
append partial progress and the observed trigger to the active-run State,
then install the last safe cursor and trigger,
then use waiting release:

```bash
card --actor <actor> state append <routine-id> - <<'STATE'
## Partial run outcome

- Release 124 completed successfully.
- The signing service failed before release 125 was assessed.
STATE
card --actor <actor> state commit <routine-id> \
  --set 'Release 125 is unassessed; safe cursor is release-124.' \
  --next 'Resume after the signing service recovers.'
card --actor <actor> release <routine-id> \
  --waiting 'signing service recovery'
```

## Retire the operating contract

Verify that no actor owns an active run.
Set Result only when acceptors or dependents need the completed terminal
outcome.
Then use [termination.md](termination.md) to reconcile children,
publish why the operating contract ended,
and choose close or cancellation.
One terminal decision record may also carry the retirement rationale;
do not post the same conclusion twice.
