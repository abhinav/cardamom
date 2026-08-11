---
name: cardamom
description: >-
  Use when the user explicitly asks to use Cardamom, the card command, an
  existing Cardamom store, or Cardamom-coordinated local task or agent work.
  Work on an established board unless the user explicitly asks to initialize,
  create, or select one. Do not use for ordinary tasks merely because they
  could be tracked.
---

# Cardamom

Cardamom preserves work identity, shared context, custody, and recovery across
agents and sessions.
The collaboration runtime owns processes, dispatch, liveness, and worktrees;
`card` owns durable coordination.
The current agent may execute claimed work directly or delegate it when the
user and runtime call for delegation.

Run `card` directly.
If it is unavailable,
use `scripts/cardamom` from this skill directory on macOS or Linux,
or `scripts/cardamom.ps1` on Windows.

## Preserve actor identity

Use the collaboration runtime's readable agent name as `--actor`.
Add a short runtime-assigned identifier only when agents share that name.
If the runtime supplies no identity,
choose a concise stable name from the task context.
Pass the same actor on reads and writes throughout one claim, record, result,
release, mail, or lease lifecycle.
Do not rely on the machine username default.

The root agent uses its own actor when it executes work.
When work is delegated, each actual executor uses its own runtime identity
rather than an invented role such as `worker-a`.

## Resolve the coordination scope

Cardamom has three nested storage scopes with different purposes:

| Scope | Purpose |
| --- | --- |
| Store | Physical persistence plus actor mailboxes, topic subscriptions, and resource lease names |
| Project | Repository or product namespace inside a store |
| Board | Shared issue, relationship, claim, and attachment context |

Issue and attachment work resolves a board.
Mail and leases resolve only a store and may coordinate actors or resources
across boards in that store.
Separate stores are separate coordination namespaces.

Use supplied scope when available.
Otherwise let Cardamom resolve the store from the checkout and the board from
the issue, explicit or ambient selection, checkout binding, or sole board.
Report unresolved ambiguity rather than creating or persistently selecting
scope that the user did not request.
Use [scope.md](references/scope.md) when scope is not already established,
setup is requested, or work will move to another worktree.

## Model one coordinated outcome

An issue is the durable boundary for one outcome.
Continue the same issue when an actor, session, or ordinary execution phase
changes.
Create another issue only when another outcome needs independent ownership,
sequencing, evidence, or acceptance.

Each coordination mechanism answers a different question:

| Mechanism | Question |
| --- | --- |
| Issue | Which durable outcome is being produced? |
| Parent | Which larger outcome owns this work and supplies inherited context? |
| Dependency | Which prerequisite outcome must exist before this work is ready? |
| Label | How is this issue classified or selected from an action pool? |
| Claim | Which actor currently owns execution? |
| Waiting | Why is unclaimed work deliberately outside automatic claim pools? |

Workstreams and routines may contain children.
Tasks and checkpoints are leaf outcomes.
Containment does not establish readiness,
and labels do not replace lifecycle, dependency, custody, or waiting state.
The claim owner owns issue execution
and maintains its durable records and handoff.
An assisting actor without issue custody returns evidence to the claim owner.
A resource lease, when needed, owns one external resource.

## Model the parts of an issue

Importance determines whether knowledge must be durable.
Its future reader and lifetime determine the record:

Title identifies the outcome in lists, routing, and commands.
Summary, Details, State, Log, and Result are Markdown bodies.
Write each body as a durable reference for its reader.
Include the prerequisites needed to understand the record,
keep names stable,
make material causes and boundaries visible,
and distinguish evidence from inference or remaining uncertainty.
Omit context that does not help the reader use the record.

| Record | Reader | What the body establishes |
| --- | --- | --- |
| Summary | Descendants and reviewers | Stable outcome, inherited constraints, and acceptance boundary |
| Details | Executors and reviewers | Operative issue-local contract, current behavior, owned area, accepted decisions, and evidence requirements |
| State | Active execution and recovery | Current position, operative facts and uncertainty, and a separate next action |
| Log | Deeper recovery | Material decision or completed position with rationale, evidence, alternatives, and consequences |
| Result | Acceptors and dependents | Completed outcome, validation, material scope, and remaining gaps |

Summary and Details establish what work means.
Before primary work begins,
the issue contract and inherited context must let an executor act and a reviewer
assess the outcome without chat or reconstructing the intended change from
source.
Unknown implementation choices remain explicit investigation rather than
plausible detail.
Use [planning.md](references/planning.md) before creating or materially
revising executable issues.

Every ancestor Summary enters descendant context and each Summary has a
configured byte limit.
Details keep issue-local contract material out of sibling and descendant
context.

The current issue and each ancestor contribute State to contextual output.
`state set` replaces the active State body and optional next action,
so every replacement retains the facts that remain operative.
The next action is a separate Markdown field,
not a closing sentence buried in State.

Log preserves committed State snapshots and material decision context;
finite-work recovery may replay it completely.
Result is completion evidence rather than active recovery context or decision
history.

Progressively disclose context according to the executor's question:

- Initial execution starts from the current Summary and Details,
  inherited ancestor Summaries,
  completed dependency Results,
  and the board and execution environment.
- Continuing or recovering execution also relies on current and ancestor State.
- Deeper recovery expands Log, ancestor Details, descendant outcomes,
  or earlier Results only when they affect continuation.
- Acceptance and dependent work consume Result.

## Synchronize before dependent work

For claimed work,
State and Log are live execution memory on the primary task's critical path.
They are not conversational status or separate tracker administration.
Thinking, research, and execution may produce new knowledge.
Durable publication finishes that material transition;
the first edit, command, dispatch, or answer that consumes the knowledge comes
afterward.
Immediately before that consuming action,
compare the issue records with what the action assumes:

- State must contain the complete current premise and next action.
- Log must already contain a newly selected material design, strategy, policy,
  or behavior choice and its useful rationale.
- If accepted knowledge changes the stable contract for remaining work,
  Details must contain the complete new contract.

One transition may affect several records,
but publish each distinct effect once.
Materiality follows the behavior or constraint a choice establishes,
not the size of its implementation.
When a choice changes the active approach,
State carries its operative conclusion while Log carries its distinct rationale;
neither substitutes for the other.

Publication is part of completing a material transition.
A material choice is not selected until its applicable records are current,
and a new active position is not established until State matches it.
Work that has not changed the recoverable position may continue without an
update.
Delivery pressure may make the record concise;
it does not move publication to final handoff.
Final handoff summarizes an issue record already kept current by execution.

## Understand lifecycle and custody

Lifecycle, dependency readiness, custody, and waiting are distinct dimensions.
Cardamom presents an issue through these execution positions:

| Position | Meaning |
| --- | --- |
| Blocked | The issue is open and an unfinished dependency prevents execution |
| Ready | The issue is open, unclaimed, not waiting, and eligible for execution |
| In progress | The issue is open and one actor owns its claim |
| Waiting | The issue is open and unclaimed while directed continuation, acceptance, or an external condition is pending |
| Closed or cancelled | The issue has reached a terminal lifecycle |

A claim changes custody without closing the issue.
Only the claim owner releases ordinary custody.
Ordinary release leaves the issue open and returns it to its dependency-derived
position.
Waiting release leaves it open but outside automatic pools.
When the intended continuation can proceed,
an executor claims the issue directly by ID;
the claim clears waiting and establishes new custody.
An acceptor may inspect and close waiting work without claiming it when no
further execution is required.
The waiting reason communicates the continuation being awaited.
It does not reserve the issue, assign an actor,
or establish authority to perform that continuation.

Checkpoint approval or denial records an external decision rather than claimed
execution.
Routines acquire custody only through direct claim by known ID and do not
appear in automatic ready or blocked pools.

When an issue uses `phase:<name>` labels,
load [phased-workflows.md](references/phased-workflows.md) before changing its
State, phase label, or custody.

## Load the reference for the task

The shipped references contain the operating guidance needed without access to
Cardamom's source repository.

| Task | Reference |
| --- | --- |
| Resolve scope, perform requested setup, or prepare a worktree handoff | [scope.md](references/scope.md) |
| Define issue boundaries, self-contained contracts, relationships, or approval gates | [planning.md](references/planning.md) |
| Claim, execute, record, hand off, accept, or finish ordinary work | [execution.md](references/execution.md) |
| Recover interrupted work or reassign a stopped executor | [recovery.md](references/recovery.md) |
| Use `phase:<name>` labels for a non-standard workflow | [phased-workflows.md](references/phased-workflows.md) |
| Create, run, wait, or retire a recurring routine | [routines.md](references/routines.md) |
| Send actor or topic notifications through Cardamom | [mail.md](references/mail.md) |
| Decide whether an external resource needs exclusive coordination | [leases.md](references/leases.md) |
| Decide whether file bytes belong in durable attachment storage | [attachments.md](references/attachments.md) |

Use `card --actor <actor> <command> --help` for command forms or options not
shown by the selected workflow.
Installed CLI help owns complete command schemas such as `card apply`.
Put global flags before the subcommand,
and use `--json` when another program or agent parses output.

## Tests

When changing this skill, read [tests/README.md](tests/README.md).
Run the relevant scenarios with fresh subagents that have empty context windows.
