# Retrieve existing work

When the issue ID is known, use `show <id> --context`.
For a known Log entry, use `log show <log-id>`.
Otherwise search the selected board with distinctive problem, component,
decision, or artifact terms:

```bash
card --actor <actor> --board <board-id> search 'migration rollback'
```

Keep the default field scope for historical questions so Log and Result
remain searchable.
Use `--in title,summary,details` when execution history obscures likely
duplicate issues.
Inspect the matching issue or Log entry before relying on its conclusion;
search results locate evidence.
Use `search --help` for query syntax and other filters.
