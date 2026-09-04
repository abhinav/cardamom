# Find existing work and historical context

Use this workflow when the selected board may hold an issue or durable record
needed to reuse work or answer a question about historical work,
but its owner is unknown.

## Start from what is known

| Known information | First action |
| --- | --- |
| Issue ID | `card --actor <actor> show <issue-id> --context` |
| Log ID | `card --actor <actor> log show <log-id>` |
| Distinctive terms only | `card --actor <actor> search '<terms>'` |

Use supplied scope and resolve board ambiguity through
[scope.md](scope.md) before searching.
Do not search when the exact issue or Log entry is already known.

## Find the owning record

Search distinctive outcome, component, decision, or artifact terms:

```bash
card --actor <actor> --board <board-id> search '<terms>'
```

Write required terms next to each other;
adjacency is AND.
Use uppercase `OR` for alternatives,
uppercase `NOT` to exclude an expression,
parentheses for grouping,
double quotes for a phrase,
and a trailing `*` for a word prefix:

| Need | Query argument |
| --- | --- |
| Every term | `migration rollback` |
| Exact phrase | `"schema version"` |
| Either expression | `migration OR upgrade` |
| Exclude an expression | `migration NOT legacy` |
| Group alternatives | `(migration OR upgrade) rollback` |
| Match a word prefix | `migrat*` |

Keep the default field scope for historical questions because rationale and
outcomes often live in Log or Result.
When execution history obscures likely duplicate work,
use `--in title,summary,details` to narrow the search.
Use `--literal` when copied text should be treated as one phrase.
Use `card search --help` for metadata filters and other options.

A search result locates evidence;
it does not replace the source record.
Inspect the matching issue with `show --context` or the matching Log entry with
`log show` before relying on its conclusion.
