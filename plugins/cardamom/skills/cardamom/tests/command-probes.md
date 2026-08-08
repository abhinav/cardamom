# Cardamom disposable-store command probes

These probes validate command behavior used by the shipped workflows.
Build the branch binary to a temporary path rather than replacing an installed
binary.

## Isolate the store

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
  '.store.directory == $store and
   (.configuration.issue.id.prefix | type == "string" and length > 0)'
```

Every later command in this file inherits `CARDAMOM_STORE`.
Stop if `info` does not identify the temporary store.
Remove `PROBE_DIR` after recording the results.

## Commit State and post independent reasoning

```bash
issue="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" \
  create 'Repair parser behavior')"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" claim "$issue"

"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" state set "$issue" \
  'The parser regression is reproduced.' \
  --next 'Select a repair that preserves escape-state evidence.'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" state commit "$issue" \
  --set 'The reproduction is recorded; repair selection is in progress.' \
  --next 'Choose the repair and update the regression.'

"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" log post "$issue" - <<'LOG'
The scanner interface retains escape evidence.
Moving normalization into the token reader would erase evidence needed by
validation.
LOG

"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json state show "$issue" \
  | jq -e \
    '.body == "The reproduction is recorded; repair selection is in progress." and
     .next_action == "Choose the repair and update the regression."'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json log show "$issue" \
  | jq -s -e \
    'length == 2 and
     any(.[]; .kind == "state_snapshot" and
       .body == "The parser regression is reproduced.") and
     any(.[]; .kind == "post" and
       (.body | startswith("The scanner interface retains escape evidence")))'
```

Verify the committed action outcome entered Log,
the replacement State became active,
and separate design reasoning was posted without duplicating the snapshot.

## Complete and hand off

```bash
"$CARDAMOM_BIN" --actor worker-a result set "$issue" \
  'Implemented and validated parser escape-state preservation.'
"$CARDAMOM_BIN" --actor worker-a state set "$issue" \
  'Execution is complete; Result records the outcome and validation.' \
  --next 'Inspect Result and accept or return the issue.'
"$CARDAMOM_BIN" --actor worker-a --json release "$issue" \
  --waiting 'root acceptance' \
  | jq -e '.status == "waiting" and .active_claim == null'

"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json log show "$issue" --limit 1 \
  | jq -s -e \
    '.[0].kind == "state_snapshot" and
     .[0].body == "Execution is complete; Result records the outcome and validation." and
     .[0].next_action == "Inspect Result and accept or return the issue."'
"$CARDAMOM_BIN" --actor worker-b --json claim "$issue" --context \
  | jq -e '.issue.status == "in_progress" and
           (.issue | has("waiting") | not)'
```

Verify waiting release ends custody,
preserves changed State automatically,
and an explicit claim clears waiting.

## Clear State with and without history

```bash
temporary="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" \
  create 'Discard temporary diagnosis')"
"$CARDAMOM_BIN" --actor worker-a claim "$temporary"
"$CARDAMOM_BIN" --actor worker-a state set "$temporary" \
  'Temporary diagnosis that should not enter history.'
"$CARDAMOM_BIN" --actor worker-a state set "$temporary" ''
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json state show "$temporary" \
  | jq -e '.body == null'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json log show "$temporary" \
  | jq -s -e 'length == 0'

completed="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" \
  create 'Finish bounded phase')"
"$CARDAMOM_BIN" --actor worker-a claim "$completed"
"$CARDAMOM_BIN" --actor worker-a state set "$completed" \
  'The bounded phase is complete.'
"$CARDAMOM_BIN" --actor worker-a state commit "$completed" --set ''
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json state show "$completed" \
  | jq -e '.body == null'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json log show "$completed" \
  | jq -s -e \
    'length == 1 and .[0].kind == "state_snapshot" and
     .[0].body == "The bounded phase is complete."'
```

Verify empty `state set` discards temporary State without history,
while `state commit --set ''` snapshots and clears it.

## Progressive context

```bash
parent="$("$CARDAMOM_BIN" --actor coordinator create \
  --type workstream \
  --summary 'Children must preserve tenant isolation.' \
  --details 'The accepted cache rationale and examples live here.' \
  'Migrate cache')"
"$CARDAMOM_BIN" --actor coordinator state set "$parent" \
  'The migration contract is accepted.'
child="$("$CARDAMOM_BIN" --actor coordinator create \
  --parent "$parent" \
  --summary 'Implement tenant-first cache keys.' \
  --details 'Start with the key-construction regression.' \
  'Implement cache keys')"

context="$("$CARDAMOM_BIN" --actor worker-a --json show "$child" --context)"
printf '%s\n' "$context" | jq -e \
  --arg summary 'Children must preserve tenant isolation.' \
  '.context[0].summary == $summary and
   .context[0].details_bytes > 0 and
   (.context[0] | has("details") | not) and
   .context[0].state == "The migration contract is accepted." and
   .issue.details == "Start with the key-construction regression."'
"$CARDAMOM_BIN" --actor worker-a --json show "$parent" \
  | jq -e '.details == "The accepted cache rationale and examples live here."'
```

Verify ancestor Summary and State are inherited,
ancestor Details are represented by availability metadata,
current Details are included,
and ancestor Details can be retrieved on demand.

## Preserve literal record text

```bash
details=$(cat <<'DETAILS'
## Expanded contract

Preserve `$TARGET` and the literal command `$(date)`.
Keep `\n` as the documented escape spelling on this line.
DETAILS
)
literal="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create \
  --summary 'Preserve literal shell text.' \
  --details "$details" \
  'Literal text')"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json show "$literal" \
  | jq -e --arg expected "$details" \
    '.details == $expected and (.details | contains("\\n"))'
```

Verify the record contains real line breaks and literal Markdown and shell
metacharacters.

## Preserve and retrieve attachment bytes

```bash
artifact_issue="$("$CARDAMOM_BIN" --actor worker-a \
  create 'Preserve validation report')"
printf '%s\n' '{"tenant":"matched"}' >validation-report.json
attachment="$("$CARDAMOM_BIN" --actor worker-a --json attachment add \
  --issue "$artifact_issue" validation-report.json)"
attachment_id="$(printf '%s\n' "$attachment" | jq -r .id)"
"$CARDAMOM_BIN" --actor worker-a log post "$artifact_issue" \
  "Every row matched the expected tenant; report: %$attachment_id"
"$CARDAMOM_BIN" --actor worker-b --json attachment show "$attachment_id" \
  | jq -e --arg id "$attachment_id" '.id == $id'
"$CARDAMOM_BIN" --actor worker-b attachment get \
  "$attachment_id" recovered-report.json
cmp validation-report.json recovered-report.json
```

Verify attachment retrieval preserves bytes and the issue Log preserves their
meaning.

## Exercise a lease lifecycle

```bash
"$CARDAMOM_BIN" --actor worker-a lease acquire device-7 --ttl 30m
"$CARDAMOM_BIN" --actor worker-a --json lease show device-7 \
  | jq -e '.name == "device-7" and .owner == "worker-a"'
"$CARDAMOM_BIN" --actor worker-a lease renew device-7 --ttl 30m
"$CARDAMOM_BIN" --actor worker-a lease release device-7
! "$CARDAMOM_BIN" --actor worker-a --json lease show device-7
```

Verify one actor can acquire, renew, inspect, and release the resource lease.
