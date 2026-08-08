# Plan durable work

Plan the smallest issue graph that gives each future executor a clear outcome,
self-contained working contract, ownership boundary, prerequisite set,
and acceptance decision.

## Choose the outcome boundary

| Type | Use |
| --- | --- |
| `workstream` | A finite executable deliverable that may contain any issue type |
| `task` | A bounded executable leaf outcome with separate ownership, sequencing, evidence, or acceptance |
| `checkpoint` | A leaf approve-or-deny decision made by external authority |
| `routine` | A reusable, executable, nestable operating contract awakened outside Cardamom |

A small deliverable may be executed directly as a workstream;
it does not need a task merely to hold implementation.

Create one issue when one actor can own the outcome coherently.
The root agent may own and execute that issue directly.
Create a child when another outcome can finish independently
and benefits from its own custody, evidence, artifact,
dependency, or acceptance.
Changing executors or ordinary execution phases does not create another
outcome.

## Write a self-contained contract

A new executor should be able to begin without chat history from:

- the current issue's Summary and Details;
- inherited ancestor Summaries;
- completed direct dependency Results; and
- the board description and supplied execution environment.

Summary states the issue's concise outcome and the constraints or acceptance
boundary needed to recognize useful completion.
Every ancestor Summary enters descendant context and every Summary must fit the
configured byte limit,
so include a conclusion in a containing Summary only when every descendant
needs it.

Details complete the current issue's stable working contract.
Use Details for issue-local procedure, locations, interfaces, examples,
accepted decisions, and evidence requirements that an executor needs
but descendants should not inherit automatically.
Do not duplicate parent Summaries or dependency Results.
Do not put chronology or an unstable implementation diary in either record.

Frame the contract before execution or delegation:

```bash
summary=$(cat <<'MARKDOWN'
Repair quoted-input parsing without changing command syntax.

The work is complete when malformed escapes have regression coverage
and required parser validation passes.
MARKDOWN
)

details=$(cat <<'MARKDOWN'
## Working context

- Preserve documented literal shell examples.
- The scanner package owns quoted-input recognition.
- Record regression and validation evidence in Result.
MARKDOWN
)

workstream_id=$(card --actor <actor> create \
  --type workstream \
  --label area:parser \
  --summary "$summary" \
  --details "$details" \
  'Repair parser behavior')
```

When an open prerequisite may change the approach,
record the dependency rather than an unstable plan.
After the prerequisite closes,
read its Result and inspect the resulting system before execution planning.

## Assign graph meaning

Use each relationship for the question it answers:

| Need | Mechanism |
| --- | --- |
| Larger outcome owns the work and supplies inherited context | `--parent` |
| Another outcome must finish first | `--depends-on` |
| Classification or action-pool routing | `--label` |
| External authority must approve or deny | `checkpoint` issue plus dependency |

Discovery does not establish a dependency.
Add a dependency only when the dependent outcome cannot proceed or be accepted
without the prerequisite Result.
Containment does not create that readiness edge.

Create one issue and its known relationships together:

```bash
task_id=$(card --actor <actor> create \
  --type task \
  --parent "$workstream_id" \
  --depends-on <prerequisite-id> \
  --label implementation \
  --summary 'Implement the bounded parser change and its regression test.' \
  'Implement parser repair')
```

## Choose the graph mutation surface

| Change | Command |
| --- | --- |
| Create one issue and its initial relationships | `card create` |
| Revise one existing issue or relationship set | `card edit` |
| Create or reconcile several related issues and relationships atomically | `card apply` |

`card apply` can create a complete multi-issue graph in one transaction,
using document-local aliases to connect new issues.
It can also reconcile existing producer-owned issues through IDs or stable
keys.
Choose how the document treats an existing target from the producer's
authority over that target:

| Policy | Use when |
| --- | --- |
| `error` | An existing target is unexpected and may identify the wrong input, issue, or board |
| `skip` | The existing issue is authoritative and the document should create only missing issues |
| `update` | The document producer owns every supplied field and reruns should reconcile those fields |

`update` is an ownership decision rather than a convenience for idempotence.
Supplied set-valued fields replace the complete set,
so use `skip` or `error` when people or another process own any supplied field.
Installed `card apply --help` owns the complete input schema.
Use `--dry-run` when a non-mutating preview helps evaluate a generated graph.

Before a material graph revision,
publish the evidence that changed the relationship.
Cardamom graph mutations are concurrency-safe;
coordinate only claims or contracts on the affected issues
when their current executors must account for the change.
Make each related edit atomically, then inspect affected ready and blocked work:

```bash
card --actor <actor> log post <issue-id> \
  'Schema inspection established that this issue now requires %<schema-id>.'
card --actor <actor> edit <issue-id> \
  --depends-on <schema-id> \
  --label +integration
card --actor <actor> --json ready
card --actor <actor> --json blocked
```

Use signed values such as `--depends-on -<id>` and `--label -integration` for
removals.
Use `--parent=` to remove containment.

## Add a phased workflow only when needed

Ordinary investigation, implementation, validation, and handoff positions need
no phase labels.
When a non-standard workflow needs human-visible positions or distinct
label-selected worker pools,
use [phased-workflows.md](phased-workflows.md).

## Place an approval gate

Use a checkpoint when an external authority must approve or deny work.
Make work that requires the decision depend on the checkpoint:

```bash
checkpoint_id=$(card --actor <actor> create \
  --type checkpoint \
  --summary 'Approve after the policy owner accepts the mapping and rollback plan.' \
  'Approve support taxonomy')
card --actor <actor> edit <migration-id> \
  --depends-on "$checkpoint_id"
```

Obtain the authority decision outside Cardamom, then record it:

```bash
card --actor <actor> checkpoint approve "$checkpoint_id" \
  --reason 'The policy owner accepted the mapping and rollback plan.'
```

The command actor attributes the record;
it does not establish who holds approval authority.
Before denying a checkpoint,
use the cancellation-impact procedure in [execution.md](execution.md).

When supplied file bytes must remain available
after their current path or worktree,
apply the admission criteria in [attachments.md](attachments.md).
