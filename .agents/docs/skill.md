# Cardamom skill maintenance

Use this guide when changing `plugins/cardamom/skills/cardamom`.
Also load the `writing-and-updating-skills` skill before drafting a change.

## Purpose

The Cardamom skill teaches an agent how to coordinate work with `card`.
It is an operating procedure organized around agent tasks and decisions,
not product documentation or a catalog of Cardamom behavior.

Write requirements, examples, and persisted scenarios in terms of Cardamom
concepts and observable capabilities available to an arbitrary installation.
Use invented provider-neutral scenarios for behavioral tests.
Treat local tools, paths, infrastructure, and incidents as transient validation
inputs.
Integration-specific content belongs in the distributed skill when Cardamom
explicitly supports that integration.

Every passage in `SKILL.md` and `references/` must help an agent decide what to do,
which command or option to use,
what durable information to record,
or which workflow reference to load.
If removing a passage would not change an agent's action or decision,
remove it from the skill.

Keep complete product contracts in user documentation or CLI help.
Repeat only the behavior an agent needs to choose, sequence, record,
or verify a Cardamom workflow,
at the decision point where the behavior is needed.
Keep implementation contracts in the packages that own them.
Agents may assume documented Cardamom behavior works without the skill
restating parser, resolver, renderer, protocol, storage, or fallback behavior.

## Progressive disclosure

Keep `SKILL.md` small enough to load for every Cardamom task.
It should contain:

- the operating rules every coordinator and worker needs;
- the recurring decision points that protect durable work;
- compact command forms needed in most workflows; and
- a map that points to a reference at the moment it becomes useful.

Organize `references/` by agent workflow,
with information needed together in the same file.
A reference should answer a concrete need such as planning a graph,
executing and handing off work,
recovering an interrupted issue,
running a routine,
or preserving an attachment.
Do not organize references as exhaustive command or feature inventories.

Prefer one-level references loaded directly from `SKILL.md`.
Split a reference when an agent can complete one workflow
without loading the other material.
Merge or remove references that repeat the same decision boundary.

## Content test

Use this table when reviewing a proposed addition:

| Keep in the skill | Put elsewhere or omit |
| --- | --- |
| Commands and options needed to perform an agent workflow | Exhaustive command and option documentation |
| Criteria for choosing between supported agent actions | Product behavior that does not affect that choice |
| Required ordering, ownership, handoff, and recovery steps | Internal code paths and storage or protocol mechanics |
| Durable-record guidance needed by future workers | Historical decisions, rejected designs, and unsupported forms |
| A concise example that teaches a recurring decision | Multiple examples that only demonstrate syntax variants |

Describe supported actions positively in `SKILL.md` and `references/`.
Do not preserve a rejected proposal there as a warning or non-goal.
State a limitation only when the agent must account for that limitation
to complete the workflow correctly.

Keep CLI help useful for command discovery.
The skill may repeat a command form when the command is part of a workflow,
but it should not duplicate the command's full help text.

## Maintenance loop

Follow the workflow in the `writing-and-updating-skills` skill.
Start from an observed agent failure
or an established agent workflow requirement.
For a new requirement,
identify the user task and the unsupported agent decision or action
before drafting skill text.

Review the whole affected workflow,
not only the sentence that prompted the change.
Reject a patch that fixes the immediate wording
while leaving duplicated, contradictory, misplaced, or documentary prose.

Use behavioral scenarios to test agent choices and produced records.
Do not turn scenario files into inventories of product contracts.
Each scenario or command probe must identify the retained agent decision
or operational recipe it protects.
Scenarios may include unsupported choices when they are necessary
to pressure-test that decision.
Use a disposable store probe only when command behavior materially affects
the workflow being taught.
