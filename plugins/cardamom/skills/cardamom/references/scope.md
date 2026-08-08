# Resolve scope

Projects organize boards but are not a separate execution selection.

## Resolve existing scope

Prefer scope already supplied by the user or handoff.
Cardamom can resolve an existing store through these routes:

1. `--store <path>` for this invocation;
2. `CARDAMOM_STORE` from the environment; or
3. automatic discovery of an existing `.cardamom` store from the checkout.

Automatic discovery is convenient for work that stays in one checkout.
Pass the resolved store explicitly
when another worktree, process, or handoff might discover a different location.

Board-scoped commands can resolve a board through these routes:

1. the owning board inferred from supplied issue IDs;
2. `--board` or `CARDAMOM_BOARD` selection;
3. the checkout binding persisted by `board use`; or
4. the store's sole board.

An explicit board must agree with the board that owns supplied issue IDs.
When more than one board remains possible,
report the stable identities and stop rather than choosing one implicitly.
When a handoff supplies a board ID, pass it explicitly throughout that work.

Store-scoped mail and lease operations stop after store resolution.
Do not resolve or persist a board solely for those operations.

Inspect unresolved scope without changing it:

```bash
card --actor <actor> --json project list
card --actor <actor> --json board list
card --actor <actor> --board <board-id> --json board show
card --actor <actor> --board <board-id> --json info
```

`project list` and `board list` require a store but not a selected board.
`info` describes the effective store, project, board, schema, and configuration
after a board is selected.

Select or create a board only when the user requested that persistent change.

When matching work may already exist,
search the selected board before creating another issue:

```bash
card --actor <actor> --board <board-id> --json list \
  --status ready,blocked,in_progress,waiting,closed,cancelled \
  --title-regexp '<title-regexp>' --limit 0
card --actor <actor> --board <board-id> --json \
  show <candidate-id> --context
```

Continue the issue that owns the same outcome.

## Perform requested setup

Use setup commands only when initialization or persistent selection is part of
the requested outcome:

```bash
card --actor <actor> init --board-name '<board-name>'
card --actor <actor> --json project create \
  --prefix <prefix> '<project-name>'
card --actor <actor> --json board create \
  --project <project-id> '<board-name>'
card --actor <actor> board use <board-id-or-exact-name>
```

Creating a project does not create or select a board.
Creating a board does not change the physical store.
Persisted names may be ambiguous;
use returned project and board IDs for later selection.

## Prepare another worktree

Before dispatching board-scoped work into another worktree,
establish all context that automatic discovery might change:

- the absolute store path;
- the board ID;
- the worker's stable actor;
- the working directory and owned files;
- the intended `card` executable when several builds may exist; and
- the issue ID and required validation.

Verify that the supplied store and board reach the intended context from that
worktree before material execution begins.
