---
name: cardamom
description: >-
  Use when the user explicitly asks to use Cardamom or the card command,
  operate on an existing Cardamom store, board, or issue, or coordinate local
  or agent work through Cardamom, or asks about prior work or decisions recorded
  on a Cardamom board. Do not use for ordinary tasks merely because they could
  be tracked, or for explaining Cardamom concepts when no store, board, issue,
  or skill change is requested.
---

# Cardamom

Keep goals, progress, decisions, and outcomes understandable across sessions.
Follow the user's chosen workflow, delegation, and update cadence.

## Start from the work

Use the supplied store and board, and a stable `--actor` from the runtime
or task context.
Resolve setup or ambiguity with [scope.md](references/scope.md).
Reuse the issue for the same goal across sessions and ordinary work phases;
use [search.md](references/search.md) when its ID is unknown.

Put global flags before the command:

```bash
card --actor <actor> --store <store-path> --board <board-id> --json show <id> --context
```

Use `claim <id> --context` to take known unclaimed work;
use `show` to read or continue your own claim.
Context includes ancestor Summary and State, dependency Results, and board pins.
Read Log or ancestor Details for missing history.
For uncertain ownership or reassignment of a stopped executor, use
[recovery.md](references/recovery.md).
Coordinate with live claim owners, including any shared-record updates.
Helpers may return findings without a separate issue or claim.

## Record what the next reader needs

Write concrete facts for a reader without the chat.
A short issue may need only a Summary.
The command fragments below follow `card --actor <actor>` and the supplied scope.

| Record | Useful content | Command |
| --- | --- | --- |
| Summary | Problem, goal, and key constraints; inherited by descendants | `create '<title>' --summary '<brief>'` |
| Details | Optional context, approach, or acceptance criteria | `edit <id> --details '<context>'` |
| State | Current progress, useful evidence, blockers, and remaining work | `state set <id> '<progress>'` |
| Log | Consequential choices or discoveries with their reasons | `log post <id> '<choice and reason>'` |
| Result | Delivered outcome, supporting evidence, and material gaps | `result set <id> '<outcome>'` |

Capture consequential choices while their reasons are available.
Update progress after meaningful changes and at handoffs, respecting the user's cadence.
Use [records.md](references/records.md) for complex records or multiline input.

`state set` replaces State and its optional `--next` field;
retain still-useful facts when replacing it.
Use `state commit` when a displaced State should remain in Log.
Release and terminal operations already preserve changed State automatically.

## Leave a clear stopping point

On completion, record Result and `close <id>` when the task's acceptance
requirements are satisfied.
An acceptor can close satisfied work without claiming it.
Workstreams and routines require their children to be closed or cancelled.

The claim owner uses `release <id>` for another eligible actor to continue,
or `release <id> --waiting '<reason>'` for a named successor, acceptance, or external event.
Waiting issues stay outside automatic selection.
Direct claim clears waiting; resume only when authorized and its waiting condition is met.

## Load detail for the operation

| Need | Reference |
| --- | --- |
| Parents, dependencies, checkpoints, or generated issue graphs | [planning.md](references/planning.md) |
| Cancellation, checkpoint denial, or reopening | [termination.md](references/termination.md) |
| Recurring work and cursors | [routines.md](references/routines.md) |
| Exclusive use of a shared external resource | [leases.md](references/leases.md) |
| Notifications through Cardamom mail | [mail.md](references/mail.md) |
| File bytes that must outlive their current location | [attachments.md](references/attachments.md) |

Use `card <command> --help` for additional options.
Use `--json` for programmatic reads; list and Log output may be JSON Lines.
If `card` is unavailable, use this directory's `scripts/cardamom`
or `scripts/cardamom.ps1` on Windows.
