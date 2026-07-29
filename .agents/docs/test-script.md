# Test scripts

Use this guide when adding or changing a `.txt` scenario in
`testdata/script`.
These txtar scripts verify `card` through the process boundary.

## Scenario shape

Name scripts `<command>_<scenario>.txt`.
Use a name that identifies the user workflow or failure shape.
Keep setup, action, and assertions visible in the script.

The harness provides `card` as a command
and gives each script an isolated `$WORK` directory and `$HOME`.
Git system configuration is disabled,
and deterministic test author identity is configured.

Invoke external commands with `exec`.
Initialize a board explicitly when the scenario needs one:

```text
exec git init -b main
exec card init --prefix tst-
cmp stdout $WORK/golden/init.txt

-- golden/init.txt --
initialized .cardamom
```

## Script commands

Testscript commands are not shell syntax.
Redirection, pipes, command substitution, `cat`, and `ls` are not available.

Use testscript assertions instead:

- `stdout 'pattern'` and `stderr 'pattern'` match command output.
- `! stdout .` and `! stderr .` require an empty stream.
- `cmp actual expected` compares complete file contents.
- `cmpjson actual expected` compares complete decoded JSON values.
- `cmpenv actual expected` compares complete file contents after expanding
    environment variables in the expected file.
- `exists path` and `! exists path` check filesystem state.
- `env NAME=value` sets an environment variable.
- `cd path` changes the script working directory.
- A leading `!` requires the command to fail.

Check the stream that owns the contract.
Do not accept a diagnostic on standard output
or requested data on standard error merely because the text is present.

Use complete golden comparisons for stable core command output
and generated files whose complete contents are the contract.
Choose `cmp` when bytes are the contract,
`cmpjson` when the complete decoded JSON value is the contract,
and `cmpenv` when environment expansion makes otherwise stable bytes portable.
Empty-stream assertions and existence-only checks may remain.

Use a targeted assertion when the scenario protects one specific help clause,
one property of a large generated asset,
or output that includes unrelated external metadata.
Prefer a small complete derived value when it expresses the same contract
without hiding relevant output.
Do not add normalization or a test hook merely to eliminate a pattern match;
stabilize a value when that value is material to the protected behavior.

## Deterministic issue IDs

Use public board configuration to make issue IDs deterministic.
The default adaptive-random strategy is intentionally nondeterministic.
Give each scenario an isolated board and explicitly select the sequential
strategy before referring to expected public IDs:

```text
exec card init --prefix tst-
exec card config set --scope store issue.id.strategy sequential
exec card create 'Write guide'
cmp stdout $WORK/golden/create.txt

-- golden/create.txt --
tst-1
```

Use `card` commands for setup, state discovery, and assertions.
Do not add a testscript helper that resolves IDs
and do not query SQLite directly.

When a scenario exercises adaptive-random IDs,
assert the lower-case base32 shape through public `card` output.
Keep test determinism at the public process boundary.

## Structured output

Use a complete comparison for stable structured output contracts.
Store the expected output in a txtar supporting file
and compare the entire stream.
For a multi-record command, compare the JSON Lines bytes with `cmp`:

```text
exec card --json list --state open
cmp stdout $WORK/golden/open.jsonl

-- golden/open.jsonl --
{"id":"tst-1","title":"First","type":"task","status":"open"}
{"id":"tst-2","title":"Second","type":"task","status":"open"}
```

Use `cmp` when JSON formatting, JSON Lines record order, object member order,
or other output bytes are part of the contract.
Use `cmpjson` when the complete decoded value is the contract
for a single-record command,
but insignificant whitespace and object member order are not:

```text
exec card --json show tst-1
cmpjson stdout $WORK/golden/issue.json

-- golden/issue.json --
{"id":"tst-1","title":"Test","type":"task","status":"open","priority":2,"created":1783857600,"updated":1783857600,"revision":2,"labels":[],"depends_on":[],"blocks":[],"comments":[],"blocked":false,"parent_id":null}
```

`cmpjson` does not expand environment variables in either operand.
It compares JSON numbers by exact mathematical value,
so equivalent forms compare equal without rounding large integers.

Do not use `stdout`, `stderr`, or `grep` string matching to assert JSON output.
Make dynamic values deterministic at the test boundary,
then compare the complete structured stream with `cmp`
or a single JSON value with `cmpjson`.

## Supporting files

Define input and golden files after the command section with txtar markers:

```text
cmp actual.txt $WORK/golden/actual.txt

-- golden/actual.txt --
expected contents
```

Everything after a marker belongs to that file
until the next marker or end of script.
Keep golden output limited to behavior the scenario intends to protect.

Testscript `-update`, exposed by `mise run test:script --update`,
rewrites mismatched txtar supporting files used as the expected operand of
ordinary `cmp` assertions.
Review every rewritten fixture before accepting it.
The custom `cmpjson` command and testscript's `cmpenv` command do not support
`-update`, so update those expected fixtures manually.
Preserve environment-variable placeholders when manually updating a `cmpenv`
expectation.

## Running scenarios

Run one scenario while developing:

```bash
mise run test:script --run claim_labels
```

Rewrite ordinary `cmp` supporting files for one scenario:

```bash
mise run test:script --run claim_labels --update
```

This update mode does not rewrite `cmpenv` expectations;
update them manually.

Run the complete integration script suite before handoff:

```bash
mise run test:script
```
