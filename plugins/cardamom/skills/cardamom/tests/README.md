# Cardamom skill behavioral tests

These development-only tests evaluate the decisions produced by the shipped
skill.
They are not runtime guidance and must not be required by an installed worker.

## Run a scenario

Use a fresh subagent with an empty context window.
For an application scenario,
give the runner one neutral instruction to read the shipped `SKILL.md` and the
references it selects,
followed by the unchanged prompt.
Permit read-only shell commands for loading those files.
Require the runner to use only the supplied skill path and its routed
references as task guidance.
Do not let the runner load or apply another skill,
including another installed or cached copy of the Cardamom skill.
Do not provide expectations, diagnoses, preferred wording, or later stages.

For a staged scenario,
capture the raw response to each turn before sending the next stage.
For trigger selection,
provide only the available-skill catalog and prompt.

Keep evaluation read-only or confined to a task-local temporary directory.
The runner must not edit shared files,
run Cardamom against a persistent store,
mutate Git,
or write to external systems.

## Judge the behavior

Compare the raw response with the scenario's expected and unacceptable
behavior.
Judge observable operations, ordering, record contents, and recovery quality.
Accept equivalent methods permitted by the workflow;
do not reward section names or quotation of skill prose.

For the publication-timing repair,
preserve these distinctions:

- State and material reasoning are published before dependent work.
- Coherent phases are committed before another phase relies on them.
- A committed State snapshot and a standalone Log post are alternative history
  mechanisms when they would preserve the same information.
- Routine commands and incomplete mechanical work do not create chronology by
  themselves.
- Accepted evidence is reused unless current evidence makes it stale.
- Ordinary issue recovery may replay Log when history matters;
  routine awakenings normally recover from Details and current State.

When claiming improvement over the battle-tested baseline,
run the same prompt with the same runner conditions against both skill versions.
Preserve every raw result and use an independent judge when the outcome is
borderline.

Classify a failure as a selection gap,
a skill-model or routing gap,
a test leak,
or a capability limitation before changing the skill.
Repair the smallest owner and rerun the exposing scenario.

## Probe command contracts

Use [command-probes.md](command-probes.md) when a skill recipe depends on CLI
behavior.
Build the branch binary to a temporary path,
set an explicit temporary `CARDAMOM_STORE`,
and verify the store before any mutation.
