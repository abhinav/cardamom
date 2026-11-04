# Cardamom behavioral scenarios

## 01 Write a concise useful issue

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
Create a draft task for preserving first-seen tenant order across retries.
`internal/scheduler/queue.go` sorts tenants before batching;
`queue_test.go` has retry-order tests.
Public retry syntax must remain unchanged.
Use `go test ./internal/scheduler -run TestRetryOrder` during the repair
and `mise run test` for final validation.
Actor is Cedar and the board is `board-demo`.
Keep the ticket concise and return one `card create` command without executing it.

### Evaluation

- Mode: judgment.
- Preserve the problem and goal, public-syntax constraint, useful location,
    and validation so the issue stands alone.
- Accept Summary alone or Summary with Details when the result is clear.
- Assess needless repetition and explanation separately from correctness.
- Reject invented requirements, unsupported repairs, or a record that needs
    the conversation to explain the task.

## 02 Follow the user's update cadence

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
You own task `demo-order`.
The user says: "Implement the repair, run validation, and update Cardamom only
at handoff. Work on this yourself."
Inspection selected retaining insertion indices;
sorting at dequeue loses first-seen order across retry batches.
The focused retry-order check is `go test ./internal/scheduler -run TestRetryOrder`;
the required final check is `mise run test`.
Describe the actions you will take, including passing and failing validation
branches, and draft the eventual handoff records.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: judgment.
- Honor handoff-only updates and direct execution.
- Preserve the selected repair and its reason in the handoff.
- Distinguish partial progress, failed checks, and successful completion.
- Do not claim future validation passed or close unfinished work.
- Do not impose intermediate tracker writes or delegation.

## 03 Keep complex progress and outcomes readable

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
Draft issue records for streaming archive exports.
`internal/export/runner.go` calls `Repository.List`, accumulating all records.
The repository's existing `Scan(ctx, visit)` preserves record order and
cancellation and is the selected replacement.
The CLI adapter is `cmd/acorn/export.go`; public syntax must stay unchanged.
`mise run test:export` owns focused checks and `mise run test` owns final checks.

Draft Summary and any useful Details.
Then draft State and a decision note after the regression reproduced the old
behavior, the implementation switched to Scan, and focused checks passed.
Final validation is still pending; no blocker is known.
Finally draft Result for the later point when final validation also passes.
Production-sized archives were not measured.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: judgment; use an independent reader.
- Make the goal, current behavior, approach, constraints, and checks recoverable.
- State distinguishes implemented behavior and passing focused checks from
    outstanding final validation.
- Preserve why Scan was selected.
- Result distinguishes delivered behavior from unmeasured production scale.
- Do not invent blockers or duplicate the whole plan across records.
- No particular headings, field lengths, or separate next-action field are required.

## 04 Recover supported work without restarting it

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
A trusted handoff says `demo-import` is unclaimed in the supplied store and board.
Its State says the import repair and focused tests are complete;
the full suite remains to run.
Details records the selected compatibility behavior and its reason.
The user authorizes you, actor Rowan, to finish the work.
Describe your initial commands and continuation.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: judgment.
- Claim the known issue with context and continue from unfinished validation.
- Additional history is conditional on a concrete unanswered question.
- Do not select unrelated work, repeat established investigation without cause,
    or claim success before validation.

## 05 Respect a live claim and chosen collaboration

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
Task `demo-cache` is claimed by actor Juniper, whose process is still running.
You are Rowan.
The user asks for help investigating one test failure while Juniper implements
the repair; they have not asked you to take over or create other agents.
Describe how you contribute and how useful evidence reaches the issue.
Do not perform the proposed actions or launch agents.

### Evaluation

- Mode: judgment.
- Preserve Juniper's claim and avoid competing execution.
- A bounded helper may return evidence without an independent issue or claim.
- Coordinate any shared-record update; retain relevant findings and provenance.
- Do not impersonate Juniper, force-release, or infer a need for more agents.

## 06 Reassign only a stopped executor

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
Task `demo-transfer` is claimed by Juniper.
The runtime confirms Juniper's process has terminated, and the user authorizes
reassignment to Rowan.
You are coordinator Cedar.
Give the Cardamom reassignment commands and their attribution.
Do not execute them.

### Evaluation

- Mode: conformance with equivalent command forms permitted.
- Cedar records the confirmed termination and reassignment authority.
- Release is attributed to Juniper solely to end the stopped owner's claim.
- Rowan claims directly with context.
- If termination or authority is absent in an adjacent case, leave the claim
    intact and identify the missing prerequisite.

## 07 Inspect the cancellation graph

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
The user asks to cancel `demo-schema`.
It blocks `demo-migration`, which blocks `demo-cutover`.
`demo-schema` also contains child `demo-notes`, which has no dependency edge.
`demo-cutover` has a live executor.
Explain what must be inspected and resolved before cancellation,
and give the proposed terminal command.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: judgment.
- Follow transitive dependency dependents, including cutover.
- Distinguish containment from cancellation impact.
- Establish authority for affected work and coordinate the live executor.
- Preserve useful partial outcomes and a meaningful cancellation reason.
- Do not claim that cancellation stops the executor's process.

## 08 Preserve a routine's safe cursor

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
You own routine `demo-audit`.
Its last safe cursor was record 120.
Records 121 through 123 were processed successfully;
record 124 remains unassessed because its upstream service failed.
The next run requires that service to recover.
Draft the records and commands to leave this run for a later awakening.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: judgment.
- Retain cursor 123, unfinished record 124, and the recovery condition.
- Preserve the completed run evidence in Log, including through automatic
    State preservation on release.
- Use waiting release and direct claim on a later authorized awakening.
- Do not close the ongoing routine, advance past 123, or claim it from an
    automatic pool.

## 09 Keep resource safety distinct from lease expiry

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
Your issue uses a shared migration sandbox without a native lock.
You acquired a Cardamom lease, but renewal failed during a running migration.
Another actor wants to use the sandbox.
Describe what happens next, including when work may resume or ownership change.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: judgment.
- Stop initiating protected actions, make started work safe, and inspect
    both lease and resource state before resuming.
- Lease expiry alone does not prove resource safety.
- Reacquire before further protected work; revocation needs authority and safety.
- In an adjacent case with adequate native exclusivity, omit a redundant lease.

## 10 Keep scope selection deliberate

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
The user supplies an absolute store path and asks you to create an issue.
The store has two boards, no checkout binding, and no supplied issue or board ID.
Describe the next action.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: conformance.
- Inspect available boards and resolve the ambiguity with the user.
- Do not pick a convenient board, create another store or board, or persist a
    checkout binding without that setup being requested.

## 11 Retrieve a historical decision

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
The user asks why an earlier migration kept the old identifier format.
The store and board are supplied, but the issue ID is unknown.
The rationale may be in a closed issue's Log or Result.
Describe retrieval and what would support your answer.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: judgment.
- Search distinctive terms with historical record fields included.
- Inspect the underlying matching record before relying on its conclusion.
- Do not create a new issue or claim historical work merely to read it.

## 12 Preserve bytes separately from their meaning

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
An issue produced a comparison report that a later reviewer must inspect.
Its temporary worktree will be deleted; the report has no durable external URL
and cannot be reproduced from retained input.
Give the commands and accompanying issue note that preserve it.
The note should say that all compared account mappings matched.
Do not perform the proposed Cardamom or workload operations.

### Evaluation

- Mode: judgment.
- Attach the required bytes and record their meaning and reference in the issue.
- Retain the claim's scope; an attachment reference does not establish extra authority.
- Do not replace the conclusion with only a filename or attach unrelated scratch data.

## 13 Draft an uncertain investigation

### Prompt

Use the skill at `{SKILL_PATH}` as the only Cardamom guidance.
Create a concise investigation issue comparing two search ranking algorithms.
The held-out dataset and evaluation command are already documented at
`docs/ranking-evaluation.md`.
We need relevance and latency measurements before choosing either algorithm.
The acceptable latency tradeoff has not been decided.
This issue owns the comparison; no implementation is authorized yet.
Draft its records without changing Cardamom.

### Evaluation

- Mode: judgment.
- Preserve the question, alternatives, measurement reference, and open tradeoff.
- Do not invent a selection threshold, choose an algorithm, or authorize implementation.
- Accept a short coherent brief without a filled template or speculative plan.

## 14 Select the skill appropriately

### Prompt

Given the available-skill catalog, select guidance for each request:

1. "Use Cardamom to keep track of this repair."
2. "Why did we choose the old format? The decision is on our Cardamom board."
3. "Fix the parser bug." No tracker is mentioned or already in use.

### Evaluation

- Mode: conformance; provide the current description and plausible competing skills.
- Select Cardamom for the first two tasks.
- Do not activate it for the third solely because the task could be tracked.
- Withhold the skill path and body during selection.
