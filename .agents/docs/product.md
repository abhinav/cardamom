# Product concepts

Use this guide when changing projects, boards, issues, relationships, claims,
records, or their user-visible semantics.

## Storage and scope

A store is the physical persistence and replication boundary.
It has no workflow meaning.

A project is the logical repository or product namespace within a store.
A board is an explicitly created shared coordination context within a project.
Boards do not nest.

Every issue belongs to one board.
Store selection and board selection are independent.
Do not infer workflow scope from the selected database path.

Actor mailboxes, topic subscriptions, and resource lease names belong to the
store rather than to one board.
An actor may receive mail sent while working in another board in the same
store.
A resource lease coordinates the named resource across every board and project
in the store,
so the lease name must identify the resource at its actual exclusivity scope.
Separate stores are separate coordination namespaces.

## Issues

The issue types are `workstream`, `task`, `checkpoint`, and `routine`.

Workstreams are nestable containers that may contain any issue type.
Routines are persistent, nestable workstreams
that acquire custody only by direct claim.
Ready and blocked pools exclude routines.

Workstreams, tasks, and routines are executable.
Workstreams and routines close explicitly after every direct child is closed or
cancelled.
A routine result is optional.

## Lifecycle and custody

Issue lifecycle has three persisted states: `open`, `closed`, and `cancelled`.

Claim custody is separate durable state with at most one active claim per issue.
Presentation may derive `in_progress` from an open issue's active claim,
but mutations use claim operations rather than storing `in_progress`.

## Relationships and metadata

Dependencies control readiness.
Containment records tree membership and inherited context;
containment does not substitute for a dependency.

Labels are inert metadata used for filtering and grouping.
Actor identity records custody and authorship.
Neither labels nor actor identity changes eligibility.

## Records

The title is the issue's one-line identity.
The summary is the concise stable issue contract inherited by descendants.
Details are optional expanded stable Markdown disclosed on demand.
State is the issue's persistent mutable active memory:
the current operative facts, progress, choices, constraints, and blockers,
with an optional planned next action.
Its persistence supports handoff and recovery across claims.
Log entries are immutable chronological records,
including committed State snapshots and standalone posts.
Replacing State moves the active working position forward
without preserving the displaced version.
Committing State snapshots its current version in Log
while replacing or clearing the active State.
The result is the current durable outcome.

Preserve these distinctions in command, protocol, domain, repository,
and web behavior.
