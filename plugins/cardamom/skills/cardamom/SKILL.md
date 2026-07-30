---
name: cardamom
description: >-
  Use when the user explicitly asks to use Cardamom, the card command, an
  existing Cardamom store, or Cardamom-coordinated local task or agent work.
  Work on an existing selected board unless the user explicitly asks to
  initialize, create, or select one.
  Coordinate persistent workstreams, bounded tasks, reusable routines,
  dependencies, claims, durable context, mail, checkpoints, and resource
  leases. Do not use for ordinary tasks unless the user asks for Cardamom.
---

# Cardamom

Use `card` with the selected Cardamom store for durable coordination and
recovery.
The default project-local store is `.cardamom`,
but `--store` or `CARDAMOM_STORE` may select another physical store directory.
The collaboration runtime owns agent dispatch, liveness, and worktrees;
`card` owns durable coordination state.

Run workflow commands with `card` directly.
If the shell reports that `card` is unavailable,
retry the same command with `scripts/cardamom` from this skill directory
on macOS or Linux,
or `scripts/cardamom.ps1` on Windows.
The launcher preserves every argument.

## Establish identity and scope

Choose one stable actor name before the first `card` invocation.
Pass that name with `--actor` on every command,
including reads.
Each root agent and each subagent has its own actor identity.
Do not share an actor name between concurrently operating agents.
Keep the same actor for the full claim, record, result, release, mail,
or lease lifecycle that identity owns.
Do not rely on the machine username default.

Examples use names such as `coordinator` and `worker-a`.
Replace them with identities that distinguish the participating agents.

Cardamom scopes durable concepts as follows:

| Scope | Concepts |
| --- | --- |
| Store | Actor identities and mailboxes, topic subscriptions, resource lease names |
| Project | Repository or product namespace containing boards |
| Board | Description, issues, containment and dependencies, claims, attachments |

Issue and attachment work requires a selected board.
Mail and leases require a store but do not require a selected board.

## Start from the agent task

When matching work may already exist,
search the selected board and inspect plausible issues before creating one.
Continue an existing issue across agents and sessions instead of allocating a
replacement for the same outcome.

Start issue execution from `claim --context` or `show --context` as directed by
the execution or recovery workflow.
The board description, ancestor summaries and states,
direct dependency results,
and current issue records provide progressively narrower context.
Read ancestor details, full log entries, terminal descendants,
or prior results only when the current decision needs them.

Before using an external resource that other actors may also use,
decide whether concurrent work could overlap unsafely.
When no resource-native protection or established coordinator reliably
prevents that overlap,
load [leases.md](references/leases.md) and acquire a lease before resource use.

## Keep records distinct

Each issue record serves one reader task:

| Record | Reader and contents |
| --- | --- |
| Title | Compact plain-text identity for lists, routing, and commands |
| Summary | Concise stable Markdown inherited by descendants: outcome, constraints, acceptance criteria, and conclusions children must know |
| Details | Expanded stable Markdown retrieved on demand: rationale, procedure, examples, and supporting context |
| State | Mutable recovery facts and an optional planned next action |
| Log | Committed State snapshots plus standalone decisions, reasoning, and evidence needed to replay the work |
| Result | Durable outcome and validation evidence for acceptors and dependent work |

Structure Markdown records for the reader's scanning task.
Use short paragraphs for one connected thought,
bullets or tables for independent facts,
and small domain-specific headings when a record has several sections.
Separate paragraphs with blank lines;
single line breaks still render as one paragraph.

Treat State as the first recovery surface.
When execution establishes a materially new position,
replace the State body with the complete current recovery facts
before later work depends on them.
Supply the optional planned transition with `--next`;
omit it when no next action is established.
Structure the body around the work's domain concepts,
without generic `Current state` or `Next action` wrapper headings.
During active execution,
use `state commit` when a phase produces a coherent recovery position,
such as a selected strategy or a coherent implementation awaiting validation.
Incomplete mechanical work and command duration do not create checkpoints by
themselves.
Use `log post` for material design, strategy, or policy choices
whose evidence, rationale, alternatives, or consequences help replay the work.
Use it also for other replay-worthy evidence or handoff material
that does not belong in current State.
Release and terminal lifecycle operations preserve changed State automatically.
Keep Result separate from both State and Log:
Result holds the completed outcome and validation,
while the final State body names the acceptance position
and its next action directs acceptance.
Use `state set <issue-id> ''` when intentionally removing State without
preserving it.

When a conclusion becomes materially necessary for child tasks,
promote a concise version into the containing issue's summary.
Keep supporting rationale and evidence in details or log entries.

## Author object references

When a State, Log, or Result cites a published external artifact needed for
inspection or continuation,
use its full navigable URL as the durable reference.
Resolve the URL when only local shorthand is available;
the shorthand may remain as link text.

Use a Cardamom reference when navigation helps the record's intended reader:

| Target | Markdown form | Use |
| --- | --- | --- |
| Issue | `%<issue-id>` | Refer to an issue |
| Log entry | `%log_<id>` | Cite chronology that helps replay or inspect reasoning |
| Attachment | `%att_<id>` | Link the attachment under its stored filename |

Use `%log_<id>` only when its chronology helps the intended reader.
Keep every materially important conclusion directly in the appropriate durable
record or surrounding prose;
a reference supplies navigation rather than the conclusion.

For a custom attachment label,
use `[label](attachment:<id>)`.
For an image,
use `![alt](attachment:<id>)`.
Load [attachments.md](references/attachments.md)
when preserving or recovering attachment bytes.

Classify each durable body by its intended rendered value,
not by a serialized draft.

| Body shape | Shell form | Important behavior |
| --- | --- | --- |
| One line | Single-quoted scalar argument | Safe default that preserves shell metacharacters |
| Multiline State, Log, Result, Summary, Details, mail, or analogous text | Single-quoted heredoc (required) | Real line breaks, dollar signs, backticks, and backslashes are preserved |

`card` does not interpret escape sequences.
A quoted `\n` remains two bytes,
so replace serialized line-break tokens with real lines in the required heredoc.

## Preserve custody and decisions

Meaningful execution belongs to a claimed issue.
While a claim is active,
keep its State body aligned with established recovery facts
and its optional next action aligned with the planned transition.

Choose release behavior by who should be eligible to continue:

| Continuation intent | Release behavior |
| --- | --- |
| Any eligible actor may select the issue from a pool | Ordinary `release` |
| A coordinator will direct the next executor | `release <issue-id> --waiting "<reason>"` |
| Acceptance or an external condition must occur next | `release <issue-id> --waiting "<reason>"` |

The next executor claims waiting work explicitly by issue ID with `--context`.
For acceptance,
the executor records a useful outcome with `result set` before release.
The acceptor inspects with `result show`,
records the decision with `log post`,
and runs `close` without claiming unless more execution is required.

Record the selected position of each material choice in State
before dependent work begins or resumes,
then commit State when that choice produces a coherent recovery position during
active execution.
When a design, strategy, or policy choice constrains later work,
also write one standalone Log post with the material evidence,
rationale, rejected alternatives, and downstream consequence.
Repeat only enough of the selected position to orient that reasoning;
do not copy the complete State or planned next action.
Do not record command-by-command narration when several operations only carry
out a choice already recorded.

## Load only the needed workflow

The guidance above is sufficient when a task only drafts or revises generic
board-issue record prose.
Load the workflow reference for every command-oriented task:

| Agent task | Reference |
| --- | --- |
| Discover a store or board, resolve board-scoped work, or perform explicitly requested setup | [boards.md](references/boards.md) |
| Create or revise issues, relationships, labels, or checkpoints | [planning.md](references/planning.md) |
| Move one issue through optional human-visible phases or automatic action pools | [phased-workflows.md](references/phased-workflows.md) |
| Build or reconcile a multi-issue graph with `card apply` | [apply.md](references/apply.md) |
| Claim, execute, delegate, record progress, hand off, release, close, or cancel work | [execution.md](references/execution.md) |
| Resume unfinished work from durable context | [recovery.md](references/recovery.md) |
| Create or revise routine records; awaken, run, release, or retire a routine | [routines.md](references/routines.md) |
| Preserve, reference, retrieve, or reuse file bytes | [attachments.md](references/attachments.md) |
| Send or receive ephemeral coordination messages | [mail.md](references/mail.md) |
| Coordinate exclusive ownership of an external resource | [leases.md](references/leases.md) |

For board-scoped issue or attachment work,
load `boards.md` first when the prompt, handoff,
or current context does not establish the selected board.
For store-scoped mail or lease work,
use the supplied `--store`, `CARDAMOM_STORE`, or automatic store discovery.
If those inputs do not establish one store,
report the ambiguity instead of selecting or inspecting a board.

Before composing or running a `card` command,
read its workflow reference and use the command form shown there.
For an uncommon option or input shape not shown there,
run `card --actor <actor> <command> --help`.
Put global flags before the subcommand.
Use `--json` whenever another program or agent will parse structured output.
Plain `card create` output is the stable issue ID and may be captured directly.

Structured output uses these framing contracts:

| Shape | Commands | Consumption |
| --- | --- | --- |
| Scalar issue ID | Plain `create` | Capture the complete output as one opaque ID |
| One JSON value | Single-record commands | Parse one value directly |
| JSON Lines | `board list`, `list`, `ready`, `blocked`, `log show`, `mail publish`, `mail recv`, `mail subscriptions`, `lease list` | Stream records or use `jq -s` for an array |
| Collection envelope | `attachment list` | Read the fields documented by its workflow or help |

## Tests

When changing this skill,
read [tests/README.md](tests/README.md).
Use fresh subagents with empty context windows for behavioral validation.
Run mechanical validation and disposable-store command probes when guidance
depends on CLI behavior.
