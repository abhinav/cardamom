# Contributing to clu

Thanks for your interest in `clu` — a SQLite-backed issue tracker and
coordination substrate for AI agents. This guide covers how to build, test,
and submit changes.

## Before you start

- **Bug reports & small fixes** are always welcome — open an issue or a PR.
- **Larger features:** please open an issue first to discuss the design.
  `clu` keeps a deliberately tight scope (see the "Not in scope" section of
  the [docs](https://arjia-labs.github.io/clu/) and `CLAUDE.md`); a quick
  conversation saves everyone a round-trip.
- By contributing, you agree your work is licensed under the project's
  [MIT License](LICENSE).

## Prerequisites

- **Go 1.26+** (the module targets the version in `go.mod`).
- No CGo, no system libraries — SQLite is pure Go (`modernc.org/sqlite`).
- Optional, only if you touch them:
  - **Web UI** (`web/clu-web/`) — Node 22 + `pnpm`.
  - **Docs** (`docs/`) — Node + `pnpm` (fumadocs).

## Build, test, lint

```bash
go build ./...           # must compile clean
go test ./...            # all tests must pass
golangci-lint run        # CI pins v2.12.2; match it locally if you can
```

Run `go build ./... && go test ./...` **before every commit** — CI runs the
same on Linux, macOS, and Windows. To build the binary:

```bash
go build -o clu ./cmd/clu
```

For an end-to-end sanity check, `./demo.sh` exercises the happy path against a
throwaway database, and `./demo-workflow.sh` runs the workflow/checkpoint flow.

## Project layout

```
cmd/clu/          entrypoint
internal/cli/     one file per kong subcommand
internal/store/   SQLite layer, split by domain (issues, deps, claim, …)
internal/workflow/ YAML template loader + planner
internal/http/    REST API (backs the web UI / `clu http`)
web/clu-web/      TanStack Start web dashboard
docs/             fumadocs site (published to GitHub Pages)
```

## Conventions

These keep the codebase consistent — please match them:

- **One file per command** in `internal/cli/`. Register it on the root `CLI`
  struct in `cli.go` and add the name to `completionCmds` in `completion.go`.
- **`r.notice(...)`** for narrative output ("closed clu-123", "claimed …") —
  it's auto-suppressed under `--quiet`/`--json`. Data (IDs, lists, JSON) goes
  straight to stdout.
- **Every `--json` invocation emits exactly one JSON value** on stdout — no
  leading notice, no trailing summary.
- **Migrations are append-only.** Never edit a past migration; only add a new
  one (`internal/store/store.go` / `migrations.go`, gated by `user_version`).
- **Sentinel errors are entity-specific** (`ErrNotFound`, `ErrKVNotFound`, …) —
  add a new one rather than overloading an existing one.
- Tests live beside the code: store tests in `internal/store/store_test.go`,
  CLI tests in `internal/cli/cli_test.go`. New behaviour needs a test.

Tests verify code correctness; for feature work, also run the relevant smoke
test (`demo.sh` or an ad-hoc `/tmp` workdir) before declaring it done.

## Commits & pull requests

- **Commit style:** [Conventional Commits](https://www.conventionalcommits.org/)
  are encouraged — `feat(sync): …`, `fix(claim): …`, `docs: …`. Keep each
  commit focused and self-contained (compiles clean, tests pass).
- **Branch** off `main`; don't commit directly to it in your fork's PR.
- **Open a PR** with a clear description of *what* and *why*. Make sure
  `go build ./... && go test ./...` and `golangci-lint run` are green — CI
  will check the same.
- Keep PRs reviewable: one logical change per PR where practical.

## Reporting security issues

Please **do not** open a public issue for security vulnerabilities — see
[SECURITY.md](SECURITY.md) for private disclosure.

## A note on conduct

Be respectful and assume good faith. We're here to build good tools, not to
win arguments. Harassment or hostility isn't welcome.
