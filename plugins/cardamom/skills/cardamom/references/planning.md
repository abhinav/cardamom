# Organize related work

Split work when another outcome benefits from separate ownership, dependencies,
or acceptance; each issue should be useful to track on its own.

| Type | Use |
| --- | --- |
| `task` | Work without child issues |
| `workstream` | A deliverable that may contain child issues; one actor may execute it directly |
| `checkpoint` | An external authority's approve-or-deny decision |
| `routine` | Recurring work awakened by an external scheduler or coordinator |

## Choose relationships by their meaning

Use a parent when the child contributes to that parent's deliverable.
Use a dependency when unfinished prerequisite work prevents an issue from
advancing or being accepted.
Containment supplies inherited context and completion structure;
it does not by itself block the parent's execution.
Labels classify issues; they do not order work or transfer claims.
Reflect consequential child or helper findings in parent progress, retaining provenance.

```bash
card --actor <actor> create 'Validate import compatibility' \
  --type task --parent <workstream-id> --depends-on <schema-id> \
  --summary 'Verify existing imports against the revised schema.'
```

When replacing an outcome, retain useful conclusions and link predecessors
and successors so later readers can follow the change.
Use cancellation guidance from the entry point for abandoned or superseded work.

## Record required external decisions

Create with `--type checkpoint` and make affected work depend on its ID.
Obtain the decision from its authority, then use `checkpoint approve` or
`checkpoint deny` to record it.
The command actor supplies attribution, not approval authority.
Read termination guidance before denial because it cancels dependent work.

## Reconcile generated graphs

Use `create` or `edit` for individual issues.
Use `apply` when one producer must reconcile several issues or relationships
atomically; its installed help owns the input schema.

| Existing-target policy | Use when |
| --- | --- |
| `error` | An existing target is unexpected |
| `skip` | The existing issue is authoritative |
| `update` | The producer owns every supplied field |

Supplied set-valued fields replace the complete set under `update`.
