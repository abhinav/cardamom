# Resolve scope

Resolve only the persistence scope the selected operation needs.
Projects organize boards;
they are not a separate issue-execution selection.

## Resolve an existing store and board

Use scope supplied by the user or handoff.
A store resolves, in precedence order, from `--store`, `CARDAMOM_STORE`,
or discovery of an existing `.cardamom` store from the checkout.
Pass an absolute store path explicitly when another process or worktree might
discover a different store.

Issue and attachment commands also resolve a board from:

1. the board that owns a supplied issue ID;
2. `--board` or `CARDAMOM_BOARD`;
3. the checkout binding installed by `board use`; or
4. the store's sole board.

An explicit board must agree with supplied issue IDs.
If several boards remain possible,
report their stable IDs and stop instead of choosing one.
Mail and leases stop after store resolution;
do not select a board solely for those operations.

Inspect unresolved scope without changing it:

```bash
card --actor <actor> --json project show <project-id>
card --actor <actor> --json config show --project <project-id>
card --actor <actor> --json board list
card --actor <actor> --board <board-id> --json info
```

`project show` combines project configuration and board inventory.
`config show --project` works before the project has a board.
`info` describes effective configuration after a board is selected.

## Perform only requested setup

Initialization, creation, and persisted selection change durable scope.
Use them only when setup is part of the requested outcome:

```bash
card --actor <actor> init --board-name '<board-name>'
card --actor <actor> --json project create --prefix <prefix> '<project-name>'
card --actor <actor> --json board create --project <project-id> '<board-name>'
card --actor <actor> board use <board-id-or-exact-name>
```

Creating a project does not create or select a board.
Creating a board does not change the physical store.
Use returned IDs rather than a possibly ambiguous name.

## Cross a worktree boundary explicitly

Give a worker the absolute store path, board ID, stable actor, issue ID,
working directory and owned files, intended `card` executable,
and required validation.
Verify that this scope reaches the intended context from the target worktree
before primary work begins.
