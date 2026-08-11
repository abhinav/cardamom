# Cardamom skill behavioral tests

These development-only scenarios test decisions produced by a candidate skill.
Installed workers must not depend on this directory.

## Run a scenario

Use a fresh subagent with an empty context window.
Replace `{SKILL_PATH}` with the candidate skill directory.
Give an application runner only the scenario's Prompt and this neutral
instruction:

> Use the skill at `{SKILL_PATH}` as the only Cardamom task guidance.
> Read its `SKILL.md` and only the references it directs you to.

For trigger selection,
give only the persisted available-skill catalog and Prompt.
Do not reveal the target path or body.

Withhold expected behavior, unacceptable behavior, diagnoses,
and later stages.
Keep trials read-only or isolated in a task-local temporary store.
The runner must not edit the candidate, mutate shared Git state,
or contact external systems.
A Prompt instruction not to execute commands forbids the described Cardamom
and primary work;
it does not forbid read-only access needed to load the candidate skill and its
routed references.

For a named pressure or adjacent case,
append only its Runner prompt addition to the original Prompt.
Judge it against the Expected and Unacceptable behavior nested under that
variant.
Every variant uses this structure:

### <Variant kind>: <Variant name>

#### Runner prompt addition

#### Expected behavior

#### Unacceptable behavior

## Establish Red, then Green

Run the exposing scenario against the current skill before a behavior repair.
A Red baseline must exhibit the behavior the repair should prevent.
If it passes,
strengthen the pressure without leaking the expected answer or report a
reproduction gap.

Capture raw output, proposed operations, event order, and rationalizations.
Classify the owner as skill guidance, routing, test or evaluator,
application support, capability, or authority before editing guidance.

Rerun the exact Red scenario against the candidate,
then run its pressure and adjacent-valid cases.
Repeat important or borderline cases two or three times with fresh agents.

## Judge behavior

Judge observable choices, ordering, record contents, and recovery quality.
Accept equivalent operations permitted by the workflow;
do not reward section names or quotation of guidance.

For publication ordering,
require an exact ordered action sequence or retain the raw transcript from a
task-local trial.
A final explanation without the intervening actions does not establish order.
For substantial record-writing artifacts,
use a separate fresh judge with the source input,
raw artifact, and held-out expectations.

A case passes only when every expected behavior holds and no unacceptable
behavior appears.

## Probe command assumptions

Use [command-probes.md](command-probes.md) when guidance depends on CLI
behavior.
Build the candidate binary to a temporary path,
use an explicit temporary `CARDAMOM_STORE`,
and verify the store before mutation.
Every probe names the decision its observation protects.

The reusable gamut is [scenarios.md](scenarios.md).
