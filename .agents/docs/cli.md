# Command-line interfaces

Use this guide when changing command behavior, flags, configuration, output,
or exit status.

## Commands are adapters

A command parses command-line syntax, builds a typed request,
calls a domain operation, and translates the result
into the command's output contract.

Do not make command implementations a second business-logic layer.
State transitions, eligibility, and reusable workflows belong in domain
packages.

Keep parser-specific types at the command boundary.
Do not pass Kong contexts, command structs, raw flag maps,
or arbitrary argument slices into lower layers.

## Typed inputs

Parse external values once at the boundary.
Pass types that carry their invariants into lower layers.

Use a request type when an operation has several invocation values
or the input is likely to grow.
Use `Options` only for optional inputs;
a nil `*Options` means defaults.
Use `Config` only for construction-time values.

## Output contracts

Treat standard output, standard error, and exit status as separate interfaces.
Use standard output for the requested result,
especially data intended for pipes or files.
Use standard error for diagnostics and notices.

Structured output must not contain incidental diagnostics.
When human and structured modes expose the same operation,
derive both renderings from the same domain result.
Do not parse human-readable output when structured output is available.

## Errors and validation

Validate command syntax and representation at the command boundary.
Let the called operation enforce domain invariants.
Do not duplicate domain policy in flag validation.

Return errors to the executable boundary.
Only the executable entry point owns process exit status.

## Verification

Test user workflows at the process boundary when command syntax, streams,
exit status, configuration, or discovery behavior is part of the contract.
Assert standard output, standard error, exit status,
and persisted state separately.

Read `test-script.md` before changing a process-boundary scenario.
