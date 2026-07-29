# Working with attachments

Use an attachment when file bytes must remain available after the producing
path, process, session, or worktree disappears.
Keep the meaning of those bytes in an issue record.

| Need | Store as |
| --- | --- |
| Outcome, contract, current state, reasoning, or validation conclusion | Summary, details, state, log, or result |
| File contents needed by a later agent or acceptor | Attachment referenced from an issue record |

Attachments belong to one board.
Their optional issue association records where the file entered the board and
supports discovery;
it does not limit access from other issues on that board.

## Add and reference bytes

Add the file,
capture the emitted attachment ID,
and place a reference in the record that explains the file:

```bash
artifact=$(
  card --actor worker-a --json attachment add \
    --issue <issue-id> <artifact-path>
)
artifact_id=$(printf '%s\n' "$artifact" | jq -r .id)
artifact_ref="%$artifact_id"
card --actor worker-a log post <issue-id> \
  "Validation report: $artifact_ref"
```

Use `%<attachment-id>` when the stored filename is the right link text.
Use `[name](attachment:<id>)` for a custom downloadable-file label
or `![name](attachment:<id>)` for an image.
Use the reference in a result when the file is part of the durable outcome.
Keep the file's meaning and any essential conclusion in the surrounding issue
record rather than behind the attachment reference.

Reuse the same reference when another issue on the selected board needs the
file.
Do not upload another copy merely to associate it with another issue:

```bash
card --actor worker-a state append <same-board-issue-id> \
  "Evidence from %<issue-id>: $artifact_ref"
```

## Discover and retrieve bytes

When a record retains a `%<attachment-id>` or `attachment:<id>` reference,
inspect metadata and replica-local availability before depending on the bytes,
then retrieve them to a new path:

```bash
card --actor recovery-worker --json attachment show <attachment-id>
card --actor recovery-worker --json attachment get \
  <attachment-id> <output-path>
```

`attachment get` verifies the complete content.
It refuses to replace an existing output path unless `--force` is supplied.

When the originating issue is known but its records do not retain the
reference,
discover associated attachments with:

```bash
card --actor recovery-worker --json attachment list --issue <issue-id>
```

`attachment list` returns one stable page.
When `next_page_token` is present,
repeat the command with `--after <token>` until the token is absent.

Recover meaning from the issue context;
an attachment supplies file bytes rather than replacing that context.
