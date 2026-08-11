# Cardamom behavioral scenarios

These scenarios evaluate decisions and produced records.
Runner-visible prompts do not include the expected or unacceptable behavior.

## Trigger selection

### Prompt

You have access to the available-skill catalog.

For each request,
state whether you would load the Cardamom skill and why:

1. "Use card to coordinate this migration on our existing release board."
2. "Refactor this function and run its tests."
3. "Explain what Cardamom issue State means without changing or using a
   store."

### Expected behavior

- Selects the skill for the explicit `card` coordination request.
- Does not select it merely because ordinary implementation could be tracked.
- Does not select it for explanation alone when no Cardamom operation or skill
  change is requested.

### Unacceptable behavior

- Treats Cardamom as mandatory task tracking for ordinary work.
- Initializes or inspects a store for the explanation request.

## Write self-contained issues without inherited pollution

### Prompt

Read the shipped Cardamom skill and the reference it routes to for planning
durable work.

A release workstream will contain three implementation tasks.
Every task needs the release outcome and compatibility boundary.
Only the parser task needs scanner file locations,
the malformed-input reproduction,
and parser-specific validation commands.
The store enforces a configured Summary byte limit,
and the parser executor will not receive the coordinator's chat history.

Explain how to divide the workstream and parser-task information between
Summary and Details so the parser task is executable without polluting sibling
context.
Also state which issue types may contain the parser task.
Do not execute commands.

### Expected behavior

- Places only the concise release outcome and shared compatibility boundary in
  the workstream Summary.
- Gives the parser task its own concise outcome and acceptance boundary in its
  Summary.
- Places parser-specific locations, reproduction, accepted decisions, and
  validation procedure in parser Details.
- Treats parser Summary plus Details and inherited workstream Summary as
  sufficient to start without chat.
- Avoids copying the parent Summary into parser Details.
- Identifies workstreams and routines as container types and tasks and
  checkpoints as leaf types.

### Unacceptable behavior

- Copies all parser working context into the workstream Summary.
- Omits issue-local knowledge because it exists only in chat.
- Treats Details as chronology or a copy of inherited context.
- Makes a task the parent of another issue.

## Preserve runtime actor identity during direct execution

### Prompt

Read the shipped Cardamom skill.

The collaboration runtime names the root agent `Orion`.
The user asks the root agent to execute one claimed issue directly.
A subagent named `Vega` is available but has not been requested or assigned
work.

State the execution and `--actor` choice.
Do not execute commands.

### Expected behavior

- Lets the root agent execute directly rather than requiring delegation.
- Uses `Orion` on the root agent's reads and writes throughout its custody.
- Does not invent a role label such as `worker-a` or use the machine username.

### Unacceptable behavior

- Spawns or assigns the available subagent merely because Cardamom is in use.
- Replaces runtime-provided names with generic roles.

## Delegate issue custody with its record loop

### Prompt

Read the shipped Cardamom skill and the reference it routes to for executing
and handing off issue work.

Root agent `Orion` owns workstream `cm-release`.
Before assigning executors, Orion also claimed child task `cm-parser`.
The child is a self-contained outcome with its own validation and Result.
Orion now delegates the complete child outcome to subagent `Vega`,
who can run `card` and perform the work independently.
Orion remains accountable for the release and wants one concise reporting
surface.

State what Orion sends in the delegation packet,
how custody changes,
who maintains each issue's State, Log, and Result,
and how child evidence reaches the workstream.
Do not execute commands.

### Expected behavior

- Supplies the shipped skill, issue ID, Vega's runtime actor,
  store and board selection, working directory and owned files,
  validation, expected Result, and completion or handoff expectation.
- Supplies an absolute `card` path when binary resolution may differ.
- Preserves any material child position Orion established,
  then has Orion release its claim and Vega claim before material work.
- Makes Vega responsible for the child's State, Log, Result,
  and execution handoff while Vega owns it.
- Has Vega report the child outcome to Orion at handoff.
- Keeps Orion responsible for the workstream record
  and has Orion incorporate only child conclusions that change its active
  position or decision trail.

### Unacceptable behavior

- Retains Orion's child claim while Vega performs the work.
- Makes Orion a proxy writer for Vega's child progress or decisions.
- Has Vega write the workstream record without owning its claim.
- Uses Orion's actor for Vega's writes.
- Copies every child checkpoint into the workstream.

## Keep helper evidence with the issue owner

### Prompt

Read the shipped Cardamom skill and the reference it routes to for executing
issue work.

Root agent `Orion` owns claimed task `cm-parser`.
Orion asks subagent `Vega` to inspect one call path and return evidence.
Vega does not own a separate outcome;
Orion will choose and implement the repair, validate it, and set the Result.

State whether Vega needs another issue or claim,
who maintains `cm-parser`,
and how Vega's evidence enters its records.
Do not execute commands.

### Expected behavior

- Keeps `cm-parser` as one issue under Orion's existing claim.
- Gives Vega a bounded inspection request and evidence-return expectation.
- Keeps Orion responsible for `cm-parser` State, Log, Result,
  and completion.
- Publishes accepted evidence only when it changes the active position,
  decision trail, or outcome.

### Unacceptable behavior

- Creates another issue solely because a subagent performs the inspection.
- Gives Vega issue-record responsibility without issue custody.
- Omits material evidence from `cm-parser` because it originated with Vega.

## Publish material choices before dependent work

Give the prompt and stages to one runner in separate turns.
Capture each response before providing the next stage.

### Prompt

Read the shipped Cardamom skill and the references it routes to for maintaining
issue records and transferring custody.

You are `export-worker` and own claimed task `cm-91`.
Details contain the accepted compatibility contract.
State says implementation has begun and names the serializer as the next area.
Earlier Log entries contain supporting research and a failing regression.

The accepted compatibility contract already requires unknown plugin settings
to survive a configuration round trip.
Inspection shows that the serializer must now choose a representation that
preserves those settings.
The serializer test and edit are ready,
and the primary task is urgent.

State your next Cardamom record operations and work actions in order.
Do not execute commands or modify files.

### Stage two

Assume the proposed operations and implementation are complete.
The accepted command contract requires quiet mode to print the artifact path
because automation consumes that line.
The adapter now needs a small design choice that preserves that behavior,
and the reviewer asks you to keep moving.

State your next record operations and work actions in order.

### Stage three

Assume implementation and required validation are complete.
Root must independently accept the issue.

Prepare the issue for root acceptance.

### Expected behavior

- Before each dependent edit,
  publishes the selected operative design in State.
- Preserves replay-worthy rationale and consequences in a concise standalone
  Log post before dependent work without copying active State.
- Leaves Details unchanged because the choices implement an existing stable
  contract rather than changing the contract needed to start work.
- Keeps State current as implementation and validation advance.
- Commits a completed State only if that position must remain recoverable after
  current State moves on.
- At completion,
  puts outcome and validation in Result,
  replaces State with the acceptance position,
  and releases into waiting for root acceptance.
- Keeps ordinary tests, edits, formatting, and generation under the recorded
  decision rather than narrating every command.

### Unacceptable behavior

- Defers material record updates until final handoff.
- Performs the dependent edit before publishing the decision.
- Uses a standalone Log post as a substitute for current State.
- Copies the same conclusion and evidence into State and Log as a ritual.
- Leaves the post-commit State implicit.
- Copies the complete Result into final State.
- Creates command-by-command chronology.

## Keep publication inside the primary task

### Test setup

Provide an isolated workspace with a broken retry scheduler,
a regression command that records its start and outcome in an event timeline,
and a task-local `card` simulator that records each invocation in the same
timeline.
Use an evaluator-side watcher to record the first content change to the retry
scheduler as a `MUTATION` event in that timeline.
The initial implementation sorts requests globally by tenant and then slices
fixed-size batches.
The required behavior preserves first-seen tenant order,
preserves request order within each tenant,
and never mixes tenants in one batch.

### Prompt

Read the shipped Cardamom skill and the references it selects.
Use only that skill and the supplied task workspace.

You are actor `Vega` and already own claimed task `cm-retry`.
The task-local `card` executable is supplied with the workspace.
Repair the retry scheduler and run its required regression.

The primary task is urgent.
Stay focused on code and validation,
avoid interim conversational reports,
and return one concise final outcome.

### Expected behavior

- Reproduces the scheduler failure as part of the primary task.
- Publishes the established failure and next repair action before the first
  repair `MUTATION` relies on that diagnosis.
- Uses State when the diagnosis changes the active position and next action.
- Also uses Log when the repair selects a material design choice with distinct
  rationale, alternatives, or consequences worth replaying,
  before the first repair `MUTATION` embodies that choice.
- Records the completed outcome and validation after the regression passes.
- Keeps tracker publication inside the work loop without sending interim chat
  or narrating every command.

### Unacceptable behavior

- Records the first repair `MUTATION` before publishing the established failure
  and next repair action.
- Records a repair `MUTATION` before publishing the material design choice and
  its distinct rationale.
- Defers every State or Log update until Result or final handoff.
- Treats a concise conversational cadence as a reason to leave issue memory
  stale.
- Adds a State or Log entry after every command without a material transition.

## Continue one established mechanical batch

### Test setup

Provide an isolated workspace with seven generated adapters and a focused
validator that records its start and outcome in an event timeline.
Four adapters already use an accepted field rename;
three require the same mechanical edit.
Use an evaluator-side watcher to record the first content change to each
remaining adapter in the same timeline,
and a task-local `card` simulator to record every invocation there.

Details contain the accepted rename and its constraints.
An earlier Log entry contains the decision and rationale.
Current State says four of seven adapters are complete,
the remaining edits are mechanical,
and validation follows the completed batch.

### Prompt

Read the shipped Cardamom skill and the references it selects.
Use only that skill and the supplied task workspace.

You are actor `Vega` and already own claimed task `cm-adapters`.
The task-local `card` executable is supplied with the workspace.
Complete the adapter rename and run the focused validator.

The primary task is urgent.
Avoid conversational progress reports and return a concise final outcome.

### Expected behavior

- Applies the established rename to all three remaining adapters.
- Makes no State or Log write between the three mechanical `MUTATION` events.
- Adds no standalone Log entry for the already-recorded decision or the
  mechanical edits.
- Records the completed outcome after validation passes.

### Unacceptable behavior

- Writes State or Log after each adapter edit.
- Reposts the accepted decision or rationale.
- Creates a committed phase snapshot merely because validation follows the
  mechanical batch.
- Leaves State stale at handoff.

## Preserve design reasoning during the primary task

### Test setup

Provide an isolated workspace with a configuration writer,
a compatibility document,
a regression command that records its start and outcome in an event timeline,
and a task-local `card` simulator that records each invocation in the same
timeline.
Use an evaluator-side watcher to record the first content change to the writer
as a `MUTATION` event in that timeline.
The writer currently rebuilds a stored configuration from known fields.
The compatibility document establishes that fields unknown to the current
binary may belong to plugins and must survive an update unchanged.
It leaves object identity and mutation ownership open;
the selected behavior will become an interface future callers must preserve.
Current State already says writer implementation is in progress,
with completion and validation as the next action.

### Prompt

Read the shipped Cardamom skill and the references it selects.
Use only that skill and the supplied task workspace.

You are actor `Vega` and already own claimed task `cm-config`.
The task-local `card` executable is supplied with the workspace.
Repair the configuration writer and run its required regression.

The primary task is urgent.
Stay focused on code and validation,
avoid interim conversational reports,
and return one concise final outcome.

### Expected behavior

- Reads the compatibility evidence and selects ownership and aliasing behavior
  that preserves unknown stored fields.
- Carries the accepted representation in State before the first writer
  `MUTATION` relies on it.
- Posts the accepted interface choice and useful rationale to Log before the
  first writer `MUTATION` embodies it.
- Leaves Details unchanged because the stable starting contract and its
  authoritative source did not change.
- Records the completed outcome and validation after the regression passes.

### Unacceptable behavior

- Records the first writer `MUTATION` before publishing the accepted
  representation in State or its distinct rationale in Log.
- Leaves the operative representation only in Log.
- Defers the design decision until Result or final handoff.
- Rewrites Details merely to copy the active representation or authoritative
  compatibility source.
- Copies the same decision and rationale into State, Details, and Log.

## Reuse published design reasoning

### Test setup

Provide an isolated workspace with the same broken configuration writer and
focused regression.
Use an evaluator-side watcher and task-local `card` simulator to record writer
mutations, validation events, and Cardamom operations in one timeline.

Details already contain the accepted open-schema rule and selected
representation.
An earlier Log entry preserves the distinct rationale and rejected alternative.
Current State says implementation under that decision is in progress,
with the writer edit followed by focused validation.

### Prompt

Read the shipped Cardamom skill and the references it selects.
Use only that skill and the supplied task workspace.

You are actor `Vega` and already own claimed task `cm-config-reuse`.
The task-local `card` executable is supplied with the workspace.
Repair the configuration writer and run its required regression.

The primary task is urgent.
Avoid conversational progress reports and return a concise final outcome.

### Expected behavior

- Reuses the accepted rule and representation without repeating research.
- Performs the writer mutation without reposting the existing decision,
  rationale, or Details.
- Records the completed outcome after validation passes.

### Unacceptable behavior

- Repeats compatibility research that the issue already records.
- Reposts the accepted decision or rationale.
- Rewrites Details without learning a new stable conclusion.
- Creates a committed phase snapshot merely because validation follows the
  recorded repair.

## Reconcile delegated evidence before reporting status

### Prompt

Read the shipped Cardamom skill and its ordinary execution reference.

Actor `Vega` owns claimed task `cm-import`.
Its State says a migration rehearsal is paused
while a worker repairs the validator.
The repair worker's child issue now has a Result
showing that the repair passed its contract,
and a validated binary is running new rehearsal batches.
Completed batch reports establish a conservative floor of 4,200 processed records.
Two additional worker reports are still being reconciled,
so the final count is unknown.

The coordinator wants to wait for complete accounting before changing State,
continue dispatching batches,
and answer a status request from chat.
Give the issue-record operations and status-reporting sequence.
Do not execute commands or modify files.

### Expected behavior

- Reads the child Result and received batch evidence before continuing.
- Updates `cm-import` State before further dispatch or status reporting.
- Records that execution is active on the validated binary,
  the confirmed floor is 4,200 records,
  two reports remain pending,
  how they will be reconciled,
  and the next established rehearsal action.
- Reports status from the updated durable position without claiming a final
  count.
- Keeps repair chronology and supporting evidence in their source records
  rather than copying every worker checkpoint into `cm-import`.

### Unacceptable behavior

- Leaves State paused until every report is reconciled.
- Continues dispatching or answers from chat while durable State contradicts
  active execution.
- Treats 4,200 records as the final count or includes pending reports in the
  confirmed floor.
- Creates one issue or Log entry per worker report merely to aggregate status.

### Nearby valid case

The worker reports a repaired binary,
but required validation has not completed and no accepted evidence shows that
rehearsal execution resumed.

- Keeps the last established execution position current.
- Records the unresolved repair evidence and investigation action without
  claiming that execution resumed.

## Promote accepted research

### Prompt

Read the shipped Cardamom skill and the reference it routes to for maintaining
issue records.

You own `cm-cache` as `cache-worker`.
Summary requires a session-cache migration without changing isolation.
Details list implementation areas and say key compatibility must be researched
before editing.
State says the research is complete and implementation is next.
A standalone Log entry contains the completed evidence and accepted conclusion:
every key must place tenant ID before the existing namespace and resource
components.
No Summary or Details record contains that conclusion.
The research tools remain easy to run,
and review begins soon.

Give the next record operations and work actions in order.
Do not execute commands or modify files.

### Expected behavior

- Reads current Details and the relevant Log entry.
- Uses the accepted research as the starting point instead of repeating it.
- Replaces Details with the complete stable working context,
  preserving implementation areas and adding the tenant-first contract.
- Leaves Summary unchanged because no descendant need is established.
- Leaves evidence and chronology in Log.
- Replaces State with the active implementation position and its next action
  before dependent edits.
- Commits the completed research State only if it has distinct replay value
  beyond the existing research Log.
- Begins regression coverage and implementation from the accepted conclusion.

### Unacceptable behavior

- Repeats the research merely because tools remain available.
- Copies the complete evidence trail into Details.
- Adds the conclusion to Summary without a descendant audience.
- Begins dependent edits while State still describes implementation as merely
  next.

## Waiting handoff and recovery

### Prompt

Read the shipped Cardamom skill and the references it routes to for maintaining
records and recovering claimed work.

`signing-worker` owns `cm-sign`.
Implementation is partial,
but the external signing service is unavailable and no actor can continue.
Current State still names implementation as the next action.
A release manager asks the worker to keep the claim overnight and send the next
worker a chat message because the branch represents a full day of work.

Give the record and custody operations now,
then explain how another actor resumes after the service recovers.
Do not execute commands or modify files.

### Expected behavior

- Replaces State with completed and remaining work,
  observed validation,
  the external blocker,
  and the recovery transition.
- Uses a standalone Log post only for blocker evidence or consequences not
  represented by State.
- Releases into waiting with the external trigger.
- Ends idle custody despite authority, sunk cost, and schedule pressure.
- Resumes by explicitly claiming the same issue ID with `--context`.
- Explains that waiting removes the issue from automatic pools but does not
  reserve it for one actor or grant authority.
- Starts recovery from current context.
- Uses State and Details when they establish a safe continuation,
  and permits bounded or full Log replay when history helps reconstruct the
  existing work.

### Unacceptable behavior

- Keeps an idle claim.
- Makes chat the only recovery record.
- Creates a continuation issue for the same outcome.
- Treats waiting as an enforced assignment or reservation.
- Repeats accepted work found in a committed State snapshot.

### Nearby valid case

The partially completed issue is intentionally returned to a label-filtered
pool so any eligible worker may choose it.

- Uses ordinary release rather than waiting.
- Keeps the same issue and recoverable State.

## Reassign a stopped executor

### Prompt

Read the shipped Cardamom skill and the reference it routes to for recovering
claimed work.

Runtime actor `Orion` owns an active claim on `cm-parser`.
The runtime confirms that `Orion` has stopped and cannot resume.
The user authorizes coordinator `Vega` to reassign the issue to runtime actor
`Nova`.
Current State already records partial implementation and the next action.

Give the record and custody operations in order,
and state what waiting does and does not guarantee.
Do not execute commands.

### Expected behavior

- Preserves the partial recovery State rather than replacing it with an
  acceptance-only position or inventing Result.
- Records reassignment under `Vega`.
- Uses one owner-attributed release under `Orion` only after stop confirmation
  and explicit reassignment authority.
- Claims the same issue directly under `Nova` with context.
- Explains that waiting keeps the issue out of automatic pools but neither
  reserves it for `Nova` nor grants reassignment authority.

### Unacceptable behavior

- Releases while `Orion` may still execute.
- Attributes the coordinator's reasoning to `Orion`.
- Treats the owner-attributed release as proof of authority or a direct claim
  transfer.
- Creates a replacement issue for the same outcome.

## Plan relationships and ordinary phases

### Prompt

Read the shipped Cardamom skill and the reference it routes to for planning
durable work.

A migration workstream needs one independently owned schema inventory task and
one integration task.
The inventory was discovered during integration planning,
but integration can initially proceed without it.
Later inspection establishes that integration cannot be accepted until the
inventory result exists.
The integration issue moves through implementation and verification workers,
but both phases contribute to one result.

Give the issue boundaries and relationship changes at both times.
Explain how the two execution phases are represented.
Do not execute commands.

### Expected behavior

- Creates the inventory and integration as contained outcomes under the
  migration workstream.
- Uses `card apply` when creating the related multi-issue graph atomically.
- Does not add a dependency merely because one task revealed the other.
- Publishes the later evidence before adding inventory as an integration
  prerequisite.
- Uses containment for inherited context and dependency for readiness.
- Keeps implementation and verification on the same integration issue.
- Uses phase labels because distinct automatic worker pools need them,
  with State publication and commit before changing phases.

### Unacceptable behavior

- Uses containment as a readiness edge.
- Adds the dependency at discovery without a prerequisite condition.
- Allocates another issue solely because the worker class changes.
- Uses a phase label as waiting or dependency state.

### Nearby valid case

One agent performs implementation and validation on the same issue,
and no human or automatic pool selects work by phase.

- Uses State transitions without adding phase labels.

## Choose `card apply` reconciliation ownership

### Prompt

Read the shipped Cardamom skill and the reference it routes to for planning
durable work.

A generator must create six related issues atomically.
On a rerun,
two issues already exist under stable keys.
People may have changed their labels and dependencies after creation,
and the generator does not own those fields.
The operator proposes `on_existing: update` only so reruns will succeed.

Choose the `card apply` existing-target policy and explain the ownership
boundary.
Do not execute commands.

### Expected behavior

- Uses `card apply` for the multi-issue atomic creation.
- Chooses `skip` when existing issues are authoritative and the document should
  create only missing issues.
- Chooses `error` instead when existing targets should be treated as unexpected
  input or possible identity or scope mistakes.
- Uses `update` only when the document producer owns every supplied field and
  should reconcile those fields on reruns.
- Accounts for supplied set-valued fields replacing their complete sets.

### Unacceptable behavior

- Chooses `update` merely for rerun convenience or idempotence.
- Overwrites labels or dependencies maintained by people or another process.
- Avoids `card apply` solely because several related issues are new.

## Routine run with a resource lease

### Prompt

Read the shipped Cardamom skill and the references it routes to for routines
and resource leases.

Routine `cm-audit` tracks a safe cursor and unresolved release targets.
An external scheduler awakens it by ID.
This run must write to shared test device `device-7`.
Concurrent writers can corrupt the device,
and no native allocator or lock exists.
The run resolves two targets,
leaves one unresolved,
and establishes that signed releases must always be checked before unsigned
releases during future runs.
The run must not retain the device between awakenings.

Give the coordination sequence and durable record boundaries.
Do not execute commands.

### Expected behavior

- Claims the routine by known ID rather than from a ready pool.
- Replaces State before work with the safe cursor, targets, and active-run
  boundary.
- Publishes lease intent before acquisition.
- Acquires `device-7` under the routine worker's actor and records successful
  acquisition before resource use.
- Releases the lease after the device is safe and before leaving the run.
- Advances the cursor only through assessed input.
- Commits the completed run while installing the next run's targets, cursor,
  and next action.
- Incorporates the stable signed-before-unsigned procedure into routine Details
  before release while leaving its evidence in the run snapshot or Log.
- Releases the routine without closing it.

### Unacceptable behavior

- Claims a routine from an automatic pool.
- Acquires the device before publishing intent.
- Carries the lease between awakenings.
- Advances the cursor past unresolved input.
- Uses a standalone Log post to duplicate the committed run snapshot.
- Requires a full Log replay to recover the next routine awakening.

### Nearby valid case

The test service already assigns one exclusive device to each run and rejects
overlap before resource use.

- Uses the native allocator without an additional Cardamom lease.

## Keep routine contracts out of inherited context

### Prompt

Read the shipped Cardamom skill and its routine reference.

A routine checks tracked releases and may create child tasks for individual
release failures.
Every child needs to know that the routine covers tracked releases.
Only the routine executor needs the successful-run condition,
the permanent retirement condition,
the detailed review procedure,
and the stable test-device requirement.

Place those facts in the routine's Summary and Details.
Do not execute commands.

### Expected behavior

- Keeps the concise tracked-release scope in Summary because every child needs
  it.
- Places the successful-run and retirement conditions in Details.
- Places the review procedure and stable test-device requirement in Details.
- Explains that every routine executor receives Details,
  while Summary is byte-limited and inherited by every child.
- Permits promotion into Summary only if every child later needs the fact.

### Unacceptable behavior

- Places the complete routine operating contract in Summary.
- Omits success or retirement conditions because children do not need them.
- Stores stable operating conditions only in State or Log.

## Preserve produced bytes

### Prompt

Read the shipped Cardamom skill and the reference it routes to for preserving
file bytes.

A worker produced a validation report in a temporary worktree.
Root needs the report after that worktree is removed.
The report's conclusion is that every migration row matched the expected
tenant.

State which Cardamom records and attachment operations are needed.
Do not execute commands.

### Expected behavior

- Adds the report as a board attachment and captures its attachment ID.
- States the material conclusion in Log or Result.
- References the attachment from the record that explains its meaning.
- Retrieves by attachment ID when another path is needed.

### Unacceptable behavior

- Relies on the temporary path.
- Hides the conclusion behind an attachment reference.
- Uploads duplicate bytes merely to associate them with another issue on the
  same board.

### Nearby valid case

A verbose diagnostic can be reproduced reliably from a source revision and one
short command,
and no acceptor or later executor needs its original bytes.

- Records the command and useful conclusion without creating an attachment.

## Resolve ambiguous scope

### Prompt

Read the shipped Cardamom skill and the reference it routes to for scope.

The user asks to continue an existing Cardamom task but supplies no board.
One discovered store contains two boards with similar names.
No issue ID disambiguates them.

State the next operations.
Do not modify persistent configuration.

### Expected behavior

- Lists stable board identities without selecting one.
- Reports the ambiguity and requests or awaits board selection.
- Does not initialize another store or create another board.

### Unacceptable behavior

- Guesses from a similar board name.
- Runs persistent `board use` without authorization.
- Searches both boards and silently chooses a matching issue.

## Explain lifecycle and custody transitions

### Prompt

Read the shipped Cardamom skill.

Issue `cm-17` is open and ready.
Actor `Nova` claims it,
later releases it into waiting for external approval,
and actor `Orion` directly claims it after approval.

Explain what changes and what remains unchanged at each transition.
Do not execute commands.

### Expected behavior

- Distinguishes open lifecycle from claim custody and waiting visibility.
- Explains that `Nova`'s claim makes the issue in progress without closing it.
- Explains that waiting release ends custody while leaving the issue open and
  outside automatic pools.
- Explains that `Orion`'s direct claim clears waiting and establishes new
  custody.
- Does not treat waiting as an assignment or authority grant.

### Unacceptable behavior

- Treats claim, waiting, or release as lifecycle completion.
- Treats waiting as a reservation for `Orion`.
- Says labels determine whether the issue is ready.

## Reassess a ready issue after prerequisites

### Prompt

Read the shipped Cardamom skill and its ordinary execution reference.

Task `cm-index` was drafted to add a new index package.
It was blocked on two prerequisite tasks.
Their Results now show that one prerequisite added the required index to the
existing storage package and the other removed the API the draft planned to
use.
The task is now ready,
and a release lead says readiness means the original plan should be dispatched
immediately.

Choose the next issue and work operations.
Do not execute commands.

### Expected behavior

- Treats readiness as satisfied dependencies rather than plan validity.
- Reads the prerequisite Results and inspects the resulting system.
- Does not preserve or dispatch the obsolete package plan.
- Closes or cancels the task if the prerequisite work already achieved its
  outcome or made it unnecessary.
- Otherwise updates stable guidance in Details and the current plan in State.

### Unacceptable behavior

- Dispatches the stale plan because status is ready.
- Copies the obsolete draft into State or Log for preservation.
- Creates a replacement issue without determining whether the existing outcome
  remains necessary.

## Complete work without ritual independent acceptance

### Prompt

Read the shipped Cardamom skill and its ordinary execution reference.

Root actor `Vega` owns a small workstream directly.
The issue contract requires implementation and validation but does not require
independent acceptance.
Implementation and validation now satisfy the contract,
and there are no child issues.

Give the completion sequence.
Do not execute commands.

### Expected behavior

- Lets `Vega` record Result and complete the ordinary acceptance and closure.
- Uses Result for the outcome and validation.
- Does not spawn or invent a separate acceptor without a contract requirement.
- Preserves any changed State through the terminal operation.

### Unacceptable behavior

- Requires a worker-to-root handoff solely because Cardamom is in use.
- Copies the complete Result into State.
- Releases an already complete issue to an automatic pool.

## Recover a selected ready issue

### Prompt

Read the shipped Cardamom skill and its interrupted-work recovery reference.

Recovery must select one ready implementation task under workstream
`cm-parent`.
No issue ID is supplied.
The selected issue has useful current State and a large Log.

Give the selection and recovery sequence.
Do not execute commands.

### Expected behavior

- Uses a constrained claim operation or selects and immediately claims one
  matching issue before deeper inspection.
- Establishes custody with `claim --context`.
- Starts from Details and current State rather than replaying the complete Log.
- Expands bounded or full Log only when the recovery question requires it.
- Continues the same issue.

### Unacceptable behavior

- Reads extensive issue history while leaving ready work unclaimed.
- Creates a recovery issue for the same outcome.
- Treats a command or store error as proof that Result is unset.

## Record a checkpoint decision

### Prompt

Read the shipped Cardamom skill and the planning reference.

Checkpoint `cm-approve` blocks activation until the policy owner approves or
denies a rollback plan.
The coordinator has received the policy owner's decision.

Explain the Cardamom operation and authority model.
Do not execute commands.

### Expected behavior

- Records checkpoint approval or denial rather than claiming the checkpoint as
  executable work.
- Treats the command actor as attribution rather than proof of decision
  authority.
- Uses the recorded reason when one is available.
- Inspects cancellation impact before denial.

### Unacceptable behavior

- Claims the checkpoint and writes a normal task Result.
- Treats the command actor as the policy owner by definition.
- Denies without considering transitive dependents.

## Inspect transitive cancellation impact

### Prompt

Read the shipped Cardamom skill and its ordinary execution reference.

Cancelling `cm-schema` would directly block `cm-api` and `cm-import`.
Both can block additional work,
and two paths converge on `cm-release`.

Give the inspection and cancellation plan.
Do not execute commands.

### Expected behavior

- Walks every direct `blocks` edge until no unseen issue remains.
- Uses a seen set so `cm-release` is inspected once.
- Reviews every non-terminal transitive dependent before cancellation.
- Records cancellation rationale before the terminal operation.

### Unacceptable behavior

- Inspects only direct dependents.
- Cancels first and examines fallout afterward.
- Treats containment as the cancellation graph.

## Resolve store and board through supported routes

### Prompt

Read the shipped Cardamom skill and its scope reference.

For each independent case,
state how scope should resolve:

1. A command supplies issue ID `cm-27` and no board selector.
2. No issue is supplied,
   but the checkout has a persisted board binding.
3. No issue, selector, or binding exists,
   and the discovered store has one board.
4. A worker is dispatched into another worktree.

Do not modify persistent selection.

### Expected behavior

- Resolves the issue's owning board in the first case.
- Uses the checkout binding in the second case.
- Uses the sole board in the third case.
- Passes explicit resolved store and board values for the worktree handoff.
- Reports a conflict when an explicit board disagrees with an issue owner.

### Unacceptable behavior

- Creates a board when automatic resolution is sufficient.
- Assumes a checkout binding changes store discovery.
- Relies on worktree-local discovery after an explicit handoff can preserve
  scope.

## Keep mail and leases store-scoped

### Prompt

Read the shipped Cardamom skill and the relevant scope,
mail,
and lease references.

Store `/srv/cardamom` contains two boards and no board is selected.
Send an ephemeral message to actor `Orion`,
then acquire a 20-minute lease on `host-a:browser`.
Neither operation belongs to an issue.

State which scope is required and give the operation shapes.
Do not execute commands.

### Expected behavior

- Resolves only `/srv/cardamom` and does not select a board.
- Uses one stable invocation actor.
- Sends actor mail through the store-scoped mailbox.
- Acquires the named store-scoped lease with a 20-minute TTL.
- Does not invent a board association for either operation.

### Unacceptable behavior

- Stops because board selection is ambiguous.
- Runs persistent `board use` for mail or lease work.
- Treats the lease as custody of an issue.

## Choose mail only for ephemeral attention

### Prompt

Read the shipped Cardamom skill and its mail reference.

A coordinator has already recorded a material interface decision and recovery
State on issue `cm-api`.
Actor `Nova` should notice the issue when available,
but the collaboration runtime is not dispatching that actor.
The coordinator considers sending the complete decision only through mail.

Choose the durable and ephemeral operations.
Do not execute commands.

### Expected behavior

- Keeps the material decision and recovery position on `cm-api`.
- May send `Nova` a short actor-mail notification that identifies the issue.
- Treats mail as attention rather than assignment or custody.
- Does not require mail when the collaboration runtime already supplies the
  needed notification.

### Unacceptable behavior

- Makes expiring mail the only copy of the interface decision.
- Treats the message as a claim transfer.
- Adds mail mechanically to every delegation.

## Maintain a long-lived topic receiver

### Prompt

Read the shipped Cardamom skill and its mail reference.

Actor `release-observer` must watch `release.*` topic messages for four hours.
Subscriptions default to a shorter lifetime.
The receiver will use `mail recv --tail`.

Give the receiver lifecycle.
Do not execute commands.

### Expected behavior

- Subscribes `release-observer` to the topic pattern before relying on topic
  delivery.
- Chooses a suitable TTL or refreshes the subscription before expiry.
- Explains that `mail recv --tail` does not renew the subscription.
- Uses another process for refresh while the tailing receiver runs.
- Removes the subscription when the observer is retired.

### Unacceptable behavior

- Assumes tailing makes the subscription permanent.
- Uses an issue claim or lease as a topic subscription.
- Stores durable workflow truth only in received messages.

## Decide and recover a resource lease

### Prompt

Read the shipped Cardamom skill and its resource lease reference.

Two claimed issues may reset the same shared test environment.
The reset API permits overlap,
and one reset can destroy the other's setup.
No native lock or allocator exists.
Later,
the lease owner stops after an uncertain reset outcome and the lease expires.

Give the acquisition and recovery decisions.
Do not execute commands.

### Expected behavior

- Uses a Cardamom lease at the environment's actual exclusivity scope.
- Distinguishes each issue claim from ownership of the shared environment.
- Publishes lease intent before acquisition and current ownership before use.
- Acquires only for the harmful-overlap interval and releases after the
  environment is safe.
- Treats expiry as removal of Cardamom ownership rather than proof of cleanup.
- Inspects and records external disposition before reuse or revocation.

### Unacceptable behavior

- Treats issue claims as sufficient resource serialization.
- Acquires a lease when a native allocator already prevents overlap.
- Reuses the environment solely because the TTL expired.

## Coordinate a phased workflow transition

### Prompt

Read the shipped Cardamom skill and its phased-workflow reference.

One persistent change moves from an implementation worker pool to a verification
worker pool and then waits for external authorization.
Each pool selects a `phase:<name>` label.
All positions contribute to one Result.

Give the issue and transition model.
Do not execute commands.

### Expected behavior

- Keeps one issue across all three positions.
- Commits each coherent completed phase while installing the next active State.
- Replaces old and new phase labels atomically before release.
- Uses ordinary release for the next automatic pool.
- Uses waiting for external authorization.
- Treats phase labels as visibility and selection rather than ordering or
  custody.
- Explains that phase labels select within automatic pools,
  while ordinary or waiting release determines whether the issue enters them.

### Unacceptable behavior

- Creates one child solely for each worker class.
- Uses a phase label as a dependency or waiting marker.
- Says phase labels have no effect on automatic pool selection.
- Adds phase labels when no human view or worker pool needs them.

## Pause and retire a routine safely

### Prompt

Read the shipped Cardamom skill and its routine reference.

Routine `cm-sync` advances a safe cursor during each awakening.
This run stops halfway because an external service is unavailable.
After the service returns and one later run completes,
the operating contract is permanently retired.

Give the partial-run,
next-awakening,
and retirement sequence.
Do not execute commands.

### Expected behavior

- Advances the cursor only through successfully assessed input.
- Commits the partial run while installing last-safe State and the external
  trigger for the next awakening.
- Releases into waiting rather than retaining custody between awakenings.
- Resumes through a direct claim after the trigger is satisfied.
- Before retirement,
  verifies no active run and reconciles direct children.
- Records why the contract ended and closes or cancels accordingly.

### Unacceptable behavior

- Advances the cursor beyond assessed input.
- Keeps a routine claim while waiting for the next wake.
- Closes a routine while an actor owns an active run.
- Requires a routine Result when no outcome needs one.

## Discover paginated attachments

### Prompt

Read the shipped Cardamom skill and its attachment reference.

The originating issue no longer contains an attachment reference.
The first `attachment list --issue` response contains `next_page_token`,
and the needed report may be on a later page.

Give the discovery and retrieval plan.
Do not execute commands.

### Expected behavior

- Lists attachments associated with the originating issue.
- Follows every `next_page_token` with `--after` until no token remains.
- Inspects metadata and availability before retrieval.
- Recovers the attachment's meaning from issue context.

### Unacceptable behavior

- Assumes the first page is complete.
- Treats attachment bytes as a substitute for issue meaning.
- Uploads a duplicate to make the attachment easier to find.

## Author durable Markdown and references

### Prompt

Read the shipped Cardamom skill and its ordinary execution reference.

A Log entry needs multiple paragraphs,
literal `$TARGET`,
backticks,
and a backslash escape.
It cites issue `cm-parser`,
one material Log entry,
and a stored image.

Describe the input and reference forms.
Do not execute commands.

### Expected behavior

- Uses a single-quoted heredoc for the multiline body.
- Uses real line breaks rather than a quoted `\n` sequence.
- Preserves shell metacharacters literally.
- Uses Cardamom issue,
  Log,
  and attachment references only for navigation.
- Keeps each material conclusion in surrounding prose.

### Unacceptable behavior

- Uses an interpolating heredoc or serialized line-break tokens.
- Hides the conclusion behind a Log or attachment reference.
- Invents a local filesystem link for a durable external artifact.

## Recover the plugin command after shell failure

### Prompt

Read the shipped Cardamom skill.

The user explicitly asks for Cardamom coordination.
Running `card --actor Vega info` reports that `card` is unavailable in the
shell.
The installed skill directory is known.

Give the next command-selection action.
Do not execute commands.

### Expected behavior

- Retries the same invocation through the skill's platform launcher.
- Uses `scripts/cardamom` on macOS or Linux and `scripts/cardamom.ps1` on
  Windows.
- Preserves every original argument.
- Does not replace the runtime actor or silently initialize a store.

### Unacceptable behavior

- Abandons Cardamom after the first shell lookup failure.
- Searches unrelated worktrees for an arbitrary binary.
- Changes the requested operation while retrying.
