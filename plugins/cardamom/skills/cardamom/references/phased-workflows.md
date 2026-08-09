# Coordinate phased workflows

Use a phased workflow when one persistent issue moves through non-standard
human-visible positions or label-selected worker pools.
Ordinary investigation, implementation, validation, handoff,
and acceptance do not require phase labels.

## Model a workflow position

A `phase:<name>` label answers which workflow position should be visible or
which worker pool may select the issue.
It does not impose order, create readiness, preserve work,
transfer custody, or determine whether the issue enters automatic pools.

One phase transition coordinates separate effects:

| Effect | Owner |
| --- | --- |
| Completed phase outcome | Committed State snapshot |
| Next active position and action | Current State |
| Visible position or worker pool | Phase label |
| Automatic or directed discovery | Ordinary or waiting release |
| Completed overall outcome | Result after the issue contract is satisfied |

Keep one issue when several positions contribute to one Result.
Create a child only when a bounded part needs independent ownership,
evidence, artifact, dependency, or acceptance.

Use one current phase label when the phase itself must be visible or an
automatic pool selects it.
Other labels may answer independent classification questions.
When State already communicates an ordinary active position and no pool needs
a phase label,
the ordinary execution workflow is sufficient.

## Transition between phases

At a coherent phase boundary:

1. Make the completed phase outcome current State.
2. Commit that State while installing the next position and action.
3. Replace the old and new phase labels in one edit.
4. Release according to how the next continuation should be discovered.

For an implementation-to-verification transition:

```bash
card --actor <actor> state set <issue-id> \
  'Implementation satisfies its contract and is ready for verification.' \
  --next 'Move the issue to verification.'
card --actor <actor> state commit <issue-id> \
  --set 'Verification is the active position; implementation is recorded.' \
  --next 'Run the verification contract.'
card --actor <actor> edit <issue-id> \
  --label -phase:implement \
  --label +phase:verify
card --actor <actor> release <issue-id>
```

Choose the final release from the next continuation:

| Continuation | Release |
| --- | --- |
| Any eligible actor may select the new phase from an automatic pool | Ordinary release |
| Continuation is directed, awaits acceptance, or needs an external event | Waiting release with the reason |

A waiting issue keeps its new phase label but remains outside automatic pools.
When the intended continuation can proceed,
the next actor claims the same issue directly by ID.
The waiting reason communicates that continuation;
it does not grant authority or reserve the issue.

Result remains unset across intermediate phase transitions.
Set Result only when every required position has satisfied the overall issue
contract.

Use a checkpoint instead when an external decision needs its own durable
approve-or-deny outcome and downstream readiness effect.
The dependent issue still uses State, phase labels,
and release mode for its own continuation.
