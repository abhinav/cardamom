# Retain local board mutation history

Date: 2026-09-16

Status: Accepted

## Context

Cardamom stores each board's current state in relational tables.
These projections support local reads and edits,
but they do not retain the semantic mutations that produced the state.
Each semantic mutation records what changed in Cardamom terms,
such as a board name, issue field, relationship, or attachment.
Future board merge and synchronization work needs those mutations
without deriving them from differences between snapshots.

Every board already has two identities with different scopes:

- `boards.id` identifies the logical board.
  A future clone will preserve this identity.
- `board_replica_identities.writer_uid` identifies one stored replica.
  A future clone will assign a new writer UID to the new replica.

Active claims and assignment custody coordinate work within one replica.
They are not part of the shared board state.

## Decision

Cardamom retains an append-only local mutation journal for every board.
The current relational projection remains the read model.
Each replica keeps a writer tip that identifies its last complete mutation.
Each mutation contains bounded payload parts followed by a terminal marker.

```text
writer tip
    |
    v
terminal marker ---- previous ----> prior marker
    |
    +---- payload digest ----> bounded parts ----> semantic events
    |
    +---- observed frontier ----> writer tips known before the mutation
```

One repository transaction performs all of these actions:

- update the relational projection;
- append the mutation payload in bounded parts;
- append a terminal marker that commits those parts; and
- advance the local writer tip to that marker.

The marker digest covers the mutation identity, previous marker digest,
observed writer tips, event count, part count, and payload digest.
The observed writer tips form the mutation's causal frontier:
the tips that the writer knew before it created the mutation.
The repository advances the writer tip only after it retains and verifies
the complete mutation.

Journal format V1 records typed semantic events.
Origin events contain a complete board or issue value.
Later events record one field assignment or one relationship change,
so concurrent changes to unrelated values remain independent.

V1 includes:

- board settings and archive state;
- issue identity, fields, lifecycle, waiting state, and records;
- relationships and pins; and
- attachment metadata and tombstones.

V1 excludes active claims, assignees, leases, and mail.

Existing boards receive one baseline mutation during migration.
The migration streams the current shared projection through the same V1 events
that later mutations use.
The migration transaction writes bounded parts and one terminal marker
with an empty observed frontier.
The baseline is not a separate snapshot format.

This decision does not define replay, merge, conflict resolution, cloning,
remotes, or transport.

## Consequences

- Later merge work can exchange retained semantic mutations instead of
  comparing complete board snapshots.
- Mutation history consumes additional storage.
- To let attachment history outlive the current attachment metadata,
  blob collection must treat `board_mutation_blob_refs`
  as retained references.
- Repository operations that change shared board state must append one
  mutation in the same transaction.
- Store verification must reject incomplete parts, invalid marker digests,
  invalid observed frontiers, broken writer chains, and tips that do not
  identify the final retained mutation.
