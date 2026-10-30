# Preserve ambiguous issue references after ID collisions

Date: 2026-10-29
Status: Accepted

## Context

An issue has an immutable private UID and a public ID used in commands and text.
Public IDs are unique throughout a store, including across boards.
Relationships identify issues by UID, so changing a public ID does not change
the issue or its relationships.

[Local cloning](2026-10-21-local-board-cloning.md) preserves issue identities
and [mutation history](2026-09-16-local-board-mutation-history.md).
Separate replicas can then create different issues with the same public ID.
Merging those replicas must preserve both issues and their existing references.
Choosing one issue to keep the ID would silently assign ambiguous references
to that issue.

## Decision

Every distinct issue involved in a public-ID collision receives a replacement ID.
The original ID becomes a permanent reference marker listing the candidates.
A marker is not an executable issue and cannot acquire claims or relationships.
Reading it shows the candidates without selecting one.
Existing text references continue to identify the marker;
users can replace them with a candidate's new ID when the intended issue is known.

Each replacement appends the issue's full UID in hexadecimal to the retired ID.
This choice depends on identity, not arrival order or the destination allocator.
If a replacement itself collides, the same rule applies to that ID.
A later participant in the original collision receives its own replacement;
it does not change the earlier participants' replacements.

Collision evidence records the retired ID and each candidate's UID and logical
board ID.
Candidate membership only grows.
Each affected board retains the evidence so transferring either board preserves
the original reference's meaning.
The same transaction renames locally present participants and reserves the old
ID, including when participants belong to different destination boards.

Marker reads follow local candidates by UID and display their current public IDs.
A candidate whose board is absent remains visible as unavailable locally.
Transferring collision evidence does not transfer unrelated boards or issue bodies.
Clone carries marker associations alongside its shared projection and history.

## Consequences

- Public-ID collisions do not require choosing a winner or rewriting old text.
- A merge of one board can rename issues on another destination board.
- Replacement IDs are longer than ordinary allocated IDs.
- Retired IDs remain reserved, and marker associations consume persistent space.
- Issue inspection must distinguish a store-scoped marker from a board-owned
    issue before applying board selection.
- Portable formats must preserve the meaning of retained references.
