# Testing

Use this guide when adding, changing, deleting, or reviewing tests.

## Regression tests

A bug fix requires a regression test that fails before the fix
and passes after the fix.
Demonstrate the failure against the uncorrected behavior before presenting the
repair as complete.

Prefer a unit test when the behavior can be exercised directly.
Use a test script when the behavior is a command workflow, process contract,
store-discovery scenario, or other end-to-end interaction.

When SQLite, the filesystem, Git discovery, or process behavior matters,
prefer a real-boundary test over a mock that proves only that a helper was
called.

## Useful tests

A useful test protects an observable promise: what callers can rely on,
what users can see, which state transition must occur, or what an error means.

Avoid assertions tied only to private implementation shape.
If a different correct implementation would fail the test,
the test may be coupled to mechanics.

Delete a test when it protects no meaningful contract
or only repeats a contract that stronger coverage already exercises.
Make that decision when ordinary work exposes the redundancy;
do not retain low-value tests for a later test audit.

Keep the scenario visible in the test body.
Use helpers when they name real setup or assertion operations.
Do not use helpers only to hide required fields, mock construction,
or assertions.

## Assertions and context

Use Testify in new tests and tests whose assertions are materially rewritten.
Use `require` only when failure makes later assertions unsafe or meaningless.
Use `assert` for independent conditions so one failure does not hide the rest.

Use `t.Context()` when a test needs a context.
Use deferred cleanup that can report a failure without preventing later
cleanup.

## Test organization

Use table tests for simple scenarios that share setup and teardown.
Do not put `func` fields in test tables.
Use subtests when scenarios need distinct setup or enough local detail that a
table would obscure the behavior.

Name top-level tests with GoCase symbols:

```text
Test{Name}
Test{Type}_{Method}
Test{Name}_{scenario}
```

Name every `t.Run` subtest in CamelCase.
Place test helpers below the tests that use them.

Read `comments.md` when a changed test helper, fixture, type, field,
or setup block owns a material contract or invariant.

## Mocks

Use `go.uber.org/mock/gomock` only when a real dependency would make a test
slower, less deterministic, or unable to isolate the behavior under test.

Generate mocks with a documented `//go:generate mockgen` directive.
Use typed mocks and keep the directive next to the consumer contract:

```go
//go:generate mockgen -destination mocks_test.go -package issue -typed . Changes
```

Create the controller with `gomock.NewController(t)`.
Do not call `Finish` explicitly;
the controller registers its own cleanup.

Format multi-part expectations so the operation, arguments,
and result remain visible.

## Test scripts

Process-boundary scenarios live in `testdata/script`
and run through the root package's `TestScript`.
Read `test-script.md` before adding or changing one.
