#!/usr/bin/env bash
# demo-workflow.sh — exercise the workflow-template feature end-to-end.
#
# Usage:
#   ./demo-workflow.sh                 # uses ./cli (local build)
#   BD=/path/to/cli ./demo-workflow.sh # override binary path
#
# Runs entirely inside a temp directory; cleans up on exit.

set -euo pipefail

BD="${BD:-$(cd "$(dirname "$0")" && pwd)/cli}"
if [[ ! -x "$BD" ]]; then
    echo "error: cli binary not found at $BD — run 'go build -o cli ./cmd/cli' first" >&2
    exit 1
fi

WORK=$(mktemp -d -t cli-workflow-demo-XXXXXX)
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"

hr()    { printf '\n\033[1;36m═══ %s ═══\033[0m\n' "$*"; }
note()  { printf '\033[2m# %s\033[0m\n' "$*"; }
run()   { printf '\033[1m$ %s\033[0m\n' "$*"; "$@"; }
timed() { printf '\033[1m$ %s\033[0m\n' "$*"; time "$@"; }

hr "init a fresh database in $WORK"
run "$BD" init

# -----------------------------------------------------------------
hr "drop a release template into .db/templates/"
mkdir -p .db/templates
cat >.db/templates/release.yaml <<'YAML'
name: release
description: Standard build → test → deploy release
vars:
  version:
    required: true
    pattern: '^\d+\.\d+\.\d+$'
  channel:
    default: stable
steps:
  - id: build
    title: "Build {{version}}"
    priority: 1
  - id: test
    title: "Test {{version}} on {{channel}}"
    needs: [build]
  - id: deploy
    title: "Deploy {{version}}"
    needs: [test]
YAML
note "wrote .db/templates/release.yaml"

# A second template — fan-out + fan-in shape.
cat >.db/templates/parallel.yaml <<'YAML'
name: parallel
description: Two parallel builds, then a join
steps:
  - id: setup
    title: "Bootstrap workspace"
  - id: build-linux
    title: "Build linux"
    needs: [setup]
  - id: build-darwin
    title: "Build darwin"
    needs: [setup]
  - id: publish
    title: "Publish artefacts"
    needs: [build-linux, build-darwin]
YAML
note "wrote .db/templates/parallel.yaml"

# -----------------------------------------------------------------
hr "template ls / show / validate"
run "$BD" template ls
run "$BD" template validate release
run "$BD" template show release | sed 's/^/  /'

# -----------------------------------------------------------------
hr "validation rejects a broken template"
cat >.db/templates/broken.yaml <<'YAML'
name: broken
steps:
  - id: a
    title: t
    needs: [ghost]
YAML
note "'broken' references a missing step — expect a non-zero exit:"
if "$BD" template validate broken; then
    echo "FAIL: broken template passed validation" >&2; exit 1
fi
rm .db/templates/broken.yaml

# -----------------------------------------------------------------
hr "dry-run instantiates nothing"
run "$BD" run release --var version=1.2.3 --dry-run
note "no issues exist yet:"
run "$BD" count --status all

# Required var missing → error.
note "missing required var should fail:"
if "$BD" run release --dry-run; then
    echo "FAIL: missing required var accepted" >&2; exit 1
fi

# Bad var (regex) → error.
note "bad var should fail:"
if "$BD" run release --var version=notsemver --dry-run; then
    echo "FAIL: bad var accepted" >&2; exit 1
fi

# -----------------------------------------------------------------
hr "instantiate the release template for 1.2.3"
timed "$BD" run release --var version=1.2.3

hr "list — parent + 3 steps, all tagged with run:<parent> and step:<id>"
timed "$BD" list

# -----------------------------------------------------------------
hr "only the first step is ready"
timed "$BD" ready
note "the other two should be blocked:"
timed "$BD" blocked

# -----------------------------------------------------------------
hr "drive the workflow: claim & close build, then test"
BUILD_ID=$("$BD" --json ready | grep -oE 'bd-[a-f0-9]+' | head -1)
run "$BD" claim "$BUILD_ID" --as worker
run "$BD" close "$BUILD_ID"

note "after closing build, test should become ready:"
timed "$BD" ready
TEST_ID=$("$BD" --json ready | grep -oE 'bd-[a-f0-9]+' | head -1)
run "$BD" close "$TEST_ID"

note "after closing test, deploy should become ready:"
timed "$BD" ready

# -----------------------------------------------------------------
hr "instantiate the parallel template too"
run "$BD" run parallel

note "two parallel builds should both be ready (after setup closes):"
SETUP_ID=$("$BD" --json list | grep -oE '"id":"bd-[a-f0-9]+","title":"Bootstrap workspace"' | grep -oE 'bd-[a-f0-9]+' | head -1)
run "$BD" close "$SETUP_ID"
timed "$BD" ready

# -----------------------------------------------------------------
hr "checkpoints — explicit approval gates"
cat >.db/templates/staged.yaml <<YAML
name: staged
description: Release with a manual checkpoint and an approval checkpoint
vars:
  version:
    required: true
steps:
  - id: build
    title: "Build {{version}}"
  - id: confirm
    type: checkpoint
    title: "Confirm staging looks good"
    wait: { manual: true }
    needs: [build]
  - id: prod-approve
    type: checkpoint
    title: "Prod approval"
    wait: { approval: [$(id -un)] }
    needs: [confirm]
  - id: deploy
    title: "Deploy {{version}}"
    needs: [prod-approve]
YAML
note "approval gate uses the current user ($(id -un))"

run "$BD" run staged --var version=3.0.0

note "checkpoints show up with checkpoint:pending label:"
"$BD" list | grep checkpoint || true

CONFIRM_ID=$("$BD" --json list | python3 -c '
import sys,json
for i in json.load(sys.stdin):
  if "step:confirm" in i.get("labels",[]):
    print(i["id"]); break
')
APPROVE_ID=$("$BD" --json list | python3 -c '
import sys,json
for i in json.load(sys.stdin):
  if "step:prod-approve" in i.get("labels",[]):
    print(i["id"]); break
')
BUILD_ID2=$("$BD" --json list | python3 -c '
import sys,json
for i in json.load(sys.stdin):
  if "step:build" in i.get("labels",[]) and "step:build" in i.get("labels",[]) and "Build 3.0.0" in i.get("title",""):
    print(i["id"]); break
')

run "$BD" close "$BUILD_ID2"
note "manual checkpoint is now ready; clear it with approve:"
run "$BD" approve "$CONFIRM_ID"

note "wrong-user approval fails: (simulating another user via fake approver)"
cat >.db/templates/wrong.yaml <<'YAML'
name: wrong
steps:
  - id: gate
    type: checkpoint
    title: "Locked"
    wait: { approval: [somebody-else] }
YAML
run "$BD" run wrong
GATE_ID=$("$BD" --json list | python3 -c '
import sys,json
for i in json.load(sys.stdin):
  if "step:gate" in i.get("labels",[]):
    print(i["id"]); break
')
if "$BD" approve "$GATE_ID" 2>/dev/null; then
    echo "FAIL: approval should have been rejected" >&2; exit 1
fi
note "rejected (good)"

note "approving the real prod gate (current user is the approver):"
run "$BD" checkpoint pass "$APPROVE_ID" --reason "ship it"

note "deploy is now ready:"
"$BD" ready

note "fail also closes the checkpoint, with a different label:"
cat >.db/templates/cancel.yaml <<'YAML'
name: cancel
steps:
  - id: review
    type: checkpoint
    title: "Final review"
    wait: { manual: true }
YAML
run "$BD" run cancel
REVIEW_ID=$("$BD" --json list | python3 -c '
import sys,json
for i in json.load(sys.stdin):
  if "step:review" in i.get("labels",[]):
    print(i["id"]); break
')
run "$BD" checkpoint fail "$REVIEW_ID" --reason "scope creep"
note "show the failed checkpoint:"
"$BD" show "$REVIEW_ID" | sed 's/^/  /'

# -----------------------------------------------------------------
hr "json output for run"
"$BD" --json run release --var version=2.0.0 --dry-run | sed 's/^/  /'

# -----------------------------------------------------------------
hr "done"
note "temp dir $WORK will be cleaned up on exit"
