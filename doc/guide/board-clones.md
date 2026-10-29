# Clone a board

`card board clone` creates another replica of a board in a different local store.
The destination store and project must already exist:

```bash
card --store /path/to/source --board "Release planning" board clone \
    --to-store /path/to/destination --to-project "Product"
```

Omit `--to-project` when the destination store contains one project.
The source board stays unchanged.
Clone preserves its name, description, archive state, board configuration
overrides, issue identities, relationships, records, pins, and attachments.
It also retains the full mutation history and attachment files referenced
by that history.

Claims, assignees, leases, mail, and unfinished uploads stay local.
An active claim does not prevent cloning,
and the cloned issue does not inherit that claim.
Waiting status and reasons are shared issue state and are preserved.

The clone has a new writer identity for its future changes.
Both stores must resolve the board's issue ID strategy to `random`.
Store and project configuration do not transfer;
only the board's own overrides transfer.
Inherited limits and defaults can therefore differ at the destination.

Clone rejects a destination that already contains the logical board,
a board with the same name in the selected project,
or conflicting issue or Log identities.
It does not rename identities to make them fit.
Clone cannot duplicate a board within one store.

Clone does not update or merge existing replicas.
Changes made after cloning remain separate.
If an import fails before commit, no destination board is published;
any verified attachment files already transferred can be collected as orphans.

`card board copy` remains a separate independent-duplication operation.
It assigns new board and private issue identities instead of preserving lineage.
