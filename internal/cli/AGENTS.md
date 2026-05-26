# Using `clu` as a Claude Code agent

`clu` is the local SQLite-backed issue tracker this project uses to
coordinate work between humans and one-or-more Claude Code sessions.
Issues live in `.clu/data.sqlite`; the file is local-only and gitignored.
Configuration (agent registry, ID prefix) lives in `.clu/config.yaml` and
*is* committed.

If you're a Claude Code session reading this: you are an agent. The rest
of this file is how to be a useful one.

## The 30-second loop

```bash
# 1. Pick up the next ready task, identifying as the agent named in config.yaml.
clu claim --agent <your-name>

# 2. Do the work. Leave a note as you go if it's non-trivial.
clu note append <id> "tried X, hit Y, working around with Z"

# 3. Close on success — unblocks downstream tasks.
clu close <id>

# OR: cancel on abandonment — leaves downstream blocked unless they're
# also cancelled (use `clu cancel` for the cascade).
clu cancel <id>
```

That's the whole core loop. Everything below is detail.

## Identifying yourself

There's exactly one identity flag everywhere: `-a` / `--agent`. It
represents *you*, whether you're the one who'll pick the work up or the
one who already has it.

What the flag does on each command:

- `create -a X` — pre-assign the new issue to X (status stays `open`
  until someone claims it).
- `list -a X` / `count -a X` / `blocked -a X` — **show me X's work**:
  exact match on `assignee = X`.
- `ready -a X` — what X could pick up next: X's pre-assigned work
  *plus* the shared unassigned pool (open + deps closed + not
  deferred). Pre-assigned sorts ahead of the pool.
- `claim -a X` — atomically take the next ready issue in X's lane and
  record X as the assignee. Pre-assigned wins over the shared pool.
- `comment add -a X` / `approve -a X` / `checkpoint pass -a X` —
  record X as the author/approver.

Bare `clu claim` (no `-a`) means "I am `$USER`, pulling from the shared
unassigned pool." Use that if you're operating ad-hoc; use
`-a <agent-name>` if you're a declared agent in `.clu/config.yaml`.

> **One column.** As of schema v13 there's a single `assignee` column;
> the historical `agent` lane was collapsed in. "Lane" is now just a
> synonym for "issues whose assignee matches you, plus the unassigned
> pool." `create -a X` sets assignee directly — same as
> `clu assign <id> X` on an existing issue.

To see who's declared:

```bash
clu agent ls               # declared agents + who's currently live
clu agent show <name>      # one agent's capabilities + pending work
```

## Finding work

```bash
clu ready                        # unblocked, from the unassigned pool
clu ready -a <your-name>         # unblocked, in your lane (pre-assigned + pool)
clu ready --json                 # machine-parseable; one JSON value
clu list --status all -n 50      # broader view
clu blocked                      # what's waiting on something else
```

`ready` is the canonical "what should I do next" query. It returns only
issues whose dependencies are all `closed`, that aren't deferred to the
future, and that match your lane.

Lane matching (the matrix that trips people up):

| your `-a` | result |
|---|---|
| (none) | shared pool: `assignee IS NULL` AND no `cap:*` label |
| `foo` | `assignee = 'foo'` OR (`assignee IS NULL` AND optional `cap:` match) |

Capabilities are declared in `config.yaml` and surface as `cap:<name>`
labels on issues. A code-reviewer with `capabilities: [go-review]` will
see shared-pool issues tagged `cap:go-review` in addition to its
pre-assigned work.

## Claiming, atomically

```bash
clu claim                            # next ready in default lane
clu claim -a <your-name>             # next ready in your lane
clu claim <specific-id>              # claim a specific issue
clu claim --wait                     # block until something is claimable
clu claim --wait --heartbeat -a X    # also publish heartbeat so coordinators can see you
```

`claim` does `UPDATE … RETURNING` atomically — two sessions racing each
other will get different issues, not the same one. If nothing's ready,
exit is nonzero and stderr says "no ready issues".

`--heartbeat` is **opt-in**: only pass it when you want to advertise
liveness via `clu agent ls`. The bare `--wait` loop does not heartbeat by
default.

> **Don't run `clu show` after `claim`.** `claim` already prints the
> full issue — title, status, description, notes, dependencies, and
> the full comment thread. Same output as `show`. Reading both is just
> a redundant DB round-trip. In `--json` mode you get the whole issue
> object back too. One command, all the context you need to start
> working.

## Watching for incoming work (Claude Code Monitor tool)

**Important: clu already implements change-detected watching. Do NOT
write a shell polling loop. Just run one of these in Monitor:**

```
Monitor: clu ready --watch -a <your-name>
```

```
Monitor: clu list --watch -a <your-name>
```

That's it. No `while true`, no `jq`, no `comm`/`diff`, no `seen=""`
tracking. The `--watch` flag is the API: clu polls on its own
interval, suppresses unchanged ticks, and emits each new state as a
clean block separated by a blank line. Monitor turns each emitted
block into one push notification — silent ticks don't wake you up.

### Which one to use

| command | shows |
|---|---|
| `clu ready --watch -a X` | only **unblocked** work in X's lane (pre-assigned + shared pool) — what X can actually start on right now |
| `clu list --watch -a X` | every open issue assigned to X (claimed, blocked, deferred, all of it) — useful for situational awareness |

For "wake me up when there's something I can pick up," use **`ready
--watch`**. For "show me everything happening in my area," use **`list
--watch`**.

### The react-loop

When a notification arrives:

1. Read the emitted block (it's already there in the notification),
   or call `clu --json ready -a <you>` if you want structured data.
2. `clu claim -a <you>` to atomically take the top one.
3. Do the work.
4. `clu close <id>` on success or `clu cancel <id>` to abandon.
5. The next Monitor notification will arrive whenever the ready set
   changes again.

### Common flags worth knowing

- `--interval 2s` — adjust how often clu polls internally (default 1s).
  Don't go shorter than ~500ms; you'll just churn the DB.
- `--heartbeat` — while watching, register `-a <name>` in
  `clu agent ls` so coordinators see you're online. Opt-in.
- `--json` is **not** supported with `--watch` (one JSON document per
  process; `--watch` is a stream). Use `clu --json ready` from within
  the react-loop if you need structured data.

### Anti-patterns (do not do this)

```bash
# DON'T. clu --watch already does change detection. This re-invents
# it badly, sleeps blindly, hides errors, and races against itself.
seen=""
while true; do
  cur=$(clu --json ready -a bug-fixer | jq -r '.[].id' | sort)
  new=$(comm -13 <(echo "$seen") <(echo "$cur"))
  …
done
```

Just: `Monitor: clu ready --watch -a bug-fixer`.

### Watch vs. wait vs. plain

Three modes, pick the right one:

| flag | semantics | when to use |
|---|---|---|
| (none) | one-shot, exit immediately | scripts, one-off "is anything ready?" |
| `--wait` | block until ≥1 issue is ready, print it once, exit | "I want to do exactly one task and stop" |
| `--watch` | continuous; emit each new state, never exits voluntarily | Monitor / long-running agent |

`--wait` and `--watch` are mutually exclusive.

## Creating work

```bash
clu create "title"                              # default lane, type=task, priority=2
clu create -p 1 -t bug "title"                  # higher priority, type=bug
clu create -a code-reviewer "title"             # route to a specific lane
clu create --capability go-review "title"       # add cap:go-review for routing
clu create -d clu-a3f8 "title"                  # new issue depends on clu-a3f8
clu create -d clu-a3f8,clu-7c11 "title"         # multiple parents
clu create --description "long-form context" --notes "working theory: X" "title"
clu link <child> <parent>                       # add a dep after the fact
clu describe <id> "longer description"          # set/replace description after the fact
clu note set <id> "freeform working notes"      # ditto for notes
clu note append <id> "addendum"
```

Returned ID goes to stdout (one per call). In `--json` mode, the whole
issue is returned.

> **One-shot create is the right pattern.** Don't `create` then
> `describe`, `link`, or `note set` as separate commands. Pass
> everything you have to `create` in one call:
>
> ```bash
> clu create \
>   -a notion-manager \
>   -p 2 \
>   -d clu-a3f8 \
>   --description "$(cat <<'EOF'
> Test scenario page: https://...
> Walk every step in the scenario, capture drift, …
> EOF
> )" \
>   "Review test scenario: First-Time Buyer (No Saved Card)"
> ```
>
> Why it matters: the two-step pattern leaves a window where the
> new issue exists open + parent-less + empty, and a watching
> `claim --watch` could grab it before the second command lands.
> One-shot create wraps everything in a single transaction — the
> issue is never visible without its edges and body.

## Dependencies + cascade

Edges go *child → parent*: "I need this done first."

- `clu link A B` — A depends on B. B must close before A becomes ready.
- `clu dep rm A B` — remove the edge.
- `clu blocked` — list issues waiting on at least one non-closed parent.

Closing a parent unblocks its children. Cancelling a parent does NOT
unblock children — that's the whole point. Use `clu cancel <id>` to
cascade cancel through descendants.

```bash
# A ← B ← C, and A ← D
clu cancel A         # cancels A, B, C, D (full cascade)
```

`closed` and `cancelled` are both terminal but mean different things:

| status | meaning | downstream |
|---|---|---|
| closed | done successfully | unblocks |
| cancelled | abandoned | stays blocked unless also cancelled |

`reopen` accepts either, so cancellation is reversible.

## Coordinating around shared resources (locks)

For work that isn't shaped like an issue — a build directory, a test
database, a deploy slot, `npm install` — use a named lock:

```bash
# Acquire (fails immediately if already held)
clu lock deploy --ttl 1h -a <your-name>

# Block until free, then acquire
clu lock deploy --ttl 1h -a <your-name> --wait

# Best form: acquire, run a command, auto-release on exit
clu lock deploy --ttl 1h -a <your-name> -- ./deploy.sh production

# Release manually (only works if you're the holder)
clu unlock deploy -a <your-name>

# See what's held
clu locks
```

Every acquire requires a finite `--ttl` (default 5m) — there are no
forever-locks. If your process crashes, the lock auto-expires after
TTL elapses, so a stuck agent can't wedge everyone else permanently.
Pick a TTL that bounds the worst-case blast radius if you crash.

**Use locks for cross-cutting concerns; use the dep graph for
issue-shaped serialization.** If A must finish before B, that's
`clu link B A`, not a lock. Locks are for "two agents can't both
`npm install` in this repo at the same time" — coordination that
doesn't correspond to a tracked work item.

The `--` trailing-command form is the recommended shape — clu owns
the acquire+release lifecycle, so leakage is impossible even on a
non-zero exit:

```bash
clu lock build --ttl 5m -- go build ./...
clu lock test-db --ttl 10m -- pytest tests/integration/
```

## Pinging another agent (fire-and-forget mailbox)

For coordination chatter that doesn't belong on a ticket — "I'm
touching auth.go for 30m, hold off on session.go", "FYI the staging
deploy is mid-flight", "did you see the new sentry alert?" — use the
mailbox. Fire-and-forget, addressed to an agent, TTL'd, doesn't
pollute the work log.

```bash
# Send. Body via args, '-', or piped stdin.
clu ping eng2 "hold off on session.go, refactoring it for ~30m"
echo "lint green, deploying preview" | clu ping reviewer -

# Read your inbox (marks read on consume).
clu inbox
clu inbox --peek                  # see without consuming
clu inbox --all                   # include already-read (TTL applies)
clu inbox --since 1h              # only recent
clu inbox --clear                 # dismiss all unread

# Continuous push feed (works with Claude Code's Monitor tool).
Monitor: clu inbox --watch -a <your-name>
```

`clu brief` shows your unread-pings count at session start, so a
fresh agent knows to check.

**When to ping vs. comment vs. ticket:**

- **Ticket** (`clu create`): there's work to do that someone needs to
  track and close.
- **Comment** (`clu comment add <issue>`): durable note attached to a
  specific work item.
- **Ping** (`clu ping <agent>`): ephemeral signal to a person. No work
  item, no permanent record (auto-expires after 7d by default, max 30d).

Pings are the right tool when the conversation isn't about one
specific tracked issue. Don't ping things you'll need to find again —
that's what comments and notes are for.

## Talking to other agents

```bash
clu comment add <id> "found the bug in foo.go:42" -a <your-name>
clu comment ls <id>                # read the thread
clu show <id>                      # full record: description, notes, comments, deps
```

Use comments for discrete observations directed at other readers; use
`note append` for working scratch you might want to come back to.

## JSON for scripting

Every command supports `--json` and emits exactly one JSON value to
stdout, no narration mixed in. Read commands return typed objects/arrays;
write commands return the affected record(s).

```bash
clu --json ready                                    # array of Issue
clu --json show clu-abcd                            # one Issue
clu --json claim -a me                              # the claimed Issue
clu --json create -p 0 "title" | jq -r '.id'        # capture the new ID
```

If you're parsing IDs out of human output, the regex is `clu-[a-f0-9]+`
(the prefix is configurable per project; check `clu info`).

## Quick reference

| task | command |
|---|---|
| set up a fresh project | `clu init` |
| find next work | `clu ready -a me` |
| claim it | `clu claim -a me` |
| finish | `clu close <id>` |
| abandon (cascade) | `clu cancel <id>` |
| undo a close/cancel | `clu reopen <id>` |
| add a dep | `clu link <child> <parent>` |
| see one issue fully | `clu show <id>` |
| add a comment | `clu comment add <id> "…" -a me` |
| set freeform notes | `clu note set <id> "…"` |
| append to notes | `clu note append <id> "…"` |
| set/clear assignee | `clu assign <id> [<to>]` |
| change priority | `clu priority <id> 0..4` (0 highest) |
| add labels | `clu tag <id> foo bar` |
| filter list | `clu list -l label --status all --sort priority` |
| see live agents | `clu agent ls` |
| see one agent | `clu agent show <name>` |
| key-value scratch | `clu kv set foo bar` / `clu kv get foo` |
| database health | `clu doctor` |
| schema/db info | `clu info` |
| export everything | `clu export -o dump.jsonl` (preferred over `> dump.jsonl`) |
| import a dump | `clu import dump.jsonl` |

## Things not to do

- **Don't poll `ready` in a tight loop.** Use `claim --wait` with an
  interval (default 250ms) — it sleeps between checks.
- **Don't mix `--json` parsing with notice text.** In `--json` mode the
  stdout is exactly one JSON value; never interleaved with human prose.
- **Don't pass `-a <name>` to `claim` if you actually want the default
  lane** — `-a` filters to that lane.
- **Don't forget that cancel cascades.** If you only want to cancel the
  one issue without taking out its dependents, use
  `clu update --status cancelled <id>` instead.

## Workflows

A workflow template declares a graph of issues + deps in YAML.
`clu run <template>` instantiates it: one parent issue plus one
child per step, with `link` edges following the `needs:` list. No
new tables, no special status — it's plain issues you already know
how to claim, work, and close.

Templates live in `.clu/templates/*.yaml`. List, inspect, validate:

```bash
clu template ls                    # what's available
clu template show <name>           # parsed YAML
clu template validate <name>       # check structure without running
```

### Template shape

```yaml
name: release                      # used as the template handle
description: Build → test → deploy

# Shared context. Prepended to every step's description so every
# issue is self-contained — agents don't have to chase a separate
# doc. {{var}} interpolation applies. Use "@scaffold-spec.md" to
# load the body from a sibling file when it gets long.
spec: |
  ## Project context
  Repo: {{repo}}. Conventions: kebab-case, no force-pushes.
  Each step should leave a final comment on its issue summarizing
  what changed and where downstream steps can find their inputs.

vars:
  repo:
    required: true
    label: "Repo (org/name)"       # short name; shown by the prompter, future docs/GUI
  version:
    required: true
    pattern: '^\d+\.\d+\.\d+$'     # validated when running
  channel:
    default: stable                # used if -v channel=… omitted

steps:
  - id: build                      # step ID — local to this template
    title: "Build {{version}}"     # {{var}} interpolated from -v bindings
    description: |                 # per-step acceptance criteria
      Run `pnpm build`. Acceptance: dist/ contains the bundled output.
      On close, post a comment with the path to the artefact.
    priority: 1                    # optional; defaults to 2
  - id: test
    title: "Test {{version}} on {{channel}}"
    description: |
      Run the integration suite against {{channel}}. Acceptance: all
      tests pass. If any flake, link the flake to this issue.
    needs: [build]                 # test only becomes ready after build closes
  - id: gate
    type: checkpoint               # special: blocks downstream until you pass/fail
    title: "Confirm staging looks good"
    wait: { manual: true }         # any user can `clu approve`
    needs: [test]
  - id: deploy
    title: "Deploy {{version}}"
    needs: [gate]
    agent: deployer                # pre-assigns this step to a specific lane
```

Fields:

- **Template:** `name` (handle), `description` (one-liner), `spec`
  (multi-line shared context), `vars` (declared inputs), `steps`.
- **Var:** `required`, `default`, `pattern` (regex), `label`
  (short human name; shown by the prompter, reused by docs/JSON).
- **Step:** `id` (kebab-case), `title`, `description`
  (acceptance criteria), `type` (`task` or `checkpoint`), `priority`
  (0..4), `needs` (list of step IDs), `agent` (pre-assign to a lane),
  `wait` (only for `checkpoint`).

Each step's issue gets `spec + "\n---\n" + step.description` as its
body — `clu show <step-id>` is fully self-contained.

### Running one

```bash
clu run release -v version=1.2.3                # writes to DB
clu run release -v version=1.2.3 --dry-run      # print plan, no writes
clu run release -v version=1.2.3 --json         # emit the plan as JSON
clu run release                                  # interactive: prompts for required vars
clu run release --no-prompt                      # fail fast on missing vars (CI)
```

`clu run` prompts on stdin for any required var you didn't pass, when
stdin is a TTY. Pattern-validated; bad input re-prompts. Skipped under
`--json`, `--quiet`, when stdin is a pipe, or with `--no-prompt`.

After running, you'll see:

- One **parent issue** with label `template:<name>` and the title from
  `description` + var values.
- One **child issue per step** with labels `run:<parent-id>` and
  `step:<step-id>`.
- `link` edges wiring each step's `needs:` to the prior step's issue.
- Checkpoint steps additionally get `checkpoint:pending`.

Use those labels to find a run's issues later:

```bash
clu list -l run:clu-abcd        # everything in this run
clu list -l step:build          # all "build" steps across runs
clu list -l checkpoint:pending  # all gates waiting on approval
```

### Driving a workflow as an agent

It's the same loop as standalone work — `needs:` becomes `link` edges,
so `ready` only surfaces a step when its prerequisites are closed:

```bash
clu ready                          # the only ready step is the first one
# pick it up
clu claim
clu close <id>
clu ready                          # now the next step appears
# repeat until you hit a checkpoint or the end
```

Pre-assigned (`agent: foo`) steps appear only when claiming with
`-a foo`. Coordinators distribute work by routing steps into specific
lanes via the `agent:` field.

### Checkpoints

A checkpoint step is an issue that blocks downstream work until
explicitly cleared. Two kinds, set via `wait`:

| `wait` | how to clear |
|---|---|
| `{ manual: true }` | anyone runs `clu approve <id>` (or `clu checkpoint pass <id>`) |
| `{ approval: [alice, bob] }` | one of the listed users runs `clu approve <id>` (`-a` must match) |

Approval enforces the caller identity:

```bash
clu approve <id>                          # uses $USER as the approver
clu approve <id> -a alice --reason "lgtm" # explicit identity + audit note
clu checkpoint pass <id> --reason "ship"  # same as approve
clu checkpoint fail <id> --reason "nope"  # closes with checkpoint:failed; downstream STAYS blocked
```

Passing clears `checkpoint:pending`, closes the issue, and unblocks the
next step. Failing closes it but with `checkpoint:failed`, leaving the
rest of the workflow stuck — you'll need to `reopen`, fix the
checkpoint, or cancel the parent to move on.

### Cancelling a run

A run is just a parent + cascade of children. Cancel the parent to take
out everything still open:

```bash
clu cancel <parent-id>             # closed steps stay closed; open + in_progress go cancelled
```

If you want to abandon a single step but keep going, `clu cancel <step>`
cascades through that step's specific descendants only.

### See it end-to-end

`demo-workflow.sh` runs a real release template through claim → close
→ checkpoint → approve, plus failure paths and the parallel fan-out
shape. Read or run it when you need a concrete reference.

## Running in a worktree

Multi-agent workflows often want isolated checkouts so two agents can
work on different branches without stomping each other. `clu worktree`
wraps `git worktree add` with a project-defined bootstrap (the
`worktree:` block in `.clu/config.yaml`) so a fresh worktree gets its
gitignored files (.env, secrets) and per-checkout setup (pnpm install,
db migrate) automatically.

```bash
# Bare name → .worktrees/feat-foo inside the main worktree.
clu worktree add feat-foo -b feat/foo --bootstrap

# Explicit relative or absolute paths are honored as-is.
clu worktree add ../wt-feat -b feat/foo --bootstrap

# Bootstrap an existing worktree (re-runnable after editing the recipe).
clu worktree bootstrap feat-foo

# Remove with safety checks (blocks on uncommitted changes; warns on
# no-upstream / unpushed commits / stashes).
clu worktree remove feat-foo
```

Path resolution:
- Bare name → `<main>/<worktree.dir>/<name>` (default `<worktree.dir>` =
  `.worktrees`). The first such add appends `.worktrees/` to
  `.gitignore` so the directory doesn't pollute `git status`. Override
  the location with `worktree.dir:` in config.yaml.
- `./foo` / `../foo` / `/abs/foo` → honored verbatim against cwd.

Order inside the bootstrap is **copy → commands**, fail-fast. Both run
from the *main* worktree (auto-detected via git).

**Shared state.** All clu state (the SQLite DB, config.yaml, templates)
lives in the main worktree's `.clu/`. From inside any secondary
worktree, clu auto-resolves to the main worktree's `.clu/` — so `clu
list`, `clu create`, `clu claim` etc. all see the same data regardless
of which checkout you ran them from. No symlinks, no env vars, no
duplicate DBs. If you `clu init` inside a worktree by accident, you'll
get a second DB; delete the worktree's `.clu/` to fall back to the
shared one.

## Where to dig deeper

- `clu <command> --help` — every subcommand has its own help.
- `CLAUDE.md` — project-level sticky decisions, code conventions, and
  design rationale. Read this before making structural changes.
- `demo.sh` — runnable end-to-end happy-path exercise of most commands.
- `internal/store/` — domain-split source (one file per concern), if you
  need to know exactly what a command does at the SQL level.
