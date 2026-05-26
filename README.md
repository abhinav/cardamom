# clu

<p align="center">
  <img src="docs/header.jpg" alt="clu — agent-coordination issue tracker" width="100%">
</p>

<p align="center">
  <a href="https://github.com/Rovak/agents-clu/actions"><img src="https://img.shields.io/github/actions/workflow/status/Rovak/agents-clu/ci.yml?branch=main&label=build" alt="Build status"></a>
  <a href="https://goreportcard.com/report/github.com/rovak/clu"><img src="https://goreportcard.com/badge/github.com/rovak/clu" alt="Go report card"></a>
  <a href="https://pkg.go.dev/github.com/rovak/clu"><img src="https://pkg.go.dev/badge/github.com/rovak/clu.svg" alt="Go reference"></a>
  <img src="https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.22+">
  <img src="https://img.shields.io/badge/sqlite-pure%20go-003B57?logo=sqlite&logoColor=white" alt="SQLite (pure Go)">
  <img src="https://img.shields.io/badge/license-MIT-green" alt="MIT licensed">
</p>

> **clu** — a SQLite-backed issue tracker for coordinating work between
> humans and AI coding agents (Claude Code, Cursor, Aider, …) on a
> single machine. Named after Tron's *Codified Likeness Utility*.

## Why?

When you run more than one AI coding session against the same project,
they need a shared, durable place to:

- pick up work without stepping on each other (atomic claim),
- record what they tried, what worked, what didn't,
- gate risky steps behind human approval,
- and surface what's unblocked vs. waiting on something else.

`clu` is that place. It's a small, fast, single-binary CLI backed by a
local SQLite database. No daemon, no server, no account, no network.

## Highlights

- **Single binary, pure Go.** SQLite via `modernc.org/sqlite` — no CGo,
  no system libraries.
- **Local-first.** One file: `.clu/data.sqlite`. Commit `config.yaml`,
  ignore the DB.
- **Atomic claim.** `UPDATE … RETURNING` with a subquery — two agents
  racing each other get different issues.
- **Capability routing.** Agents declare capabilities in
  `config.yaml`; issues tagged `cap:foo` flow to agents that can
  handle them.
- **Cascading cancel.** `clu cancel <id>` walks the dep graph forward
  and marks everything downstream as `cancelled` (distinct from
  `closed` — see below).
- **Workflow templates.** Declarative YAML graphs of issues + deps
  with optional human-approval checkpoints.
- **JSON everywhere.** Every command takes `--json` and emits exactly
  one JSON value to stdout — clean for scripting.
- **No reach into the network.** No telemetry, no sync, no cloud
  features. If you want to share a tracker between machines, copy the
  file.

## Install

```bash
go install github.com/rovak/clu/cmd/clu@latest
# or, from a clone:
make install
```

Add `$HOME/go/bin` to your `PATH`. `clu --help` should now work.

## Quickstart

```bash
mkdir my-project && cd my-project
clu init                                    # creates .clu/ with DB + config
clu create -p 1 "fix the login redirect"    # → clu-a3f8
clu create "add tests for the redirect"
clu link clu-XXXX clu-a3f8                  # tests depend on the fix

clu ready                                   # what's unblocked?
clu claim                                   # atomically take the next one
clu close clu-a3f8                          # done → unblocks the tests
clu ready                                   # tests are now ready
```

That's the whole core loop. See `demo.sh` for a runnable end-to-end
exercise, or `AGENTS.md` for the agent-facing operational guide.

## Status semantics

| status | meaning | downstream effect |
|---|---|---|
| `open` | not yet started | normal |
| `in_progress` | claimed; an agent is working | normal |
| `closed` | done successfully | **unblocks** dependents |
| `cancelled` | abandoned | dependents stay blocked (or cascade-cancel) |

`clu cancel <id>` marks the target *and all transitive descendants* as
cancelled. `clu reopen <id>` reverses either terminal state.

## Multi-agent setup

Declare your agents in `.clu/config.yaml`:

```yaml
id_prefix: clu-
agents:
  code-reviewer:
    description: "Reviews Go code for correctness and security"
    capabilities: [go-review, security-review]
  doc-writer:
    description: "Writes README + docs/ updates"
    capabilities: [docs]
```

Then each agent claims from its own lane:

```bash
clu claim --agent code-reviewer --wait --heartbeat
```

`--heartbeat` is opt-in; without it the claim loop doesn't advertise
liveness. With it, `clu agent ls` shows who's currently online and
what they were last seen doing.

Coordinators route work by either assigning directly
(`clu create -a doc-writer ...`) or by tagging capability
(`clu create --capability docs ...`). Capability-tagged issues in the
default lane flow to whichever agent advertises that capability.

## Workflows

Drop a YAML template into `.clu/templates/`:

```yaml
name: release
vars:
  version: { required: true, pattern: '^\d+\.\d+\.\d+$' }
steps:
  - id: build
    title: "Build {{version}}"
  - id: test
    title: "Test {{version}}"
    needs: [build]
  - id: approve
    type: checkpoint
    title: "Approve {{version}} for prod"
    wait: { approval: [alice, bob] }
    needs: [test]
  - id: deploy
    title: "Deploy {{version}}"
    needs: [approve]
```

```bash
clu run release -v version=1.2.3      # instantiates parent + 4 children + deps
```

Agents drive it by claiming `ready` issues as each step closes;
humans clear `checkpoint` steps via `clu approve <id>`. See
`demo-workflow.sh` for the full demo.

## Layout

```
cmd/clu/                 entrypoint
internal/cli/            one file per kong subcommand
internal/store/          SQLite layer, split by domain
  ├── models.go            bun model types
  ├── migrations.go        manual schema migrations (PRAGMA user_version)
  ├── issues.go            create/get/close/reopen/cancel/update
  ├── claim.go             ready/claim atomic queries
  ├── deps.go              dependency edges + cycle detection
  ├── …                    labels, comments, kv, cron, agents, doctor
internal/workflow/       YAML template loader + planner
internal/config/         config.yaml parsing
.clu/                    per-project storage (DB, config, templates)
```

## Design notes

- **One identity flag.** `-a` / `--agent` is both the lane filter and
  the actor identity. There is no `--as`. Local single-user tool —
  the user/agent distinction was deliberately collapsed.
- **Hand-rolled migrations** via `PRAGMA user_version`. Append-only,
  never edit an applied migration.
- **Bun + sqlitedialect** for queries. Raw SQL escape hatches in
  exactly two places: the atomic claim and the cancel-cascade CTE.
- **Kong** for the CLI struct, with struct-tag commands and
  intermixed flags.

The rationale for each sticky decision lives in `CLAUDE.md`.

## Not in scope

`clu` is deliberately small. It does not try to be:

- a cross-machine sync layer (if you need that, copy the file or
  layer something on top),
- a generic project-management tool (no sprints, milestones, OKRs),
- a bridge to GitHub/Linear/Jira (issue trackers integrate with each
  other badly; pick one),
- an agent runtime (the agent is whoever runs `clu claim`).

## Contributing

PRs welcome. Before sending:

```bash
go build ./... && go test ./...
./demo.sh && ./demo-workflow.sh
```

See `CLAUDE.md` for code conventions (one file per Kong command,
sentinel errors per entity, JSON-clean output, etc.).

## License

MIT — see [LICENSE](LICENSE).
