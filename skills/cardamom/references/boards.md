# Store and board selection

A store is Cardamom's physical persistence directory and database.
The default project-local store directory is `.cardamom`;
`--store` or `CARDAMOM_STORE` may select another directory.
A store may contain multiple projects and boards.
A board is one coordination namespace containing its own description,
issues, graph, and claims.

Store discovery chooses the database.
Board selection chooses the coordination namespace inside that store.
`CARDAMOM_STORE` and `CARDAMOM_BOARD` provide invocation defaults;
explicit `--store` and `--board` flags override them.
When a handoff names a board ID,
or the checkout selection is not established,
pass `--board <board-id>` on every command for that work.

## Inspect existing context

Inspect discovery and board selection without changing either:

```bash
card --actor coordinator --json info
card --actor coordinator --json board list
card --actor coordinator --board <board-id> --json board show
```

`info` requires a selected board and identifies that board's physical store,
project, schema, and effective configuration.
If `board show` cannot resolve one board,
use `board list` to report the available identities and stop unless the user
explicitly asked to select one.

Store-scoped mail and lease operations do not use `info` or board inspection
to establish a board.
Use the supplied `--store`, `CARDAMOM_STORE`, or automatic store discovery;
report ambiguity when those inputs do not establish one store.

Store selection and board selection are separate decisions.
Use `--store /path/to/.cardamom` when discovery has not been established.
Use `--board <board-id-or-exact-name>` for an invocation-scoped board selection
that the user requested.

When unsure whether matching work already exists,
search the selected board and inspect plausible matches:

```bash
card --actor coordinator --board <board-id> --json list \
  --status ready,blocked,in_progress,waiting,closed,cancelled \
  --title '<title-term>' --limit 0 \
  | jq -r '.id'
card --actor coordinator --board <board-id> --json \
  show <candidate-id> --context
```

## Explicit setup

Run initialization, board creation, or persistent board selection only when
the user explicitly requests that operation:

```bash
card --actor coordinator init --board-name "<board-name>"
card --actor coordinator --json board create \
  --project <project-id> "<board-name>"
card --actor coordinator board use <board-id-or-exact-name>
```

Creating or selecting a board does not change the physical store.
Changing the selected board does not select a different mailbox,
subscription namespace, or lease namespace.

## Worktree handoffs

Before dispatching board-scoped work into another worktree,
establish the explicit store and board values that reach the intended context.
Then follow the complete delegated-work checklist in `execution.md`.
