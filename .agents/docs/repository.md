# Repository boundaries

Use this guide when changing persistence interfaces, repository packages,
or an operation that must be atomic.
Also read `database.md` for SQL and migration rules.

## Product operations

Repository implementations serve Cardamom's finite product operations.
Do not expose a generic query builder, mutation bag, transaction callback,
or persistence scope to a domain caller.

Define the persistence interface in the domain package that consumes it.
Keep each interface limited to the operations that consumer calls.
Return domain outcomes only after the repository transaction has ended.

One repository may implement several consumer interfaces when those operations
share one consistency boundary.

## Transaction boundary

The board repository owns issue-graph consistency.
Keep load, policy decision, projection writes, revision publication,
and commit in one repository operation.

Do not expose `store.View`, `store.Change`, SQL transactions,
or transaction callbacks to domain operations.
Preserve no-op publication behavior and committed revision semantics.

## Persistence ownership

The production repository packages are:

| Package | Owned persisted state |
| --- | --- |
| `store` | Schema migrations, store state, revision allocation, connections, and store-wide integrity. |
| `project` | Projects and boards. |
| `board` | Issue graph, claims, records, external keys, and projection revisions. |
| `attachment` | Attachment metadata, upload receipts, and blob lifecycle. |
| `mail` | Mailbox and subscriptions. |
| `lease` | Resource leases. |
| `information` | Store-wide composition of identity and inventory reads. |

The board repository may read a board description for issue context and dump
projections.
Standalone board settings mutations belong to the project repository.

Only repository packages may name persistence scope types.
Domain, CLI, and Connect packages must not import repository implementations.
Generated query records remain inside the repository implementation that maps
them to domain values.

## Organization

Organize repository files by product workflow, not SQL verb, table,
declaration kind, or generic read/write grouping.

Keep SQL statements in the package that owns the persisted state.
Name sqlc source files and operations with their repository owner,
then organize each source file by the owning repository workflow.
All authored repository SQL joins one target that generates the private
`internal/repository/internal/query` package.
Repository owner tests may inspect related persisted state when needed to prove
an operation's atomic contract.

The information package may pass one retained store view to finite owner-local
inventory operations;
information itself does not issue SQL.
