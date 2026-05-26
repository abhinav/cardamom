# Project notes for Claude

A SQLite-backed issue-tracker CLI. Module: `github.com/rovak/beadsv2`. Binary:
`cli` (produced by `go build -o cli ./cmd/cli`).

## Sticky decisions — don't re-debate

- **Binary is `cli`, not `bd`.** The user has the upstream `bd` on PATH; renaming
  was deliberate to avoid the conflict.
- **Don't reintroduce "beads" branding** in user-facing strings (help, errors,
  descriptions). Storage paths were renamed to `.db/` / `DB_DIR` / `data.sqlite`.
  Module path `beadsv2` and issue IDs `bd-XXXX` stay — they're internal/storage,
  not branding.
- **Stack is settled:**
  - SQLite via `modernc.org/sqlite` (pure Go — no CGo).
  - **Bun** + sqlitedialect for queries. Raw SQL only for `Claim` (UPDATE …
    RETURNING with subquery WHERE) and the recursive-CTE cycle check.
  - **Kong** for CLI (struct-tag commands, intermixed flags, `IsSet`/env).
  - **Hand-rolled migrations** via `PRAGMA user_version` in
    `internal/store/store.go`'s `migrations[]`.
  - We tried **sqlc**; reverted. Don't propose it again unless the user does.
- **Migrations are append-only.** Never edit a past entry; only add new ones.

## Code conventions

- One file per kong command in `internal/cli/`. Register on the root `CLI`
  struct in `cli.go`; add the name to `completionCmds` in `completion.go`.
- **`r.notice("…")`** for narrative output ("closed bd-X", "claimed …").
  Auto-suppressed by `--quiet` and `--json`. Data output (IDs, lists, JSON,
  whole-issue blocks) goes straight to `r.stdout`.
- **Sentinel errors are entity-specific.** `ErrNotFound` (issues),
  `ErrKVNotFound` (kv). Add a new sentinel rather than overloading.
- **`eachID(r, ids, fn)`** helper centralises variadic-ID commands
  (`close`, `reopen`, `undefer`). Aggregates errors via `errors.Join`,
  emits a JSON array under `--json`.
- **Every `--json` invocation emits exactly one JSON value** on stdout.
  No leading notice, no trailing summary. Read commands → typed object
  or array. Write commands → the affected record(s).
- Sugar verbs (`assign`, `priority`, `tag`, `link`, `describe`) delegate
  to existing store methods; they're 10–20 LOC each.

## Tests

- Store tests: `internal/store/store_test.go`. Use `newTestStore(t)` and the
  package-level `var ctx = context.Background()`.
- CLI tests: `internal/cli/cli_test.go`. Use `newTestCLI(t)`, then
  `c.run(...)` or `c.runFail(...)`.
- **Run before every commit:** `go build ./... && go test ./...`.
- **Smoke test convention:** built binary against `/tmp/<scratch>/`. The
  `demo.sh` script runs the full happy path end-to-end.

## Working docs

- **`work.md`** is the living planning doc — vision, per-commit log,
  proposed next batches, decisions log, deliberate "skipped" list. Update
  at the end of a batch, not mid-task. Treat the per-commit log as
  append-only.
- **`bugs.md`** is QA findings from manual exploration. Mark items as
  fixed inline; leave the file in place.
- Don't write extra `.md` files unless the user asks.

## Not in scope (deliberate)

Upstream beads has these; we chose not to copy:
- Dolt anything — we chose SQLite.
- Memory system (`prime`, `remember`, `recall`) — agent runtime, not tracker.
- Integrations: Jira, Linear, GitHub, GitLab, Notion, ADO.
- `swarm`, `ship`, `convoy` abstractions.
- Generic federation / branch / worktree.

## Workflows

Templates live in `internal/workflow/` and instantiate to plain issues
+ deps + labels — no new tables. Two layers:

- `internal/workflow/` — pure: YAML loader, validation, var
  resolution, `MakePlan(Template, vars) → Plan`. No store calls.
- `internal/cli/run.go` + `internal/cli/template.go` — CLI walks the
  Plan, calls `store.Create` per step, writes deps and the
  `run:<parent>` / `step:<id>` labels. Checkpoint steps additionally
  get `checkpoint:pending` and a `cp:<issue-id>` KV entry holding
  `{kind, approvers}` JSON.

Templates are loaded from `.db/templates/*.yaml`. The format and
shape live in `internal/workflow/template.go`. See `demo-workflow.sh`
for an end-to-end run.

Don't propose adding these without the user explicitly asking.

## Workflow expectations

- When asked to "do batch X", break into commits per logical group. Each
  commit must compile clean and pass tests.
- When asked to fix a list of bugs, work in priority order. Make separate
  commits per bug (or per closely-related pair) so each is reviewable.
- When a feature touches multiple files, run the end-to-end smoke test
  (`demo.sh` or an ad-hoc `/tmp/...` workdir) before declaring done. Tests
  verify code correctness, not feature correctness.
- When spawning subagents, prefer worktree isolation; if unavailable in
  the env, do groups sequentially rather than in parallel in the same tree.
