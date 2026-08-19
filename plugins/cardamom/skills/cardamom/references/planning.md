# Plan durable work

Plan the smallest issue graph that gives each outcome a clear contract,
owner, prerequisites, evidence, and acceptance decision.

## Reuse the outcome that already owns the work

When matching work may exist,
search the selected board and inspect plausible issues before creation:

```bash
card --actor <actor> --board <board-id> --json list \
  --status ready,blocked,in_progress,waiting,closed,cancelled \
  --title-regexp '<title-regexp>' --limit 0
card --actor <actor> --board <board-id> --json show <candidate-id> --context
```

Continue the issue with the same durable outcome across actors, sessions,
and ordinary phases.
Resolve ambiguous scope with [scope.md](scope.md) before searching.

## Choose an outcome boundary

| Type | Durable outcome |
| --- | --- |
| `workstream` | Finite executable deliverable, with or without children |
| `task` | Bounded leaf needing separate custody, evidence, or acceptance |
| `checkpoint` | External authority's approve-or-deny decision |
| `routine` | Recurring operating contract awakened outside Cardamom |

One actor may plan and execute a workstream directly.
Create another issue when a bounded outcome needs independent custody,
sequencing, evidence, artifact, or acceptance.
Make it a child only when its outcome is a constituent of the parent's promised
deliverable: without that outcome, the parent's contract would remain
incomplete.
Changing actors or ordinary execution phases is not a new outcome.

## Preserve lineage when outcome boundaries change

When accepted knowledge requires one outcome to split,
several outcomes to merge,
or an outcome to be superseded,
publish the rationale on every issue whose current boundary will change before
the replacement graph consumes that decision.
Give each successor its own executable contract.
After creating the successor,
make every predecessor identify its successor or successors,
and make the successor identify a predecessor when that lineage affects
execution, recovery, or review.

Carry forward the material premise, evidence, and remaining contract each
successor needs rather than copying complete predecessor records.
Keep the material conclusion in the applicable record;
an issue reference supplies navigation rather than meaning.
Changing only an actor or ordinary phase continues the same issue without
predecessor or successor lineage.

When the original outcome is no longer valid,
follow [termination.md](termination.md) and cancel it as superseded after the
successor contracts and navigable lineage are established.
Do not invent a successful Result for the replaced outcome.

## Establish the executable contract

Summary is the concise outcome and inherited acceptance boundary.
Every descendant receives ancestor Summaries,
so include a conclusion there only when every descendant needs it.

Details is the issue-local stable contract.
Record the established problem and intended behavior, owned area,
constraints, accepted choices, and completion evidence.
If the implementation is not established,
define the investigation boundary and evidence needed to choose it.
For a decision-producing investigation or experiment,
record the question, method, applicable baseline or comparison,
evidence or metric, stopping condition, durable artifact when one is needed,
and decision rule before primary work.
Include only elements that can change the decision;
a bounded source inspection does not need invented metrics or thresholds.

Define completion evidence as behavior an acceptor can observe or judge.
For a qualitative outcome,
record the criteria an acceptor will apply rather than an unjudgeable quality
claim.
When the fastest useful iterative check differs from final acceptance evidence,
name both and do not treat the iterative check as completion.
One check may fill both roles when it exercises the complete contract.
If a plan is accepted,
publish it before execution even when the planner will execute directly.

Do not duplicate parent Summaries or dependency Results.
Do not put chronology or an unstable work diary in Summary or Details.
Before execution or delegation,
another actor should be able to choose the first safe action from durable
context and distinguish the intended change from a generic activity.
Otherwise repair the contract or create an investigation outcome.

When parser repair is a constituent deliverable of a compatibility workstream:

```bash
card --actor <actor> create \
  --type task \
  --parent <workstream-id> \
  --label implementation \
  --summary 'Repair quoted-input parsing without changing command syntax.' \
  --details 'Reproduce malformed escapes; preserve syntax; run parser tests.' \
  'Implement parser repair'
```

When an unfinished prerequisite may change the plan,
record the dependency rather than an unstable guess.
Read its Result and inspect the resulting system before implementation.

## Give relationships one meaning

| Question | Mechanism |
| --- | --- |
| Which larger outcome includes this constituent deliverable and supplies inherited context? | `--parent` |
| Which outcome must finish before this issue can advance from its current executable position? | `--depends-on` |
| Which classification or automatic action pool applies? | `--label` |
| Which external authority must decide? | Checkpoint plus dependency |

Discovery records provenance rather than a graph relationship.
For independently tracked work,
reference the source issue in Details when its origin affects execution,
or in Log when the origin matters only as history.
Add containment or dependency only when its criterion independently holds.
Containment makes a child part of the parent's completion;
it does not by itself require the parent to wait.
Add a dependency only when the unfinished outcome prevents the dependent issue's
current work or acceptance from proceeding.

Use `card create` for a new issue and its initial relationships.
Use `card edit` for one existing issue.
Use `card apply` only when one producer owns and must atomically reconcile
several issues or relationships.

For `card apply`, choose the existing-target policy from field ownership:

- `error` when an existing target is unexpected;
- `skip` when the existing issue is authoritative; or
- `update` only when the producer owns every supplied field.

Set-valued fields supplied under `update` replace the complete set.
Use `--dry-run` when a preview helps evaluate a generated graph;
installed `card apply --help` owns the schema.

## Publish graph revisions through the affected issue

Before a material graph change,
publish why the relationship changed.
For a claimed issue,
update its complete State when the revision changes its active position or next
action and add Log only when the rationale will aid later recovery or review.
For an unclaimed issue,
put stable contract consequences in Details or record distinct rationale in
Log before mutation.

Do not leave a claimed issue's State describing a plan invalidated by the new
graph.
After the atomic edit,
inspect affected ready and blocked work.

```bash
card --actor <actor> state set <issue-id> \
  'Schema work is now a prerequisite; implementation is blocked on its Result.' \
  --next 'Resume after %<schema-id> closes and reassess the plan.'
card --actor <actor> log post <issue-id> \
  'Schema inspection established the new prerequisite %<schema-id>.'
card --actor <actor> edit <issue-id> --depends-on <schema-id>
card --actor <actor> --json ready
card --actor <actor> --json blocked
```

Signed values such as `--depends-on -<id>` and `--label -integration` remove
relationships.
Use `--parent=` to remove containment.

## Add special workflow structure only when it changes decisions

Use [phased-workflows.md](phased-workflows.md) only for a non-standard visible
position or label-selected worker pool.
Use [attachments.md](attachments.md) only when produced bytes must outlive
their path or worktree.

Use a checkpoint when downstream work requires an external authority's
decision:

```bash
checkpoint_id=$(card --actor <actor> create \
  --type checkpoint \
  --summary 'Approve after the policy owner accepts the mapping and rollback plan.' \
  'Approve support taxonomy')
card --actor <actor> edit <migration-id> --depends-on "$checkpoint_id"
```

Obtain the decision outside Cardamom and record it with `checkpoint approve`
or `checkpoint deny`.
The command actor attributes the record;
it does not establish approval authority.
Before denial,
follow [termination.md](termination.md).
