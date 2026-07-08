# Applying multi-issue graphs

Use `card apply` when several related issues should appear atomically,
when an external producer reconciles a graph,
or when structured generation is clearer than several `card create` commands.
Use `card create` for one issue,
including its initial parent, dependencies, and labels.

`card apply` reads one strict version 1 JSON object.
Unknown fields are rejected.
The top-level object has the following fields:

| Field | Requirement | Value |
| --- | --- | --- |
| `version` | Required | Integer `1` |
| `issues` | Required | Non-empty array of issue entries |
| `on_existing` | Optional | `error`, `skip`, or `update`; defaults to `error` |

Each issue entry may contain the following fields:

| Field | Value |
| --- | --- |
| `alias` | String that names this entry within the document |
| `id` | Durable issue ID matching `[A-Za-z0-9][A-Za-z0-9-]*` in the selected board |
| `key` | Stable board-scoped identity owned by an external producer |
| `title` | Issue title; required when creating an issue |
| `type` | `workstream`, `task`, `checkpoint`, or `routine`; required when creating an issue |
| `priority` | Integer from `0` (highest) through `4` (lowest) |
| `summary` | Concise stable Markdown |
| `details` | Expanded stable Markdown |
| `labels` | `{"values": ["label", ...]}` to replace the complete label set |
| `parent` | One issue reference object to replace the containment parent |
| `clear_parent` | `{}` to remove containment; cannot appear with `parent` |
| `depends_on` | `{"values": [<issue-reference>, ...]}` to replace all prerequisites |

An issue reference object contains one string field:
`alias`, `id`, or `key`.
The issue ID grammar does not constrain apply-local aliases or external keys.
The document owns issue identities, existing-target policy, metadata presence,
containment, and dependencies.
The CLI selects the input and optional dry-run mode:

```bash
card --actor coordinator apply --help
card --actor coordinator --json apply graph.json
```

## Build the document

Choose `on_existing` by ownership intent:

| Value | Use when an entry already resolves to a durable issue |
| --- | --- |
| `error` | A collision means the input or selected board is wrong and the transaction must stop |
| `skip` | Existing issues are authoritative and the input should create only missing entries |
| `update` | The producer owns every present field and reruns should reconcile those fields |

The following graph creates or reconciles one workstream,
one child task,
and one checkpoint that depends on the task:

```json
{
  "version": 1,
  "on_existing": "update",
  "issues": [
    {
      "alias": "retirement",
      "key": "catalog:sandbox-17:retirement",
      "title": "Retire shared sandbox",
      "type": "workstream",
      "summary": "Remove sandbox 17 after its users and records are accounted for."
    },
    {
      "alias": "inventory",
      "key": "catalog:sandbox-17:inventory",
      "title": "Inventory sandbox dependents",
      "type": "task",
      "parent": {"alias": "retirement"},
      "labels": {"values": ["discovery"]}
    },
    {
      "alias": "authorization",
      "key": "catalog:sandbox-17:authorization",
      "title": "Authorize sandbox retirement",
      "type": "checkpoint",
      "parent": {"alias": "retirement"},
      "depends_on": {"values": [{"alias": "inventory"}]}
    }
  ]
}
```

Use identities according to their lifetime:

| Field | Identity |
| --- | --- |
| `alias` | Opaque reference local to one apply document |
| `id` | Known durable issue in the selected board |
| `key` | Stable board-scoped identity owned by an external producer |

Every relationship uses a typed reference object containing one `alias`, `id`,
or `key` field.
Represent structure with `parent` and readiness with `depends_on`.
The apply contract has no separate grouping layer.

## Control updates

With `on_existing: "update"`,
only fields present in an entry are updated;
omitted fields are preserved.
`labels` and `depends_on` use wrapper objects because their presence replaces
the complete set.
Use `"clear_parent": {}` to remove containment explicitly.

Update can target only open, unclaimed issues.
It cannot change lifecycle, custody, state, logs, results,
or checkpoint decisions.
The complete post-transaction containment and dependency graph must remain
valid.
Unknown JSON fields and unsupported public values are errors.

Use `--dry-run` only when a non-mutating preview helps the current decision:

```bash
card --actor coordinator --json apply --dry-run graph.json
```

Input may arrive on standard input,
including JSON generated with `jq` or another program:

```bash
generate-graph | card --actor coordinator --json apply
```
