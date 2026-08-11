# Coordinate an external resource

A claim owns issue execution.
A lease coordinates exclusive use of one named external resource for a bounded
interval.
Use both when claimed work needs exclusivity that the resource itself does not
already enforce.

## Decide whether to lease

Use a lease only when actors could overlap,
overlap could interfere or corrupt state,
and no native lock, allocator, or established coordinator already prevents it.
A lease records coordination ownership;
it grants neither credentials nor control of the resource.

Lease names are store-scoped across boards.
Name the resource at its true exclusivity scope,
including a host qualifier for a host-local resource.

## Gate resource actions on lease ownership

Put a stable lease requirement in Details.
Before acquisition,
put current intent and next action in State.
Use one actor for acquisition, renewal, and release.

```bash
card --actor <actor> state set <issue-id> \
  'Migration validation requires exclusive use of staging-db.' \
  --next 'Acquire staging-db before touching the database.'
card --actor <actor> lease acquire staging-db --ttl 30m
card --actor <actor> state set <issue-id> \
  'Lease staging-db is active; the resource has not yet been modified.' \
  --next 'Run migration validation and keep the lease renewed.'
```

If acquisition fails,
do not touch the resource.
Record the blocked or waiting position and the observed owner when useful.

Renew before the TTL can expire during long operations:

```bash
card --actor <actor> lease renew staging-db --ttl 30m
```

If renewal fails or ownership is lost,
stop initiating resource actions,
make any already-started external operation safe,
inspect both lease and resource state,
publish the recovery position,
and reacquire before resuming.
Do not infer resource safety from lease expiry or revocation.

Release only after the external resource is safe for another actor:

```bash
card --actor <actor> lease release staging-db
```

Completion requires both observed lease ownership for the protected work and a
safe resource disposition before release.

## Recover disputed ownership

Inspect Cardamom and the resource separately:

```bash
card --actor <actor> --json lease show staging-db
```

When the active owner cannot continue,
the coordinator may revoke only after authority is established and resource
safety is verified:

```bash
card --actor <coordinator-actor> lease revoke staging-db \
  --owner <prior-actor> \
  --reason 'The prior actor stopped after staging-db cleanup was verified.'
```

The owner condition preserves a lease that changed hands.
Record the observed safety and revocation result on the owning issue.
For routines,
acquire and release within each awakening.
