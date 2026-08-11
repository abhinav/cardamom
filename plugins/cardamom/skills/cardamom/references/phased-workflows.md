# Coordinate phased workflows

Use a `phase:<name>` label only when one persistent issue needs a non-standard
visible position or a label-selected worker pool.
Ordinary investigation, implementation, validation, handoff,
and acceptance do not require phase labels.

A phase label neither imposes order nor transfers custody.
Coordinate the complete transition:

| Effect | Durable mechanism |
| --- | --- |
| Completed phase outcome with replay value | Committed State |
| Current position and next action | State |
| Visible position or worker pool | One current phase label |
| Custody and discovery | Retain claim, ordinary release, or waiting release |
| Completed overall outcome | Result after the full contract is satisfied |

At a meaningful phase boundary:

1. Commit the completed State while installing the next position.
2. Replace the phase labels in one edit.
3. Choose custody from the actual next executor.

```bash
card --actor <actor> state commit <issue-id> \
  --set 'Verification is active; implementation is recorded.' \
  --next 'Run the verification contract.'
card --actor <actor> edit <issue-id> \
  --label -phase:implement \
  --label +phase:verify
```

| Next continuation | Custody action |
| --- | --- |
| Same owner continues immediately | Keep the claim; do not release |
| Any eligible actor may select the phase | Ordinary release |
| A named actor, acceptance, or external event is required | Waiting release with the reason |

A waiting issue remains outside automatic pools.
The intended actor claims it directly by ID when continuation is authorized
and ready.
The waiting reason communicates context but grants no authority.

Result remains unset across intermediate phases.
Use a checkpoint instead when an external decision needs its own durable
approve-or-deny outcome and readiness edge.
