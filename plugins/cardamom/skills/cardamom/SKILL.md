---
name: cardamom
description: >-
  Use when the user explicitly asks to use Cardamom or the card command,
  operate on an existing Cardamom store, board, or issue, or coordinate local
  or agent work through Cardamom. Do not use for ordinary tasks merely because
  they could be tracked, or for explanation alone when no Cardamom operation or
  skill change is requested.
---

# Cardamom

Cardamom preserves work identity, shared context, custody, and recovery across
agents and sessions.
The collaboration runtime owns processes, dispatch, liveness, and worktrees;
`card` owns durable coordination.

Run `card` directly.
If it is unavailable,
retry the same invocation with `scripts/cardamom` from this skill directory
on macOS or Linux, or `scripts/cardamom.ps1` on Windows.

## Enter with the right identity and scope

Use the collaboration runtime's readable agent name as `--actor`.
Add a short runtime-assigned identifier only when agents share that name.
If the runtime supplies no identity,
choose a concise stable name from the task context.
Keep that actor stable throughout one claim, record, result, release, mail,
or lease lifecycle.
Do not use the machine username or an invented role such as `worker-a`.

Only the claim owner ordinarily releases a claim.
The stopped-executor recovery workflow contains the sole owner-attributed
release exception:
it requires confirmed process termination and explicit reassignment authority,
while the coordinator records its reasoning under its own actor.

Issue and attachment work resolves a board.
Mail and resource leases resolve only a store and may span its boards.
Projects organize boards but are not a separate execution selection.
Use `card board pin`, `card board unpin`, and `card board pins`
to change or inspect the selected board's pinned issues.
Use supplied scope,
report unresolved ambiguity instead of choosing or creating persistent scope,
and load [scope.md](references/scope.md) when scope is not established,
setup is requested, or work crosses worktrees.

## Continue one durable outcome

An issue is the durable boundary for one outcome.
Continue the issue that owns the same outcome across actors, sessions,
and ordinary execution phases.
Create another issue only when another outcome needs independent ownership,
sequencing, evidence, or acceptance.
Search before creation and use [planning.md](references/planning.md) to define or
materially revise an executable issue.
Before creating or replacing Summary or Details,
use [contracts.md](references/contracts.md) to select and fill the contract
profile that matches the work's decision shape.

Start selected, unclaimed work with `claim --context`.
When the current actor already owns the claim,
use `show --context` and continue without claiming again.
Requested `claim --context` and `show --context` output includes
the selected board's pinned issue IDs and current titles.
The claim owner executes the issue, maintains its records,
and performs its handoff.
An assisting actor without issue custody returns evidence to the claim owner.

Before using a shared external resource,
load [leases.md](references/leases.md) when harmful overlap may need Cardamom
coordination.
Before adding Cardamom mail to delegation or notification,
load [mail.md](references/mail.md) when the collaboration runtime does not
already provide the needed attention channel.

## Write for the next reader

Knowledge is material when losing it could change safe continuation,
acceptance, a dependent outcome, or the explanation of a material choice.
Its future reader and lifetime determine the record:

| Record | Reader | Record establishes |
| --- | --- | --- |
| Summary | Descendants and reviewers | Inherited outcome and acceptance boundary |
| Details | Executors and reviewers | Stable issue-local contract, plan, boundaries, and acceptance evidence |
| State | Execution and recovery | Current progress, evidence, uncertainty, and next action |
| Log | Recovery and review | Material decisions, discoveries, and position history |
| Result | Acceptors and dependents | Delivered outcome, validation, deviations, and gaps |

Together the issue and its records form one living plan without making one
record serve every reader or lifetime.
Issue status and claim expose lifecycle and custody.
The issue graph exposes constituent outcomes, dependencies, and approval gates.
Summary and Details establish the promised outcome and stable working contract.
State is the current progress and recovery snapshot.
Log preserves material discoveries and decision reasoning.
Attachments preserve artifacts whose bytes must outlive their original path.
Result compares the delivered outcome with the promise for acceptance and
dependent work.

Write each Markdown body as a self-contained durable reference for that reader.
Include needed prerequisites, keep names stable,
make material causes and boundaries visible,
and distinguish evidence from inference or uncertainty.
Do not rely on chat or require the reader to reconstruct the intended outcome
from source.

Initial execution starts from the current Summary and Details,
ancestor Summaries, completed dependency Results,
and the board and execution environment.
Continuation also uses current and ancestor State.
Expand Log, ancestor Details, descendant outcomes, attachments,
or earlier Results only when the current question needs them.
Acceptance and dependent work consume Result.

## Publish before knowledge is consumed

For claimed work,
durable publication completes a material transition.
The first edit, primary-work command, dispatch, status answer,
or other action that consumes changed knowledge comes after its issue records
are current.

Apply each predicate that holds:

| Condition | Durable action |
| --- | --- |
| Material work will begin and intent is not recoverable | Put intent and next action in State |
| Evidence changes position, uncertainty, blocker, or next action | Replace State completely |
| A material choice changes the active approach | Put the operative choice in State |
| A choice has useful rationale or consequences | Add distinct evidence to Log |
| Accepted knowledge changes the stable remaining contract | Replace Details completely |
| Accepted knowledge becomes necessary for every descendant | Replace Summary completely |
| A displaced State has replay value | Commit it while installing the next State |
| Delegated evidence changes a containing issue's position | Its claim owner publishes there |
| Execution completes | Set Result, then follow the applicable completion disposition |

One transition may affect several records,
but publish each distinct effect once.
Materiality follows the behavior, constraint, or recovery position established,
not implementation size.
Consecutive commands and mechanical edits may continue under one current
recorded position.
Elapsed time and a validation command following its edit do not create record
boundaries by themselves.
Delivery pressure may make a record concise;
it does not defer publication to handoff.

Load [execution.md](references/execution.md) for the publication loop,
custody choices, delegation, status reconciliation,
completion dispositions, and command forms.

## Load the workflow that owns the decision

| Decision or task | Reference |
| --- | --- |
| Resolve scope, perform setup, or cross worktrees | [scope.md](references/scope.md) |
| Define outcomes, contracts, graphs, or approval gates | [planning.md](references/planning.md) |
| Create or replace Summary or Details | [contracts.md](references/contracts.md) |
| Execute, publish, delegate, hand off, or accept | [execution.md](references/execution.md) |
| Recover interrupted work or reassign a stopped executor | [recovery.md](references/recovery.md) |
| Close, cancel, deny, or reopen work safely | [termination.md](references/termination.md) |
| Use phase labels for visible positions | [phased-workflows.md](references/phased-workflows.md) |
| Create, awaken, pause, or retire recurring work | [routines.md](references/routines.md) |
| Send actor or topic notifications through Cardamom | [mail.md](references/mail.md) |
| Coordinate exclusive use of an external resource | [leases.md](references/leases.md) |
| Preserve file bytes beyond their path | [attachments.md](references/attachments.md) |

Use `card --actor <actor> <command> --help` for command forms or options not
shown by the selected workflow.
Put global flags before the subcommand,
and use `--json` when another program or agent parses output.

## Tests

When changing this skill,
read [tests/README.md](tests/README.md).
Run the relevant scenarios with fresh subagents that have empty context windows.
