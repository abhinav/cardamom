# Planning work

Plan the smallest durable issue graph that gives each future agent a clear
outcome, context, ownership boundary, and acceptance decision.
When the prompt or handoff already establishes the selected board,
use that selection and load only this planning workflow.
Load `boards.md` before planning only when the store or board selection is
missing or ambiguous.
Use targeted `card ... --help` output for command syntax that this workflow does
not exercise.

## Frame the deliverable

Create one issue when one actor can own the outcome coherently.

Choose the issue role from the work being coordinated:

| Type | Use |
| --- | --- |
| `workstream` | Finite deliverable with persistent context, child work, or its own acceptance boundary |
| `task` | Bounded work that benefits from separate ownership, sequencing, artifacts, or acceptance |
| `checkpoint` | Explicit human approval or denial gate |
| `routine` | Reusable operating contract awakened by an external scheduler or coordinator |

Give every issue a compact title and a concise self-contained summary.
Follow the record roles in `SKILL.md` for summary and details content.
Use issue type for the issue's durable role
and built-in lifecycle and status for execution state.
Do not duplicate those values in labels.

Keep unstable implementation ideas out of durable records.
When open prerequisites can change an approach,
record the required outcome, constraints, evidence, and dependencies.
After the prerequisites close,
inspect their results and the resulting system before recording an execution
plan or dispatching the work.

```bash
summary=$(cat <<'MARKDOWN'
Repair quoted-input parsing without changing command syntax.
The work is complete when the failing input has regression coverage
and `mise run test` passes.
MARKDOWN
)

details=$(cat <<'MARKDOWN'
## Acceptance evidence

- Preserve the literal shell example `parser inspect $TARGET`.
- Record the regression test and validation commands in the result.
MARKDOWN
)

card --actor coordinator --json create \
  --type workstream \
  --label area:parser \
  --summary "$summary" \
  --details "$details" \
  "Repair parser behavior"
```

Fresh stores allocate issue IDs with the configured prefix,
which defaults to `cm-`.
Every issue ID matches `[A-Za-z0-9][A-Za-z0-9-]*`.
Treat every returned issue ID as opaque because persisted stores may contain
IDs allocated under another valid prefix.

## Split bounded work

Create a child task when a bounded part can complete under separate ownership
and its distinct evidence, artifact, or acceptance decision makes the outcome
independently useful.
Changing worker classes or claim pools does not by itself create a child
boundary.
Keep ordinary phase transitions on the same issue.
For optional phase visibility or automatic action-pool transitions,
load [phased-workflows.md](phased-workflows.md).
Create a nested workstream when the child is itself a finite deliverable with
multiple execution or acceptance boundaries.
Put the reason for the split in the containing contract or chronology.

Create known routing, containment, and prerequisites atomically:

```bash
task_id=$(card --actor coordinator create \
  --type task \
  --label implementation \
  --depends-on <prerequisite-id> \
  --parent <workstream-id> \
  --summary "Implement the bounded parser change and its regression test." \
  "Implement parser fix")
```

Use each graph feature for its own decision:

- `--parent` records membership and inherited context.
- `--depends-on` records a prerequisite that controls readiness.
- `--label` records opaque classification or claim-pool routing.

Discovery does not establish a dependency.
Add an edge only when the dependent outcome cannot proceed or be accepted
without the prerequisite outcome.

Choose the planning command by mutation scope:

| Need | Command |
| --- | --- |
| Create one issue with its initial parent, dependencies, and labels | `card create` |
| Change an existing issue or its relationships | `card edit` |
| Create or reconcile several issues atomically from structured input | [`card apply`](apply.md) |

## Revise an existing plan

Pause dispatch while changing containment or prerequisites.
Record the evidence for a material graph change before mutating the graph,
then make the complete related edit atomically:

```bash
card --actor coordinator edit <issue-id> \
  --parent <workstream-id> \
  --depends-on <schema-id> \
  --label +integration
```

Use `--depends-on -<schema-id>` or `--label -integration` for removals.
Use `--parent=` to remove containment.
Inspect the affected ready and blocked issues before dispatch resumes.

## Put an approval gate in the graph

Use a checkpoint when an external authority must approve or deny work.
Make the work that requires the decision depend on the checkpoint;
containment does not create a readiness edge.

```bash
checkpoint_id=$(card --actor coordinator create \
  --type checkpoint \
  --summary "Approve only after the policy owner accepts the queue mapping and rollback plan." \
  "Approve support taxonomy")

card --actor coordinator edit <taxonomy-migration-id> \
  --depends-on "$checkpoint_id"
```

Obtain the authority decision outside `card`,
then record the outcome and optional reason:

```bash
card --actor coordinator checkpoint approve "$checkpoint_id" \
  --reason "The policy owner accepted the queue mapping and rollback plan."
```

Every command still carries the stable invocation actor required by the skill.
The checkpoint decision itself records outcome, reason, decision time,
and revision;
it does not designate that actor as the approver or authority.
Before `checkpoint deny`,
follow the transitive cancellation-impact procedure in `execution.md`.

## Apply a multi-issue plan

Load [apply.md](apply.md) when several related issues should appear atomically,
when an external producer reconciles a graph,
or when structured generation is clearer than several commands.

## Preserve supplied artifact inputs

When supplied file bytes must remain available after their current path,
process, session, or worktree disappears,
load [attachments.md](attachments.md),
add the file,
and reference it from the issue record that gives the bytes meaning.
