# Use a derived per-record FTS5 issue search index

Date: 2026-09-04
Status: Accepted

## Context

Cardamom stores issue titles, Summary, and Details on the issue row.
State, Result, and immutable Log entries use separate tables and have
independent record lifetimes.
Portable backup and board copy preserve those canonical records.

Full-text search must cover every record without reading all Markdown bodies
for each query.
Search must still identify the field and stable Log ID that produced the best
excerpt.
A single document per issue would lose that provenance and let a large Log
dominate relevance merely because it contains more text.

The Cardamom SQLite driver includes FTS5.
FTS5 supplies tokenization, ranking, prefix queries, and bounded snippets,
but an FTS virtual table is a derived search structure rather than canonical
issue state.

## Decision

Cardamom maintains an `issue_search_documents` projection with one document
for each searchable issue field or record.
Every document records its board, issue, field, optional source record ID,
and searchable body.
State and State snapshot documents combine the body with the optional next
action.

An external-content FTS5 table indexes the document bodies with the
`unicode61` tokenizer.
Database triggers update the document projection and FTS index in the same
transaction as each canonical write.
A schema migration backfills documents for existing stores and rebuilds the
FTS index.
Store verification checks both the canonical-to-document projection and the
FTS index before publishing an opened store.

Search ranks each matching document with FTS5 BM25 and these field weights:

| Field | Weight |
| --- | ---: |
| Title | 8 |
| Summary | 4 |
| Details | 2 |
| Result | 2 |
| State | 1 |
| Log | 1 |

An issue score is the highest weighted score among its matching documents.
The highest-scoring document supplies the excerpt and source record ID.
The result also reports every field with a matching document.
Issue ID breaks equal-score ties.

The derived tables are not part of portable backup or board copy.
Restoring or copying canonical issue records invokes the same triggers that
maintain ordinary writes.

## Consequences

Search reads an index instead of materializing all Markdown bodies.
Per-record documents retain useful result provenance and prevent the number of
Log entries from adding scores together.

Canonical issue records remain the source of truth.
Backups do not become coupled to an SQLite search representation,
and a future migration can rebuild or replace the index from canonical data.

Every canonical write path must preserve the trigger contract.
Store opening fails when the document projection or FTS index is inconsistent,
so corruption is detected before repositories serve search results.

The ranking weights become observable search behavior.
Changing those weights or the tokenizer can reorder results and requires an
intentional compatibility decision.
