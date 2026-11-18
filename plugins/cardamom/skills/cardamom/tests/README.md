# Cardamom skill behavioral tests

These development-only scenarios test useful records, recovery, and tool choices.
The runtime skill does not load this directory.

For each case in [scenarios.md](scenarios.md), give a fresh runner only Prompt,
replacing `{SKILL_PATH}` with the selected skill directory.
Give Evaluation separately to the judge.
Permit read-only access to the selected guidance even when a scenario forbids
performing the proposed operations.
For selection cases, provide the catalog description rather than the skill body.

Use one guidance version per runner.
An uninstalled candidate beside a discoverable installed version is not isolated.
Disable competing discovery and automatic loading, or replace the active copy
in place while retaining the baseline outside discovery.
Inspect prompt context and file-access traces; keep the package fixed during
each trial.

Keep trials read-only or restricted to a disposable store.
Capture produced records, proposed or executed commands, selected references,
and available cost metrics.
Distinguish drafted actions from actual execution.
Use an independent judge for substantial artifacts and repeat variable cases.
Judge correctness and cost separately; a short incomplete record is not better.

For efficiency changes, compare the same tasks under both versions even when
both pass.
A claimed behavior repair needs an observed baseline failure and a passing
rerun of that case.
Report failures and limits rather than selecting favorable runs.

Follow `$writing-agent-guidance` for the shared evaluation method.
Use [command-probes.md](command-probes.md) to check CLI assumptions that affect
recording or recovery decisions.
