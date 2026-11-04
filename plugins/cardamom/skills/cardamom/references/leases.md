# Coordinate an external resource

Use a lease when actors could overlap on a shared resource, interference would
be harmful, and no native lock, allocator, or existing coordinator prevents it.
A lease grants coordination ownership, not access.
Name the resource at its exclusivity scope, including a host qualifier for a
host-local resource; lease names span boards within one store.

## Acquire, renew, and release

```bash
card --actor <actor> lease acquire staging-db --ttl 30m
card --actor <actor> lease renew staging-db --ttl 30m
card --actor <actor> lease release staging-db
```

Renew before expiry while protected work continues.
If acquisition fails, leave the resource alone.
If renewal fails or ownership is lost, stop initiating resource actions,
make any already-started operation safe, and inspect lease and resource state
before reacquiring and resuming.
Release only when the resource is safe for another actor.
Expiry or revocation alone does not establish resource safety.

## Recover disputed ownership

Inspect `lease show <resource>` and the resource itself.
An authorized coordinator may revoke after establishing resource safety:

```bash
card --actor <coordinator> lease revoke staging-db \
  --owner <previous-owner> --reason 'The stopped executor left the database safe.'
```

The owner condition protects a lease that changed hands.
Retain the safety evidence and revocation outcome on the issue.
