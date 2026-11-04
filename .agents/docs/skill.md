# Cardamom skill maintenance

Use this guide when changing `plugins/cardamom/skills/cardamom`.
The skill helps agents preserve and retrieve useful work context through `card`.
Execution methods, delegation, validation, and update cadence come from the
user's assignment and its governing guidance.

## Teach useful recording decisions

Help a reader understand the goal, current situation, consequential choices,
and outcome without reconstructing a conversation.
Use concrete examples and criteria for what is worth retaining.
Scale record detail to the work; prefer a short brief when it is sufficient.
Keep product semantics that affect safe actions, such as claim ownership,
State replacement, cancellation impact, and recovery.

Every runtime passage should change an agent's useful action, record, or
retrieval decision.
Keep full product contracts and option inventories in user documentation or
CLI help, and implementation contracts in their owning packages.
Agents may trust documented tool behavior.

Write for arbitrary Cardamom installations.
Use invented provider-neutral examples; local infrastructure and incidents
belong in private validation evidence.
Include integration-specific content only for supported integrations.

## Keep ordinary work self-contained

`SKILL.md` should support reading, creating, updating, and finishing an ordinary
issue without loading a planning or execution manual.
Put substantial examples and optional coordination, recovery, automation,
and resource operations in references selected by the task.
Prefer one-level routes from the entry point.
Keep information needed together in one place and remove duplicated duties.

Use command examples for syntax and prose for the choice or consequence the
command cannot explain.
Describe supported behavior positively.
Keep limitations only when they affect the operation the agent is performing.

## Validate behavior and cost

Start from the user's requested outcome and the supported product behavior.
Read the complete affected guidance and references before evaluating a rewrite.
Review the cumulative result, including tests that could preserve an obsolete
workflow requirement.

Use fresh, isolated runners and hold evaluation criteria outside their prompts.
Judge useful records, recovery, user authority, and correct tool operations.
Allow alternative wording and structure that satisfy those outcomes.
Compare instruction cost and unnecessary bookkeeping alongside correctness.
Keep raw outputs and access traces outside runtime guidance.

Use disposable-store probes when a command assumption affects the instructions.
Leave persistent scenarios for reusable behavior boundaries in `tests/`;
the runtime skill must not depend on that directory.
