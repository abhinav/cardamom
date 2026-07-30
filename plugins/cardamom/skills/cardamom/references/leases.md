# Resource leases

## Decide before resource use

Before using an external resource that other actors may also use,
identify the resource at the scope where overlap could cause harm.
Use a Cardamom lease when:

- concurrent actors could overlap on that resource;
- the overlap could interfere with work, corrupt state,
    or allocate the resource unsafely; and
- no existing mechanism reliably prevents the overlap.

A resource's native protection or an established coordinator is sufficient
when it serializes access,
rejects a competing attempt before harm,
or assigns ownership unambiguously.
For example,
do not add a Cardamom lease around a complete resource mutation already
protected by an atomic native lock,
or around a sandbox already assigned exclusively by an external allocator.
Concurrent read-only use also needs no lease when the resource supports it
safely.

An issue claim owns work.
A resource lease owns one named external resource for a bounded time.
Use both when claimed work also needs exclusive infrastructure.
A lease does not grant credentials, permissions,
or authority to use the resource.
Establish authorization separately.

Leases are store-scoped.
Resolve the store but do not select a board solely to operate a lease.
Use the same name when the resource is shared across boards,
so work in either board contends for the same lease.
Include a host or another stable qualifier when the resource is local to one
host rather than shared by the whole store.
Separate stores do not coordinate lease ownership.

## Name and record the allocation

Choose a stable resource name at the actual exclusivity scope.
When an issue or routine has a stable recurring lease requirement,
put the resource scope and the acquire-and-release requirement in details.
Keep the current allocation, cleanup status,
and recovery action in state.
Record successful acquisition and material progress or cleanup outcomes
in log entries as those events occur.
Do not put live lease status in details.

For claimed work,
put lease intent in the State body
and acquisition in the optional next action before acquiring the lease.
After acquisition succeeds,
add a log entry and update state with the current allocation
and recovery action as needed.
Keep later progress logs in execution order.

```bash
card --actor worker-a state set <issue-id> \
  "Migration validation requires exclusive use of staging-db." \
  --next "Acquire staging-db for migration validation."
card --actor worker-a lease acquire staging-db --ttl 30m
card --actor worker-a log post <issue-id> \
  "Acquired lease staging-db for migration validation."
card --actor worker-a state set <issue-id> \
  "Lease staging-db is active. Inspect it before release during recovery." \
  --next "Run migration validation."
```

## Acquire and maintain a lease

Use one actor for the full lease lifecycle:

```bash
card --actor worker-a lease acquire staging-db --ttl 30m
card --actor worker-a lease renew staging-db --ttl 30m
card --actor worker-a lease release staging-db
```

Only the active lease owner may renew or release the resource.
Choose a TTL long enough for the owner to renew before expiry,
but short enough to bound stale coordination state.
Renew during long operations and release promptly after the external resource
is actually safe for another owner.

Acquire a lease only for the bounded interval that needs exclusivity.
For a long-lived service,
lease each bounded maintenance operation rather than the service lifetime.
For a routine,
acquire and release the lease during each run rather than carrying ownership
between runs.

## Inspect and recover

Inspect lease state with the same actor identity used for the surrounding work:

```bash
card --actor coordinator --json lease show staging-db
card --actor coordinator --json lease list
```

When the active owner is known to be unable to continue,
a coordinator may revoke the lease after establishing the external resource's
disposition:

```bash
card --actor coordinator lease revoke staging-db \
  --owner worker-a \
  --reason "worker-a stopped after staging-db cleanup was verified"
```

Use the observed owner as `--owner`.
The owner condition preserves an active lease if ownership changed before the
revocation,
and the resulting error reports the current holder.
Use the coordinator's actor identity for attribution
rather than operating as the unavailable owner.
Actor identity does not establish authority;
establish the coordinator's recovery authority separately.

Revocation removes only Cardamom coordination state.
It does not stop, reset, unlock,
or establish the safety of the external resource.
Record the external resource disposition before revocation,
then record the revocation result in the owning issue when one exists.

Lease expiry removes Cardamom ownership,
but it does not prove that the external process, device, or environment was
cleaned up.
Before reusing an expired resource,
inspect the resource itself and the owning issue's recovery evidence.
Record the observed cleanup status and disposition before acquiring a new
lease.

Do not use a lease as a substitute for an issue claim,
and do not keep material resource state only in ephemeral mail.
