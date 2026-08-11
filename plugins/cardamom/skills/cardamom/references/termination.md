# Change issue lifecycle safely

Choose a terminal disposition from the issue contract and established outcome,
not from the convenience of ending work.
Terminal lifecycle operations preserve changed State automatically;
do not commit State solely to prepare for one.

## Close only satisfied work

Close when the recorded outcome and evidence satisfy the complete issue
contract.
Before closing a workstream or routine,
reconcile its direct children and confirm that each is closed or cancelled.
When the outcome was abandoned, superseded, or invalidated,
do not invent a successful Result or close it;
use cancellation instead.

The actor responsible for acceptance reads Result and material child outcomes
without claiming merely to inspect them.
Record the acceptance decision before closing:

```bash
card --actor <actor> --json result show <issue-id>
card --actor <actor> log post <issue-id> \
  'The recorded outcome satisfies the issue contract and required validation.'
card --actor <actor> close <issue-id>
```

When the contract permits self-acceptance,
the executor may perform this sequence while it still owns the claim.
When independent acceptance is required,
the executor first follows the waiting handoff in
[execution.md](execution.md),
and the acceptor performs this sequence without claiming.

## Cancel or deny only after impact review

Cancellation and checkpoint denial can alter other actors' readiness or
discard active work.
Establish authority, impact, and current custody before the command.

### Inspect transitive impact

For each requested root,
traverse dependency dependents rather than containment descendants.
Maintain a set of seen issue IDs so shared input does not cause repeated
review.
Inspect every non-terminal affected issue,
including claims, waiting reasons, State, and its relationship to the root.

Use a worklist:

1. Add every requested root.
2. Remove one unseen ID and inspect it with `show --json`.
3. Add each ID in its `blocks` field.
4. Repeat until the worklist has no unseen IDs.
5. Keep the non-terminal observed set as the cancellation review set.

`card cancel` cancels requested roots and all non-terminal transitive
dependents.
`checkpoint deny` denies the checkpoint and performs the same dependent
cancellation atomically.
Containment does not define this impact graph.

Before the terminal operation:

- confirm that the user or governing workflow authorizes the decision;
- stop or coordinate with active executors whose work would be invalidated;
- preserve any material result or partial position its readers still need;
- publish the rationale on the owning root or checkpoint; and
- report unresolved authority, custody, or impact instead of guessing.

Use the terminal command only after that review:

```bash
card --actor <actor> log post <issue-id> \
  'The prerequisite is invalid; dependent work is no longer actionable.'
card --actor <actor> cancel <issue-id>
```

For a checkpoint,
put the external authority's actual reason on the atomic denial:

```bash
card --actor <actor> checkpoint deny <checkpoint-id> \
  --reason 'The policy owner rejected the mapping and rollback plan.'
```

The command actor attributes the mutation;
it does not establish decision authority.

## Reopen without inventing recovery

`card reopen` makes a terminal issue open and unclaimed.
It does not reopen prerequisites, restore an old claim,
or establish the current execution position.

Before reopening,
record why the terminal decision no longer holds and inspect whether
prerequisites, descendants, contract, and evidence still apply.
After reopening,
repair Details and State as needed,
then let the intended executor claim through the ordinary recovery workflow.

```bash
card --actor <actor> log post <issue-id> \
  'New compatibility evidence invalidates the earlier completion decision.'
card --actor <actor> reopen <issue-id>
```
