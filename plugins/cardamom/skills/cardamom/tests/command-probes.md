# Cardamom disposable-store command probes

These probes validate CLI assumptions used by skill decisions.
Build the candidate binary to a temporary path instead of replacing an
installed binary.

## Isolate the store

Protected decision:
every behavioral observation comes from the candidate binary and a disposable
store.

```bash
export CARDAMOM_BIN=/absolute/path/to/card
export CARDAMOM_ACTOR=skill-probe
export PROBE_DIR="$(mktemp -d)"
export CARDAMOM_STORE="$PROBE_DIR/store"
cd "$PROBE_DIR"

"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" \
  init --board-name 'Probe board'
info="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json info)"
printf '%s\n' "$info" | jq -e --arg store "$CARDAMOM_STORE" \
  '.store.directory == $store'
```

Stop if `info` does not identify the disposable store.
Every later command inherits `CARDAMOM_STORE`.
Remove `PROBE_DIR` after recording results.

## Publish, commit, and hand off

Protected decision:
`state commit` preserves a completed position while installing a new one,
and waiting release preserves the active State before direct recovery.

```bash
issue="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" \
  create 'Repair parser behavior')"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" claim "$issue"

"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" state set "$issue" \
  'The parser regression is reproduced.' \
  --next 'Select a repair that preserves escape-state evidence.'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" state commit "$issue" \
  --set 'The repair is complete; validation is active.' \
  --next 'Run parser validation.'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json state show "$issue" \
  | jq -e \
    '.body == "The repair is complete; validation is active." and
     .next_action == "Run parser validation."'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" log post "$issue" - <<'LOG'
The scanner retains escape evidence.
Token-reader normalization was rejected because it erases validation evidence.
LOG

"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" state set "$issue" \
  'Execution is paused; the next actor continues from this State.' \
  --next 'Claim this issue directly and resume execution.'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json release "$issue" \
  --waiting 'direct continuation' \
  | jq -e '.status == "waiting" and .active_claim == null'

"$CARDAMOM_BIN" --actor worker-b --json claim "$issue" --context \
  | jq -e '.issue.status == "in_progress" and
           .issue.state ==
             "Execution is paused; the next actor continues from this State." and
           (.issue | has("waiting") | not)'
"$CARDAMOM_BIN" --actor worker-b --json log show "$issue" \
  | jq -s -e \
    'any(.[]; .kind == "state_snapshot" and
      .body == "The parser regression is reproduced.") and
     any(.[]; .kind == "state_snapshot" and
      .body ==
        "Execution is paused; the next actor continues from this State.") and
     any(.[]; .kind == "post" and
      (.body | startswith("The scanner retains escape evidence")))'
```

Verify that the committed State and independent rationale remain distinct,
release ends custody,
and direct claim clears waiting without losing State.

## Discard or preserve displaced State

Protected decision:
temporary State may be cleared without history,
while replay-worthy State is committed before displacement.

```bash
temporary="$("$CARDAMOM_BIN" --actor worker-a \
  create 'Discard temporary diagnosis')"
"$CARDAMOM_BIN" --actor worker-a claim "$temporary"
"$CARDAMOM_BIN" --actor worker-a state set "$temporary" \
  'Temporary diagnosis that should not enter history.'
"$CARDAMOM_BIN" --actor worker-a state set "$temporary" ''
"$CARDAMOM_BIN" --actor worker-a --json state show "$temporary" \
  | jq -e '.body == null'
"$CARDAMOM_BIN" --actor worker-a --json log show "$temporary" \
  | jq -s -e 'length == 0'

completed="$("$CARDAMOM_BIN" --actor worker-a \
  create 'Finish bounded phase')"
"$CARDAMOM_BIN" --actor worker-a claim "$completed"
"$CARDAMOM_BIN" --actor worker-a state set "$completed" \
  'The bounded phase is complete.'
"$CARDAMOM_BIN" --actor worker-a state commit "$completed"
"$CARDAMOM_BIN" --actor worker-a --json state show "$completed" \
  | jq -e '.body == null'
"$CARDAMOM_BIN" --actor worker-a --json log show "$completed" \
  | jq -s -e \
    'length == 1 and .[0].kind == "state_snapshot" and
     .[0].body == "The bounded phase is complete."'
```

## Preserve literal record text

Protected decision:
single-quoted heredoc input preserves Markdown and shell metacharacters as
written.

```bash
details=$(cat <<'DETAILS'
## Expanded contract

Preserve `$TARGET` and the literal command `$(date)`.
Keep `\n` as the documented escape spelling on this line.
DETAILS
)
literal="$("$CARDAMOM_BIN" --actor worker-a create \
  --summary 'Preserve literal shell text.' \
  --details "$details" \
  'Literal text')"
"$CARDAMOM_BIN" --actor worker-a --json show "$literal" \
  | jq -e --arg expected "$details" \
    '.details == $expected and (.details | contains("\\n"))'
```

## Preserve and retrieve attachment bytes

Protected decision:
attachments preserve complete bytes while issue records preserve their
meaning.

```bash
artifact_issue="$("$CARDAMOM_BIN" --actor worker-a \
  create 'Preserve validation report')"
printf '%s\n' '{"tenant":"matched"}' >validation-report.json
attachment="$("$CARDAMOM_BIN" --actor worker-a --json attachment add \
  --issue "$artifact_issue" validation-report.json)"
attachment_id="$(printf '%s\n' "$attachment" | jq -r .id)"
"$CARDAMOM_BIN" --actor worker-a log post "$artifact_issue" \
  "Every row matched the expected tenant; report: %$attachment_id"
"$CARDAMOM_BIN" --actor worker-b attachment get \
  "$attachment_id" recovered-report.json
cmp validation-report.json recovered-report.json
```
