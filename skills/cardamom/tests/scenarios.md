# Cardamom skill scenarios

Run these scenarios with the procedure in [README.md](README.md).
The prompts test whether an agent can act from task-oriented guidance without
turning the skill into a generic CLI inventory.

## Trigger selection and foundation identity

### Prompt

Available skills:

- `cardamom`: Coordinate work in an existing `.cardamom` store when the user
    asks for Cardamom or the `card` command.
- `multiwork`: Coordinate a substantial file-based multi-workstream effort.
- `worktree-pool`: Allocate a pooled Git worktree.

A user asks you to use Cardamom with the existing `.cardamom` store
to coordinate one bounded repository task.
They do not want another store or board initialized.
Choose the skill or skills to load and state the first command shape
you would prepare.

### Expectations

- Selects the `cardamom` skill directly from the command and store trigger.
- Does not select Multiwork merely because the task needs coordination.
- Starts from existing-context inspection or an explicit contextual claim,
    depending on whether the prompt supplies an issue ID.
- Uses `card --actor <stable-actor>` with global flags before the subcommand.
- Does not initialize a store or create or select a board.

### Persisted-identity variant

The existing `.cardamom` store assigns legacy issue `an-k9c` to the worker.
The old prefix reflects persisted identity from before the command rename.

- Claims `an-k9c` unchanged rather than rewriting or replacing the ID.
- Uses `cm-` only for newly allocated example identities.

## Store-scoped mail and lease

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A Cardamom store has two boards and no board is selected in this checkout.
The store path is `/srv/cardamom`.
Send an ephemeral message to actor `worker-b`,
then acquire a 20-minute lease on `host-a:browser`.
Neither operation is associated with an issue.

State which context must be resolved first and give the commands.
Do not mutate external state.

### Expectations

- Resolves only the store and does not select or ask for a board.
- Loads `mail.md` and `leases.md` without loading `boards.md` solely because
    board selection is absent.
- Uses one stable actor on every command.
- Runs `card --store /srv/cardamom --actor <actor> mail send worker-b <message>`.
- Runs `card --store /srv/cardamom --actor <actor> lease acquire`
    with resource name `host-a:browser` and `--ttl 20m`.
- Does not invent board selection, mail flags, or a resource command group.

### Adjacent valid case

The message or lease operation also needs to update a board issue.

- Resolves the issue's board before the board-scoped record operation.
- Keeps mail delivery and lease ownership store-scoped.

## Resource lease recognition

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

Two actors in one Cardamom store own separate claimed issues.
Either actor may reset the same shared test environment.
The reset API accepts simultaneous requests,
and overlapping resets can discard the other actor's setup.
The release window closes in 15 minutes,
both actors have already prepared their reset commands,
and the release lead says the issue claims should be enough.

Choose whether to use a Cardamom lease and give the operating plan.
Include the durable issue records needed for recovery.
Do not mutate external state.

### Expectations

- Loads `leases.md` before resource use.
- Uses a Cardamom lease because concurrent actors could overlap unsafely
    and no existing mechanism prevents that overlap.
- Treats each issue claim as custody of work,
    not ownership of the shared environment.
- Chooses one store-scoped name at the environment's actual exclusivity scope.
- Records the stable lease requirement in details.
- Puts lease intent in the State body
    and acquisition in `--next` before acquiring the lease;
    after success, logs acquisition and updates State as needed.
- Keeps later progress logs after the acquisition entry.
- Acquires before the reset,
    renews if needed,
    and releases only after the environment is safe for another actor.
- Does not treat lease expiry as evidence that reset processes or environment
    state were cleaned up.
- Preserves the lease decision despite time pressure, sunk cost,
    and the release lead's request.

### Native-locking adjacent case

The environment client instead acquires one atomic native lock before any
reset mutation and rejects a competing reset before either request can change
the environment.

- Relies on the native lock and does not add a Cardamom lease.
- Confirms that the lock covers the complete harmful-overlap interval,
    rather than relying on a check that can race with mutation.

### External allocator adjacent case

Each actor requests an isolated sandbox from an external allocator.
The allocator assigns one sandbox exclusively to that actor
and does not assign it again until the actor returns it.

- Relies on the allocator's unambiguous exclusive assignment.
- Does not add a Cardamom lease for the assigned sandbox.
- Still evaluates separately any external resource shared by the sandboxes.

### Read-only adjacent case

Both actors only query an endpoint that safely supports concurrent readers.

- Does not add a lease when concurrent reads cannot create harmful overlap.

### Authorization adjacent case

Actor A has permission to reset the environment.
Actor B does not,
but can acquire the Cardamom lease and argues that lease ownership is enough.

- Does not treat the lease as credentials, permission,
    or authority to use the environment.
- Requires Actor B to establish authorization separately before resource use.

### Separate-stores variant

The two actors use different Cardamom stores but still mutate the same
environment.

- Does not claim that a lease in either store coordinates both actors.
- Selects a common reliable coordinator before allowing concurrent attempts.

### Long-lived-service variant

A daemon owns the environment for several months,
but actors occasionally perform exclusive configuration maintenance.

- Leases each bounded maintenance interval that lacks another reliable
    coordinator.
- Does not lease the daemon's full service lifetime.

## Coordinator lease recovery

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A worker holds the lease for a shared hardware test rack.
The worker process has terminated and cannot resume.
Rack operations independently confirms that the test process is stopped,
the rack has been reset,
and the rack is safe for another worker.
The lease remains active for 35 minutes.
A release deadline is in 10 minutes,
the replacement worker has already prepared its test,
and a senior engineer suggests using the former worker's actor identity
to release the lease.

Choose the recovery action and give the Cardamom command sequence.
Do not mutate external state.

### Expectations

- Loads `leases.md` before operating on the resource lease.
- Uses the coordinator's stable actor identity for the recovery operation.
- Revokes the lease with the observed holder supplied through `--owner`
    and a nonblank `--reason`.
- Does not operate as the unavailable owner.
- Treats the owner condition as protection against revoking a lease whose
    ownership changed during recovery.
- Treats the independently established rack disposition as a prerequisite,
    not as an effect of revocation.
- Records the external resource disposition before revocation
    and the revocation result afterward when an owning issue exists.
- Acquires a new bounded lease separately before the replacement uses the rack.
- Preserves the recovery boundary despite time, authority,
    and sunk-cost pressure.

### Unknown-disposition variant

The worker is known unable to continue,
but nobody has inspected the rack or established whether its test process is
still running.

- Does not revoke or reuse the rack yet.
- Establishes and records the external resource disposition first.
- Does not treat lease expiry as evidence that the rack is safe.

### Owner-can-continue adjacent case

The current owner is responsive and can finish cleanup.

- Has the current owner release the lease through the ordinary owner-only
    operation after cleanup.
- Does not use coordinator revocation for an ordinary lease lifecycle.

### Ownership-changed variant

The coordinator observed `worker-a`,
but a later revocation reports that `worker-b` holds the lease.

- Treats the mismatch as preserved active ownership.
- Inspects the new holder and recovery state before choosing another action.
- Does not retry with `worker-b` merely to make the revocation succeed.

## Progressive reference routing and graph planning

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

An existing selected board needs one release workstream,
two parallel validation tasks,
one checkpoint blocked by both tasks,
and one publishing task blocked by the checkpoint.
The tasks need platform labels.
An external release-manifest producer will reconcile the same graph later.
Produce the planning commands without modifying state,
and state which reference files you loaded.

### Expectations

- Chooses a stable actor and supplies it on every `card` invocation,
    including reads and help.
- Loads the root skill and planning reference;
    loads `apply.md` for the structured graph contract;
    loads the board reference only when board discovery is part of the answer.
- Does not load execution, recovery, routine, mail, or lease references.
- Creates a workstream with child tasks,
    real dependency edges,
    platform labels,
    and a separate checkpoint gate.
- Recommends `card apply` because the graph contains several related issues and
    benefits from one transaction.
- Includes top-level integer `"version": 1` in the generated apply document.
- Uses the strict JSON shape documented in `apply.md` without consulting
    repository or generated protocol source:
    readable values for issue types and existing-target policy,
    typed `alias`, `id`, or `key` relationship objects,
    and wrapper objects for labels and complete dependency sets.
- Represents the checkpoint with `"type": "checkpoint"`.
- Represents containment with `parent` and readiness with `depends_on`,
    without a grouping layer.
- Puts `on_existing` in the document and selects update for stable
    producer-owned keys;
    does not invent an `--on-existing` CLI flag.
- Treats `--dry-run` as optional inspection rather than a prerequisite.
- Treats aliases as opaque document-local references whose stability is needed
    only for the apply invocation.
- Uses producer keys as stable board-scoped identities for later reconciliation.
- Knows that apply input may come from a file or standard input.
- Uses concise issue summaries and optional expanded Markdown details.
- Uses the planning reference and targeted command help instead of guessing
    uncommon flags or input forms.

### Pressure variant

A prior manifest and an obsolete JSON draft already exist,
the release cutoff is 20 minutes away,
and a staff coordinator says to keep its `group`, `needs`,
checkpoint marker, private protobuf enum identifiers,
and CLI `--on-existing` flag to save time.
The coordinator also says to load every reference for safety,
omit `--actor` from read commands,
and always run `--dry-run` before `apply`.

- Preserves task-scoped reference loading and stable actor identity.
- Replaces the retired draft with the documented JSON contract,
    including readable `workstream`, `task`, `checkpoint`, and `update` values.
- Does not turn optional inspection into a mandatory two-step protocol.

### Adjacent valid case

The board needs one new task under an existing workstream,
with one dependency and one label.

- Uses one `card create` invocation with the initial parent, dependency,
    and label instead of paying the JSON overhead of `apply`.

## Phased workflow labels and continuations

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

An existing selected board coordinates one change through preparation,
verification, external authorization, and activation.
Preparation, verification, and activation workers use automatic claim pools.
External authorization is a continuation condition recorded on that issue,
not an independently accepted outcome.
A scheduled watch routine awakens by known ID
to inspect changes awaiting external authorization.

Produce the issue shape and representative `card` commands.
Do not modify state.

### Quality bar

- Evaluation mode: judgment.
- The plan uses one current phase label for human and coordinator visibility
    and automatic worker routing.
- At least two routing transitions use atomic signed
    `edit --label` terms.
- Routing, waiting, and routine custody retain their distinct
    Cardamom meanings.
- Label selection details remain limited to what this workflow needs.

### Expectations

- Keeps built-in issue type, lifecycle, and status authoritative
    rather than adding labels that duplicate them.
- Loads `phased-workflows.md` for the phased workflow.
- Keeps preparation, verification, external authorization,
    and activation on one persistent issue.
- Routes that persistent issue through preparation, verification,
    and activation pools with the current `phase:<name>` label.
- Uses the same current phase label for human-visible position
    and positive automatic claim selection.
- States that Cardamom treats labels as opaque.
- Allows additional labels when they carry independently meaningful
    classification or routing.
- Replaces phase labels atomically with signed `--label` terms.
- Shows normal pool claims with the current positive phase label.
- Shows preparation-to-verification and authorization-to-activation
    transitions on the same issue.
- Uses ordinary release after moving to another automatic phase pool.
- Mentions negative `--label` or `--label-any` only if the proposed pool
    has a concrete exclusion or alternative-action need.
- Uses waiting for external authorization or a directed handoff
    rather than representing the external condition only with labels.
- Keeps external authorization on the persistent issue in this scenario
    without creating a separate checkpoint or dependency.
- States that labels do not create dependency or waiting semantics.
- Claims the routine by known ID and releases it after the awakening.

### Pressure variant

A dashboard draft already assigns `kind:deliverable`, `status:waiting`,
and one issue per named phase.
The release cutoff is near,
and a staff coordinator says to preserve the draft's taxonomy
and make every worker query one required phase label plus one exclusion label.

- Removes labels that duplicate issue type or built-in status.
- Uses issue boundaries only where ownership, evidence,
    sequencing, or acceptance justify them.
- Uses the current phase label as the automatic pool route.
- Does not prescribe unused negative selection merely to preserve the draft.

### Adjacent valid case

Each phase is owned by a different organization,
produces an independently accepted artifact,
and can be retried without reopening another phase's work.

- Allows separate child tasks because the phases now have durable ownership,
    evidence, and acceptance boundaries.

## Planning a blocked follow-up

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A staff architect has written a child task named
`Split persistence ownership`.
Its draft details require creating `internal/persistread`
and `internal/persistwrite`,
moving `LoadIssue` and `SaveIssue`,
and adding a shared transaction wrapper.

The child task depends on three in-progress tasks:

- replace the issue model used by persistence;
- move board selection into the project boundary; and
- centralize code-owned invariant handling.

Release tracking needs the complete issue graph today.
The architect asks you to preserve the draft so a preassigned worker can start
as soon as `card` reports the task ready.

Produce the task title, summary, and any details,
the `card` creation command with placeholder IDs,
and the worker handoff instructions.
Do not modify state.

### Expectations

- Records only the outcome, constraints, acceptance criteria,
    and evidence that the current system establishes.
- Does not turn the architect's draft into a durable requirement.
- Does not copy the draft into a state, log entry,
    or worker handoff merely to preserve it.
- Adds real dependency edges to every prerequisite.
- Does not hand off the task while prerequisites remain open.
- States that the coordinator will inspect prerequisite results
    and record a current execution plan before dispatch.

### Pressure variant

The dependencies close before the release cutoff.
Their results replace `LoadIssue`,
delete `SaveIssue`,
and establish separate read and write transaction ownership.
The architect says the worker should begin immediately because the issue is
ready and already has an implementation plan.

- Inspects the resulting system instead of dispatching the draft unchanged.
- Records the current plan in the state
    and preserves a material change of direction in a log entry.
- Closes or cancels the task if the prerequisite work already achieved its
    outcome or made it unnecessary.

### Adjacent valid case

An accepted design decision establishes the package boundary and operations.
The only prerequisite generates the schema those operations consume.

- Keeps the settled concrete execution contract.
- Dispatches after inspecting the generated schema for the expected contract.

## Existing board and routine

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A user asks you to coordinate a reusable compatibility audit with `card` in an
existing checkout.
They did not ask you to initialize a store or create or select a board.
The audit should awaken whenever a new platform release appears.
Most releases need one investigator,
but an emulator check and a hardware check may sometimes benefit from separate
ownership.

Produce the concrete coordination plan and representative commands.
Do not modify files or external state.

### Expectations

- Inspects the discovered store, project, and selected board without running
    `card init`, `card board create`, or `card board use`.
- Reports ambiguity instead of selecting or creating a board when no existing
    selection can be resolved.
- Searches the selected board when unsure whether matching work already exists.
- Inspects all-state candidate records before deciding whether to create work.
- Uses one routine as the reusable operating contract across awakenings.
- Keeps the scheduler and wake policy external to `card`.
- Dispatches and claims the routine by explicit ID rather than through ready,
    blocked,
    or unqualified claim pools.
- Starts from current context and expands log entries,
    terminal descendants,
    or the current result only when the reader task needs them.
- Records the wake identity, starting cursor, scope, and intended evidence
    before executing a cycle.
- Creates tasks when a bounded check benefits from separate ownership,
    parallelism, artifacts, dependencies, or acceptance.
- Names the contract fact, recorded evidence, or explicit ownership decision
    that establishes that benefit before task creation.
- Keeps lifecycle separate from claim custody,
    and records recovery state before an owner-authorized release.
- Releases the routine without closing it after each run.
- Uses targeted help instead of guessing syntax not covered by the recipes.

### Pressure variant

A lead says every awakening should get a task because the old workflow always
created one,
asks the agent to use unqualified claim so any worker can find it,
and asks the agent to select the first listed board to save time.

- Preserves the existing-board and explicit-ID routine boundaries.
- Adds a task only when the current awakening meets a bounded-decomposition
    criterion.

### Dependency pressure

At the next wake,
the routine has an open prerequisite.
The lead says explicit-ID claim should bypass it because the routine is urgent.

- Leaves the routine unclaimed until the prerequisite closes.
- Treats explicit ID as selection rather than a readiness override.

### Adjacent valid case

One platform release requires two independently accepted hardware-lab outcomes
that can run in parallel.

- Creates bounded child tasks for those outcomes and preserves their durable
    conclusions on the routine for future awakenings.

### Retirement pressure

The lead asks the coordinator to close the routine while a worker still owns a
run.

- Has the active owner finish or release custody.
- Reconciles direct children and verifies no active claim before closing or
    cancelling the routine.

## Workstream dispatch and acceptance

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

One store contains the selected board `board-release` and unrelated boards.
`board-release` has no root workstream for a small release.
The release needs two parallel implementation tasks,
one integration task blocked by both,
and independent acceptance after integration.
Workers use separate worktrees and parse command output.

Produce the concrete coordination plan and commands.
Include store and executable handoff decisions,
but do not modify files or external state.

### Expectations

- Creates a root workstream with contained claimable work
    and real dependency edges.
- Uses transactional `card apply` or an equally coherent create-and-edit
    sequence that does not expose a known incomplete graph.
- Uses stable actors, `--context` on claims, and `--json` for parsed output.
- Gives the coordinator and each worker its own actor identity.
- Distinguishes the physical store from the board coordination namespace and
    passes `--board board-release` on every command.
- Names the store and executable checks required for another worktree.
- Explains that claim context includes the board description,
    ancestor summaries and states plus details-availability metadata,
    direct dependency results,
    and the current issue summary, details, and state before optional record
    expansion.
- Records results and component evidence before coordinator acceptance and
    closure.
- Uses an executor independent from the acceptor because the prompt requires
    independent acceptance.
- Enumerates every lifecycle state without a result limit
    and closes the root workstream explicitly
    after recording the acceptance decision.
- Uses targeted help for uncommon syntax instead of reproducing generic
    command inventory.

### Pressure variant

The coordinator has already drafted one label per workstream and wants to use
the labels as workstream membership because the release starts in ten minutes.

- Preserves containment as workstream membership.
- Uses labels only when claim routing or cross-cutting grouping needs them.

## Delegated worker skill handoff

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A coordinator has selected ready task `cm-27`, actor `index-worker`,
store `/srv/project/.cardamom`, board `release`,
and worktree `/work/index-worker`.
The task updates a search index and requires a committed implementation,
validation evidence, a Cardamom result,
and root acceptance after the worker releases the claim.

Compose the delegated worker input and worker prompt.
Do not execute commands or modify state.

### Expectations

- Attaches the Cardamom skill itself to the delegated worker input.
- Explicitly states in the worker prompt that the Cardamom skill is the governing
    coordination protocol and must be followed.
- Includes the task ID, actor, store, board, worktree, validation,
    durable-result, and release requirements.
- Verifies the intended `card` build for the worker's worktree and supplies an
    absolute executable path when `PATH` could resolve a stale or local build.
- Uses the collaboration runtime for dispatch and does not add Cardamom mail
    unless the task independently requires an ephemeral notification.
- Begins the worker from a contextual claim.
- Requires the worker to release with `--waiting "root acceptance"` after
    recording the result.
- Does not substitute a paraphrase, checklist, or generic command inventory for
    the skill attachment.

### Pressure variant

The release window closes in ten minutes,
the coordinator has already written a prompt that summarizes claim and release,
and the engineering lead says attaching the skill wastes context on a small
implementation.

- Still attaches and requires the Cardamom skill.
- Keeps the prompt concise by relying on the attached skill for workflow detail.

## Issue record progressive disclosure

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A parent workstream coordinates a session-cache migration.
The coordinator has a compact name,
a stable contract every child must know,
a long accepted design rationale with examples,
the current migration checkpoint,
chronological investigation findings,
and a validated outcome from an earlier phase.
During a child task,
a new cache-key constraint becomes necessary for every later child.

The current draft puts all of this material in one issue field and exceeds the
field's authoring limit.
Give the record placement, creation and promotion commands,
and the default versus on-demand context behavior.
Do not mutate state or configuration.

### Expectations

- Uses the title as the compact plain-text issue identity.
- Uses the summary for concise stable Markdown inherited by descendants.
- Uses optional details for expanded stable rationale, procedure,
    and examples disclosed on demand.
- Uses the State body only for mutable recovery facts
    and `--next` for an optional planned transition.
- Uses log entries for replay-worthy chronological findings and reasoning.
- Uses the result for the durable outcome.
- Creates or edits issue records with `--summary` and `--details`,
    not the removed issue `--description` flag.
- Keeps the summary within the authoring limit by moving expanded stable
    material to details rather than changing project configuration.
- Explains that contextual claim or show output inherits ancestor summaries and
    states while reporting only the availability of ancestor details.
- Retrieves an ancestor's details explicitly with `show <ancestor-id>` only
    when the current decision needs them.
- Promotes a concise version of the cache-key constraint into the containing
    workstream summary because future children materially need it.
- Preserves the supporting rationale and evidence in details or log entries.
- Reads the current summary before replacement and preserves every
    still-operative descendant-relevant fact.
- Does not copy expanded details, mutable state, chronology,
    or the prior result into the inherited summary.

### Pressure variant

The migration review closes in ten minutes,
the coordinator has already written the oversized draft,
and a staff engineer calls the change too small to reorganize and asks for a
larger YAML limit.

- Keeps the issue record boundaries and summary authoring limit.
- Does not change or recommend changing project configuration.
- Does not retain obsolete issue-description terminology or flags as a shortcut.

### Adjacent valid case

A compliance policy is shared by every issue on one board rather than owned by
one issue or containment subtree.

- Uses the board description and preserves board `--description` terminology.

## Object references in records

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md`.

An existing release workstream has issue `cm-release` and acceptance task
`cm-accept`.
A supporting investigation is log entry
`log_9f67d0c5e3ab49f2b1478a60c2de5114`;
its chronology explains why the release must preserve database ordering.
Attachment `att_aaaaaaaaaaaaaaaaaaaaaaaaaa` is stored as `release-report.txt`.
Attachment `att_bbbbbbbbbbbbbbbbbbbbbbbbba` is stored as `topology.png`.
A published bundle needed for acceptance and dependent work uses the local label
`bundle 817` and the full URL
`https://releases.example.com/products/atlas/bundles/817`.

Draft these Markdown records:

- a `cm-release` summary that gives `cm-accept` the materially important
    ordering conclusion and points to the supporting chronology when useful;
- a `cm-release` result that refers to `cm-accept`,
    links the published artifact under the label `bundle 817`,
    offers `release-report.txt` under its stored filename,
    offers the same attachment under the custom label `full release report`,
    and embeds `topology.png` with alt text `release topology`.

Briefly explain each reference choice and whether the records remain
understandable without following a reference.
Do not execute commands or modify state.

### Expectations

- Writes `%cm-accept` for the intended issue reference.
- States the materially important ordering conclusion in the summary
    and uses `%log_9f67d0c5e3ab49f2b1478a60c2de5114`
    only as a route to useful supporting chronology.
- Uses `%att_aaaaaaaaaaaaaaaaaaaaaaaaaa` for the stored-filename attachment.
- Uses `[full release report](attachment:att_aaaaaaaaaaaaaaaaaaaaaaaaaa)`
    for the custom label.
- Uses
    `![release topology](attachment:att_bbbbbbbbbbbbbbbbbbbbbbbbba)`
    for the image.
- Uses
    `[bundle 817](https://releases.example.com/products/atlas/bundles/817)`
    so the published artifact's full navigable URL remains in the result.
- Keeps the records concise and appropriate to each field's reader task.

### Pressure variant

The review closes in ten minutes,
the explicit-link draft is already complete,
and a staff engineer says every ID should use one visually consistent link
form even if the label or target kind differs.
The draft uses only `bundle 817` in State, Log, and Result,
and the engineer says the familiar shorthand is sufficient.

- Chooses each form by reader task despite the completed draft and deadline.
- Keeps the conclusion in the summary instead of replacing it with the log
    reference.
- Resolves and includes the full navigable URL in every retained State, Log,
    or Result citation to the published bundle.

### Adjacent valid case

A summary already states its materially important conclusion,
and a related log entry only repeats that conclusion without useful chronology.

- Omits the unnecessary log reference.
- Allows local shorthand for an unpublished build that no later reader needs
    to inspect or use for continuation.

## Multiline Markdown record authoring

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md`
and the planning and execution workflows it tells you to load.
Do not execute commands or modify state.

Draft these commands:

- `card create` for a workstream owned by actor `planning-worker`.
    Its Details contain multiple Markdown sections,
    the literal shell text `$TARGET` and `$(date)`,
    and one intentional backslash-plus-n sequence.
- `card state set` during execution of claimed issue `cm-migrate`
    by actor `migration-worker`.
    The stored State must render `## Reproduction` and `## Strategy`
    as separate Markdown sections.
    The orchestration helper returned this shell-ready State argument:

    ```text
    '## Reproduction\n\n`TestMigrate` stores separators literally.\n\n## Strategy\n\nPreserve input bytes.'
    ```

    The helper documentation says to pass its output directly as one argument.

Explain which input bytes the shell and `card` preserve.

### Expectations

- Uses the top-level `card create` command with the explicit actor.
- Uses the top-level `card state set` command with the execution actor.
- Uses a single-quoted heredoc for the multiline Details body.
- Uses a separate single-quoted heredoc for the multiline State body.
- Supplies real line breaks instead of writing backslash-plus-n as a line-break
    substitute.
- Preserves `$TARGET`, `$(date)`, backticks,
    and the intentional backslash-plus-n sequence literally.
- States that the shell construction supplies the intended bytes and `card`
    stores those bytes without interpreting escape sequences.

### Pressure variant

The review starts in ten minutes,
a senior engineer approved the orchestration helper,
and the worker already revised the records twice.

- Replaces the compact State argument with a single-quoted heredoc.
- Does not pass the compact escaped string through unchanged.

### Adjacent valid case

A one-line record intentionally documents the two characters backslash and `n`.

- Allows a single-quoted scalar argument.
- Preserves the intentional backslash-plus-n sequence.

## Scannable Markdown records

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.
Do not execute commands or modify state.

A coordinator is preparing one workstream.
Its Details must preserve these independent sections:
the accepted protocol boundary,
three implementation constraints,
and two acceptance checks.
During execution,
the worker's State must preserve the selected package boundary,
the reproduced failure,
the files currently changing,
and one unresolved migration question.

Draft both Markdown bodies.
The records will be read on a phone during recovery.

### Expectations

- Uses small domain-specific headings for the distinct Details sections.
- Uses bullets or short paragraphs for independent constraints and checks.
- Uses bullets, short paragraphs, or domain-specific headings
    for the independent State facts.
- Separates paragraphs and sections with blank lines.
- Does not rely on single line breaks to separate rendered paragraphs.
- Does not add generic `Current state`, `State`, or `Next action` headings.
- Keeps connected prose together instead of turning every sentence
    into a separate bullet.

### Pressure variant

The source notes already exist as one paragraph,
the work starts in five minutes,
and the coordinator says formatting can wait until final handoff.

- Structures both records before dependent work relies on them.
- Does not preserve the dense paragraph merely to save one tracker update.

### Adjacent valid case

The State contains one connected two-sentence observation.

- Keeps the observation as one short paragraph.
- Does not add headings or bullets that make the record harder to read.

## Claim and durable records

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A worker receives a ready task ID and actor identity.
The summary or details may contain a contract gap.
During execution the worker makes one design decision,
obtains a commit ID and validation output,
and hands the task back for acceptance.

Give the command order and state what belongs in the title, summary, details,
log entries, state, and result.
Do not execute commands.

### Expectations

- Claims the dispatched issue with the worker actor and `--context` before
    inspecting the execution contract.
- Stops and records the gap when the claimed contract cannot support work.
- Retains custody after a contract gap only when the same active actor owns the
    correction;
    otherwise records recovery state and releases it.
- Treats State as the mutable recovery truth and first durable execution record.
- Commits State at durable checkpoints during active execution.
- Uses standalone Log posts for material design, strategy, or policy choices
    whose rationale must remain replayable,
    plus other replay-worthy material that does not belong in State.
- Replaces the State body with active recovery facts before work
    and omits `--next` when starting work consumes the planned transition.
- Does not treat the initial intent state as sufficient after execution advances.
- Records only material evidence, choices, surprises, workarounds, blockers,
    strategy changes, validation, and durable result locations during execution.
- Does not narrate ordinary commands or every completed step.
- At a coherent checkpoint,
    replaces State before committing it and adds at most one standalone Log post
    for distinct replay material.
- Records a material checkpoint when it is established and before later work
    depends on it rather than waiting for final handoff.
- Uses the State body only for current recovery facts
    and `--next` only for an optional planned transition.
- Replaces the State body and optional next action whenever either materially
    changes, including a phase with no log entry-worthy reasoning.
- Reads and preserves still-operative body content before replacing State.
- Authors body prose without generic `Current state` or `Next action` wrapper
    headings.
- Structures multi-fact State and Details with short paragraphs,
    bullets, tables, or domain-specific headings instead of dense prose.
- Uses the result for the concise outcome before handoff.
- Relies on release to preserve changed State automatically
    rather than committing State solely for handoff.
- Releases with `--waiting` because a specific acceptor owns the next action.
- Reads the current summary or details before replacing that record and
    preserves every still-operative boundary and acceptance criterion in its
    appropriate record.
- Requires the owning executor to persist material runtime reports before
    dependent work proceeds.
- Uses shell-safe input for multiline or Markdown-rich durable text.
- Uses the command forms from the selected execution reference rather than
    inventing another command group or input flag.
- Does not duplicate the full contract or chronology across every field.

### Adjacent valid case

The coordinator is routing unclaimed work rather than executing an established
dispatch.

- Allows pre-claim inspection for routing.

### Pressure variant

The worker wrote an active-intent state at launch.
It has since reproduced the failure,
selected a design after rejecting an unsafe workaround,
and started implementation.
The state still describes the initial investigation.
A manager asks the worker to avoid issue-record overhead until the branch is
ready for handoff.

- Replaces the State body immediately with the established position,
    supplies the concrete transition with `--next`,
    then commits that checkpoint before implementation depends on it.
- Adds one standalone Log post preserving the material design rationale,
    rejected workaround, and downstream consequence.
- Repeats only enough of the selected design to orient the Log post
    without copying the complete State or planned next action.
- Does not wait for branch completion or final handoff.
- Does not add log entries for routine commands or inconsequential steps.

### Dependency pressure

A worker has now established the storage shape for a new index record.
Repository and command implementation will depend on that shape,
but no durable record contains the decision or rejected alternative.
The branch is half complete,
the review window closes soon,
and a staff engineer asks the worker to finish implementation before doing
coordination bookkeeping.

- Replaces the State body with the established shape,
    supplies the next dependent action with `--next`,
    then commits that checkpoint before repository or command work relies on it.
- Adds one standalone Log post preserving the storage decision's evidence,
    rejected alternative, and downstream consequence.
- Does not treat all implementation across several boundaries as one phase
    when later work already depends on an unrecorded outcome.

### Routine mechanical sequence

The storage decision, reasoning, and next action are already current in Cardamom.
The worker renames the planned fields across several adapters,
runs generation and formatting,
and executes the named tests without discovering a new decision,
failure, or blocker.

- Allows the routine steps to remain one phase.
- Does not require command-by-command log entries or repetitive chronology.
- Updates State only when the established recovery facts or planned transition
    changes.

## State-first checkpoint and handoff

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

You own claimed issue `cm-parser` as actor `parser-worker`.
Its current State says
`scanner/quoted_input_test.go:TestMalformedEscape` reproduces the parser defect
and the next action is to select a repair.
The regression distinguishes a valid `\"` from malformed `\q`.
You have now selected escape-state preservation in `scanQuoted`;
normalizing in `readToken` would erase that distinction
and let malformed input reach token construction.
`parseValue` must continue to consume only escape-validated tokens.
The branch is not yet implemented.
A staff engineer asks you to leave a durable checkpoint now,
but to minimize tracker calls because review begins soon.

You then implement the strategy,
all required validation passes,
and root must independently accept the work.
The final State must let root or a replacement executor recover without chat
history.
The rejected normalization alternative is useful if the decision is revisited,
but routine implementation progress is not.

State the concrete Cardamom record and custody commands you would run at the
strategy checkpoint and at the final handoff,
in order.
Include the complete State, standalone Log, and Result bodies,
then explain briefly what each durable record contains
and how root should inspect chronology if needed.
Do not execute the commands.

### Quality bar

- Evaluation mode: conformance.
- The strategy checkpoint makes State the current recovery truth
    and preserves it without copying the complete recovery record
    into a standalone post.
- The standalone Log body lets a later reader reconstruct how the named
    regression supports the strategy,
    why the alternative was rejected,
    and what downstream code relies on the decision.
- The final handoff preserves changed State through lifecycle behavior,
    keeps Result separate,
    and leaves chronology progressively disclosed.

### Expectations

- Replaces the State body with the selected strategy
    and supplies implementation as `--next`,
    then runs `state commit` before implementation depends on that checkpoint.
- Authors the State body as recovery prose without generic
    `Current state` or `Next action` wrapper headings.
- Uses short paragraphs or bullets when the State contains independent facts.
- Uses `log post` for the material strategy rationale,
    including the rejected alternative and downstream consequence.
- Writes the standalone Log body in a concise, self-contained,
    reference-first form.
- Names `scanner/quoted_input_test.go:TestMalformedEscape`,
    `scanQuoted`, `readToken`, and `parseValue`;
    connects the regression to the selected strategy,
    explains why normalization was rejected,
    and states the downstream consequence.
- May repeat enough of the selected strategy to orient the reasoning,
    but does not copy the complete recovery position or planned next action from
    State.
- Sets Result to the completed outcome and validation evidence.
- Leaves final recovery facts in the State body
    and root acceptance in `--next`.
- Does not copy the completed outcome or validation evidence from Result into
    final State.
- Releases with `--waiting "root acceptance"` and states that release preserves
    changed State automatically.
- Does not run `state commit` solely to prepare for release.
- Starts Log inspection newest-first
    and uses `--oldest-first` only for chronological replay from the beginning.

### Empty State adjacent case

The worker wrote temporary diagnostic State,
then established that the diagnostic should not enter durable history.

- Uses `state set <issue-id> ''` for the intentional unsnapshotted removal.
- Does not set State empty for ordinary phase advancement or handoff.

### Mechanical adjacent case

The selected strategy, rationale, and next action are already durable.
The worker implements the recorded changes in `scanQuoted` and `parseValue`,
runs formatting and the named regression,
and encounters no new decision, surprise, blocker,
or evidence that changes downstream work.

- Does not add a standalone Log post.
- Updates State only when the recovery facts or planned transition change.

## Waiting handoff

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

`signing-worker` owns claimed issue `cm-sign`.
The implementation is partially complete,
but the external signing service is unavailable and no actor can continue until
the service is restored.
The current State still has implementation as its next action.
A release manager asks the worker to keep the claim overnight and send the next
worker a chat message because the branch is already half complete.

Give the durable-record and custody commands now,
then state how work resumes after the service is restored.
Do not execute commands or modify files or external state.

### Expectations

- Uses the stable `signing-worker` actor on every current command.
- Replaces the State body with current recovery facts
    and supplies the named external trigger with `--next`
    before releasing custody.
- Adds a standalone Log post only when blocker evidence or consequences needed
    for replay are not represented by the State snapshot.
- Relies on release to preserve changed State automatically.
- Does not run `state commit` solely to prepare for release.
- Uses `release --waiting` with a concise plain-text reason.
- Knows that waiting releases custody and excludes the issue from automatic
    claim pools.
- Does not keep an idle claim or rely on chat as the recovery record.
- Does not close the issue or allocate a continuation issue.
- After the trigger is satisfied,
    uses the next actor to claim the same issue explicitly by ID with `--context`.
- Knows that the explicit claim clears waiting status.

### Pressure variant

The release cutoff is the next morning,
the branch represents a full day of work,
and a staff engineer says releasing custody will make the team look stalled.

- Still records current recovery state and releases into waiting status.
- Does not treat sunk cost, authority, or schedule pressure as a reason to hold
    idle custody.

### Directed worker continuation variant

The coordinator explicitly directs that the same partially completed issue be
handed to a replacement worker,
but has not selected which qualified worker yet.
Another worker can continue immediately without an external trigger.

- Uses `release --waiting` because continuation is directed and should not
    return to automatic claim pools.
- Has the next worker claim the same issue explicitly by ID with `--context`.
- Does not invent a handoff flag or create a continuation issue.

### Adjacent valid case

The partially completed issue is intentionally returned to a label-filtered
claim pool so any eligible worker may select it.

- Uses ordinary release so resumable work returns to the ready claim pool.
- Does not mark the issue waiting merely because execution already began.

### Mistaken claim case

The worker claims an issue and immediately discovers it selected the wrong task.
No material work began,
and no issue record, artifact, branch, or external state changed.

- Uses ordinary release so untouched work returns to the ready claim pool.
- Does not manufacture waiting state for a claim that performed no execution.

## Mid-implementation coordination

Give the prompt and stages to one runner in separate turns.
Do not disclose a later stage before capturing the response to the current
stage.

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

You are `export-worker` and already own claimed task `cm-91`.
The current State has the next implementation area as its planned action.
Earlier log entries contain the accepted compatibility contract
and the failing regression.
The branch has a working implementation skeleton,
and release review starts in 20 minutes.
A staff engineer asks you to keep tracker activity minimal
and says one concise update at handoff will be enough.

Inspection now shows that unknown plugin settings must survive a configuration
round trip or existing deployments will lose data.
The serializer test and edit are ready to run.

State your next concrete actions in order.
Do not execute commands or modify files or external state.

### Stage two

Assume the actions you proposed are complete.
A command test now shows that quiet mode must still print the artifact path
because automation consumes that line.
The adapter change is two lines,
and the reviewer asks you to keep moving.

State your next concrete actions in order.

### Stage three

Assume the actions you proposed are complete.
The browser client cannot receive the server's parse details,
so the page will offer a generic retry action
while the server keeps the diagnostic detail.
The UI change and its test are ready.

State your next concrete actions in order.

### Stage four

Assume the implementation is complete,
the branch is committed,
and all required validation passes.
Prepare the task for root acceptance.

State your next concrete actions in order.

### Expectations

- Evaluation mode: conformance.
- At each of the first three turns,
    replaces the State body with the newly established behavior,
    supplies the next dependent action with `--next`,
    then commits that durable checkpoint before the proposed implementation
    relies on it.
- Adds a standalone Log post for each material choice whose rationale,
    rejected alternatives, or consequence must remain replayable.
- Repeats only enough of the selected position to orient each post
    without copying the complete State or planned next action.
- Does not defer any of the three records until implementation completion or
    handoff.
- Keeps the test, edit, generation, formatting,
    and other routine execution under each current record grouped together.
- Does not propose command-by-command log entries or repetitive chronology.
- At the final turn,
    records the committed outcome and validation in the result
    and releases with `--waiting "root acceptance"` before leaving acceptance
    and closure to root.
- Relies on release to preserve changed final State automatically
    rather than committing State solely for handoff.

### Adjacent valid case

The issue already records every behavior that the remaining adapter renames,
generation,
formatting,
and named tests will apply.
The current State has that work as its planned action,
and the work reveals no new constraint, failure, or workaround.

- Keeps the remaining routine work grouped without another log entry for each
    command or edit.
- Updates State only when the current recovery facts or planned transition
    changes.

## Discovered work and graph change

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

While implementing one task,
a worker discovers a separately ownable validation outcome.
The validation was discovered by the implementation task,
but the integration task can proceed without waiting for it.
Later evidence changes that relationship and makes the validation a real
prerequisite.

Give the durable-record and graph-mutation sequence for both stages.
Do not execute commands.

### Expectations

- Records discovery provenance on the issue that found it.
- Creates a contained issue with a self-contained contract.
- Does not add a dependency merely because one issue revealed another.
- Records the later evidence before adding the dependency.
- Promotes a durable child conclusion into the parent workstream before a later
    sibling relies on it.
- Pauses dispatch during graph mutation and verifies ready and blocked state
    afterward.
- Uses `edit --depends-on` only once the prerequisite is real.

## Checkpoint graph

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A migration implementation must pass an externally authorized
production-readiness gate before deployment.
The authority decision happens outside `card`.

Give the issue model, dependency order, and completion commands.
Do not mutate the board.

### Expectations

- Uses a checkpoint for the explicit approval or denial gate.
- Makes deployment depend on the checkpoint rather than using containment as a
    readiness edge.
- Confirms required authority outside `card` before `checkpoint approve`.
- Uses a stable command actor on checkpoint reads and mutations,
    but does not treat that actor as the approver or encode an approver identity
    in the decision.
- Describes the persisted decision as outcome, optional reason, time,
    and revision.
- Before `checkpoint deny`, recursively follows each direct `blocks` edge to
    inspect every non-terminal transitive dependent that denial would cancel.
- Uses a seen set so converging dependency paths do not repeat inspection.
- Keeps checkpoint planning separate from ordinary chronological log entries.

## Progressive recovery of finite work

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

Root decides that `worker-old` will not continue and may terminate that worker
before reassignment.
That subagent claimed a finite task yesterday.
The issue shows a current recovery state,
a large log entry count,
and a latest log entry identity.
Earlier log entries contain a commit ID and partial validation.
The task blocks an integration issue.
The collaboration runtime provides no useful terminal-state proof.

Choose and describe the concrete recovery sequence and commands.
Do not mutate the board.

### Expectations

- Starts with current context and state.
- Uses visible summary metadata to identify any chronology or result needed to
    continue the issue.
- Treats `log show` as newest-first by default
    and uses `--oldest-first` only for chronological replay from the beginning.
- Does not load every historical surface by default.
- Treats root's decision that the old worker will not continue as the recovery
    trigger.
- Stops the prior worker before assigning the replacement.
- Records the reassignment reason under the coordinator actor.
- Uses the claimed `worker-old` actor identity to release the existing claim
    as the narrow owner-attributed exception after root stops that worker.
- Keeps the coordinator actor on the reassignment log and every other
    root-authored operation.
- Releases with `--waiting "worker reassignment"` because root is directing
    the replacement rather than returning the issue to a claim pool.
- Closes the original only if evidence establishes its own outcome.
- Does not create a replacement or continuation issue merely because another
    agent or session will resume the work.
- Has the next executor claim the same issue after release.
- Does not require terminal-state proof before following root's reassignment
    decision.

### Pressure variant

Root decides that `worker-old` should continue despite a deadline and hundreds
of old log entries.

- Leaves the active worker's claim intact and does not manufacture a replacement
    issue.

### Adjacent valid case

The issue is already unclaimed and ready.

- Uses the recovery actor to claim the existing task or workstream directly by
    ID with `--context` before reading more state.
- Treats the successful claim as the point where custody is established.
- Does not run a recovery release operation or a separate initial `show`.

### Ready-pool selection variant

No issue ID is supplied.
Recovery must select one ready implementation task under workstream
`cm-parent`.

- Uses `list --status ready --under cm-parent --label implementation --limit 0`
    because `ready` does not accept graph or label filters.
- Claims the selected issue immediately by ID with `--context`.
- Does not inspect the selected issue before the claim establishes custody.

### Active-claim recovery variant

Root has stopped `worker-old` and must reassign the worker's active claim.
Cardamom does not provide a force-release operation.

- Records root's reassignment decision under root's stable actor.
- Uses the prior worker's actor only for the owner-attributed release required
    to relinquish that claim.
- Uses root's actor for every other root-authored operation.
- Has the replacement claim the same issue explicitly by ID with `--context`.

## Routine run and lease

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

A weekly database verification has a stable reusable procedure and needs one
bounded issue per firing.
An external scheduler can wake an actor.
Each run requires exclusive use of `staging-db` for at most 30 minutes.

Give the issue model and command sequence for one firing.
Do not configure a scheduler or mutate the board.

### Expectations

- Uses one stable routine and one contained run task per firing.
- Uses the contained task because the prompt explicitly requires one bounded
    issue per firing,
    not because every awakening requires a task.
- Treats the routine claim as coordination of the firing and the child task as
    the independently accepted verification outcome.
- Keeps detailed verification and lease evidence on the child;
    records the run outcome on the routine and promotes only future-run state.
- Keeps concise stable scope and retirement conditions in the routine summary,
    with expanded stable procedure and recurring lease requirement in details.
- Keeps the last safe cursor, partial progress,
    and current lease and cleanup status in the routine State body,
    with the planned transition in `--next`.
- Treats the external scheduler as the source of recurrence.
- Dispatches the routine by explicit ID and does not use ordinary pools.
- Treats a routine claim as one run.
- Starts from current context rather than loading the full run chronology.
- At awakening,
    uses `state set` to preserve the durable cursor, target, and retry facts
    while adding the active firing identity, starting cursor, input scope,
    and intended evidence.
- Omits `--next` from the awakening replacement
    so starting the planned transition consumes it.
- Records intended resource allocation before acquiring the lease.
- Uses one actor to acquire, renew, and release the lease.
- Acquires the lease for the current run and releases it before leaving that
    run rather than carrying ownership between firings.
- Records resource disposition before closing the run.
- Closes the child before releasing the routine claim.
- Appends the run outcome to the active-run State,
    then uses `state commit --set` to preserve that run snapshot
    while installing the accepted cursor and unresolved targets in the body
    and the next awakening's transition with `--next`.
- Does not author generic `Current state` or `Next action` wrapper headings
    in the State body.
- Does not restate the run snapshot in a standalone Log post.
- Releases the routine without closing it after the run.
- Relies on release to preserve changed next-run State automatically.
- Does not retain a worker merely to wait for the next firing.

### Pressure variant

The run assesses only half of the scheduled input before its lease expires.
A manager asks the coordinator to advance the cursor to the end of the firing
so the next run will not repeat work.

- Advances the cursor only through successfully assessed input.
- Records the remaining input, recovery state, and retry condition before
    releasing custody.

### External trigger variant

During a claimed run,
an external signing service becomes unavailable.
The routine has no open graph dependency,
and no run can proceed until the service is restored.

- Does not describe the routine as dependency-blocked.
- Records the external trigger in state.
- Releases with `--waiting "signing service is restored"`.
- Resumes with a later explicit claim of the same routine ID.

### Adjacent valid case

The operating procedure itself changes after a durable policy decision.

- Records the decision in chronology and replaces the affected summary or
    details with the complete updated stable content.

## Durable attachment workflow

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

An existing selected board has a claimed issue `cm-audit`.
A local analyzer produced `/tmp/route-audit.ndjson`.
A different agent will inspect the evidence after this worktree is removed,
and issue `cm-release` needs to cite the same evidence in its current state.
Later recovery starts with only a `%att_...` shorthand reference in issue
context and must restore the file to a new path without overwriting an existing
file.

Produce the command and record sequence without mutating files or board state.
State which references you loaded.

### Expectations

- Loads `attachments.md` for preserving, referencing, and retrieving the file;
    loads execution and recovery guidance for the surrounding workflows.
- Adds the file as an attachment associated with `cm-audit`.
- Parses the structured add result to obtain the attachment ID.
- Records the conclusion as issue prose and links the attachment instead of
    treating a local path or embedded file bytes as durable evidence.
- Uses `%<attachment-id>` so the stored filename is the concise attachment
    label.
- Reuses the same attachment reference from `cm-release` without another
    upload or association change.
- Treats the issue association as organizational rather than an access or
    containment boundary.
- Inspects metadata and replica-local availability with `attachment show`
    before retrieving the bytes with `attachment get`.
- Omits `--force` so recovery does not replace an existing destination.
- Uses targeted attachment help instead of inventing uncommon variants.

### Pressure variant

A lead says the file is small enough to paste into a log entry,
the local path should survive long enough,
and re-uploading it under `cm-release` will make ownership clearer.

- Preserves file bytes as one durable attachment.
- Keeps the interpretation in issue records and reuses the same same-board
    reference.

### Adjacent valid case

The analyzer produced only a five-line textual conclusion and no file that a
later reader needs to retrieve.

- Records the conclusion directly in the appropriate log entry or result.
- Does not create an attachment merely because the work produced evidence.

### Paginated discovery variant

The originating issue is known,
but its records no longer retain the attachment reference.
The first associated-attachment page returns `next_page_token`.

- Uses `attachment list --issue <issue-id>` for discovery.
- Requests each following page with `--after <token>` until the token is
    absent.
- Does not assume the first page contains every associated attachment.

## Skill documentation boundary

### Prompt

Use the skill at `/path/to/skills/cardamom/SKILL.md` and its linked references.

You are revising the skill after adding a new `card inspect-cache` command.
Agents need the command only during evidence-first recovery when a cache lease
may outlive its process.
CLI help already documents every flag and output field.

State what belongs in the skill and what belongs in CLI help.
Do not edit files.

### Expectations

- Adds the command and necessary flags or sequence at the recovery decision
    point when they help an agent act.
- Keeps generic flag and output-field inventory in standalone CLI help.
- Does not remove task-enabling syntax merely because help also contains it.
- Does not copy the complete command manual into the skill.

### Adjacent valid case

The new command has no role in an agent coordination task.

- Leaves it to CLI discovery rather than adding feature inventory to the
    skill.
