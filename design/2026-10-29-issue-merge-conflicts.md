# Preserve unresolved board edits

Date: 2026-10-29
Status: Accepted

## Context

Board replicas retain semantic mutations and the writer tips observed before
each mutation, as described in the
[mutation history decision](2026-09-16-local-board-mutation-history.md).
Two replicas can independently edit the same issue.
Their histories preserve both edits but do not establish which one is current.
Independent relationship changes can also combine into an invalid graph,
such as two dependency edges that form a cycle.

Current issue projections supply one value per field.
Selecting a provisional winner would let readers and execution policies mistake
a disputed instruction or lifecycle for settled state.
A previous agreed value is also insufficient:
concurrent resolutions can share an unresolved past rather than one agreed value.

## Decision

Merge independent changes and retain incompatible alternatives explicitly.
A disputed field has no current value.
Each alternative retains the identities of its contributing history events;
equal concurrent values share one alternative without losing those identities.
Large alternative bodies load separately from issue collection metadata.

| Operation | Behavior while a conflict remains |
| --- | --- |
| Read, search, or inherit context | Identify disputed content and expose its alternatives. |
| Edit an undisputed field or append Log | Continue under the operation's existing invariants. |
| Replace a disputed field | Require explicit conflict resolution. |
| Automatically claim work | Skip affected issues and report the count with conflict-list guidance. |
| Explicitly claim or change lifecycle | Require the resolved inputs used by that operation. |

A conflict identifier binds its subject, unit, and current causal contenders.
Resolution uses the identifier the user reviewed.
If the local contender set changed, resolution requires a fresh inspection.
Resolution appends ordinary semantic assignments under the current frontier;
it does not erase history or settle edits that have not arrived yet.
Later unseen concurrent edits may reopen a conflict.

Structural conflicts identify the implicated relationships.
Resolution removes selected dependency edges or chooses containment parents,
then validates the retained graph before committing.
It neither chooses an arbitrary edge nor enumerates every possible valid graph.

Execution constraints also cover incompatible kind, lifecycle, and waiting values.
Their review identity includes all alternatives of the implicated units,
including alternatives already disputed as individual fields.
A repair selects a compatible tuple and leaves unrelated conflicts alone.
Restoring an unchanged retained value does not author another assignment.
Inspection retains compact identities;
keeping a waiting value loads only that selected body, and clearing waiting loads none.

Board metadata and board configuration overrides follow the same rule.
An unresolved override differs from a cleared override that inherits a default.
Only operations that need the disputed configuration value are blocked.

Active claims remain local.
Merge releases claims incompatible with the resulting supported issue state
in the same transaction and reports those releases.
Compatible claims survive unrelated content conflicts.
Public-ID collisions use the separate
[reference marker decision](2026-10-29-issue-reference-markers.md).

## Consequences

- Issue reads and mutations must represent partial knowledge explicitly.
  A conflict badge attached to a selected current value is insufficient.
- Conflict inspection and resolution are shared domain operations for CLI
  and browser callers.
- Relational conflict projections, history, and current values commit together.
- Automatic selection remains useful when some work is conflicted,
  and its diagnostics distinguish conflict exclusions from an empty pool.
- Repeated and differently ordered merges must preserve the same alternatives;
  display ordering does not establish authority.
