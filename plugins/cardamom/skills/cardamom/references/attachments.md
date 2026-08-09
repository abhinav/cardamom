# Preserve file bytes

Use an attachment when a later executor or acceptor needs file bytes after the
producing path, process, session, or worktree disappears.
Keep the meaning and material conclusion in an issue record;
the attachment preserves bytes rather than context.

Attachments consume durable store capacity.
Use them for bytes with continuing value,
not as scratch space or a substitute for concise issue records.
Prefer a command, source location, or external durable artifact when a later
executor can reproduce or retrieve the same bytes reliably.
Do not attach transient command output, working directories, dependency caches,
or files whose useful conclusion fits in State, Log, or Result.

Attachments belong to one board.
Their optional issue association records where the file entered the board and
supports discovery.
The same attachment may be referenced by other issues on that board.

## Add and explain an attachment

Add the file, capture its stable ID,
and reference it from the record that explains what the bytes establish:

```bash
artifact=$(card --actor <actor> --json attachment add \
  --issue <issue-id> <artifact-path>)
artifact_id=$(printf '%s\n' "$artifact" | jq -r .id)
card --actor <actor> log post <issue-id> \
  "Validation report: %$artifact_id"
```

Use `%<attachment-id>` when the stored filename is useful link text.
Use `[label](attachment:<id>)` for another label,
or `![alt](attachment:<id>)` for an image.
Reference an attachment from Result when its bytes are part of the completed
outcome.

Reuse the same reference on another issue in the board instead of uploading a
duplicate solely for association.

## Inspect and retrieve bytes

Inspect metadata and replica-local availability before depending on an
attachment:

```bash
card --actor <actor> --json attachment show <attachment-id>
card --actor <actor> --json attachment get \
  <attachment-id> <output-path>
```

`attachment get` verifies complete content.
It replaces an existing output path only with `--force`.

When the originating issue is known but its records do not retain a reference,
list associated attachments:

```bash
card --actor <actor> --json attachment list --issue <issue-id>
```

When `next_page_token` is present,
repeat with `--after <token>` until it is absent.
Recover the attachment's meaning from issue context.
