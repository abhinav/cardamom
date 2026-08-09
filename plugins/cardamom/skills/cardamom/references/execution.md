# Execute issue work

This workflow carries one ordinary issue from custody through execution,
handoff, and completion.

## Establish custody and context

When an issue is already selected,
claim it by ID and receive its assembled context:

```bash
card --actor <actor> --json claim <issue-id> --context
```

When choosing work from an automatic pool, constrain both the containing outcome
and the action class:

```bash
card --actor <actor> --json claim \
  --under <workstream-id> \
  --label <action-label> \
  --context
```

Repeated `--label` filters require every label.
Repeated `--label-any` filters permit any listed action label.
Use a negative label only for a real exclusion from a shared pool.

Before material work, confirm that the current Summary and Details,
inherited ancestor Summaries, completed dependency Results,
and execution environment establish the outcome, constraints,
and acceptance boundary without chat history.
When the contract cannot support execution, publish the gap and stop.
Retain custody only when the same actor owns the correction;
otherwise leave recoverable State and release the issue.

When work is delegated, give the executor this shipped skill,
the issue ID and runtime actor, the store and board selection,
the working directory and owned files, the required validation,
and the expected Result.
Use an absolute `card` path when another worktree may resolve a different
binary.

The claim owner remains responsible for the claimed issue's current State.
When delegated evidence changes that issue's active position or next action,
incorporate the operative conclusion before further work relies on it.
Keep worker-local chronology and supporting evidence on the child issue or
source record;
do not copy every checkpoint into the claimed issue mechanically.

## Reassess dependency-sensitive plans

Ready means that graph prerequisites are satisfied;
it does not establish that an earlier execution plan still fits.
When implementation choices depended on unfinished prerequisites,
read their Results and inspect the resulting system before continuing.

Use the new evidence to choose the current outcome:

- Incorporate a conclusion into Details only when it changes the stable
  issue-local contract needed to start work.
- Put the current execution position and next established transition in State.
- Preserve a changed or rejected approach in Log when its reasoning will matter
  later.
- Close or cancel the issue when prerequisite work already achieved its outcome
  or made the outcome unnecessary.

Do not preserve a superseded draft merely because work was previously planned.

Do not put every accepted conclusion in Details.
If an issue begins under-specified and accepted research establishes the
approach a new executor needs to begin,
replace Details with the complete stable working contract.
Keep the research evidence and alternatives in Log,
then replace State with the active execution position and next action.
Commit the completed research State first only when that distinct position has
replay value beyond the existing research Log.
If a conclusion changes only the continuation of work already in progress,
leave Details unchanged and carry the accepted approach in State.

## Keep active State current

State is the complete active working position,
not a transcript.
Its optional next action is the established transition from that position.
Because `state set` replaces both, every update retains the facts
that remain operative.

Publish intent before starting material work when interruption would otherwise
leave the active position ambiguous:

```bash
card --actor <actor> state set <issue-id> \
  'The parser failure is being reproduced; no repair is selected.'
```

As an action produces useful facts,
replace State with the current result and next established transition.
Partial work needs an update when the recoverable position changes,
not after every command.

Incomplete reconciliation does not justify retaining State
that established evidence has disproved.
Record the confirmed position,
label unresolved evidence as pending,
and state how it will be reconciled.
For quantitative work, a conservative confirmed floor is sufficient
until pending evidence is reconciled.
Current State must not retain an earlier execution position
after accepted evidence establishes a transition.
For example, State cannot still describe work as paused
after accepted evidence establishes that execution resumed.

Before reporting current status, compare State and its next action
with active execution and accepted delegated results.
Update State to resolve a material contradiction before answering.

## Preserve decisions and completed positions

Use a standalone Log post for material design, strategy, policy,
or behavior choices and for distinct reasoning, evidence, alternatives,
or consequences needed to reconstruct the choice or path.

```bash
card --actor <actor> log post <issue-id> - <<'LOG'
The command adapter retains the existing result type.
Adding a second result type would move transport policy into the domain API and
force non-rendering callers to depend on command concerns.
LOG
```

Replace State separately only when the choice changes the active position or
next action.

Use `state commit` when the current State must remain available after the next
active position replaces it:

```bash
card --actor <actor> state set <issue-id> \
  'The malformed escape is reproduced in the quoted-input regression.' \
  --next 'Select a repair that preserves escape-state evidence.'

card --actor <actor> state commit <issue-id> \
  --set 'The reproduction is recorded; repair selection is in progress.' \
  --next 'Choose the repair and update the regression.'
```

The commit snapshots the completed reproduction State into Log and selects the
new active State.
It needs no standalone Log entry containing the same reproduction.
Elapsed time, one long command, and incomplete mechanical edits do not create a
snapshot by themselves.

Promote only the smallest conclusion every descendant needs into the containing
Summary.
Keep decision evidence, alternatives, and chronology in Log.
Read Summary or Details before replacing it,
retain every still-operative part, and replace obsolete guidance
rather than accumulating history.

## Author durable records

Write records for their rendered meaning.
Use a single-quoted scalar for a simple one-line body
and a single-quoted heredoc for multiline or Markdown-rich input:

```bash
card --actor <actor> log post <issue-id> - <<'LOG'
## Parser evidence

`parser inspect $TARGET` preserves the literal target spelling.
The documented escape spelling is `\n`.
LOG
```

The quoted delimiter preserves dollar signs, backticks, and backslashes.
Use real lines rather than serialized escape sequences when the rendered
record needs paragraphs or lists.

Cardamom references provide durable navigation:

| Target | Form |
| --- | --- |
| Issue | `%<issue-id>` |
| Log entry | `%log_<id>` |

Keep the material conclusion in the surrounding record;
a reference does not carry its meaning.
Use a full navigable URL for an external artifact needed for inspection or
continuation.

Use [attachments.md](attachments.md) for attachment references
and when later work needs produced file bytes.

## Finish, hand off, or wait

Result owns the completed outcome and validation.
Do not repeat that same completion or validation in standalone Log;
use Log at finish only for distinct reasoning or evidence that Result and
committed State do not preserve:

```bash
card --actor <actor> result set <issue-id> "$(cat <<'RESULT'
Implemented the parser repair.

The quoted-input regression and required parser validation pass.
RESULT
)"
```

At completion or acceptance handoff, replace State with the operative
acceptance position rather than repeating Result:

```bash
card --actor <actor> state set <issue-id> \
  'Execution is complete; Result records the outcome and validation.' \
  --next 'Inspect Result and accept or return the issue.'
```

At a partial or interrupted handoff, State instead retains completed
and remaining work,
relevant files or artifacts, observed validation, blockers,
and the next established action.

Choose release by the intended continuation:

```bash
# Any eligible actor may select the issue from an automatic pool.
card --actor <actor> release <issue-id>

# Continuation is directed, awaits acceptance, or needs an external event.
card --actor <actor> release <issue-id> \
  --waiting '<continuation reason>'
```

Release ends custody and preserves changed State in Log automatically.
Ordinary release returns the open issue to its dependency-derived position.
Waiting keeps the open issue outside automatic pools.
The next executor claims waiting work directly by ID with `--context`.

Use an independent acceptor only when the issue contract requires independent
acceptance.
Otherwise the executor responsible for the outcome may complete the ordinary
acceptance and closure work.

## Accept or terminate work

An acceptor reads Result and material child outcomes,
records the acceptance decision,
then closes the issue:

```bash
card --actor <acceptor-actor> --json result show <issue-id>
card --actor <acceptor-actor> log post <issue-id> \
  'The recorded outcome satisfies the issue contract and required validation.'
card --actor <acceptor-actor> close <issue-id>
```

Workstreams and routines close after every direct child is closed or cancelled.
Terminal operations preserve changed State automatically.

`cancel` and `checkpoint deny` cancel non-terminal transitive dependents.
Before either operation, walk every direct `blocks` edge,
tracking seen issue IDs
until no unseen dependent remains:

```bash
issue_json=$(card --actor <actor> --json show <issue-id>)
printf '%s\n' "$issue_json" | jq -r '.blocks[]'
```

Inspect the resulting non-terminal set before applying the terminal operation.
Record rationale before `close`, `reopen`, or `cancel`;
those commands do not accept a reason flag.

Use `state set <issue-id> ''` only when intentionally discarding temporary
State without preserving it.
