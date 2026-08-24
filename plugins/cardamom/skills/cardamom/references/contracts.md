# Author issue contracts

Use this reference when creating an issue or replacing Summary or Details
because accepted knowledge changed the stable contract.

## Write the inherited Summary

Summary gives descendants and reviewers the outcome and acceptance boundary
they must inherit.
Write one concise statement in this shape:

```markdown
<Observable outcome>. Accept when <concise acceptance boundary>.
```

Include only constraints or conclusions that every descendant needs.
Keep issue-local area, procedure, evidence detail, and chronology in the record
that owns them.

## Choose the Details profile

Details gives an executor the stable issue-local contract.
Choose the profile from the decision the work must produce,
not merely from the issue type or labels.
Fill every required section with established knowledge.
Omit a conditional section when it cannot change execution or acceptance,
and record a material unknown instead of inventing its contents.

Use headings when the contract has several independently scanned elements.
A bounded issue whose complete contract is one connected thought may use a
short paragraph instead.
Do not preserve empty headings merely to resemble a template.

Define acceptance as behavior an acceptor can observe or judge.
For a qualitative outcome,
record the criteria the acceptor will apply rather than an unjudgeable quality
claim.
When the fastest useful iterative check differs from final acceptance evidence,
name both and do not treat the iterative check as completion.
One check may fill both roles when it exercises the complete contract.

### Implementation

Use for a bounded behavior or artifact change.

```markdown
## Outcome

<Behavior or artifact that must exist.>

## Scope

<Owned area and material exclusions.>

## Acceptance

<Observable final evidence or reviewer rubric.>

<Fastest useful check, only when it differs from final acceptance evidence.>

## Constraints and accepted decisions

<Only constraints and choices that affect execution.>
```

`Outcome`, `Scope`, and `Acceptance` are required.
`Constraints and accepted decisions` is conditional.

### Investigation or experiment

Use when the issue must establish a decision before implementation is safe.

```markdown
## Question

<Decision the investigation must support.>

## Boundaries and baseline

<Applicable system, comparison, and material exclusions.>

## Method

<How the question will be answered.>

## Evidence and stopping condition

<Evidence or metric to collect and when collection is sufficient.>

## Decision rule

<How the evidence selects the next disposition.>

## Durable artifact

<Artifact needed by later work, when any.>
```

The first five sections are required for a decision-producing investigation.
`Durable artifact` is conditional.
A bounded source inspection that does not select among alternatives may instead
use the implementation profile and state the inspection outcome it must
produce.

### Workstream

Use when one issue coordinates several constituent outcomes.

```markdown
## Outcome

<Combined deliverable promised by the workstream.>

## Scope

<Owned boundary and material exclusions.>

## Acceptance

<Evidence that the combined deliverable and required children are complete.>

## Cross-cutting constraints

<Constraints every constituent outcome must preserve.>
```

Do not copy child contracts or graph relationships into Details.
The issue graph owns containment and dependencies.
Omit `Cross-cutting constraints` when none exist.

### Checkpoint

Use when an external authority must approve or deny a decision.

```markdown
## Decision

<Exact decision requested.>

## Authority and inputs

<Who decides and which evidence or artifacts they need.>

## Criteria

<Observable approval and denial criteria.>

## Consequences

<What approval or denial permits or invalidates.>
```

All sections are required when their facts are established.
If authority or criteria are unresolved,
make that gap explicit rather than implying that the command actor decides.

### Routine

Use for a recurring operating contract.

```markdown
## Trigger and outcome

<What awakens a run and what that run must accomplish.>

## Operating rule

<Stable procedure, interpretation rule, and constraints.>

## Evidence

<What each run records or validates.>

## Failure and retirement

<How a failed run stops safely and when the routine should be retired.>
```

Keep per-run position and chronology in State and Log rather than rewriting the
routine contract.

## Check the completed contract

Before execution or delegation,
confirm that another actor can identify the outcome,
the owned boundary,
the first safe action,
and the final acceptance evidence from assembled context.
Confirm that the chosen profile has no missing required section,
no unsupported choice,
and no copied parent or dependency material.
Do not put chronology or an unstable work diary in Summary or Details.
Repair the contract or publish the unresolved gap before primary work consumes
it.
