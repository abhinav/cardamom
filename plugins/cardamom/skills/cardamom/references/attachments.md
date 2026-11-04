# Retain an artifact's bytes

Attach a file when a later reader needs its bytes after the original location
disappears.
Prefer a durable URL, source location, or reproducible command when sufficient.
Keep the artifact's meaning in the issue:

```bash
artifact=$(card --actor <actor> --json attachment add \
  --issue <id> <artifact-path>)
artifact_id=$(printf '%s\n' "$artifact" | jq -r .id)
card --actor <actor> log post <id> \
  "All compared account mappings matched; the report is %$artifact_id."
```

Attachments belong to a board and may be referenced from several issues.
Use `%<attachment-id>` for the filename as link text,
`[label](attachment:<id>)` for another label,
or `![alt](attachment:<id>)` for an image.

Retrieve bytes with `attachment get <attachment-id> <output-path>`.
Use `attachment list --issue <id>` to recover a lost reference.
Follow `next_page_token` with `--after <token>` until absent when listing all results.
