# Executing work

## Reassess dependency-sensitive work

A ready issue has satisfied its graph prerequisites;
readiness does not establish that an earlier implementation plan still fits.
Before dispatching an issue whose implementation choices depended on unfinished
prerequisites,
read the prerequisite results and inspect the resulting system.

Keep the summary centered on the durable outcome, constraints,
and acceptance criteria.
Put expanded stable implementation guidance established by the prerequisites
in details when future executors may need it on demand.
Record the execution plan derived from the completed prerequisites in the state,
and add a log entry when the reassessment changes or rejects an earlier approach.
If the prerequisites already achieved the outcome or made the issue unnecessary,
close or cancel the issue instead of dispatching obsolete work.

## Dispatch delegated work

Attach the Cardamom skill to every delegated worker prompt
and state that the worker must follow it as the governing coordination protocol.

Check every handoff before dispatch:

- Attach this skill rather than paraphrasing its duties.
- Name the issue ID and stable worker actor.
- Supply the store and explicit board selection.
- Supply the working directory or worktree and owned files.
- State required validation and the expected durable result.
- For another worktree,
    verify that the supplied store path reaches the intended store and that the
    worker's `card` executable is the intended build.
- Pass an absolute executable path when `PATH` may resolve a worktree-local or
    stale binary.
- Dispatch the prepared prompt through the collaboration runtime.
    Use Cardamom mail only when the task independently requires an ephemeral
    notification.

The attached skill supplies the record and workflow decisions;
a paraphrase of selected duties is not an equivalent handoff.

## Claim established work

When dispatch already selected an issue,
claim that issue by ID and start from its inherited context:

```bash
card --actor worker-a --json claim <issue-id> --context
```

When selecting from a pool,
filter by the containing workstream and a positive action label
that identifies work the actor may perform:

```bash
card --actor worker-a --json claim \
  --under <workstream-id> \
  --label implementation \
  --context
```

When one persistent issue moves through several visible phases or action pools,
load [phased-workflows.md](phased-workflows.md) before changing its labels.

Repeated positive `--label` filters require every label.
Use a negative term such as `--label -platform:windows`
only when a shared pool must exclude a classification the actor cannot serve.
Use repeated `--label-any` terms when the same actor may take work from any
of several positive action pools.
Claiming creates active custody while lifecycle remains open.
Only the owning actor may release that custody.

Confirm that the title, summary, and any necessary details support the assigned
outcome before material work begins.
If the contract is incomplete,
record the gap and stop.
Retain custody only when the same actor owns the corrective action;
otherwise record the recovery state and release the issue.

## Keep State current

Use the canonical record roles in `SKILL.md`.
Update them at these execution boundaries:

| Moment | Record operation | Must happen before |
| --- | --- | --- |
| A material investigation or change begins and interruption would leave intent ambiguous | Put active recovery facts in the State body and consume a started next action | Starting that work |
| A reproduction, strategy choice, implementation outcome, validation result, blocker, or planned transition changes | Replace the State body and optional next action | Dependent work |
| A phase produces a coherent recovery position during active execution | Commit State | Dependent work that should rely on the phase outcome |
| A material design, strategy, or policy choice establishes rationale that later work may need to replay | Add one standalone Log post | Dependent work that relies on the choice |
| Other replay-worthy evidence does not belong in current State | Add one standalone Log post | Dependent work that needs the material |
| A conclusion becomes necessary for child tasks | Promote the concise conclusion to the containing summary | Dispatching dependent children |
| An acceptor needs the finished outcome | Set the result | Releasing for acceptance |

The State body is the recovery source for established facts.
The optional next action is the planned transition from those facts.
`state set` replaces both,
so retain every still-operative fact in the body
and pass an established transition with `--next`.
When work begins that transition,
omit `--next` from the replacement until another action is established.
When State contains several independent facts or recovery dimensions,
use bullets, short paragraphs, or small domain-specific headings
instead of one dense paragraph.
The command output supplies State and next-action labels,
so do not add generic wrapper headings for those fields.
Commit current State when a phase produces a coherent recovery position,
such as a selected strategy or a coherent implementation awaiting validation.
Incomplete mechanical work and command duration do not create checkpoints by
themselves.
Do not wait for final handoff when dependent work already relies on that
phase outcome.

Use `log post` for a material design, strategy, or policy choice
when later work may need its evidence, rationale, rejected alternatives,
or downstream consequence.
The State body should still name the selected position as current recovery truth.
The standalone post may repeat that position briefly to orient its reasoning,
but must not copy the complete State or planned next action.
Use `log post` also for replay-worthy evidence, surprises,
or handoff material that does not belong in current recovery truth.
Write a standalone post reference-first:
name the stable referents and evidence that support its observation, decision,
or conclusion,
preserve material rationale or a rejected alternative,
and state the downstream consequence.
Include from State only the concise decision context needed to understand the
post,
not the complete recovery position or planned next action.
When a phase only carries out an already recorded choice
and produces no new decision, surprise, blocker, or evidence consequence,
do not add a standalone post.
Do not log each command when consecutive commands only carry out a choice
already recorded.
When a stable record benefits from a direct path to one log entry,
cite `%log_<id>` only when that chronology helps the intended reader.
Keep the materially important conclusion in the appropriate durable record.

At a strategy checkpoint,
write recovery truth first and commit it:

```bash
card --actor worker-a state set <issue-id> "$(cat <<'STATE'
`scanner/quoted_input_test.go:TestMalformedEscape` reproduces the parser defect.
Preserving token escape state is the selected repair strategy.
STATE
)" --next 'Implement escape-state preservation in `scanQuoted`.'
card --actor worker-a state commit <issue-id>
```

If the rejected alternative remains useful for later replay,
record a concise standalone explanation:

```bash
card --actor worker-a log post <issue-id> - <<'LOG'
`scanner/quoted_input_test.go:TestMalformedEscape` distinguishes a valid `\"`
from malformed `\q`.
Preserving escape state in `scanQuoted` keeps that evidence available through
validation.
Normalizing in `readToken` was rejected because it erases the distinction
and would let malformed input reach token construction.
`parseValue` can therefore continue to consume only escape-validated tokens.
LOG
```

Summary and details edits also replace their selected value;
inspect the current issue before preserving and promoting stable content.

Record a useful outcome before handing control to an acceptor:

```bash
card --actor worker-a result set <issue-id> "$(cat <<'RESULT'
Implemented the parser fix at `abc123`.

`mise run test` passed with 142 tests.
RESULT
)"
```

At acceptance handoff,
State should say that execution is complete and direct the acceptor to Result.
Do not copy the completed outcome or validation evidence from Result into State.

When temporary State should be removed without entering history,
set it to an explicitly empty body:

```bash
card --actor worker-a state set <issue-id> ''
```

Use an empty `state set` for intentional unsnapshotted removal,
not for ordinary phase advancement, handoff, or completion.

## Preserve produced artifacts

Attach a produced file when a later agent or acceptor needs its bytes after the
current process, session, or worktree ends.
Load [attachments.md](attachments.md),
then link the attached evidence from the log or result that explains it.

## Hand off or release

Before releasing custody,
keep the State body current with the recovery position
and use its optional next action to identify continuation,
including the responsible actor or external trigger when relevant.
Release preserves changed State in the Log automatically.
Do not run `state commit` solely to prepare for release.
Add a standalone Log post when the handoff introduces a material choice
or other replay-worthy material that deserves its own reasoning record.
Use an ordinary release when the issue should return to automatic claim pools,
whether the work is new or partially complete:

```bash
card --actor worker-a release <issue-id>
```

Release leaves the issue open and ends the actor's custody.
Release into waiting status when continuation is deliberately directed,
requires acceptance,
or depends on an external condition:

```bash
card --actor worker-a release <issue-id> \
  --waiting "root acceptance"
```

`--waiting` requires a reason and removes the issue from automatic claim pools.
Use waiting for a worker-to-worker handoff when a coordinator will explicitly
choose or dispatch the replacement,
even when that replacement can continue immediately.
Do not use waiting when the intent is to let any eligible actor select the
issue from a claim pool.
For a directed handoff,
the current actor leaves recoverable state and waiting custody before the
selected replacement claims the same issue:

```bash
card --actor worker-a state set <issue-id> \
  "Implementation is partial." \
  --next "Continue the recorded parser strategy."
card --actor worker-a release <issue-id> \
  --waiting "worker reassignment"
card --actor worker-b --json claim <issue-id> --context
```

The explicit claim clears waiting status and resumes custody.
An acceptor may instead inspect and close the waiting issue without claiming it.
There is no force release or claim transfer.

Do not make chat history the only source of unfinished recovery state.

## Inspect transitive cancellation impact

`cancel` and `checkpoint deny` cancel every non-terminal transitive dependent.
Before either operation,
walk the direct `blocks` edges until no unseen issue remains:

1. Put each issue that may be cancelled in a queue.
2. Run `card --actor coordinator --json show <issue-id>` for each unseen issue.
3. Record the issue ID and status,
    then add every ID from `.blocks[]` to the queue.
4. Repeat until the queue contains no unseen ID.
5. Inspect the resulting non-terminal set before applying the operation.

For one traversal step:

```bash
issue_json=$(card --actor coordinator --json show <issue-id>)
printf '%s\n' "$issue_json" | jq -r '.blocks[]'
```

Treat IDs as opaque and keep a seen set so converging graph paths are inspected
once.

## Complete and accept

The executor records the result.
When a separate acceptor owns the next action,
the executor releases custody with `--waiting` after recording the result.
The actor responsible for acceptance inspects the result and material child
outcomes, records acceptance in a log entry, and closes the issue:

```bash
card --actor coordinator --json result show <issue-id>
card --actor coordinator log post <issue-id> - <<'LOG'
## Acceptance

The recorded outcome satisfies the workstream criteria.
Required validation and resource disposition are recorded on the child issues.
LOG
card --actor coordinator close <issue-id>
```

Workstreams close explicitly after every direct child is closed or cancelled.
Use a separate acceptor only when the contract requires independent acceptance.
Record rationale in a log entry before `close`, `reopen`, or `cancel`;
those commands do not accept a reason flag.
Terminal operations preserve changed State automatically.
Do not commit State separately only to prepare for the terminal operation.
Use the transitive cancellation-impact procedure above before `cancel`.
