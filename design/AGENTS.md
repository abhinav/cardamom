# Architecture decision records

The `design/` directory is Cardamom's architecture decision log.
Each Architecture Decision Record (ADR) preserves one decision and the
conditions that shaped it so a future maintainer can decide whether it still
applies.
An ADR records what Cardamom decided and why.
It is not a complete architecture description,
an implementation plan, a discussion transcript, or an enforcement mechanism.

## Decide whether to write a record

Write an ADR while a decision is being made when a future maintainer will need
the rationale from that time to evaluate or safely change the system.
Useful signals include a costly reversal, a change to system structure or
ownership, a published interface, persistence, coupling, a quality attribute,
an external constraint, a non-obvious deviation, or a recurring debate.
These signals guide judgment; they are not a checklist.

Do not write an ADR merely because work touches architecture.
Routine local choices, established patterns, and cheaply reversible changes
usually belong in code, tests, or implementation documentation.
Do not invent or backfill rationale that the available evidence does not
establish.

Choose the action that preserves useful history:

- Create a proposed ADR for a current decision that needs durable rationale.
- Revise a proposed ADR while the decision remains under review.
- Keep and reference an existing ADR when it already governs the decision.
- Update only status and cross-links when only lifecycle metadata changes.
- Create a new ADR when an accepted decision changes.
  The new ADR supersedes the old ADR.
- Propose an ADR without creating it when the record is useful but the task
  does not authorize documentation changes.
- Omit the ADR when the decision does not need durable rationale.

## Establish the record's evidence

Before drafting, identify the baseline, actors, boundaries, forces,
constraints, and scope that the decision relies on.
Define each necessary prerequisite in the ADR,
link a stable source that defines it for the same audience and scope,
or identify a material evidence gap.
Do not draft as though a missing prerequisite were established.

Use the supplied facts and inspected sources as the boundary for each claim.
A statement that a decision affects another area establishes the relationship,
but it does not establish another decision about that area.
For example, evidence that a storage choice affects caching does not select the
cache owner or its invalidation policy.
Keep behavior, ownership, data flow, interfaces, representation, lifetime,
ordering, validation, and implementation unresolved unless the evidence
establishes those choices.
Choosing an owner does not by itself choose which components call it,
depend on it, construct it, or deliver data to it.

Completeness means that a future maintainer can evaluate the recorded decision;
it does not require a detailed system design.
One supported paragraph per required section can be enough.
Before retaining a sentence in Decision or Consequences,
identify the supplied fact, inspected source, or necessary logical consequence
that supports it.
Omit plausible details that have no such support,
and identify a material unresolved question when the gap affects evaluation.

## Name and format records

Name each record `YYYY-MM-DD-slug.md`.
Use the decision date and a short lowercase slug that identifies the decision.
Start with one level-one heading for the title,
then put `Date` and `Status` on plain metadata lines.
The date must match the filename.

Use one of these status values:

- `Proposed` while the decision is under review.
- `Accepted` after the project accepts the decision.
- `Rejected` after the project rejects the decision.
- `Superseded` after an accepted replacement takes effect.

Use this template:

```markdown
# <Decision title>

Date: YYYY-MM-DD
Status: Proposed

## Context

<Facts, forces, constraints, alternatives, and scope needed to evaluate the
decision.>

## Decision

<The proposed or accepted choice and why it fits the context.>

## Consequences

<Material outcomes, costs, constraints, and follow-up implications.>
```

Context must introduce every project-specific term, actor, boundary,
invariant, and baseline before the ADR reasons from it.
Include alternatives only when they materially explain the choice.
Keep observed facts, supported inferences, and recommendations distinct.

State a proposed decision as proposed.
State an accepted decision as an unambiguous declaration.
Consequences must include the material positive, negative, and neutral effects
that a future maintainer needs to evaluate the decision.
Consequences follow from the recorded decision;
they do not add implementation or validation requirements that the decision
does not establish.
Do not invent content to fill the template.

## Preserve decision history

A proposed ADR may change while the decision remains under review.
After an ADR leaves `Proposed`,
its Context, Decision, and Consequences are immutable historical evidence.
Do not revise those sections to match later understanding,
implementation, or direction.
A correction may repair wording or a link only when it does not change the
historical claim.
Status and supersession links may change as the lifecycle advances.

When an accepted decision changes,
create a new proposed ADR and link the records in both directions.
Add `Supersedes: [<old title>](<old filename>)` near the new record's metadata.
After the replacement is accepted,
set the old record's status to `Superseded` and add
`Superseded by: [<new title>](<new filename>)` near its metadata.

## Maintain the decision log

`README.md` is generated; do not edit it by hand.
After adding, renaming, or changing the date, status, or title of an ADR,
run `../tools/update-adr-table.sh` from this directory.
Run `../tools/update-adr-table.sh --diff` to verify that the generated table is
current.

When an ADR informs other work,
read the relevant record fully and follow its status and supersession links.
Treat a conflict between an active ADR and the current implementation as a
finding; do not silently change either one.

Before completing an ADR, confirm that a future maintainer can identify the
decision and its scope, understand the conditions and rationale,
distinguish its lifecycle status, identify its material consequences,
and decide whether changed conditions justify a new decision.
