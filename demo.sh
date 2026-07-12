#!/usr/bin/env bash
# demo.sh — exercise clu through a realistic workflow so you can eyeball
# speed and output.
#
# Usage:
#   ./demo.sh                 # uses ./clu (local build)
#   BD=/path/to/clu ./demo.sh # override binary path
#
# Runs entirely inside a temp directory; cleans up on exit.

set -euo pipefail

BD="${BD:-$(cd "$(dirname "$0")" && pwd)/clu}"
if [[ ! -x "$BD" ]]; then
    echo "error: clu binary not found at $BD — run 'go build -o clu ./cmd/clu' first" >&2
    exit 1
fi

WORK=$(mktemp -d -t clu-demo-XXXXXX)
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"

hr()    { printf '\n\033[1;36m═══ %s ═══\033[0m\n' "$*"; }
note()  { printf '\033[2m# %s\033[0m\n' "$*"; }
run()   { printf '\033[1m$ %s\033[0m\n' "$*"; "$@"; }
timed() { printf '\033[1m$ %s\033[0m\n' "$*"; time "$@"; }

hr "init a fresh database in $WORK"
run "$BD" init

# -----------------------------------------------------------------
hr "create 20 issues with varied attributes"
# Deterministic-ish randomness using $i so output is stable enough to eyeball.
TYPES=(task task task bug bug feature chore)
AGENTS=("" "" "" code-reviewer code-reviewer writer planner)
LABELS=("security" "perf" "ux" "tech-debt" "p0" "p1" "docs" "release")

IDS=()
for i in $(seq 1 20); do
    t=${TYPES[$(( i % ${#TYPES[@]} ))]}
    a=${AGENTS[$(( (i * 3) % ${#AGENTS[@]} ))]}
    p=$(( i % 5 ))
    args=(-p "$p" -t "$t")
    [[ -n "$a" ]] && args+=(-a "$a")
    id=$("$BD" create "${args[@]}" "issue #$i: $t in lane '${a:-default}'")
    IDS+=("$id")
    # ~half the issues get one or two labels
    if (( i % 2 == 0 )); then
        "$BD" label add "$id" "${LABELS[$(( i % ${#LABELS[@]} ))]}" >/dev/null
    fi
    if (( i % 3 == 0 )); then
        "$BD" label add "$id" "${LABELS[$(( (i * 5) % ${#LABELS[@]} ))]}" >/dev/null
    fi
done
note "created ${#IDS[@]} issues"

# -----------------------------------------------------------------
hr "wire dependencies (issues 6,7,8 depend on issue 1; issue 8 depends on 7)"
run "$BD" link "${IDS[5]}" "${IDS[0]}"
run "$BD" link "${IDS[6]}" "${IDS[0]}"
run "$BD" link "${IDS[7]}" "${IDS[6]}"

# -----------------------------------------------------------------
hr "comment, describe, note (issue 1)"
A="${IDS[0]}"
run "$BD" comment add "$A" "Repro on staging this morning." --agent alice
run "$BD" comment add "$A" "Looking into it — bug is in session.go around the cache evictor." --agent bob
run "$BD" describe "$A" "Cache eviction races with concurrent claim writes."
run "$BD" note set "$A" "Working theory: hold the mutex around cache lookup."
run "$BD" note append "$A" "After more digging: it's the WAL-mode interaction. Need to verify."

# -----------------------------------------------------------------
hr "list everything (default = open, default lane)"
timed "$BD" list

hr "list code-reviewer lane"
timed "$BD" list -a code-reviewer

hr "list with label filter (-l security)"
timed "$BD" list -l security

hr "list with label-pattern glob"
timed "$BD" list --label-pattern "tech-*"

hr "list sorted by title, reversed"
timed "$BD" list --sort title -r -n 5

hr "ready"
timed "$BD" ready

hr "blocked"
timed "$BD" blocked

hr "stats / count / info"
timed "$BD" stats
timed "$BD" count -t bug
timed "$BD" count --status all
timed "$BD" info

# -----------------------------------------------------------------
hr "show one issue in full"
timed "$BD" show "$A"

# -----------------------------------------------------------------
hr "claim, close, defer, reopen, priority, assign"
note "bare 'claim' picks from the default (unassigned) lane:"
run "$BD" claim
note "-a code-reviewer claims from the code-reviewer lane:"
run "$BD" claim -a code-reviewer
note "cancel cascades through dependents:"
run "$BD" cancel "${IDS[6]}"  # cancels IDS[6] + IDS[7] (which depends on it)
run "$BD" close "${IDS[2]}"
run "$BD" close "${IDS[3]}"
run "$BD" defer "${IDS[10]}" "+2h"
run "$BD" reopen "${IDS[2]}"
run "$BD" priority "${IDS[4]}" 0
run "$BD" assign "${IDS[4]}" alice

hr "list --status all (now mixed statuses)"
timed "$BD" list --status all -n 30

# -----------------------------------------------------------------
hr "delete forms — we don't have hard issue-delete; we have…"
note "close = logical delete (status goes to 'closed')"
note "label rm, dep rm, comment rm = remove individual relations"
run "$BD" label rm "${IDS[0]}" "security" 2>/dev/null || true

# Add a throwaway comment, then remove it by parsing its (#N) header.
NOTICE=$("$BD" comment add "$A" "ephemeral comment" --agent ghost)
echo "$NOTICE"
CID=$(echo "$NOTICE" | grep -oE '#[0-9]+' | tr -d '#' | head -1)
if [[ -n "$CID" ]]; then
    run "$BD" comment rm "$CID"
fi
run "$BD" dep rm "${IDS[5]}" "${IDS[0]}"

# -----------------------------------------------------------------
hr "doctor (integrity check)"
timed "$BD" doctor

hr "doctor (JSON)"
"$BD" --json doctor | sed 's/^/  /'

# -----------------------------------------------------------------
hr "export to JSONL and re-import into a fresh DB"
timed "$BD" export -o "$WORK/dump.jsonl"
note "$(wc -l < "$WORK/dump.jsonl" | tr -d ' ') lines in dump.jsonl"

mkdir -p "$WORK/clone"
cd "$WORK/clone"
"$BD" init >/dev/null
timed "$BD" import "$WORK/dump.jsonl"

ORIG=$("$BD" --dir "$WORK/.clu" count --status all)
COPY=$("$BD" count --status all)
note "original: $ORIG issues   clone: $COPY issues"

# -----------------------------------------------------------------
hr "done"
note "temp dir $WORK will be cleaned up on exit"
