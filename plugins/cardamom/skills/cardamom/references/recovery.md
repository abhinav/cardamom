# Recover interrupted work

Establish unknown ownership with `show <id> --context`.
Coordinate with a possibly live owner; leave its claim intact.
Read history when the reason for a decision or the current position is missing:

```bash
card --actor <actor> --json log show <id> --limit 20
```

Log is newest-first; widen the window only when the answer lies further back.
Inspect ancestor Details, attachments, or child outcomes for unanswered questions.
Refresh facts that may have changed independently and correct affected records.

## Reassign a stopped executor

The runtime must confirm that the previous executor stopped, and reassignment
must be authorized.
Release under the stopped owner solely to end its claim;
record the reason and take new ownership under the respective actors:

```bash
card --actor <coordinator> log post <id> \
  'The runtime confirmed the previous executor stopped; reassignment is authorized.'
card --actor <previous-owner> release <id> --waiting 'Stopped executor reassignment'
card --actor <replacement> --json claim <id> --context
```
