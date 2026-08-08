# Run recurring work

A routine is one persistent operating contract awakened by an external
scheduler or coordinator.
Cardamom preserves its context and run history;
it does not schedule the next awakening.

## Create the operating contract

Use one routine for one stable scope and success definition.

| Record | Routine contents |
| --- | --- |
| Summary | Stable scope and contract facts every child needs |
| Details | Successful-run and retirement conditions unless every child needs them, plus procedure, evidence requirements, and stable resource requirements |
| State | Cursor, current targets, active-run facts, and partial progress |
| Next action | Transition for the next awakening, when established |
| Log | Committed run snapshots and replay-worthy policy decisions |

```bash
summary=$(cat <<'MARKDOWN'
Assess tracked releases.
MARKDOWN
)
details=$(cat <<'MARKDOWN'
Each run succeeds when every tracked release is accepted or returned with an
actionable failure.
Retire when this release process ends.

Inspect the targets and safe cursor in State.
Record review and validation evidence for every assessed target.
MARKDOWN
)
routine_id=$(card --actor <actor> create \
  --type routine \
  --summary "$summary" \
  --details "$details" \
  'Assess releases')

card --actor <actor> state set "$routine_id" "$(cat <<'STATE'
## Current targets

- release-121
- release-122

## Safe cursor

release-120
STATE
)" --next 'Assess the current release targets.'
```

Routines do not appear in ready or blocked claim pools.
Dispatch a routine by its known ID.

## Begin one awakening

Claim the routine for one run:

```bash
card --actor <actor> --json claim <routine-id> --context
```

Explicit selection still respects dependencies.
If an open dependency blocks the routine, leave it unclaimed
and let the external wake policy retry.

Beginning the run consumes the planned transition.
Before external work, replace State with the persistent cursor and targets
plus an active-run boundary:

```bash
card --actor <actor> state set <routine-id> "$(cat <<'STATE'
## Current targets

- release-121
- release-122

## Safe cursor

release-120

## Active run

- Wake: 2026-07-16T10:00:00Z
- Starting cursor: release-120
- Evidence: review and validation status for every inspected release
STATE
)"
```

Create a child task when part of the run needs independent ownership,
dependencies, artifacts, or acceptance.
The routine owns coordination;
the child owns that bounded outcome.

## Finish or pause the run

Append the run outcome to active State as it becomes known.
Advance a cursor only through successfully assessed input.
At the run boundary, commit the completed run while installing State
and the next action for the following awakening:

```bash
card --actor <actor> state append <routine-id> "$(cat <<'STATE'
## Run outcome

- Releases 121 through 123 are resolved.
- Release 124 has a failing validation job and remains active.
STATE
)"

next_state=$(cat <<'STATE'
## Current targets

- release-124: validation job `test` is failing

## Safe cursor

release-124
STATE
)
card --actor <actor> state commit <routine-id> \
  --set "$next_state" \
  --next 'Recheck release 124 before advancing the cursor.'
card --actor <actor> release <routine-id>
```

The commit preserves the completed run.
Release preserves the installed next-run State automatically.
A standalone Log post is useful only when the run establishes a material
policy choice or other reasoning not represented by either State snapshot.

## Upgrade knowledge between awakenings

Every routine claim returns the routine's Details and current State.
Keep those records sufficient to begin the next awakening without replaying the
routine's complete Log.

When a run establishes stable procedure,
interpretation rules, resource requirements,
or policy that later runs need,
incorporate the conclusion into Details before release.
Preserve the evidence and decision trail in the committed run State or a
standalone Log entry,
but replace obsolete Details instead of accumulating run chronology there.

Keep current targets, cursors, partial progress, and wake-specific conditions
in State.
Keep Summary limited to the stable routine scope
and conclusions every child must inherit.
Keep successful-run and retirement conditions in Details
unless every child needs them.
Retrieve bounded Log history only when a specific past run or decision affects
the current awakening.

## Wait for external continuation

When no run can proceed until an external event,
commit the partial run while installing the last safe cursor and trigger,
then release into waiting:

```bash
next_state=$(cat <<'STATE'
## Current targets

- release-125: not yet assessed

## Safe cursor

release-124

## External trigger

The signing service must recover before assessment resumes.
STATE
)
card --actor <actor> state commit <routine-id> \
  --set "$next_state" \
  --next 'Resume after the signing service is restored.'
card --actor <actor> release <routine-id> \
  --waiting 'signing service is restored'
```

An external condition is waiting state, not dependency-derived blocked status.
The next direct claim clears waiting after the trigger is satisfied.

Acquire and release a required resource lease within each run.
Do not carry a lease between awakenings.
Use [leases.md](leases.md) for the acquisition and recovery boundary.

## Retire the routine

Verify that no actor owns an active run, reconcile direct children,
record why the operating contract ended, then close or cancel the routine:

```bash
card --actor <actor> --json show <routine-id> --context
card --actor <actor> --json list \
  --under <routine-id> \
  --status ready,blocked,in_progress,waiting,closed,cancelled --limit 0
card --actor <actor> log post <routine-id> \
  'This routine is retired because its operating contract no longer applies.'
card --actor <actor> close <routine-id>
```

A routine Result is optional.
