# Cardamom disposable-store command probes

Build the branch `card` binary to a task-local path that does not replace the
installed accepted binary,
then run the probes whose behavior supports a changed task recipe.
The branch binary must never discover or open the live coordination store.
Use a temporary directory with no existing store:

```bash
export CARDAMOM_BIN=/absolute/path/to/card
export CARDAMOM_ACTOR=skill-probe
export PROBE_DIR="$(mktemp -d)"
cd "$PROBE_DIR"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" init --board-name "Probe board"
info="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json info)"
printf '%s\n' "$info" | jq -e \
  '(.store.directory | endswith("/.cardamom")) and
   .configuration.issue.id.prefix == "cm-"'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json board list | jq -s .
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json board show
project="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json project create \
  --prefix probe-next- "Probe next" | jq -r .id)"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json project list | jq -s .
board="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json board create \
  --project "$project" "Explicit probe board" | jq -r .id)"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" board use "$board"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json board show
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" board use "Probe board"
```

Remove the directory after recording results.
When a schema-changing behavior needs realistic existing data,
use a disposable store copy and pass its path explicitly with `--store`.

## Routine claim and progressive context

```bash
routine="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create --type routine \
  --summary 'Audit each release against the compatibility policy.' \
  --details 'Inspect compatibility evidence for every release candidate.' \
  'Audit compatibility')"
initial_state=$(cat <<'STATE'
## Current targets

- release-held: retry after external validation finishes

## Safe cursor

release-0

## Retry state

Recheck release-held on the next wake.
STATE
)
initial_next='Assess release-held before processing input after release-0.'
"$CARDAMOM_BIN" --actor coordinator state set "$routine" "$initial_state" \
  --next "$initial_next"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json state show "$routine" \
  | jq -e --arg body "$initial_state" --arg next "$initial_next" \
    '.body == $body and .next_action == $next and
     (.body | test("(?m)^## (Current state|Next action)$") | not)'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json list --type routine | jq -s .
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json ready | jq -s .
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json blocked | jq -s .
! "$CARDAMOM_BIN" --actor worker-a claim
prerequisite="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create 'Routine prerequisite')"
blocked="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create --type routine --depends-on "$prerequisite" \
  'Blocked routine')"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json show "$blocked" --context
! "$CARDAMOM_BIN" --actor worker-a claim "$blocked"
"$CARDAMOM_BIN" --actor ordinary-worker --json claim
"$CARDAMOM_BIN" --actor ordinary-worker release "$prerequisite"
"$CARDAMOM_BIN" --actor worker-a --json claim "$routine" --context
run_state=$(cat <<'STATE'
## Current targets

- release-held: retry after external validation finishes

## Safe cursor

release-0

## Retry state

Recheck release-held on the next wake.

## Active run

Release 1 started at cursor 0.
Its scope is one release,
and its required evidence is the compatibility report.
STATE
)
"$CARDAMOM_BIN" --actor worker-a state set "$routine" "$run_state"
"$CARDAMOM_BIN" --actor worker-a --json state show "$routine" \
  | jq -e --arg body "$run_state" \
    '.body == $body and (. | has("next_action") | not)'
"$CARDAMOM_BIN" --actor worker-a state append "$routine" \
  'Run release-1 completed; release-held remains pending external validation.'
next_state=$(cat <<'STATE'
## Current targets

- release-held: retry after external validation finishes

## Safe cursor

release-1

## Retry state

Recheck release-held on the next wake.
STATE
)
next_action='Assess release-held before processing input after release-1.'
"$CARDAMOM_BIN" --actor worker-a state commit "$routine" \
  --set "$next_state" --next "$next_action"
"$CARDAMOM_BIN" --actor worker-a --json state show "$routine" \
  | jq -e --arg body "$next_state" --arg next "$next_action" \
    '.body == $body and .next_action == $next and
     (.body | test("(?m)^## (Current state|Next action)$") | not)'
"$CARDAMOM_BIN" --actor worker-a --json release "$routine"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json show "$routine" --context
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json log show "$routine" | jq -s .
"$CARDAMOM_BIN" --actor worker-b --json claim "$routine" --context
run_state=$(cat <<'STATE'
## Current targets

- release-held: retry after external validation finishes

## Safe cursor

release-1

## Retry state

Recheck release-held on the next wake.

## Active run

Release 2 started at cursor 1.
Its scope is one release,
and its required evidence is the compatibility report.
STATE
)
"$CARDAMOM_BIN" --actor worker-b state set "$routine" "$run_state"
"$CARDAMOM_BIN" --actor worker-b --json state show "$routine" \
  | jq -e --arg body "$run_state" \
    '.body == $body and (. | has("next_action") | not)'
"$CARDAMOM_BIN" --actor worker-b state append "$routine" \
  'Run release-2 completed; release-held remains pending external validation.'
next_state=$(cat <<'STATE'
## Current targets

- release-held: retry after external validation finishes

## Safe cursor

release-2

## Retry state

Recheck release-held on the next wake.
STATE
)
next_action='Assess release-held before processing input after release-2.'
"$CARDAMOM_BIN" --actor worker-b state commit "$routine" \
  --set "$next_state" --next "$next_action"
"$CARDAMOM_BIN" --actor worker-b release "$routine"
"$CARDAMOM_BIN" --actor coordinator log post "$routine" \
  'Retiring the routine because compatibility tracking ended.'
"$CARDAMOM_BIN" --actor coordinator close "$routine"
```

Verify the routine is absent from ordinary execution pools,
requires an explicit-ID claim,
and remains unclaimable while a dependency is open.
Verify the ordinary prerequisite remains available to an unqualified claim.
Verify current routine context does not embed log entry bodies.
Verify committed active-run snapshots remain available explicitly,
release automatically preserves changed next-run State,
release leaves lifecycle open,
the next run receives the promoted state,
each replacement preserves the unresolved target and retry state,
each awakening consumes its planned action through an explicit State
replacement,
the State body does not duplicate the separately stored next action,
and explicit retirement closes the routine without a result.

## Workstream routing and acceptance

```bash
root="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create \
  --type workstream 'Root workstream')"
workstream="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create \
  --type workstream --parent "$root" 'Nested workstream')"
task="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create \
  --label implementation --parent "$workstream" 'Task')"
outside="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create --label implementation 'Outside')"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json list --under "$root" --status ready | jq -s .
"$CARDAMOM_BIN" --actor worker-a --json claim \
  --under "$root" --label implementation --context
"$CARDAMOM_BIN" --actor worker-a result set "$task" 'Validated outcome.'
"$CARDAMOM_BIN" --actor worker-a release "$task" \
  --waiting 'root acceptance'
"$CARDAMOM_BIN" --actor coordinator --json result show "$task" \
  | jq -e '.body == "Validated outcome."'
"$CARDAMOM_BIN" --actor coordinator log post "$task" \
  'Accepted the validated task outcome.'
"$CARDAMOM_BIN" --actor coordinator close "$task"
"$CARDAMOM_BIN" --actor coordinator log post "$workstream" \
  'Accepted the nested workstream after its task closed.'
"$CARDAMOM_BIN" --actor coordinator close "$workstream"
"$CARDAMOM_BIN" --actor coordinator log post "$root" \
  'Accepted after inspecting component evidence.'
"$CARDAMOM_BIN" --actor coordinator close "$root"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json show "$outside"
```

Verify descendant selection excludes the root workstream and outside task,
the label route claims the nested task,
each workstream remains open until explicitly closed,
and closing a workstream permits terminal direct children.

## Public apply document

```bash
cat >graph.json <<'JSON'
{
  "version": 1,
  "on_existing": "update",
  "issues": [
    {
      "alias": "release",
      "key": "probe:release",
      "title": "Prepare release",
      "type": "workstream"
    },
    {
      "alias": "linux",
      "key": "probe:linux",
      "title": "Validate Linux",
      "type": "task",
      "parent": {"alias": "release"},
      "labels": {"values": ["validation", "platform:linux"]}
    },
    {
      "alias": "gate",
      "key": "probe:gate",
      "title": "Approve release",
      "type": "checkpoint",
      "parent": {"alias": "release"},
      "depends_on": {"values": [{"alias": "linux"}]}
    }
  ]
}
JSON

"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json apply --dry-run graph.json \
  | jq -e \
    '.dry_run and .counts.create == 3 and all(.entries[]; .action == "create")'
receipt="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json apply graph.json)"
printf '%s\n' "$receipt" \
  | jq -e \
    '.counts.create == 3 and (.entries | length) == 3 and
     all(.entries[]; .action == "create")'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json apply graph.json \
  | jq -e \
    '.counts.no_change == 3 and all(.entries[]; .action == "no_change")'

cat >retired-group.json <<'JSON'
{
  "version": 1,
  "issues": [
    {
      "alias": "retired",
      "title": "Retired grouping",
      "type": "task",
      "group": "release"
    }
  ]
}
JSON

! "$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" apply retired-group.json
```

Verify dry-run does not allocate durable identities,
the commit returns deterministic entry receipts with readable action values,
stable producer keys reconcile without changes,
typed references establish containment and readiness,
and strict JSON rejects the retired grouping field.

## Waiting release and resume

```bash
waiting="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create \
  'Validate signing service')"
"$CARDAMOM_BIN" --actor worker-a claim "$waiting"
"$CARDAMOM_BIN" --actor worker-a state set "$waiting" \
  'Implementation is partial.' \
  --next 'Resume after signing service recovery.'
"$CARDAMOM_BIN" --actor worker-a --json release "$waiting" \
  --waiting 'signing service is restored' \
  | jq -e '.status == "waiting" and .active_claim == null'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json log show "$waiting" --limit 1 \
  | jq -s -e \
    '.[0].kind == "state_snapshot" and
     .[0].body == "Implementation is partial." and
     .[0].next_action == "Resume after signing service recovery."'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json ready \
  | jq -s -e --arg id "$waiting" 'all(.id != $id)'
"$CARDAMOM_BIN" --actor worker-b --json claim "$waiting" --context \
  | jq -e '.issue.status == "in_progress" and (.issue | has("waiting") | not)'
```

Verify waiting release requires and preserves a plain-text trigger,
ends custody,
removes the issue from automatic claim pools,
preserves changed State as a snapshot without an explicit State commit,
and an explicit claim of the same issue clears waiting status.

## Checkpoint graph

```bash
checkpoint="$(
  "$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create --type checkpoint \
    'Approve retry release'
)"
implementation="$(
  "$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create 'Implement retry'
)"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" edit "$implementation" \
  --depends-on "$checkpoint"
decision="$("$CARDAMOM_BIN" --actor decision-recorder --json \
  checkpoint approve "$checkpoint" \
  --reason 'External authority accepted the recorded evidence.')"
printf '%s\n' "$decision" | jq -e \
  '.decision.outcome == "approved" and
   (.decision | has("reason") and has("decided_at") and has("revision")) and
   (.decision | has("actor") | not)'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json ready | jq -s .
```

Verify the checkpoint requires explicit resolution before implementation is
ready and approving it makes the dependent ready.
Verify the invocation still has an explicit actor while the persisted decision
contains no approver identity.

## Context and durable records

```bash
summary=$(
  cat <<'SUMMARY'
Preserve literal shell text while implementing the command contract.
SUMMARY
)
details=$(
  cat <<'DETAILS'
## Expanded contract

Preserve `$TARGET` and the literal command `$(date)`.
Keep `\n` as the documented escape spelling on this line.
DETAILS
)
issue="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create \
  --summary "$summary" --details "$details" 'Literal text')"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json show "$issue" \
  | jq -e --arg expected "$details" \
    '.details == $expected and (.details | contains("\\n"))'
"$CARDAMOM_BIN" --actor worker-a --json claim "$issue" --context
"$CARDAMOM_BIN" --actor worker-a state append "$issue" \
  'Active intent: verify that durable records retain literal shell text.'
"$CARDAMOM_BIN" --actor worker-a state set "$issue" \
  '`ship inspect $TARGET` retains `$(date)`.' \
  --next 'Validate output.'
"$CARDAMOM_BIN" --actor worker-a state commit "$issue"
"$CARDAMOM_BIN" --actor worker-a log post "$issue" \
  'No escaping workaround was required.'
"$CARDAMOM_BIN" --actor worker-a result set "$issue" \
  'Implemented `ship inspect $TARGET`.'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json log show "$issue" | jq -s .
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json state show "$issue"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json result show "$issue"
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json show "$issue" --context
```

Verify details contain the heredoc's real line breaks and intentional `\n`,
Markdown metacharacters remain literal,
the Log retains the committed State snapshot and distinct attributed post,
and the current issue result is available from `result show` rather than the
current issue section of `show --context`.

## Intentional unsnapshotted State removal

```bash
cleared="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create \
  'Remove temporary diagnosis')"
"$CARDAMOM_BIN" --actor worker-a state set "$cleared" \
  'Temporary diagnosis that should not enter history.'
"$CARDAMOM_BIN" --actor worker-a state set "$cleared" ''
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json state show "$cleared" \
  | jq -e '.body == null'
"$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" --json log show "$cleared" \
  | jq -s -e 'length == 0'
```

Verify an explicitly empty `state set` removes unsnapshotted State without
adding a Log entry.

## Summary and Details progressive disclosure

```bash
parent="$("$CARDAMOM_BIN" --actor coordinator create --type workstream \
  --summary 'Children must preserve the public parser contract.' \
  --details 'The parser rationale and rejected alternatives live here.' \
  'Parser program')"
child="$("$CARDAMOM_BIN" --actor coordinator create --parent "$parent" \
  --summary 'Implement the accepted parser boundary.' \
  --details 'Start with TestParse/QuotedInput.' \
  'Implement parser')"
context="$("$CARDAMOM_BIN" --actor worker-a --json show "$child" --context)"
printf '%s\n' "$context" | jq -e \
  --arg summary 'Children must preserve the public parser contract.' \
  '.context[0].summary == $summary and .context[0].details_bytes > 0 and
   (.context[0] | has("details") | not)'
printf '%s\n' "$context" | jq -e \
  '.issue.summary != null and .issue.details != null'
"$CARDAMOM_BIN" --actor worker-a --json show "$parent" \
  | jq -e '.details == "The parser rationale and rejected alternatives live here."'

oversized_summary="$(printf '%02049d' 0)"
! "$CARDAMOM_BIN" --actor coordinator create \
  --summary "$oversized_summary" 'Oversized summary'
"$CARDAMOM_BIN" --actor coordinator create \
  --summary 'Expanded material remains available on demand.' \
  --details "$oversized_summary" 'Large details'
```

Verify ancestor summaries are inherited,
ancestor details are represented only by availability metadata,
and the current issue includes its own summary and details.
Verify `show` retrieves ancestor details on demand,
the default summary limit rejects oversized summaries,
and the same stable expanded material is accepted as details.

## Recovery continues the existing issue

```bash
issue="$("$CARDAMOM_BIN" --actor "$CARDAMOM_ACTOR" create --type workstream 'Recover parser work')"
"$CARDAMOM_BIN" --actor worker-a claim "$issue"
"$CARDAMOM_BIN" --actor worker-a state set "$issue" \
  'A failing parser test exists.' \
  --next 'Implement the parser fix.'
"$CARDAMOM_BIN" --actor worker-a state commit "$issue"
"$CARDAMOM_BIN" --actor coordinator log post "$issue" \
  'Root ended worker-a assignment and reassigned this issue for recovery.'
"$CARDAMOM_BIN" --actor worker-a release "$issue"
"$CARDAMOM_BIN" --actor worker-b --json show "$issue" --context
"$CARDAMOM_BIN" --actor worker-b --json log show "$issue" | jq -s .
"$CARDAMOM_BIN" --actor worker-b --json log show "$issue" --oldest-first \
  | jq -s .
"$CARDAMOM_BIN" --actor worker-b --json claim "$issue" --context
```

Verify release ends the first actor's custody without closing the issue,
the second actor can reconstruct state from the same issue,
newest-first Log inspection is the default,
chronological replay requires `--oldest-first`,
and the second claim continues that issue rather than allocating a replacement.

## Durable attachment references and recovery

```bash
origin="$("$CARDAMOM_BIN" --actor worker-a create 'Produce route audit')"
consumer="$("$CARDAMOM_BIN" --actor coordinator create 'Accept route audit')"
printf '%s\n' '{"route":"stable"}' >route-audit.ndjson
added="$("$CARDAMOM_BIN" --actor worker-a --json attachment add \
  --issue "$origin" route-audit.ndjson)"
attachment_id="$(printf '%s\n' "$added" | jq -r .id)"
attachment_ref="%$attachment_id"
log_entry="$("$CARDAMOM_BIN" --actor worker-a --json log post "$origin" \
  'The route audit established stable ordering.')"
log_id="$(printf '%s\n' "$log_entry" | jq -r .id)"
log_ref="%$log_id"
"$CARDAMOM_BIN" --actor worker-a result set "$origin" \
  "Stable ordering is required. Supporting chronology: $log_ref. Report: $attachment_ref"
"$CARDAMOM_BIN" --actor coordinator state set "$consumer" \
  "Review evidence from %$origin: $attachment_ref"
"$CARDAMOM_BIN" --actor recovery-worker --json attachment show "$attachment_id"
"$CARDAMOM_BIN" --actor recovery-worker --json attachment list \
  --issue "$origin" | jq -e --arg id "$attachment_id" \
  '.attachments | any(.id == $id)'
"$CARDAMOM_BIN" --actor recovery-worker --json state show "$consumer" \
  | jq -e --arg issue "%$origin" --arg ref "$attachment_ref" \
  '.body | contains($issue) and contains($ref)'
"$CARDAMOM_BIN" --actor recovery-worker --json result show "$origin" \
  | jq -e --arg log "$log_ref" --arg ref "$attachment_ref" \
  '.body | contains($log) and contains($ref)'
"$CARDAMOM_BIN" --actor recovery-worker --json log show "$origin" \
  | jq -s -e --arg id "$log_id" 'any(.id == $id)'
"$CARDAMOM_BIN" --actor recovery-worker attachment get \
  "$attachment_id" recovered-route-audit.ndjson
cmp route-audit.ndjson recovered-route-audit.ndjson
! "$CARDAMOM_BIN" --actor recovery-worker attachment get \
  "$attachment_id" recovered-route-audit.ndjson
```

Verify the attachment-add and log-post results supply the IDs needed to author
their reference forms,
issue, log, and stored-filename attachment references survive record storage,
the originating issue association supports filtered discovery,
the same reference remains valid in another issue on the board,
and recovery verifies bytes while preserving an existing destination.

## Mail and lease ownership

```bash
"$CARDAMOM_BIN" --actor scheduler mail send coordinator 'Resource check is ready.'
"$CARDAMOM_BIN" --actor coordinator mail recv
"$CARDAMOM_BIN" --actor coordinator mail send worker-a 'Resource check assigned.'
"$CARDAMOM_BIN" --actor worker-a mail recv
"$CARDAMOM_BIN" --actor worker-a mail recv
"$CARDAMOM_BIN" --actor worker-a lease acquire staging-db --ttl 30m
! "$CARDAMOM_BIN" --actor worker-b lease renew staging-db --ttl 30m
! "$CARDAMOM_BIN" --actor worker-b lease release staging-db
"$CARDAMOM_BIN" --actor worker-a lease renew staging-db --ttl 30m
"$CARDAMOM_BIN" --actor worker-a lease release staging-db
"$CARDAMOM_BIN" --actor worker-a lease acquire device-slot --ttl 30m
! "$CARDAMOM_BIN" --actor coordinator lease revoke device-slot \
  --owner worker-b --reason 'worker-b cannot continue'
"$CARDAMOM_BIN" --actor coordinator --json lease show device-slot \
  | jq -e '.owner == "worker-a"'
revocation="$("$CARDAMOM_BIN" --actor coordinator --json lease revoke \
  device-slot --owner worker-a --reason 'worker-a cannot continue')"
printf '%s\n' "$revocation" | jq -e \
  '.lease.name == "device-slot" and
   .lease.owner == "worker-a" and
   .revoked_by == "coordinator" and
   .reason == "worker-a cannot continue" and
   (.revoked_at | type == "string")'
! "$CARDAMOM_BIN" --actor coordinator lease show device-slot
```

Verify the wake is consumed after one receive
and only the active lease owner can renew or release the resource.
Verify a revocation owner mismatch preserves the active lease,
while a matching revocation removes it and returns the complete operation
context.
