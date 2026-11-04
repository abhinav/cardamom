# Select the intended store and board

A store resolves from `--store`, then `CARDAMOM_STORE`, then an existing
`.cardamom` store discovered from the checkout.
Use an absolute store path across processes or worktrees to keep discovery
tied to the same work.

Issue and attachment commands resolve a board from the supplied issue ID,
an explicit `--board` or `CARDAMOM_BOARD`, the checkout's `board use` binding,
or the store's sole board, in that order.
An explicit board must agree with supplied issue IDs.
Mail and leases need only a store.

When several boards remain possible, inspect them and ask for the intended one:

```bash
card --actor <actor> --store <store-path> --json board list
card --actor <actor> --store <store-path> --board <board-id> --json info
```

Create stores, projects, boards, or persisted checkout bindings when setup is
part of the requested work; use `init`, `project`, and `board` help for commands.
Pass the absolute store path, board or issue ID, actor, and working location
when handing work to another process or worktree.
