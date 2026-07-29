# Phased workflows

Use a phased workflow when one persistent issue moves through several
human-visible positions or automatic worker pools.
Built-in lifecycle and status remain authoritative for readiness, custody,
waiting, and terminal state.
Labels are opaque conventions;
they do not replace issue type, relationships, lifecycle, or status.

A phase label such as `phase:verify` records the issue's human-visible workflow
position and makes the issue eligible for automatic claim by verification
workers.
Use one current `phase:<name>` label for each phase,
and have automatic workers select that same positive label.
Additional labels may classify or route the issue
when they answer an independently meaningful coordination question.
Labels do not create dependency edges or waiting semantics.
For a phased transition,
repeat signed `--label` terms in one `issue edit` command:
`+` adds a label and `-` removes one.
Apply the phase-label change before releasing custody.
When continuation is external or directed,
release the persistent issue into waiting even if a dependency or checkpoint
records a separate prerequisite or decision.
A checkpoint is useful only when that decision has its own acceptance boundary;
it does not replace the waiting handoff on the persistent issue.

## Keep ordinary phases on one issue

Keep one issue across ordinary phases when the phases contribute to one
coherent outcome.
The issue that owns the overall result remains the work unit;
changing the eligible worker class transfers custody rather than creating
another ownership boundary.
Preserve the current recovery facts in State before changing phases.

Create a child issue only when a bounded part can complete under separate
ownership and its distinct evidence, artifact,
or acceptance decision makes the outcome independently useful.
Do not create one child merely to represent each phase.
Use real dependency edges when one outcome must precede another;
phase labels do not impose that order.

## Move through automatic and directed continuations

Suppose one change moves through preparation, verification,
external authorization, and activation.
Coordinators need to see the current phase,
and preparation, verification, and activation workers select work from
automatic pools.
Assume `<change-id>` is contained by `<workstream-id>`
and starts with `phase:prepare`.
Represent all four ordinary phases as label states of `<change-id>`.

A preparation worker claims by the current phase label:

```bash
card --actor preparer --json claim \
  --under <workstream-id> \
  --label phase:prepare \
  --context
```

After preparation,
the worker records recoverable State and atomically moves the phase label
before an ordinary release:

```bash
card --actor preparer issue edit <change-id> \
  --label -phase:prepare \
  --label +phase:verify
card --actor preparer release <change-id>
```

The ordinary release returns the issue to automatic claim pools.
A verification worker can then claim `<change-id>`
with `--label phase:verify`.

When verification completes,
external authorization is the next continuation.
Move the phase label atomically and release into waiting:

```bash
card --actor verifier issue edit <change-id> \
  --label -phase:verify \
  --label +phase:authorize
card --actor verifier release <change-id> \
  --waiting "external authorization"
```

Waiting records that automatic workers must not continue yet;
the labels alone would not establish that condition.
After authorization is established,
a directed coordinator or worker claims the same issue explicitly by ID:

```bash
card --actor coordinator --json claim <change-id> --context
```

That actor records the authorization and activation as the next action,
then atomically moves to the activation phase and releases normally:

```bash
card --actor coordinator state set <change-id> \
  "External authorization is established; evidence: <reference>." \
  --next "Activate the authorized change."
card --actor coordinator issue edit <change-id> \
  --label -phase:authorize \
  --label +phase:activate
card --actor coordinator release <change-id>
```

An activation worker claims with `--label phase:activate`.
If activation instead belongs to a directed actor or another external
condition,
use a waiting release.
