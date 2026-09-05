# Store schema

`20260726181403_baseline.sql` defines the original application schema and seed
state.
Later timestamped migrations extend that schema without changing the baseline.
Goose owns migration history and applies every migration before `Store` is
published.

The migration sequence is the sqlc schema source and therefore defines every
current table, index, trigger, constraint, and seed row.

## Authoring migrations

Create schema migrations with:

```sh
mise run migration:new:sql -- add_issue_status
```

SQL migrations own tables, columns, indexes, constraints,
and other schema changes.
Keep schema changes in this directory because sqlc reads these files
as the authoritative repository schema.

Create a Go migration only when a data transition needs Go logic
that would be unclear or impractical in SQL:

```sh
mise run migration:new:go -- backfill_issue_status
```

Go migrations are transactional, data-only transitions.
Replace the generated placeholder error with the forward data migration.
Do not change schema from a Go migration because sqlc cannot derive
the repository schema from Go code.

Both workflows normalize simple word names to lowercase snake case
and use the current UTC timestamp as the shared SQL and Go version.
The workflows reject a timestamp already used by either migration format
and never open a store database.
Cardamom migrations are forward-only.

After release, migrations are append-only.
Schema changes remain timestamped SQL files in this directory so sqlc and the
runtime migration registry read the same authoritative schema sequence.
Data-only migrations may use explicit Go registrations owned by the store
package.

## Current schema families

### Store revisions

`store_state` is a singleton that owns store-wide issue allocation
and the canonical committed revision head.
Each successful write publishes the next `current_revision`
in the same SQLite transaction as its projection changes.
`verifyStore` requires non-negative revisions and enforces
`issue.revision <= board.revision <= store_state.current_revision`.

`next_issue_number` is a positive, store-wide sequence.
Sequential issue creation reserves and advances it inside the same write
transaction as the new issue.

### Store lineage and board-copy receipts

`store_lineage` assigns one persistent random lineage to a physical store
history.
A filesystem clone retains the lineage because the value lives in SQLite.

`board_copy_receipts` records the source lineage, board, semantic snapshot,
destination options, destination board, and destination revision for each
successful copy.
Separate mapping tables retain every issue, Log, and attachment identity
decision.
The schema permits several snapshot receipts for one source board;
first-release changed-snapshot rejection is command policy rather than a
schema constraint.

### Project catalog

`projects` and `boards` define logical namespaces inside the physical store.
A board belongs to one project and is the workflow scope for all of its issue
graph state.
Restricted deletes prevent removing a project with boards
or a board with issues.

Each board retains the latest committed revision that changed its projection.
Board revisions provide pagination snapshots and change cursors
without retaining semantic mutation history.

Nullable project and board configuration columns inherit from the preceding
configuration layer.
Configured byte limits govern later write admission and do not invalidate
retained summaries or attachments.

### Issue graph and records

`issues`, `active_claims`, `issue_labels`, `dependencies`, `containment`,
`issue_external_keys`, `issue_results`, and `issue_log_entries` hold the board-scoped
issue graph and its records.

Issue IDs are store-wide primary keys
matching `[A-Za-z0-9][A-Za-z0-9-]*`.
`UNIQUE (board_id, id)` also supports the composite foreign keys repeated by
issue-owned tables.
Those composite references reject a child row or relationship whose declared
board differs from the referenced issue's board.

An open issue has no `closed_at`; a closed or cancelled issue requires one.
Only open, unclaimed issues may have `waiting_reason` and `waiting_since`.
The waiting columns are either both set or both unset.
Claim custody remains separate in `active_claims`, and `started_revision`
records the committed revision that acquired custody.
`verifyStore` rejects an active claim for a terminal issue or a checkpoint.

Dependency and containment rows reject self-edges.
The `containment.child_id` primary key also gives each issue at most one parent.
The planning domain prevents dependency and containment cycles because those
graph-wide constraints cannot be expressed as row-local checks.

External keys are unique within a board.
Each issue has at most one current state and result,
while the repository treats log entries as append-only chronological records.
Each issue retains the latest committed revision that changed its projection.
Log entry IDs are random durable handles.
The private `local_sequence` preserves local chronological ordering and does
not cross the repository boundary.
Issue-owned projections and records cascade if an issue row is removed.

### Issue search

`issue_search_documents` projects the canonical issue text into one row per
searchable field or Log entry.
`issue_search_fts` is an external-content FTS5 index over those documents.
Database triggers update both derived structures in the transaction that
changes the canonical record.
Store verification compares the document projection with the canonical rows
and asks FTS5 to verify the index before a store is published.

The derived search tables are not portable state.
Backup, restore, and board copy operate on canonical records; the same triggers
construct search documents for inserted records.

### Attachments

`attachment_blobs`, `attachments`, and `attachment_uploads` retain logical
attachment metadata and resumable upload state.
Blob descriptors are content-addressed and immutable.
Attachment metadata is board-scoped and immutable except for one monotonic
transition from active metadata to a complete removal tombstone.

An upload retains the maximum size admitted when the upload begins.
Progress offsets cannot move backward, and terminal upload receipts are
immutable until collection.
Committed receipts must identify active attachment metadata with matching
board, origin, filename, actor, size, and optional expected digest.

### Ephemeral coordination

`mailbox`, `subscriptions`, and `leases` are store-wide coordination state;
they do not belong to a project or board.
Application timestamps are stored as Unix seconds.
Expiry timestamps are exclusive, and repository write operations remove mail
and subscriptions at or beyond that boundary before applying a new operation.

Each mailbox delivery has a random durable ID.
The private `local_sequence` preserves local ordering and polling progress.
Direct deliveries have no source topic.
Topic publication records the literal source topic on every subscriber
delivery.
The `(listener, pattern)` primary key gives each listener one subscription per
pattern; refreshing a subscription updates its expiry and preserves its
creation time.
A lease name identifies one externally coordinated resource and therefore has
at most one current owner row.
