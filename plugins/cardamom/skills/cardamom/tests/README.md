# Cardamom skill behavioral tests

Run each scenario in `scenarios.md` with a fresh subagent that has an empty
context window.
For a trigger-selection scenario,
give the runner only the available-skill catalog and scenario prompt;
do not provide the target skill path or body.
For an application scenario,
prefix the runner input with a neutral instruction to read
`plugins/cardamom/skills/cardamom/SKILL.md` and the reference files that the
scenario exercises before answering.
Put that loading instruction and the unchanged scenario prompt in the same
runner turn.
Keep the prefix limited to file loading;
do not add the diagnosis, expected actions, or evaluator-only text.
Hide the expectations until after capturing the raw response.
For a staged scenario,
give the initial prompt and capture the response,
then give each stage separately in order.
Do not disclose later stages or evaluator-only text to the runner.

Tests must be read-only or confined to a task-local temporary directory
outside the target repository.
Do not let the tested subagent edit shared files, commit, publish,
or mutate external systems.

Compare the raw response with the expectations afterward.
Record the decision, skipped steps, command inventions, and rationalizations.
Retest a failed scenario after repairing the smallest skill boundary that
addresses the failure.

Before accepting a new behavior-shaping scenario,
run its prompt once without the skill or the target guidance.
If that control can satisfy the expectations by following cues in the prompt,
rewrite the scenario around operational facts and realistic pressure
without naming the diagnosis or desired decision.

Run the disposable-store checks in
[command-probes.md](command-probes.md)
when skill guidance depends on command behavior.
