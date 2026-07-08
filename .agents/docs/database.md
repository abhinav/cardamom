# Database and migrations

Use this guide when changing SQL, the store lifecycle, schema verification,
or migrations.

## Store lifetime

Create one process-lifetime store in `internal/process`.
Repository implementations receive that store and hide its physical database
from domain callers.

The repository backing file is the source of truth.
Preserve transaction boundaries whose partial completion would expose an
invalid issue graph or incoherent projection revisions.

Document configuration parameters with their behavior, defaults,
and operational reason.
Document non-obvious connection, locking, journal,
and verification behavior where maintainers must preserve it.

## SQL ownership

Runtime SQL belongs only in the repository package that owns the affected
persisted state.
Keep SQLite types and implementation details out of domain contracts.
The shared sqlc target reads this migration directory as its schema and
generates low-level operations beneath `internal/repository/internal/query`.
Select the columns required by each repository operation;
select a complete table row only when that operation needs the complete
persisted representation.

Generated Go is production source and remains a low-level repository detail,
not a repository or product API.
Authored SQL and SQL fixtures remain inside the owning repository package.
Only `_test.sql` fixtures may inspect several table families for a test-only
contract.

## Migrations

Timestamped SQL files under `internal/repository/store/migrations` define the
schema.
Name each migration `YYYYMMDDHHmmss_description.sql`.

Migrations are append-only after release.
Apply each migration and its history record in one transaction.
Embed, order, checksum, and apply migrations from the store package.

Migration code owns historical representations locally.
Do not import current product types to decode old persisted state;
current domain rules may change independently.

Document migration SQL like implementation code.
Explain table purpose, cross-table invariants,
and constraints that are not obvious from the statements.

## Verification

Store-wide verification may inspect several table families before a store is
published.
Comment each verification phase with the invariant it establishes and why the
phase belongs at the store boundary.

Repository owner tests should prove operation-level persistence contracts.
Store tests should prove migration, connection,
and store-wide integrity behavior.
