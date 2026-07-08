# Design

Use this guide when changing package boundaries, domain responsibilities,
dependencies, constructors, or cross-package APIs.

For Cardamom-specific boundaries, also read the guide that owns the affected system:

- `product.md` for projects, boards, issues, claims, and relationships.
- `layout.md` for the repository's package map and dependency direction.
- `repository.md` for persistence ownership and repository contracts.
- `database.md` for SQL, migrations, and store lifetime.

## Package responsibility

Organize packages by domain responsibility.
Each package should expose cohesive operations in its own vocabulary.

Avoid packages that collect unrelated helpers, declaration kinds,
or framework wiring without owning a domain.
Organize files by granular responsibility,
not by declaration kind or arbitrary size.

## Dependency direction

Keep external configuration and infrastructure at system boundaries.
Command and protocol packages adapt external syntax to domain operations.
Domain packages own policy and state transitions.
Infrastructure packages implement consumer-defined contracts.

Lower layers must not depend on command parser types,
generated transport requests, process environment lookups,
or caller-specific output formats.

## Cohesive operations

Prefer a deep operation that owns load, decision, mutation, and outcome
over a shallow layer that forwards calls or makes callers coordinate another
package's internals.

Expose finite product operations.
Do not expose a generic query or mutation language when callers use only a
small number of stable combinations.

## Dependencies and lifetime

Put values where their lifetime matches the abstraction.
Values fixed for an object lifetime belong on the object.
Invocation values belong on a request or method parameter.

Do not pass a super-configuration or generic ports bundle through the system.
Store direct required collaborators in private fields.

Use a constructor when construction validates, normalizes, selects behavior,
or acquires a resource.
Do not add a constructor only to copy fields.

When construction needs several required inputs,
use a `Config` type and mark required fields with `// required`.
Use `Options` only for optional values.

## Interfaces

Define an interface where the consumer needs behavioral substitution.
Keep the interface near that consumer and include only the operations it calls.
Accept the narrow interface and return a concrete implementation.

Do not introduce an interface only to hide one implementation
or make unrelated dependencies look uniform.

## Boundary types

Prefer typed request and result values at package boundaries.
Keep map-shaped, database-shaped, and transport-shaped representations inside
their owning boundaries.

Name a type for its role.
Use names such as `Request`, `Options`, `Config`, `Snapshot`, `State`,
or a specific domain term when that is what the value represents.
Do not use `Facts` as a type name in this codebase.
Name the value for the role it performs.

Parse external values into invariant-carrying types once.
Treat an invalid value created only by Cardamom code as a programming defect,
not as user-correctable input.

## Error boundaries

Expose caller-meaningful error kinds from the operation that owns the contract.
Do not leak an infrastructure error and require callers to re-enter its package
only to classify it.

Reserve sentinel errors for conditions callers match with `errors.Is`
or `errors.As`.
Wrap failures with context for the immediate sub-operation.
