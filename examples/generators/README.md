# Workflow generators (codemode)

Scripts here build an issue **graph** and print it as one JSON document on
stdout, to be piped into `clu batch`:

```bash
node feature-rollout.js auth-v2 api ui payments --approver=alice | clu batch
```

This is the programmable alternative to a static `clu run` YAML template
(see `../templates/`). The split:

- **Generation** (these scripts) — any language, any logic: loops,
  conditionals, computed fan-out, reading config/APIs to decide the shape.
  clu doesn't care what produced the graph.
- **Instantiation** (`clu batch`) — validates the graph (acyclic, every
  `needs` resolves, fields valid), allocates real IDs, and writes it in
  one transaction. "Graphed correctly" is guaranteed, not hoped.

Iterate safely with `--dry-run` (validates + prints stats, writes nothing),
then commit with `--json` to get the `alias → real-id` map back.

## Output format

A JSON array (or `{"issues": [...]}`) of issues. Per issue:

| field          | notes                                                        |
| -------------- | ------------------------------------------------------------ |
| `alias`        | local handle, unique in the batch; used to wire `needs`      |
| `title`        | required                                                     |
| `needs`        | aliases (internal edges) or existing real IDs (external)     |
| `type`         | task (default), bug, feature, epic, chore, decision, checkpoint |
| `priority`     | 0 (highest) … 4; default 2                                   |
| `assignee`     | pre-route to an agent lane                                   |
| `capabilities` | `cap:<name>` labels for capability routing                  |
| `labels`       | arbitrary extra labels                                       |
| `description` / `notes` | freeform text                                      |
| `checkpoint`   | `{}` (manual gate) or `{"approvers":["alice"]}` — a manual approval gate, same as a `clu run` checkpoint step |

## Optional helper: `clu.js`

`clu.js` is a tiny, zero-dependency convenience for Node generators — copy
it or `require("./clu.js")`. It only shapes data; `clu batch` still owns all
validation, so the helper can't drift from the real rules. The wins are
ergonomic: `add()` returns the alias so you wire `needs` by handle (typos
become JS reference errors), and duplicate aliases throw at build time.

```js
const { Graph } = require("./clu.js");
const g = new Graph();
const design = g.add("design", { title: "Design", type: "decision" });
const impl   = g.add("impl",   { title: "Implement", needs: [design] });
g.checkpoint("gate", { title: "Approve", needs: [impl], approvers: ["alice"] });
g.add("ship", { title: "Ship", needs: ["gate"] });
g.emit();                                  // → pipe into `clu batch`
```

### Phases

`Graph.phase(name, fn)` groups the issues added inside `fn()` into an
ordered stage. A phase can't start until the previous one is fully done —
the helper drops a `milestone` at each boundary that **auto-closes** when
its phase completes, which unblocks the next phase automatically (no human
gate; use `checkpoint()` inside a phase for that). Each task is labelled
`phase:<n>-<name>`.

```js
const g = new Graph();
g.phase("inventory", () => { for (const m of mods) g.add(`inv-${m}`, {title:`Inventory ${m}`}); });
g.phase("analysis",  () => { for (const m of mods) g.add(`ana-${m}`, {title:`Analyze ${m}`}); });
g.phase("migrate",   () => { for (const m of mods) g.add(`mig-${m}`, {title:`Migrate ${m}`}); });
g.emit();
```

Pair with `clu batch --group "<name>"` for an umbrella that itself
self-completes when the whole thing is done (the umbrella is a milestone
too). Milestones are a clu core type — `phase()` is just sugar that emits
them; the auto-close happens in clu, not the generator.

### Arg helpers

It also exports two tiny arg helpers so generators don't re-hand-roll the
messy parse loop:

```js
import { parseArgs, usage } from "./clu.js";
const { flags, positional } = parseArgs();      // --k=v → flags.k, --flag → true, -h → flags.help
if (flags.help || positional.length < 1) {
  usage("usage: node gen.js <thing...> [--approver=NAME]", flags.help ? 0 : 2);
}
```

It's optional: the contract is still plain JSON, so generators in any
language (or raw Node) work without it.

These examples are ES modules (`package.json` sets `"type": "module"`), so
`import` works with a plain `node <script>.js`.

## Examples

- **`feature-rollout.js`** — parameterized per-module feature rollout:
  `node feature-rollout.js <feature> <module...> [--approver=NAME]`.
  Demonstrates argument parsing, per-module fan-out, a conditional security
  audit for sensitive modules, and an approval checkpoint gating the ship
  step. Written **raw** (no helper) as the "this is just JSON, translate to
  any language" reference. Kept capability-free so a fresh `clu ready`
  surfaces work immediately — see the note in the file for how to add
  `capabilities`.

- **`release-train.js`** — per-service build → staging, one approval gate,
  then per-service prod deploy:
  `node release-train.js <version> <service...> [--approver=NAME]`.
  Built **with `clu.js`** to show the helper: fan-out → fan-in checkpoint →
  fan-out, with `needs` passed by handle.

- **`migrate.js`** — a **phased** migration (inventory → analysis → migrate
  → verify), one task per module per phase:
  `node migrate.js <target> <module...> [--approver=NAME]`.
  Shows `phase()` (auto-advancing milestone boundaries) plus a checkpoint in
  the final phase. Best with `clu batch --group "<name>"`.

- **`linear-todo.js`** — turn a Linear export into a batch graph:
  `linear issue query --json | node linear-todo.js | clu batch`.
  Each issue carries a stable `key` (`linear:<id>`), so re-running is
  **idempotent** — see below.

## Idempotent re-import (`key` + `--on-existing`)

Give an issue a stable `key` and re-running a batch won't duplicate it:

```jsonc
{ "alias": "eng-123", "key": "linear:ENG-123", "title": "Fix login" }
```

clu records the key (as a `extkey:<key>` row) on first create. On later
runs, an issue whose key already exists is, per `--on-existing`:

- `skip` (default) — left untouched; re-runs only add genuinely-new issues.
- `update` — its source-owned fields (title, type, priority, description)
  are re-synced; local workflow state (status, assignee, labels, deps,
  notes) is left alone.

Either way the alias still resolves to the existing ID, so a new issue that
`needs` an already-imported one wires up correctly. `--json` reports
`new` / `existing` / `updated` counts.

> Note: `--group` creates a *new* umbrella each run (the parent isn't
> keyed), so don't pass it on every scheduled re-import unless you want one
> umbrella per run.
