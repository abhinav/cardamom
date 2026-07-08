# Running routines

## Create a routine

Create one routine for one stable operating contract.
Apply the canonical record roles to routine-specific information:

| Record | Routine contents |
| --- | --- |
| Title | Identity used in lists and explicit dispatch |
| Summary | Stable scope, success conditions, and retirement conditions inherited by child work |
| Details | Expanded stable procedure and examples retrieved on demand |
| State body | Cursor, current targets, active run, and partial progress |
| Next action | Optional transition for the next awakening |
| Log | Committed run snapshots and replay-worthy material not represented by them |

```bash
summary=$(cat <<'MARKDOWN'
Keep tracked pull requests moving until each is merged, closed,
or returned to an implementation owner with an actionable failure.
Retire when the repository no longer uses pull-request review.
MARKDOWN
)

details=$(cat <<'MARKDOWN'
## Inputs

Read the current pull request IDs and last safe cursor from the routine state.

## Procedure

For each target, inspect review state and required CI checks.
Merge a ready pull request.
For review feedback or CI failure,
record the actionable state and keep the target for the next run.

## Successful run

Every current target is assessed.
The state retains only unresolved targets and the last safe cursor.

MARKDOWN
)

routine_id=$(card --actor coordinator create \
  --type routine \
  --label area:pull-requests \
  --summary "$summary" \
  --details "$details" \
  "Babysit pull requests")

card --actor coordinator state set "$routine_id" "$(cat <<'STATE'
## Current targets

- `acme/widgets#121`
- `acme/widgets#122`

## Safe cursor

`acme/widgets#120`
STATE
)" --next "Assess the current pull request targets."
```

Scheduling and wake policy remain external to `card`.
Routines do not appear in `ready`, `blocked`, or automatic claim results.

## Run one awakening

The external scheduler or coordinator dispatches a known routine ID.
Claim that ID for one run:

```bash
card --actor routine-worker --json claim <routine-id> --context
```

Explicit selection does not bypass dependencies.
Cardamom status `blocked` means an open graph dependency prevents execution.
If the routine is blocked for that reason,
leave it unclaimed and let the external wake policy retry later.

The claimed next action is consumed when the awakening begins.
Before work,
use `state set` to retain the durable cursor, targets, and retry facts
while adding the active run boundary.
Omitting `--next` from this replacement clears the consumed action:

```bash
card --actor routine-worker state set <routine-id> "$(cat <<'STATE'
## Current targets

- `acme/widgets#121`
- `acme/widgets#122`

## Safe cursor

`acme/widgets#120`

## Active run

- Wake: `2026-07-16T10:00:00Z`
- Starting cursor: `pull-request-120`
- Scope: active pull requests after that cursor
- Evidence: review state and CI status for every inspected pull request
STATE
)"
```

Create a child task when part of the run needs independent ownership,
dependencies, artifacts, or acceptance.
The routine owner coordinates the run;
the child owner performs that bounded work.

## Finish a run

After the run,
append the outcome to the active-run State.
Commit that durable checkpoint while replacing the State body
and optional next action with the next-run recovery position,
then release the routine without closing it:

```bash
card --actor routine-worker state append <routine-id> "$(cat <<'STATE'
## Run outcome

- Reviewed pull requests 121 through 124.
- Pull requests 121 through 123 are resolved.
- Pull request 124 still has a failing CI job.
STATE
)"

next_state=$(cat <<'STATE'
## Current targets

- `acme/widgets#124`: CI job `test` is failing

## Safe cursor

`acme/widgets#124`
STATE
)
card --actor routine-worker state commit <routine-id> \
  --replace "$next_state" \
  --next "Recheck pull request 124 before advancing the cursor."
card --actor routine-worker release <routine-id>
```

The explicit commit preserves the completed run boundary and outcome.
Release then preserves the changed next-run State automatically.
Use a standalone Log post only for replay-worthy material not represented by
either snapshot.

After partial or failed work,
advance a cursor only through successfully assessed input.
Do not retain custody merely to wait for the next wake.
Use ordinary release when the external scheduler may start the next run.

When no run can proceed until an external condition,
append the partial outcome to State,
commit it while installing the last safe State and trigger,
then release into waiting:

```bash
card --actor routine-worker state append <routine-id> "$(cat <<'STATE'
## Partial run

The run advanced through the last safe cursor.
The signing service is unavailable,
so no later input was assessed.
STATE
)"

next_state=$(cat <<'STATE'
## Current targets

- `acme/widgets#125`: not yet assessed

## Safe cursor

`acme/widgets#124`

## External trigger

The signing service must recover before assessment resumes.
STATE
)
card --actor routine-worker state commit <routine-id> \
  --replace "$next_state" \
  --next "Resume assessment after the signing service is restored."
card --actor routine-worker release <routine-id> \
  --waiting "signing service is restored"
```

An external service or event discovered during a run is waiting state,
not dependency-derived blocked status.
After the trigger is satisfied,
the next explicit routine claim clears waiting status.

## Retire a routine

Verify that no actor owns an active run,
then reconcile direct children and record the retirement reason:

```bash
card --actor coordinator --json show <routine-id> --context
card --actor coordinator --json list \
  --under <routine-id> \
  --status ready,blocked,in_progress,waiting,closed,cancelled --limit 0
card --actor coordinator log add <routine-id> \
  "Retiring this routine because its operating contract no longer applies."
card --actor coordinator close <routine-id>
```

Use `cancel` instead when the routine is retired without completion.
A routine result is optional.
