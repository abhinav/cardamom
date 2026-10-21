# Clone boards without changing shared identities

Date: 2026-10-21
Status: Accepted

## Context

Each stored board has a logical board ID and a local writer UID.
The [mutation history decision](2026-09-16-local-board-mutation-history.md)
retains semantic events, writer chains, and their observed frontiers.
A frontier identifies the writer tips known before a mutation.
The current relational projection remains the read model.

Independent board copy assigns new logical identities and starts new history.
That operation cannot create replicas whose later edits share an origin.
Local cloning provides those replicas before merge or remote transport exists.

## Decision

Clone creates a board in another local store while preserving shared identity
and the source's retained history.
It copies the current projection directly; it does not replay history.
One retained source snapshot supplies the projection, history, and attachment
descriptors so they represent the same committed state.

| Preserved | Local to the destination |
| --- | --- |
| Logical board ID and issue identities | New writer UID |
| Name, description, archive state, board overrides | Project placement and inherited defaults |
| Issues, relationships, records, pins, attachments | Projection revisions and Log sequence numbers |
| Mutation bytes, writer tips, historical attachment content | Claims, assignees, leases, mail, unfinished uploads |

The new writer starts with no mutations and no tip.
Its first mutation has sequence one and observes all imported writer tips.
Every writer that has retained mutations must still have its final tip.
Creating a synthetic baseline would falsely introduce cloned entities again,
so clone does not author one.

Clone requires random issue ID allocation in both replicas.
It transfers board overrides rather than resolved configuration;
source store and project defaults remain local.
It rejects existing destination boards and identity collisions,
instead of reconciling or renaming them.

Destination projection, history, and blob references commit together.
Verified attachment files are published while the destination writer
transaction prevents collection from racing publication.
Failures before commit leave no destination board.
Files published before a failure may remain as collectible orphans.

## Consequences

- Replicas retain a common history while recording later changes under
    different writer identities.
- Local reads continue to use relational projections without a replay engine.
- Store verification and mutation append support a new writer with no tip
    when the board has retained history from other writers.
- Cloning cannot duplicate a board within one store or refresh a replica.
    Merge, conflict resolution, and remote transport remain separate work.
