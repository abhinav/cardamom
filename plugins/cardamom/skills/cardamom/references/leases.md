# Coordinate an external resource

A claim owns one issue's execution.
A lease owns one named external resource for a bounded interval.
Use both when claimed work also requires exclusive infrastructure.

## Decide whether Cardamom must coordinate the resource

Use a lease when all three conditions hold:

- actors could overlap on the resource;
- overlap could interfere with work, corrupt state, or allocate unsafely; and
- no native lock, allocator, or established coordinator already prevents that
  overlap.

Safe concurrent reads need no lease.
A lease records coordination ownership;
it does not grant credentials, authorization,
or control of the external resource.

Lease names are store-scoped across projects and boards.
Name the resource at its actual exclusivity scope.
Include a host or another qualifier when the resource is local rather than
shared across the whole store.

## Publish intent before acquisition

When an issue has a stable recurring lease requirement,
put the resource scope and acquire-and-release requirement in Details.
Keep current intent, ownership, cleanup status, and recovery action in State.

Publish lease intent before acquisition,
then replace State with successful ownership before resource use:

```bash
card --actor <actor> state set <issue-id> \
  'Migration validation requires exclusive use of staging-db.' \
  --next 'Acquire staging-db for migration validation.'
card --actor <actor> lease acquire staging-db --ttl 30m
card --actor <actor> state set <issue-id> \
  'Lease staging-db is active; inspect resource disposition before recovery.' \
  --next 'Run migration validation.'
```

Use one actor for the full lease lifecycle:

```bash
card --actor <actor> lease renew staging-db --ttl 30m
card --actor <actor> lease release staging-db
```

Choose a TTL that permits timely renewal while bounding stale coordination.
Acquire only for the interval that needs exclusivity,
renew during long operations,
and release after the resource itself is safe for another actor.

## Recover a lease safely

Inspect Cardamom ownership and the external resource separately:

```bash
card --actor <actor> --json lease show staging-db
card --actor <actor> --json lease list
```

Lease expiry or revocation removes Cardamom ownership.
Neither operation proves that the external process stopped or that cleanup
succeeded.
Before reuse, inspect the resource
and publish its observed disposition on the owning issue.

When the active owner is known to be unable to continue
and the resource is safe,
revoke conditionally using the observed owner:

```bash
card --actor <coordinator-actor> lease revoke staging-db \
  --owner <prior-actor> \
  --reason 'The prior actor stopped after staging-db cleanup was verified.'
```

The owner condition preserves a lease that changed hands before revocation.
Record the revocation result on the owning issue.
Use the coordinator's actor for coordinator actions;
actor attribution does not establish recovery authority.

For routines, acquire and release the lease during each awakening
rather than carrying it between runs.
