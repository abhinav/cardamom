# Execute issue work

This workflow carries one ordinary issue from custody through publication,
execution, status reporting, and its next disposition.

## Enter at the current position

Use established custody, durable records, and current evidence to find the
first unresolved decision:

| Task position | Enter at |
| --- | --- |
| Ordinary selected work is unclaimed, or its executable contract is not established | [Establish custody and a usable contract](#establish-custody-and-a-usable-contract) |
| A plan depended on conditions that have now changed | [Reassess plans when conditions change](#reassess-plans-when-conditions-change) |
| Another actor may own an independent outcome | [Choose custody before dispatch](#choose-custody-before-dispatch) |
| The current actor will execute or execution is active | [Run the publication loop](#run-the-publication-loop) |
| Execution and validation are complete | [Choose the next disposition](#choose-the-next-disposition) |
| Interrupted work has uncertain prior custody or context | [Recover interrupted issue work](recovery.md) |

Start at that decision and continue only through later decisions the task still
needs.
Revisit an established stage only when its evidence is absent, invalidated,
or contradicted.

## Establish custody and a usable contract

Claim selected unclaimed work by ID and receive its assembled context:

```bash
card --actor <actor> --json claim <issue-id> --context
```

If the current actor already owns the claim,
inspect the current context instead of claiming again:

```bash
card --actor <actor> --json show <issue-id> --context
```

When choosing from an automatic pool,
constrain both the containing outcome and action class:

```bash
card --actor <actor> --json claim \
  --under <workstream-id> \
  --label <action-label> \
  --context
```

Repeated `--label` values require every label;
`--label-any` permits any listed label.

Before primary work,
confirm that the current Summary and Details, ancestor Summaries,
completed dependency Results, and execution environment establish the intended
outcome, owned area, constraints, first safe action, and acceptance evidence.
Source inspection may support that contract;
it must not be the only place that identifies what the issue means.
Use [planning.md](planning.md) to repair a generic or incomplete contract.
If established knowledge cannot support safe execution,
publish the gap and stop.

Treat accepted issue records as durable task knowledge.
Before repeating investigation,
identify the record that owns the needed conclusion and decide whether its
scope, provenance, and relevant conditions still support the next action.
When they do,
use the recorded conclusion rather than reopening its cited sources merely for
confidence.
Investigate again when the needed conclusion or provenance is missing or
ambiguous,
a relevant source, dependency, environment, or acceptance requirement changed,
or current evidence contradicts the record.
A new actor, session, or desire for personal confidence does not make accepted
knowledge stale.

## Reassess plans when conditions change

Ready means graph prerequisites are satisfied;
it does not prove that an earlier plan still fits.
When a plan depended on unfinished prerequisites,
read their Results and inspect the resulting system before dispatch or
implementation.

Use the new evidence to choose one disposition:

- keep the accepted plan when its recorded conditions still apply;
- replace Details when the stable remaining contract changes;
- replace State with the current position and next established transition;
- add Log only for distinct replay-worthy reasoning;
- close or cancel when prerequisite work already achieved the outcome or made
  it unnecessary; or
- publish the unresolved gap and stop when no safe plan is established.

Do not preserve or dispatch a superseded draft merely because the issue became
ready.

## Choose custody before dispatch

Decide whether another actor will own an independent outcome
or assist inside the issue the coordinator continues to own.

For a delegated issue,
give the executor the shipped skill, issue ID and runtime actor,
required validation, expected Result, and completion or handoff disposition.
When dispatch crosses a process or worktree boundary,
follow [scope.md](scope.md) for explicit store, board, working directory,
owned files, and `card` executable context.
Require the executor to claim before material work
and maintain the delegated issue's records while it owns execution.

If the coordinator already holds the delegated claim,
publish any material work already performed,
then release it before the executor claims.
The delegated executor owns that issue's State, Log, Result, and handoff.
The containing claim owner publishes only delegated conclusions that change the
containing issue's position or decision trail;
chronology and supporting evidence remain on the delegated issue.

For assistance within one claimed issue,
give the helper a bounded request and an evidence-return contract.
The claim owner retains record responsibility and publishes accepted evidence
when it changes the active position, decision trail, or outcome.

Use [mail.md](mail.md) only when an asynchronous Cardamom attention channel is
needed in addition to durable issue records and runtime dispatch.

## Run the publication loop

The claim owner repeats this loop:

1. Compare the issue record with the premise, choice,
   and recovery position the next action will consume.
2. Apply every matching publication predicate from the primary skill.
3. Perform the dependent primary-work action.
4. Re-enter the loop when evidence, a choice, the active position,
   or the next action changes.

State is the complete active position rather than a transcript.
Because `state set` replaces both State and its optional next action,
every update retains facts that remain operative.

Publish intent before material work when interruption would otherwise leave the
position ambiguous:

```bash
card --actor <actor> state set <issue-id> \
  'The parser failure is being reproduced; no repair is selected.' \
  --next 'Run the focused reproduction and inspect the failing path.'
```

When evidence selects a material repair,
put the operative choice in State before implementation.
Add a Log post only when distinct rationale, provenance, alternatives,
or consequences will help later recovery or review:

```bash
card --actor <actor> state set <issue-id> \
  'The malformed escape is reproduced; preserving scanner escape state is the selected repair.' \
  --next 'Implement escape-state preservation and run the focused regression.'
card --actor <actor> log post <issue-id> - <<'LOG'
The scanner retains escape state until validation.
Normalizing in the token reader was rejected because it would erase evidence
needed to distinguish valid and malformed escapes.
LOG
```

The Log entry does not replace the operative choice in State
and should not copy the complete State or next action.

Use `state commit` when a completed State has replay value after the active
position advances:

```bash
card --actor <actor> state commit <issue-id> \
  --set 'Implementation is complete; validation is the active position.' \
  --next 'Run the required validation.'
```

A committed State snapshot needs no standalone Log entry containing the same
position.
Elapsed time, one long command, and incomplete mechanical edits do not create a
snapshot by themselves.
When temporary State should disappear without entering history,
clear it with `state set <issue-id> ''`.
Use bare `state commit` instead when the current position should remain
replayable but no replacement State is active.

Before reporting status,
compare State and its next action with active execution and accepted delegated
evidence.
Resolve a material contradiction before answering.
Record confirmed facts, pending evidence, and how it will be reconciled;
do not leave a disproved or superseded position active while waiting for
complete accounting.

## Author durable bodies and references

Write each record for the decision or continuation its reader owns.
Account for graph context without relying on chat or unstated investigation.
Introduce prerequisites before dependent claims, reuse stable names,
and make material scope, causes, evidence, and uncertainty explicit.
Keep chronology only in records that own history.

Use concrete referents at the granularity needed to act.
Name the outcome, affected area, command, artifact, evidence, or decision when
that identity distinguishes the current position or next action from a generic
activity.
Reuse a concise domain term after its record establishes what it denotes;
do not copy the stable contract into State merely to avoid established
shorthand.

Structure a body for the reader's scanning task.
Use short paragraphs for one connected thought,
bullets or tables for independent facts,
and small domain-specific headings when several sections matter.
Separate paragraphs with blank lines;
single source line breaks still render as one paragraph.
Do not add generic `State`, `Current state`, or `Next action` headings around
fields whose command output already supplies those roles.

Use a single-quoted scalar for a simple one-line body.
Use a single-quoted heredoc for multiline or Markdown-rich input
so dollar signs, backticks, and backslashes remain literal:

```bash
card --actor <actor> log post <issue-id> - <<'LOG'
## Parser evidence

`parser inspect $TARGET` preserves the literal target spelling.
The documented escape spelling is `\n`.
LOG
```

Use real lines rather than serialized newline escapes.
When a multiline State also has a scalar next action,
put the stdin selector and flags before the heredoc redirection:

```bash
card --actor <actor> state set <issue-id> - \
  --next 'Implement the selected repair.' <<'STATE'
The failure is reproduced and the repair is selected.
STATE

card --actor <actor> state commit <issue-id> \
  --set - --next 'Run validation.' <<'STATE'
Implementation is complete; validation is active.
STATE
```

Reference issues as `%<issue-id>` and Log entries as `%log_<id>`.
Keep the material conclusion in the surrounding record;
a reference supplies navigation rather than meaning.
When a published external artifact is required for continuation or inspection,
store its full navigable URL rather than only local shorthand.
Use [attachments.md](attachments.md) when later work needs produced file bytes.

## Choose the next disposition

Result owns the completed outcome and validation.
Choose the disposition from the issue contract and actual execution state:

When active work will stop because focus changes,
make the issue's State recoverable before switching and choose its custody
deliberately.
Keep the claim when the same actor retains ownership and a recoverable
continuation remains established.
Release ordinarily when any eligible actor may continue,
or release waiting when continuation is directed or externally gated.
Cancel only when the outcome is authorized for abandonment or replacement;
a focus change alone does not establish either condition.
A brief interruption that changes neither custody nor the recorded position
needs no release or ritual record update.

| Condition | Required sequence |
| --- | --- |
| Complete; executor may accept | Set Result, then follow the close workflow |
| Complete; independent acceptance | Set Result, install acceptance State, release waiting |
| Partial; any eligible actor may continue | Install recoverable State, release ordinarily |
| Partial; directed or external continuation | Install recoverable State, release waiting |

Use [termination.md](termination.md) to close completed work or choose another
terminal disposition.

Do not copy Result into State or a standalone Log entry.
At independent handoff,
State identifies the acceptance position and separate next action:

```bash
card --actor <actor> result set <issue-id> - <<'RESULT'
Implemented the parser repair.

The focused regression and required parser validation pass.
RESULT
card --actor <actor> state set <issue-id> \
  'Execution is complete; Result records the outcome and validation.' \
  --next 'Inspect Result and accept or return the issue.'
card --actor <actor> release <issue-id> \
  --waiting 'independent acceptance'
```

Ordinary release returns open work to its dependency-derived pool.
Waiting release ends custody and keeps open work outside automatic pools.
Release preserves changed State automatically;
do not run `state commit` solely to prepare for release.
The next executor claims waiting work directly by ID;
the claim clears waiting but does not prove authority or satisfy its reason.

An acceptor reads Result and material child outcomes without claiming the issue.
If the outcome satisfies the contract,
follow the close workflow in [termination.md](termination.md).
If it does not,
record the concrete gaps in Log,
replace State with the returned position and next corrective action,
and leave the issue waiting for direct claim by the corrective executor.
Claim only when the acceptor will perform corrective execution.
The submitted Result remains the last proposed outcome until corrective work
replaces it.
