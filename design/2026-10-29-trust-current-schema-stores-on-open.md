# Trust current-schema stores on open

Date: 2026-10-29
Status: Accepted
Supersedes: [Use a derived per-record FTS5 issue search index](2026-09-04-01-derived-issue-search-index.md) (startup verification requirement only)

## Context

Cardamom commands open a local SQLite store before running their operations.
The store records its schema version in a migration ledger.
Opening applies pending migrations and rejects versions newer than the binary supports.

Opening also audits the whole database, including SQLite pages, foreign keys,
search documents, the full-text index, and retained board mutation history.
The [search-index decision](2026-09-04-01-derived-issue-search-index.md)
requires search verification before an opened store is published.
These audits grow with stored content even for commands that read only the board catalog.
Large stores can therefore exhaust short command timeouts before the requested operation starts.

Database constraints, triggers, and repository transactions maintain data invariants during writes.
A schema version establishes compatibility with the binary;
it does not certify the integrity of every stored record.
Routine commands need compatibility without paying for a complete integrity audit.

## Decision

Opening a current-schema store trusts its stored data without a separate validation pass.
Existing-store recognition, schema compatibility checks,
connection policy, and forward migrations remain part of opening.

Full verification remains after an invocation applies migrations
and after fresh-store initialization completes.
Normal repository operations retain their constraints and transaction guarantees.
They report errors encountered while accessing the data they need.

This decision replaces only the startup verification requirement
in the search-index decision.
Its document representation, trigger maintenance, ranking,
and backup behavior remain in force.

## Consequences

Routine startup no longer scans unrelated records, indexes, or mutation history.
Its verification cost does not grow with the amount of issue content.
Initialization and schema upgrades can still require a full scan.

Corruption outside the data used by an operation may remain undetected.
Successful opening establishes schema compatibility, not database integrity.
There is no user-facing integrity-check command in this change.

Each migration commits separately before the final verification runs.
A verification failure does not undo committed migrations.
If the schema is current on a later invocation,
opening trusts it rather than repeating the failed audit.
