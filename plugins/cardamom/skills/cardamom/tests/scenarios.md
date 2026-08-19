# Cardamom skill scenarios

Run these cases with the protocol in [README.md](README.md).

## 01 Select Cardamom only when requested

### Prompt

The available skill catalog contains:

- `cardamom`: use when the user explicitly asks to use Cardamom or `card`,
  operate on an existing Cardamom store, board, or issue,
  or coordinate work through Cardamom.
- `commit`: use for Git commits and branches.

For each request,
state which skill or skills you would load:

1. "Use Cardamom issue cm-abcd to coordinate this repair."
2. "Fix this typo."
3. "Explain what a Cardamom claim is; do not operate on anything."
4. "Update the Cardamom skill documentation."

### Expected behavior

- Selects Cardamom for requests 1 and 4.
- Does not select Cardamom merely as generic task tracking for request 2.
- Does not select operational Cardamom guidance for explanation-only request 3.
- Does not select the commit skill without Git work.

### Unacceptable behavior

- Selects Cardamom for every multi-step task.
- Misses explicit issue operation or skill maintenance.

## 02 Resolve scope and reuse an existing outcome

### Prompt

Use the skill at `{SKILL_PATH}`.

The user asks you to continue a parser repair through Cardamom.
They give store `/srv/cardamom` and board `board_parser`,
but no issue ID.
Two worktrees have different checkout bindings.
The selected board contains:

- `cm-old`, closed: investigated an unrelated lexer rewrite;
- `cm-live`, waiting: repair quoted-input parsing without syntax changes;
- `cm-new`, ready: update parser examples.

Give the Cardamom operations and decision you would make before execution.

### Expected behavior

- Passes the explicit absolute store and board rather than relying on checkout
  discovery or changing persisted selection.
- Searches and inspects plausible issues before creation.
- Continues `cm-live` by direct ID because it owns the same outcome.
- Treats the waiting reason as context rather than authority.
- Does not initialize, create a board, or create a replacement issue.

### Unacceptable behavior

- Chooses scope from a worktree binding.
- Creates persistent scope without request.
- Creates a new issue because the actor, session, or phase changed.

### Adjacent valid case: Ambiguous board

#### Runner prompt addition

The user supplies only the store.
It contains two boards with plausible parser issues and no unique board
selection.

#### Expected behavior

- Reports the candidate board IDs and stops for scope resolution.
- Does not search every board and choose a match.

#### Unacceptable behavior

- Selects a board by name similarity, issue recency, or convenience.

## 03 Establish or repair an executable contract

### Prompt

Use the skill at `{SKILL_PATH}`.

Issue `cm-work` is ready and unclaimed.
Its title is "refactor parser",
Summary is "Improve parsing",
and Details is empty.
Chat established that malformed quoted escapes must be reproduced and repaired
in the scanner package without changing documented shell syntax.
Focused scanner tests and parser validation are the acceptance evidence.
You will execute the work yourself under actor `Geordi`.

Give the ordered Cardamom and primary-work actions through the first source
edit.

### Expected behavior

- Claims by ID with context under stable actor `Geordi`.
- Recognizes that both durable contract records are deficient.
- Replaces Summary with a recognizable outcome and acceptance boundary.
- Replaces Details with the established behavior, owned area, constraints,
  accepted plan, and evidence before source work consumes them.
- Publishes execution intent in State before primary work when the initial
  position would otherwise be unrecoverable.
- Does not treat direct execution or shared chat as an exemption.

### Unacceptable behavior

- Repairs Details while leaving the generic Summary.
- Begins source inspection and promises to update the issue later.
- Invents an implementation not established by the prompt.
- Creates a child only to separate investigation from implementation.

### Adjacent valid case: Complete existing contract

#### Runner prompt addition

The issue already has a precise Summary and complete Details,
including the same owned area, constraint, accepted plan,
and validation evidence.

#### Expected behavior

- Uses the existing contract without rewriting it ritualistically.
- Claims and publishes only the active execution position needed for recovery.

#### Unacceptable behavior

- Rewrites unchanged durable records merely to demonstrate compliance.

### Pressure variant: Unsupported implementation

#### Runner prompt addition

The deadline is close and implementation is expected immediately,
but neither issue records nor accepted evidence establish a safe repair or the
investigation needed to choose one.

#### Expected behavior

- Publishes the unresolved contract gap.
- Stops or hands off safely instead of inventing implementation details.

#### Unacceptable behavior

- Treats urgency as evidence for a guessed repair.

## 04 Write records for their readers

### Prompt

Use the skill at `{SKILL_PATH}`.

Plan one workstream with two child tasks.
Every child needs to know that command syntax must remain compatible.
Only the scanner child needs the accepted escape-state repair,
exact scanner files, and focused test command.
During execution,
a reproduction proves the malformed escape,
the escape-state repair is selected over token-reader normalization,
and focused parser validation passes.
Cross-process deployment validation was not performed.

Draft the material Summary, scanner Details, active State and next action,
decision Log, and final Result.

### Expected behavior

- Puts the shared compatibility boundary in the containing Summary.
- Gives the scanner child its own recognizable Summary outcome.
- Puts scanner-local accepted choices, area, and evidence in scanner Details.
- Makes State the complete current position with a separate next action.
- Uses Log for distinct rationale and the rejected alternative,
  not a copy of State.
- Makes Result distinguish passing focused validation from the unverified
  deployment boundary.
- Keeps names stable and every body understandable without chat.

### Unacceptable behavior

- Pollutes inherited Summary with scanner-only procedure.
- Uses Log as a chronological transcript.
- Presents deployment behavior as verified.
- Uses Result to repair missing earlier publication.
- Stores only references without the material conclusion.

## 05 Publish material transitions before dependent work

### Prompt

Use the skill at `{SKILL_PATH}`.

You are actor `Vega` and own claimed issue `cm-retry`.
Details require retry batches to preserve first-seen tenant order,
request order within each tenant,
and tenant isolation.
Current State says the failure is being investigated.
The current implementation globally sorts all requests by tenant,
then slices fixed-size batches.
A reviewer recommends re-sorting each already-sliced batch because that is a
smaller change,
but slicing has already mixed tenants at that point.
Grouping by first-seen tenant before per-tenant slicing preserves all three
contract requirements.

The deadline is close,
the reviewer says tracking is secondary,
and the two source changes are small and already obvious.
Give the exact ordered Cardamom and primary-work actions through both source
changes, validation, and Result.
Show every durable body you would write.
Do not execute commands or modify files.

### Expected behavior

- Replaces State with the established failure, selected repair,
  and next action before the first source change.
- Posts distinct rationale and rejected alternatives to Log before the first
  source change embodies the choice.
- Groups both mechanical source changes without a record write between them.
- Advances State only when implementation gives way to a coherent validation
  position.
- When it writes multiline State,
  uses supported forms:
  `state set <issue-id> - --next ...` and
  `state commit <issue-id> --set - --next ...` before heredoc redirection.
- Sets Result only after validation passes.
- Keeps publication in the primary work loop despite time,
  authority, and small-change pressure.

### Unacceptable behavior

- Places either source change before State and material Log publication.
- Defers every durable write until Result or final handoff.
- Adds a State or Log entry between the two mechanical source changes.
- Treats the concise chat request as permission to leave issue memory stale.

### Adjacent valid case: Recorded mechanical batch

#### Runner prompt addition

Assume the failure, selected repair, and rationale are already current in State
and Log.
Only the two mechanical source changes and validation remain.
Give the operations you would perform.

#### Expected behavior

- Performs both mutations without another State or Log write.
- Publishes again only when evidence, choice, active position,
  or next action changes.

#### Unacceptable behavior

- Reposts the accepted repair or writes per-file progress.

### Adjacent valid case: Temporary State has no replay value

#### Runner prompt addition

Instead,
a short-lived diagnostic State has been disproved before any later work relied
on it.
No active replacement position is needed.

#### Expected behavior

- Clears State with `state set <issue-id> ''`.
- Does not preserve the discarded diagnosis as a State snapshot.

#### Unacceptable behavior

- Uses `state commit` merely because State was previously present.

## 06 Reuse, promote, or invalidate accepted knowledge

### Prompt

Use the skill at `{SKILL_PATH}`.

You are a new actor continuing claimed issue `cm-retry`.
Details records an accepted retry policy and cites the authoritative protocol
section.
Log records who verified it and the exact revision.
State says implementation is next.
No source, dependency, environment, or acceptance condition changed.
You can reopen the protocol in one minute for confidence.

State what you inspect and what you do before implementation.

### Expected behavior

- Reads the issue contract, current State, relevant ancestors,
  and dependency Results.
- Treats the accepted conclusion and provenance as durable task knowledge.
- Implements from the recorded conclusion without reopening the protocol merely
  because the actor or session changed.
- Opens deeper history only for a concrete continuation question.

### Unacceptable behavior

- Repeats research as a confidence check.
- Treats every citation as a command to reread the source.
- Ignores missing provenance or contradictory evidence.

### Adjacent valid case: Changed authoritative source

#### Runner prompt addition

The cited protocol revision changed yesterday,
and its release note says retry cancellation semantics were revised.

#### Expected behavior

- Reopens the affected investigation before implementation relies on the old
  conclusion.
- Publishes the changed position and repairs the stable contract if needed.

#### Unacceptable behavior

- Reuses the prior conclusion solely because it is durable.

### Adjacent valid case: Promote accepted research

#### Runner prompt addition

Details still says retry-key compatibility must be researched.
State says research is complete and implementation is next.
A Log entry contains accepted evidence and the stable conclusion that tenant ID
must precede the existing namespace and resource components.
No stable contract record contains that conclusion.

#### Expected behavior

- Uses the accepted research instead of repeating it.
- Replaces Details with the complete stable contract,
  preserving prior owned areas and adding the accepted conclusion.
- Keeps evidence and chronology in Log.
- Replaces State with the active implementation position before dependent work.

#### Unacceptable behavior

- Begins dependent work while Details still requires research.
- Copies the complete evidence trail into Details.
- Repeats the research because its tools remain available.

### Adjacent valid case: Promote a descendant-wide conclusion

#### Runner prompt addition

The accepted conclusion changes a compatibility boundary that every current
and future child of the workstream must apply.
The workstream Summary does not contain it.

#### Expected behavior

- Replaces the complete workstream Summary before dispatching dependent
  children.
- Includes the concise compatibility conclusion without copying its chronology.
- Keeps supporting evidence and rationale in Details or Log.

#### Unacceptable behavior

- Leaves the conclusion only in the originating child's records.
- Promotes child-local procedure that descendants do not need.

## 07 Choose direct, delegated, or helper ownership

### Prompt

Use the skill at `{SKILL_PATH}`.

You coordinate workstream `cm-parent` under actor `Picard`.
It has independent child `cm-parser`,
which runtime agent `Data` will execute.
You have not claimed that child.
Agent `Worf` will inspect one bounded test failure inside the parent outcome
but will not own an issue.

Describe dispatch, Cardamom custody, record ownership,
and how returned evidence reaches the parent.

### Expected behavior

- Attaches the shipped Cardamom skill to Data's runtime prompt.
- Gives Data the child ID, actor, explicit scope, worktree and owned area,
  validation, expected Result, and handoff disposition.
- Requires Data to claim before material child work.
- Makes Data responsible for the child's State, Log, Result, and handoff.
- Treats Worf as a helper who returns bounded evidence to Picard.
- Makes Picard publish accepted helper or child conclusions on the parent only
  when they change its operative position or decision trail.
- Keeps actor identities stable and does not use machine usernames.

### Unacceptable behavior

- Paraphrases selected Cardamom duties instead of attaching the skill.
- Has Picard ghostwrite child execution records.
- Lets a helper mutate the parent's records without custody.
- Uses mail as the only dispatch or durable context.

### Adjacent valid case: Coordinator already owns the child

#### Runner prompt addition

Picard had already claimed `cm-parser` and completed material planning before
deciding to delegate it.

#### Expected behavior

- Publishes material planning under Picard,
  then releases before Data claims.
- Does not leave simultaneous or ambiguous custody.

#### Unacceptable behavior

- Has Data work under Picard's active claim.

## 08 Reconcile partial evidence before status

### Prompt

Use the skill at `{SKILL_PATH}`.

Actor `Vega` owns claimed task `cm-import`.
Its State says a migration rehearsal is paused while a worker repairs the
validator.
The repair child now has a Result showing the repair passed its contract,
and a validated binary is running new rehearsal batches.
Completed reports establish a confirmed floor of 4,200 processed records.
Two worker reports remain unreconciled,
so the final count is unknown.

The coordinator wants to wait for complete accounting before changing State,
continue dispatching batches,
and answer a status request from chat.
Give the issue-record operations and status-reporting sequence.

### Expected behavior

- Reads the child Result and accepted batch evidence before continuing.
- Updates `cm-import` State before further dispatch or status reporting.
- Records active execution on the validated binary,
  the confirmed floor,
  the two pending reports,
  their reconciliation action,
  and the next established rehearsal action.
- Reports status from the updated durable position without claiming a final
  count.
- Keeps worker chronology and supporting evidence in their source records.

### Unacceptable behavior

- Leaves State paused until every report is reconciled.
- Dispatches or answers from chat while durable State contradicts execution.
- Presents 4,200 as the final count or includes pending reports in the floor.
- Creates one issue or Log entry per report merely to aggregate status.

### Adjacent valid case: Unvalidated repair

#### Runner prompt addition

The child reports a repaired binary,
but required validation has not completed and no accepted evidence shows that
rehearsal execution resumed.

#### Expected behavior

- Keeps the last established execution position current.
- Records unresolved repair evidence and its investigation action.
- Does not claim that execution resumed.

#### Unacceptable behavior

- Promotes an unvalidated worker report into confirmed active execution.

## 09 Hand off, accept, return, and recover

### Prompt

Use the skill at `{SKILL_PATH}`.

Independent acceptance is required for claimed issue `cm-fix`.
Execution and validation are complete.
The acceptor later finds one concrete compatibility gap.
After that,
a replacement actor receives the issue ID.

Give the lifecycle and record sequence from executor completion through
corrective recovery.

### Expected behavior

- Executor sets Result,
  installs acceptance-oriented State with a separate next action,
  and releases waiting for acceptance.
- Relies on release to preserve changed State rather than committing it solely
  for handoff.
- Acceptor records the concrete gap in Log,
  replaces State with the returned position and corrective next action,
  and leaves the issue waiting for corrective execution.
- Acceptor does not claim unless it begins corrective execution.
- Does not pretend the prior Result disappeared;
  corrective work later replaces the proposed outcome.
- Replacement inspects by ID,
  claims waiting work directly,
  and recovers only needed context before work.

### Unacceptable behavior

- Uses ordinary release for directed independent acceptance.
- Claims or releases merely to inspect and return the completed outcome.
- Claims from an automatic pool while recovering a known issue.
- Closes despite a rejected acceptance result.

### Adjacent valid case: Ordinary self-acceptance

#### Runner prompt addition

Instead,
the issue contract permits the executor to perform ordinary acceptance,
and the completed Result satisfies that contract.

#### Expected behavior

- Sets Result,
  records the acceptance decision,
  and closes the issue.
- Does not claim merely to inspect and accept the completed outcome.
- Does not invent an independent acceptance handoff.

#### Unacceptable behavior

- Installs acceptance State and releases waiting without an independent gate.

### Adjacent valid case: Another actor may still be live

#### Runner prompt addition

The issue is still claimed by another actor whose runtime process may be live.

#### Expected behavior

- Leaves custody unchanged and coordinates with the actor or runtime.
- Does not invoke the stopped-executor exception.

#### Unacceptable behavior

- Releases under the other actor or claims over them.

### Adjacent valid case: Stopped executor reassignment

#### Runner prompt addition

The runtime confirms the prior process stopped,
and the user explicitly authorizes reassignment.

#### Expected behavior

- Coordinator records evidence and decision under its own actor.
- Performs one owner-attributed release solely to end the stopped claim.
- Replacement claims by ID and re-establishes context.

#### Unacceptable behavior

- Treats actor attribution as reassignment authority.

## 10 Model relationships and reconciliation ownership

### Prompt

Use the skill at `{SKILL_PATH}`.

A migration workstream contains an implementation task.
The implementation cannot be accepted until a schema task closes.
A policy owner must approve rollout separately.
A generator reruns against an existing human-maintained issue whose labels and
Details it does not own.

Choose containment, dependencies, checkpoint structure,
and the `card apply` existing-target policy.

### Expected behavior

- Uses parent containment for inherited outcome context.
- Adds schema and checkpoint dependencies only where their Results gate work.
- Uses a checkpoint for the external authority decision.
- Uses `skip` or `error`,
  not `update`,
  for the human-maintained issue.
- Recognizes that set-valued fields under `update` replace complete sets.

### Unacceptable behavior

- Treats discovery or containment as automatic dependency.
- Uses `update` as generic idempotence.
- Treats the command actor as approval authority.

### Adjacent valid case: Graph premise changes

#### Runner prompt addition

Schema inspection newly proves that a claimed implementation issue needs the
schema dependency.
Its current State says implementation can begin immediately.

#### Expected behavior

- Publishes the changed active position before graph mutation.
- Adds distinct Log rationale only if useful later.
- Edits the dependency atomically and inspects affected readiness.
- Does not leave State asserting that implementation is immediately runnable.

#### Unacceptable behavior

- Logs the graph change but leaves contradicted State active.

## 11 Reassess a ready plan after prerequisites

### Prompt

Use the skill at `{SKILL_PATH}`.

Issue `cm-client` was blocked on `cm-server`.
The closed server Result says it already implemented the client-visible
compatibility shim,
so the original client patch may be unnecessary.
The client issue is now ready,
and its old Details still prescribe the patch.

Give the next Cardamom and investigation actions.

### Expected behavior

- Reads the prerequisite Result and inspects the resulting system before
  preserving or dispatching the old plan.
- Chooses an evidence-backed disposition:
  retain the plan,
  revise Details and State,
  close or cancel unnecessary work,
  or stop on an unresolved gap.
- Publishes any changed contract and active position before dependent work.

### Unacceptable behavior

- Dispatches the client patch merely because it became ready.
- Treats readiness as proof that the old plan still applies.
- Leaves obsolete Details or State current after selecting another disposition.

## 12 Inspect terminal impact before denial

### Prompt

Use the skill at `{SKILL_PATH}`.

Checkpoint `cm-policy` has two non-terminal transitive dependents,
one reachable through two dependency paths and one actively claimed.
The policy owner denies the checkpoint and gives a concrete reason.

Give the inspection and denial sequence.

### Expected behavior

- Traverses dependency dependents with a seen-ID set.
- Reviews each non-terminal affected issue exactly once.
- Coordinates with the active executor and preserves needed partial context.
- Records the authority's actual reason on the atomic checkpoint denial.
- Treats the command actor as attribution rather than decision authority.

### Unacceptable behavior

- Traverses containment as cancellation impact.
- Denies before inspecting or coordinating transitive impact.
- Reviews a shared dependent twice.
- Infers authority from the Cardamom actor.

## 13 Transition a visible phase without unnecessary release

### Prompt

Use the skill at `{SKILL_PATH}`.

One issue uses `phase:implement` and `phase:verify`
because separate automatic worker pools normally select those positions.
Implementation has a replay-worthy completed State.
In this instance,
the same claim owner will immediately perform verification.

Give the transition sequence and custody decision.

### Expected behavior

- Commits the completed State while installing verification State and next
  action.
- Replaces phase labels in one edit.
- Keeps the existing claim because the same owner continues immediately.
- Leaves Result unset until the overall contract is complete.

### Unacceptable behavior

- Releases merely because the label changed.
- Creates another issue only for the ordinary verification position.
- Uses phase labels as ordering or custody primitives.

### Adjacent valid case: Automatic verification pool

#### Runner prompt addition

Any eligible verification worker should select the issue from its automatic
pool.

#### Expected behavior

- Uses ordinary release after installing the new phase position.

#### Unacceptable behavior

- Uses waiting release for an open automatic pool.

## 14 Run and retire a routine

### Prompt

Use the skill at `{SKILL_PATH}`.

Routine `cm-release-watch` is awakened by an external scheduler.
State contains targets and safe cursor `release-120`.
The run resolves releases 121 through 123,
but release 124 remains active.
A new interpretation rule will apply to every later awakening.
Months later,
the release process is replaced and the routine is no longer valid.

Give the run boundary, knowledge upgrade, and retirement disposition.

### Expected behavior

- Claims the known routine ID and installs an active-run boundary before
  external work.
- Advances the cursor only through successfully processed input.
- Publishes the completed run outcome into active State before committing it.
- Commits the completed run while installing recoverable next-run State.
- Replaces Details with the stable interpretation rule.
- Releases for the next awakening rather than expecting Cardamom to schedule it.
- Cancels the invalidated routine after reconciling ownership and children.
- Uses one terminal decision record for the retirement rationale rather than
  duplicating it across acceptance and retirement posts.
- Sets Result only if acceptors or dependents need the terminal outcome.

### Unacceptable behavior

- Searches automatic ready pools for routines.
- Commits the original active-run State without adding the run outcome.
- Carries run chronology in Details.
- Calls every retirement successful close.
- Posts duplicate terminal rationale under different workflow labels.

## 15 Gate external resource work on a lease

### Prompt

Use the skill at `{SKILL_PATH}`.

Claimed migration issue `cm-db` needs exclusive `staging-db`.
No native coordinator exists.
The first acquire fails because another actor owns the lease.
After a later successful acquire and one external operation,
renewal fails and `lease show` no longer reports your ownership.

Give the Cardamom writes and resource-action gates.

### Expected behavior

- Publishes acquisition intent before acquiring.
- Does not touch the database after failed acquisition.
- Records the blocked or waiting position and observed owner when useful.
- After lost renewal,
  stops initiating resource actions,
  makes the started operation safe,
  inspects lease and resource separately,
  publishes recovery State,
  and reacquires before resuming.
- Releases only after the resource is safe.

### Unacceptable behavior

- Treats a claim as resource ownership.
- Continues because the TTL probably expired.
- Infers cleanup or process termination from lease loss.

### Adjacent valid case: Native isolation is sufficient

#### Runner prompt addition

The operation is a safe concurrent read and the database already enforces its
required isolation.

#### Expected behavior

- Does not add a Cardamom lease.

#### Unacceptable behavior

- Leases every external resource as ritual coordination.

## 16 Use mail only for ephemeral attention

### Prompt

Use the skill at `{SKILL_PATH}`.

An observer must receive `release.*` notifications for two hours.
Its subscription TTL is 30 minutes,
and it uses `mail recv --tail`.
The notification announces that issue `cm-release` has a new acceptance
position and rationale.

Give the mail and durable-record handling.

### Expected behavior

- Uses store-scoped mail only as an attention channel.
- Maintains subscription renewal separately because tailing does not renew it.
- Publishes the acceptance position and material rationale on `cm-release`.
- Uses mail to point the observer to durable issue context.

### Unacceptable behavior

- Uses mail as the only copy of the position or rationale.
- Treats mail as custody transfer, assignment, or readiness.
- Selects a board solely for mail.
- Assumes `mail recv --tail` renews the subscription.

## 17 Preserve produced bytes with durable meaning

### Prompt

Use the skill at `{SKILL_PATH}`.

A generated validation report will disappear with its producer worktree.
An acceptor needs the exact bytes,
and the report establishes which tenant rows matched.
Two issues on the same board need to reference it.

Give the attachment and issue-record handling.

### Expected behavior

- Attaches the report because its exact bytes must outlive the worktree.
- Captures its stable ID.
- Explains in Log or Result what the bytes establish.
- Reuses the same board-scoped reference from both issues.
- Retrieves complete bytes when an acceptor needs to inspect them.

### Unacceptable behavior

- Treats an attachment filename as sufficient context.
- Uploads duplicate bytes solely to associate them with another issue.
- Puts the only material conclusion in the attachment.

### Adjacent valid case: Reproducible scratch output

#### Runner prompt addition

The output is transient command noise that any later executor can reproduce
from a stable source location and documented command.

#### Expected behavior

- Records the useful conclusion or reproduction command without attaching the
  scratch output.

#### Unacceptable behavior

- Uses attachment storage as a general-purpose cache.

### Adjacent valid case: Paginated attachment discovery

#### Runner prompt addition

The originating issue is known,
but its records lost the attachment reference.
The first `attachment list --issue` response includes `next_page_token` and
does not contain the needed artifact.

#### Expected behavior

- Continues discovery with `--after <token>` until no token remains.
- Uses issue context to establish meaning rather than trusting the filename.

#### Unacceptable behavior

- Assumes the first page contains every associated attachment.

## 18 Preserve runtime actor identity

### Prompt

Use the skill at `{SKILL_PATH}`.

A collaboration runtime starts two concurrent executors named `delta-scout`.
Their runtime identifiers are `74c1` and `91bd`;
the current executor is `74c1` and receives issue `cm-482`.
The coordinator calls this executor the documentation worker.

Give the contextual claim command and explain the actor identity.
Do not execute commands.

### Expected behavior

- Uses a stable actor such as `delta-scout-74c1` that preserves the readable
  runtime name and distinguishes the executor.
- Uses the same actor for the complete issue lifecycle.
- Claims `cm-482` by ID with context.

### Unacceptable behavior

- Uses only the opaque runtime identifier.
- Replaces runtime identity with a generic role such as `worker-a` or
  `documentation-worker`.

### Pressure variant: Generic commands already drafted

#### Runner prompt addition

The release window closes in ten minutes,
three commands already use actor `worker-a`,
and a senior engineer asks to keep that shorter name.

#### Expected behavior

- Replaces the generic actor with the distinguishable runtime identity.

#### Unacceptable behavior

- Preserves ambiguous attribution because commands are already drafted.

### Adjacent valid case: Runtime supplies no identity

#### Runner prompt addition

The runtime provides no executor name or identifier.
The task context names one documentation indexer,
and no concurrent actor could be confused with it.

#### Expected behavior

- Chooses a concise stable actor such as `documentation-indexer`.

#### Unacceptable behavior

- Invents a runtime identifier or requires an unavailable environment value.

## 19 Recover the plugin command after direct failure

### Prompt

Use the skill at `{SKILL_PATH}`.

You attempted this read-only command from an installed Cardamom skill:

```bash
card --actor delta-scout --json info
```

The shell reports that `card` is unavailable.
Give the next command on macOS and explain the recovery boundary.
Do not execute commands.

### Expected behavior

- Retries the same invocation through `scripts/cardamom` from the loaded skill
  directory.
- Preserves every argument and the stable actor.
- Lets the launcher own binary acquisition and caching.

### Unacceptable behavior

- Invents a global installation procedure or cache override.
- Changes the intended Cardamom operation while recovering the executable.

### Adjacent valid case: Direct command succeeds

#### Runner prompt addition

The direct `card` invocation succeeds.

#### Expected behavior

- Uses `card` directly without a launcher preflight.

#### Unacceptable behavior

- Always probes or invokes the fallback before trying `card`.

## 20 Add a project to an existing store

### Prompt

Use the skill at `{SKILL_PATH}`.

Existing store `/srv/cardamom` contains project `Inventory` and its board.
The user asks to add a separate project named `Billing` with prefix `bill-`
and create its first board, `Ledger migration`.

Give the command sequence and identity handling.
Do not execute commands or persist selection.

### Expected behavior

- Keeps the explicit store and one stable actor on every command.
- Inspects existing projects with structured output.
- Creates `Billing` with `project create --prefix bill-`.
- Parses the returned project ID and passes it to `board create`.
- Uses stable IDs when names could be ambiguous.

### Unacceptable behavior

- Runs `card init` to add another project.
- Assumes project creation also creates or selects a board.
- Changes checkout board selection without a request.

### Adjacent valid case: Inherit the store prefix

#### Runner prompt addition

The new project should inherit the active store prefix.

#### Expected behavior

- Omits `--prefix` instead of recreating prefix policy in the agent workflow.

#### Unacceptable behavior

- Guesses or copies another project's prefix.

## 21 Author durable Markdown and references

### Prompt

Use the skill at `{SKILL_PATH}`.

Draft commands for a claimed migration issue.
Details must contain separate protocol, constraints, and acceptance sections,
including literal `$TARGET`, `$(date)`, backticks, and the spelling `\n`.
State has one reproduced failure, one selected package boundary,
two changing files, and one unresolved migration question.
A supporting Log entry is `log_9f67d0c5e3ab49f2b1478a60c2de5114`.
The material ordering conclusion must remain understandable without opening it.
A published bundle labeled `bundle 817` is available at
`https://releases.example.com/products/atlas/bundles/817`.

Draft the Details, State, and Result command forms.
Do not execute commands.

### Expected behavior

- Uses single-quoted heredocs with real line breaks for multiline bodies.
- Preserves shell metacharacters, backticks, and the intentional `\n`
  literally.
- Uses domain-specific headings, short paragraphs, or lists for independent
  facts without generic State wrapper headings.
- States the material ordering conclusion directly and uses
  `%log_9f67d0c5e3ab49f2b1478a60c2de5114` only for useful chronology.
- Stores the full bundle URL under the readable `bundle 817` label.

### Unacceptable behavior

- Passes a serialized `\n` string as multiline Markdown.
- Leaves independent recovery facts in one dense paragraph.
- Replaces the material conclusion with a bare Log reference.
- Stores only the local bundle label.

### Pressure variant: Serialized helper output

#### Runner prompt addition

An approved orchestration helper returned the State body as one shell-ready
quoted argument containing serialized `\n` tokens.
The review starts in ten minutes,
and the lead asks you to pass that argument through unchanged.

#### Expected behavior

- Reconstructs the intended Markdown with real lines in a single-quoted
  heredoc.
- Preserves only a backslash-plus-n spelling that is intentionally content.

#### Unacceptable behavior

- Treats helper approval, deadline, or prior work as evidence that `card`
  interprets serialized newline escapes.

### Adjacent valid case: One connected observation

#### Runner prompt addition

State contains one connected two-sentence observation and no next action.

#### Expected behavior

- Uses one short paragraph without adding headings or bullets.

#### Unacceptable behavior

- Adds structure that makes the simple record harder to scan.

## 22 Recover only the history needed to continue

### Prompt

Use the skill at `{SKILL_PATH}`.

Issue `cm-recover` is waiting and unclaimed.
The replacement actor has its ID and explicit scope.
Summary, Details, current State, ancestor context,
and completed dependency Results explain the outcome and current position.
One material strategy choice is unexplained.
The issue has 400 Log entries;
recent entries may identify the relevant decision,
but chronological replay from the beginning is not currently needed.

Give the recovery commands and context-expansion order.
Do not execute commands.

### Expected behavior

- Claims the known waiting issue directly by ID with context.
- Starts from the assembled current contract, position, ancestors,
  and dependency Results.
- Opens a bounded newest-first Log window for the unexplained choice.
- Expands the window or other surfaces only when the recovery question requires
  them.
- Repairs Details or State before dependent work if recovered knowledge changed
  the stable contract or active position.

### Unacceptable behavior

- Selects another issue from an automatic pool.
- Replays all 400 entries merely because the actor changed.
- Uses `--oldest-first` without a need for chronological replay.

### Adjacent valid case: Chronology determines the decision

#### Runner prompt addition

The recovery question depends on how several early decisions evolved in order.

#### Expected behavior

- Uses `--oldest-first` for the bounded chronological replay that now matters.

#### Unacceptable behavior

- Treats newest-first ordering as mandatory when it obscures the question.

## 23 Revoke a lease only after resource recovery

### Prompt

Use the skill at `{SKILL_PATH}`.

Worker `La Forge` holds the lease for `rack-7` but its runtime process stopped.
Rack operations independently confirmed that the test process ended,
the rack was reset,
and another worker may safely use it.
The lease remains active for 35 minutes.
The coordinator is actor `Riker` and has explicit recovery authority.

Give the issue-record and lease sequence through replacement use.
Do not execute commands.

### Expected behavior

- Records the observed external-resource disposition before revocation.
- Uses coordinator actor `Riker` for the recovery operation.
- Revokes with `--owner 'La Forge'` and a concrete reason.
- Treats the owner condition as protection if ownership changed.
- Has the replacement acquire a new bounded lease before touching the rack.

### Unacceptable behavior

- Operates as the stopped worker to release the lease.
- Treats revocation as proof that the rack is safe.
- Lets the replacement rely on the prior lease.

### Pressure variant: Resource state is unknown

#### Runner prompt addition

The deadline is close and the worker is known to be stopped,
but nobody has inspected the rack or its test process.

#### Expected behavior

- Leaves the lease and rack unused until external-resource disposition is
  established.

#### Unacceptable behavior

- Revokes because the worker stopped or the lease will eventually expire.

## 24 Load only the workflows that own the decision

### Prompt

Use the skill at `{SKILL_PATH}`.

The user supplies a store but no board.
They ask to create a workstream with two independently owned child tasks,
then run one child against a shared staging database that lacks native
coordination.
No mail, routine, attachment, phased workflow, recovery, or terminal operation
is involved.

State which references you load and why before giving the plan.
Do not execute commands.

### Expected behavior

- Loads `scope.md` to resolve the board without creating or guessing one.
- Loads `planning.md` for outcome boundaries and graph structure.
- Loads `leases.md` for the shared database decision.
- Loads `execution.md` only when describing claim, dispatch,
  publication, or handoff.

### Unacceptable behavior

- Loads every reference for safety.
- Skips scope resolution or chooses a board by convenience.
- Loads unrelated mail, routine, attachment, phase, recovery,
  or termination guidance.

### Adjacent valid case: Record drafting only

#### Runner prompt addition

The board and issue are already established,
and the request only asks for a generic Summary and Details draft.

#### Expected behavior

- Uses the primary record model and planning guidance needed for the contract.
- Does not load execution or resource workflows without their decision points.

#### Unacceptable behavior

- Treats every record-writing request as a full execution workflow.

### Adjacent valid case: Complete and close an issue

#### Runner prompt addition

Instead,
the board and claimed issue are already established,
execution and validation satisfy its contract,
and the executor may accept and close it.

#### Expected behavior

- Loads `execution.md` for Result and acceptance flow.
- Loads `termination.md` for the close decision and command.
- Enters at Result and acceptance without replaying completed execution or
  validation.
- Does not load planning, recovery, routine, mail, lease, attachment,
  or phase guidance.

#### Unacceptable behavior

- Treats close as an execution-only decision.
- Repeats execution or validation because those sections appear earlier in the
  workflow.
- Loads every reference because the issue is terminal.

## 25 Define iterative checks and final acceptance

### Prompt

Use the skill at `{SKILL_PATH}`.

An unclaimed task will redesign recovery messages for an interactive command.
The intended behavior and owned command are established.
No automated test can decide whether the messages give a first-time user enough
information to recover.
A quick review of one representative transcript will guide iteration.
Final acceptance requires a reviewer to confirm that every supported failure
names the failed action, preserves the user's input,
and gives one valid recovery action without exposing internal details.

Draft the task's Summary and Details before execution.

### Expected behavior

- Makes the user-visible recovery behavior and acceptance boundary recognizable
  in Summary.
- Puts the reviewer-applicable qualitative criteria in Details.
- Names the representative-transcript review as an iterative check
  and the complete failure-set review as final acceptance evidence.
- Does not treat the fast check as evidence that the complete contract passed.

### Unacceptable behavior

- Uses an unjudgeable criterion such as "the messages look good."
- Invents a numeric metric or automated test that the prompt does not support.
- Treats one representative transcript as final acceptance.

### Adjacent valid case: One check proves the complete contract

#### Runner prompt addition

Instead,
one deterministic conformance command exercises every supported failure
and is both the fastest useful check and the required final validation.

#### Expected behavior

- Records the one command as both roles without inventing a second validation.

#### Unacceptable behavior

- Requires separate checks merely to preserve a two-tier structure.

## 26 Preregister a decision-producing investigation

### Prompt

Use the skill at `{SKILL_PATH}`.

Claimed issue `cm-export` must determine whether streaming should replace
buffered export for large reports.
No implementation is selected.
The representative fixture, current buffered baseline command,
peak-memory metric, 20 percent improvement threshold,
unchanged-output requirement, three-run limit,
and report artifact path are established.
If streaming clears the threshold without changing output,
the issue will select it; otherwise it will retain buffering
and report the measured constraint.

Give the Cardamom record actions required before the first benchmark command.

### Expected behavior

- Publishes the stable question, comparison method, baseline,
  metric, threshold, output constraint, run limit, artifact,
  and decision rule in Details before measurement.
- Puts the current measurement position and first concrete benchmark action in
  State.
- Does not select implementation before the evidence satisfies the recorded
  decision rule.
- Keeps later measurements and conclusions in the records whose readers need
  them rather than turning Details into a command transcript.

### Unacceptable behavior

- Runs the benchmark before publishing the decision-producing contract.
- Records only "benchmark streaming" without a decision rule or stopping point.
- Treats collecting more measurements indefinitely as safe continuation.

### Adjacent valid case: Bounded source inspection

#### Runner prompt addition

Instead,
the issue needs one source inspection to identify which documented constant
owns a fixed retry limit.
The source boundary and evidence needed to identify the owner are established;
there is no intervention, metric, repeated measurement, or threshold.

#### Expected behavior

- Records the bounded investigation and evidence needed to choose the owner.
- Does not invent experimental fields that cannot affect the decision.

#### Unacceptable behavior

- Adds a baseline, metric, threshold, or run count merely because the work is an
  investigation.

## 27 Preserve lineage when outcomes are reorganized

### Prompt

Use the skill at `{SKILL_PATH}`.

Unclaimed issue `cm-codec` promised one independently accepted codec migration.
Accepted evidence now shows that read compatibility and write migration require
separate ownership, sequencing, evidence, and acceptance.
The user authorizes replacing `cm-codec` with two new issues.
There are no claims, dependencies, or dependents.
The old issue contains material compatibility evidence both successors need.

Give the Cardamom planning and lifecycle actions.

### Expected behavior

- Publishes the split rationale on `cm-codec` before restructuring consumes it.
- Creates separately executable successor contracts with the inherited material
  evidence and distinct outcome boundaries.
- Makes `cm-codec` identify both successors,
  and each successor identify the predecessor when that lineage aids execution
  or review.
- Keeps the material conclusion in each applicable record;
  issue references supply navigation rather than replacing meaning.
- Cancels the superseded issue only after the required terminal review
  and does not invent a successful Result.

### Unacceptable behavior

- Creates disconnected successors whose relationship to `cm-codec` can be
  recovered only from chat.
- Copies the complete old issue into both successors without selecting the
  contract and evidence each needs.
- Closes the superseded issue as though it achieved its original outcome.

### Adjacent valid case: Same outcome, new execution phase

#### Runner prompt addition

Instead,
the outcome and acceptance boundary are unchanged;
only the same issue's execution phase and actor will change.

#### Expected behavior

- Continues the existing issue and uses the ordinary phase and custody workflow.
- Does not create predecessor or successor issues.

#### Unacceptable behavior

- Creates lineage merely because execution changed actors or phases.

## 28 Choose a disposition when active focus changes

### Prompt

Use the skill at `{SKILL_PATH}`.

Actor `Sisko` owns claimed issue `cm-index`.
The reproduction is complete,
but the repair and remaining validation have not started.
The user redirects `Sisko` to unrelated urgent work for at least one day
and says any eligible actor may continue `cm-index`.
The current State still says reproduction is in progress.

Give the Cardamom actions required before switching focus.

### Expected behavior

- Treats the focus change as a disposition decision for partial work.
- Replaces State with the established reproduction,
  unresolved repair, and concrete next action.
- Releases ordinarily so another eligible actor can claim the issue.
- Does not leave obsolete State or retain custody merely because the process
  remains available.

### Unacceptable behavior

- Switches tasks and promises to repair the record later.
- Cancels work the user intends another eligible actor to continue.
- Uses waiting release when continuation is not directed or externally gated.

### Adjacent valid case: Brief interruption without custody change

#### Runner prompt addition

Instead,
the user asks one brief status question and confirms that `Sisko` should
immediately continue `cm-index`.
Its State and next action already match execution.

#### Expected behavior

- Keeps the claim and answers from current durable context.
- Does not rewrite records or release custody ritualistically.

#### Unacceptable behavior

- Treats every conversational interruption as a handoff.

## 29 Make durable records operationally specific

### Prompt

Use the skill at `{SKILL_PATH}`.

Claimed issue `cm-routing` has this State:

> Work on the important pieces and validate everything before continuing.

Inspection established that `internal/router/table.go` still rejects wildcard
segments,
`go test ./internal/router -run TestTable_Wildcard` reproduces the failure,
and no repair has been selected.

Draft a replacement State and next action.

### Expected behavior

- Names the reproduced wildcard-segment behavior,
  affected router area, and unresolved repair decision.
- Gives a concrete next action that can change the active position.
- Distinguishes established evidence from the repair that remains unknown.
- Uses only the detail needed for safe continuation.

### Unacceptable behavior

- Retains generic activities such as "make progress" or "validate everything."
- Invents a repair or claims validation beyond the focused reproduction.
- Expands the State into a complete source inventory or command transcript.

### Adjacent valid case: Established concise domain name

#### Runner prompt addition

The issue's Details already defines "wildcard routing" as the exact behavior,
owned files, and compatibility boundary.

#### Expected behavior

- Reuses the established domain name where it remains unambiguous.
- Keeps the active position concise while preserving its concrete evidence and
  next action.

#### Unacceptable behavior

- Repeats the complete Details contract solely to avoid a concise defined term.

## 30 Keep independently discovered work outside containment

### Prompt

Use the skill at `{SKILL_PATH}`.

You own claimed workstream `%cedar-804`,
whose outcome is to move billing clients to a new API.
During verification,
you notice that the billing administration viewer stops after 100 audit entries.
The migration acceptance checklist covers client cutover and can close while the
viewer defect remains.
The viewer repair is intended for a later engineer and needs its own evidence.
Because both touch billing and the defect was found during migration work,
the engineering lead says:
"Track it now and put it under the migration workstream so we keep the context
together. Review starts in ten minutes."

State the concrete Cardamom issue operation you would perform,
including any parent or dependency relationship and where you would record why
this work was found.
Do not execute commands or modify files or external state.

### Expected behavior

- Creates or reuses a separate issue for the viewer repair.
- Adds neither parent containment nor a dependency,
  because the repair is not a constituent of the migration deliverable and
  neither outcome gates the other.
- References `%cedar-804` in the new issue's Details when the discovery context
  helps execution,
  or in its Log when the origin matters only as history.
- Keeps the migration workstream's contract and active position unchanged
  unless the discovery materially changes them.

### Unacceptable behavior

- Makes the repair a child because it was discovered during the workstream,
  touches the same area, or needs separate custody and evidence.
- Adds a dependency merely to preserve provenance.
- Omits the source reference or treats an issue reference as the repair's
  executable contract.

### Adjacent valid case: Constituent deliverable

#### Runner prompt addition

Instead, the workstream promises a deployable client migration,
and its accepted rollout plan requires a client rollback command that does not
yet exist.
The workstream cannot satisfy its contract without that command.
Its other migration work can continue while the command is built.

#### Expected behavior

- Creates or reuses the rollback-command outcome as a child of the workstream.
- Gives the child its own executable contract and evidence.
- Does not reject containment merely because another actor will own the child.
- Does not add a dependency when the workstream can continue its current work.

#### Unacceptable behavior

- Keeps the constituent deliverable independent of the workstream.
- Adds a dependency solely because containment exists.

### Adjacent valid case: Independent prerequisite

#### Runner prompt addition

Instead, the viewer repair is independently acceptable,
but a verified audit export from that repair must exist before migration
acceptance can proceed.

#### Expected behavior

- Keeps the viewer repair independent rather than making it a child.
- Makes the migration depend on the viewer repair because its Result gates
  migration acceptance.

#### Unacceptable behavior

- Uses containment to represent the prerequisite.
- Omits the dependency because the outcomes are independently scoped.
