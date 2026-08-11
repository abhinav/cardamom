# Preserve file bytes

Use an attachment only when a later executor or acceptor needs file bytes after
the producing path, process, session, or worktree disappears.
Keep meaning and conclusions in issue records;
an attachment preserves bytes rather than context.

Prefer a reproducible command, source location, or external durable artifact.
Do not attach scratch output, working directories, dependency caches,
or bytes whose useful conclusion fits in State, Log, or Result.

Attachments belong to one board.
Their optional issue association records where the bytes entered that board;
other issues may reference the same attachment.

Add the file and explain what it establishes:

```bash
artifact=$(card --actor <actor> --json attachment add \
  --issue <issue-id> <artifact-path>)
artifact_id=$(printf '%s\n' "$artifact" | jq -r .id)
card --actor <actor> log post <issue-id> \
  "The complete validation report is %$artifact_id; it confirms the migration result."
```

Use `%<attachment-id>` when the filename is useful link text,
`[label](attachment:<id>)` for another label,
or `![alt](attachment:<id>)` for an image.
Reference the bytes from Result when they are part of the completed outcome.

Before depending on an attachment,
inspect metadata and retrieve the complete content:

```bash
card --actor <actor> --json attachment show <attachment-id>
card --actor <actor> --json attachment get <attachment-id> <output-path>
```

If a record lost its reference,
use `attachment list --issue <issue-id>` for discovery,
follow `next_page_token` with `--after <token>` until it is absent,
then recover meaning from issue context rather than the filename.
