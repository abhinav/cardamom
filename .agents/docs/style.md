# Go style

Use this guide when changing non-generated Go code.
Read `comments.md` before changing named concepts, comments,
or code whose invariants need explanation.

## Locality and organization

Keep a concept near its operations, request and result types,
private interfaces, and supporting helpers.
Organize files by domain responsibility,
not declaration kind or arbitrary size.

Order symbols by narrative dependency.
Keep a type declaration, constructor, and methods together.
Place operation-specific request, result, mode, interface,
and helper declarations near that operation.

## Naming

Use one stable term for one domain concept.
Do not vary names for style.
Use singular package names.
Name a package for the domain concept or capability it owns,
not a collection of declarations.
Avoid stuttering between a package name and its exported identifiers:
prefer `issue.Kind` to `issue.IssueKind`.

Name values for their actual role.
Do not use `Facts` as a type name in this codebase.
Avoid generic names such as `Data` or `Info`
when `Request`, `Options`, `Config`, `Snapshot`, `State`,
or a domain term states the contract.

Receiver names must be short and consistent for the type.
Use one to three letters derived from the type name.

Avoid import aliases unless Go requires one or two imported packages have the
same package name.

## Public APIs

An exported function or method may use only exported types in its parameters
and results.
This is an immutable Go API rule.
If callers need the value, export and document the type.
If callers do not need the operation as public API,
make the function or method private.

Every package must have package documentation.
Every exported type, constant, variable, function, method, and field
must have useful Go documentation.

## Receivers

Use a pointer receiver unless the method requires a copy of the receiver.
Small size alone does not justify a value receiver.

Methods on immutable scalar value types may use value receivers when copying is
part of the value semantics.

## Required fields

Mark every required struct field inline with `// required` when callers or
dependency injection construct the value directly.

Do not add a constructor only to copy required fields.
Use a constructor when it establishes an invariant or behavior.

## String literals

Use raw string literals with backticks for quote-heavy text when possible.
Keep an interpreted string when escapes or embedded backticks are clearer.

## Control flow

Use early returns for invalid and failure states.
Converge successful branches before shared work.
Model mutually exclusive behavior as a named mode,
not combinations of booleans.

Keep mutation close to the operation that depends on it.
Prefer a visible state-transition sequence over a compressed expression when
the sequence is easier to verify.

Extract a helper when it names a cohesive operation,
removes meaningful duplication, or isolates a real boundary.
Do not hide short policy decisions behind single-use helpers.

## Errors and exits

Use `err` for sequential operation failures.
Introduce another error variable only when multiple errors must remain
independently readable.

Use `internal/must` only for invariants owned entirely by Cardamom code.
Assert an error separately from its result,
then return the parsed or constructed value explicitly.
Do not introduce a generic helper that accepts and returns a value alongside
an error.
User input,
persisted state,
and external-system failures must return errors instead of panicking.

Wrap errors with context for the immediate sub-operation.
Use `%q` for variable strings so empty values and whitespace remain visible.
Do not use string matching to detect a typed error condition.

Do not call `log.Fatal`, `os.Exit`, or another hard exit outside `main`.
