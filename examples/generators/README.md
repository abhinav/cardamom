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

## Examples

- **`feature-rollout.js`** — parameterized per-module feature rollout:
  `node feature-rollout.js <feature> <module...> [--approver=NAME]`.
  Demonstrates argument parsing, per-module fan-out, a conditional security
  audit for sensitive modules, and an approval checkpoint gating the ship
  step.
