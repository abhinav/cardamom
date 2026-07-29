# Development workflow

## Task guides

Read every local guide that applies before editing that kind of code or prose.
Apply the guide while designing and implementing the change,
then inspect the finished work against the same guide before handoff.

| Task | Guide |
| --- | --- |
| Product concepts and behavior | `.agents/docs/product.md` |
| Package boundaries, dependencies, and APIs | `.agents/docs/design.md`, `.agents/docs/layout.md` |
| Repository interfaces and persistence ownership | `.agents/docs/repository.md` |
| SQL, store lifetime, and migrations | `.agents/docs/database.md` |
| Go implementation and organization | `.agents/docs/style.md` |
| Code comments and symbol documentation | `.agents/docs/comments.md` |
| Command behavior, flags, and output | `.agents/docs/cli.md` |
| Tests | `.agents/docs/testing.md` |
| Testscript scenarios | `.agents/docs/test-script.md` |
| Web layout, components, and styles | `.agents/docs/web.md` |
| Cardamom skill maintenance | `.agents/docs/skill.md` |
| Documentation and other external prose | `.agents/docs/docs.md` |
| Branches, commits, and pushes | `.agents/docs/git-workflow.md` |

Before changing files or Git state, verify the working directory
and preserve unrelated dirty, staged, and untracked files.

## Quick reference

| Operation | Command |
| --- | --- |
| Generate protocol and repository code | `mise run generate` |
| Format source | `mise run fmt` |
| Run every lint check | `mise run lint` |
| Build the production binary and web assets | `mise run build` |
| Run all tests | `mise run test` |
| Run all tests with race detection | `mise run test --race` |
| Run one test script | `go test ./cmd/card -run 'TestScript/$name' -count=1` |
| Generate reader documentation | `mise run docs` |
| Run live web development | `mise run web:dev` |

Prefer the smallest command that exercises the changed behavior during the
development loop.

## Development loop

For a feature or bug fix:

1. Add or update the test that establishes the intended behavior.
    For a bug, verify that the regression test fails before the fix.
2. Implement the smallest coherent change that satisfies the behavior.
3. Run the narrowest relevant test.
4. Repeat until the bounded behavior is correct.

The development loop is edit plus narrow test.
Formatting, linting, building, and broad tests belong to finalization.

## Finalization

Before handing off or accepting a code change:

1. Run `mise run fmt`.
2. Run `mise run lint` and fix every finding.
    Root must not accept a change that fails this command.
3. Run the relevant package tests and process-boundary test scripts.
4. Run `mise run build` when the executable, protocol, generated code,
    frontend, or asset pipeline changed.
5. Run any additional generation or documentation task required by the
    changed source.

Manual exercise is additional evidence, not part of the core loop.
Use a real-boundary probe when automated coverage cannot establish a material
external contract.

Do not change generated or embedded assets by hand.
The root `README.md` is generated from `doc/readme/README.md`
and its linked sources.
The top-level `USER_GUIDE.md` is generated from `doc/guide/README.md`
and its linked sources.
Edit those source files and run `mise run docs`
instead of editing either generated file.

## Repository orientation

Read `.agents/docs/layout.md` for package ownership
and `.agents/docs/product.md` for Cardamom domain concepts.
Read `.agents/docs/repository.md` and `.agents/docs/database.md`
before changing persistence behavior.

Keep command and Connect packages limited to parsing, protocol translation,
and rendering.
Domain operations own product policy.
Repository implementations own persistence and transaction boundaries.
`internal/process` owns process lifetime and dependency composition.

Read `.agents/docs/skill.md` before changing `skills/cardamom`.
