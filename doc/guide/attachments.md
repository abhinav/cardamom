# Attachments

Use an attachment when file bytes must survive the producing path,
process,
session,
or worktree.
Keep the meaning of those bytes in an issue record.

| Information | Storage |
| --- | --- |
| Contract, current state, reasoning, or outcome | Issue summary, details, state, log, or result |
| File bytes needed by a later agent or acceptor | Attachment referenced from an issue record |

## Add and reference a file

Upload the file with the issue where it entered the board:

```bash
card --actor worker-a attachment add --issue "$issue" report.txt
```

The command displays the attachment ID,
the file link,
and the image reference:

```text
att_yfivg2cnkcjaorkasemozcjiqa
[report.txt](attachment:att_yfivg2cnkcjaorkasemozcjiqa)
![report.txt](attachment:att_yfivg2cnkcjaorkasemozcjiqa)
```

Use the displayed ID in a durable record where the file's meaning is clear:

```bash
card --actor worker-a log add "$issue" \
  "Validation report: %att_yfivg2cnkcjaorkasemozcjiqa"
```

Agents and automation can capture the ID from structured output:

```bash
artifact_id=$(
  card --actor worker-a --json attachment add \
    --issue "$issue" report.txt |
    jq -r .id
)
```

Use `%<attachment-id>` when the stored filename is suitable link text.
Use `[label](attachment:<id>)` for a custom file label
or `![alt](attachment:<id>)` for an image.

Attachments belong to one board.
Their optional issue association records where the bytes entered the board;
other issues on the same board may reference the same attachment.

## Retrieve verified bytes

Inspect metadata and replica-local availability before depending on an
attachment,
then download it to a new path:

```bash
card --actor recovery-worker --json attachment show <attachment-id>
card --actor recovery-worker --json attachment get \
  <attachment-id> recovered-report.txt
```

`attachment get` verifies the complete content
and refuses to replace an existing output path unless `--force` is supplied.
When only the originating issue is known,
use `attachment list --issue <issue-id>` and follow `next_page_token`
until it is absent.
