# Stores, projects, boards, and setup patterns

A store is the physical persistence boundary selected by `--store`,
`CARDAMOM_STORE`,
or automatic discovery.
A project is the repository or product namespace within that store.
A board is an explicitly created shared coordination context within a project.
Boards do not nest,
and every issue belongs to one board.

| Scope | Owns |
| --- | --- |
| Store | Physical database, projects, actor mailboxes, topic subscriptions, and resource lease names |
| Project | Repository or product namespace inside a store |
| Board | Description, issues, graph relationships, claims, records, and attachments |

Store selection and board selection are independent.
A shared store can hold several projects and boards
without merging their issue graphs.

## Default project-local setup

`card init` creates a `.cardamom` store in the current project by default.
It also creates the project namespace and a first board named after the project
directory unless `--board-name` overrides the name
or `--no-board` skips board creation.
The project issue prefix defaults to a normalized form of the project directory
basename.
An active store prefix takes precedence over inference,
and `--prefix` sets a project override.
Repeated initialization keeps the existing project prefix
unless `--prefix` explicitly replaces it.

```bash
card --actor coordinator init
```

Use this layout when one repository owns the coordination state
and agents can discover it from the repository path.

## External or user-level store

Select an external store when several checkouts need one physical coordination
boundary or when the store should not live in the repository:

```bash
export CARDAMOM_STORE="$HOME/.local/share/cardamom/team"
card --actor coordinator init
card --actor coordinator --json info
```

With an explicit store,
the current working directory still supplies the project identity
and inferred first-board name.
Use a stable actor name across the claim, record, result, release,
mail, and lease operations that one process owns.

## Linked Git worktrees

Store selection precedence is `--store`,
then `CARDAMOM_STORE`,
then automatic discovery.
Automatic discovery searches ancestor directories
and shares the common store across linked Git worktrees.

This lets an isolated worktree use the repository's existing project and boards
without copying the database into the worktree.
Pass `--store` and `--board` explicitly in delegated work
when ambient discovery or checkout selection is not part of the handoff.

## Multiple boards

Store configuration lives at `config.yaml` inside the resolved physical store.
The default store therefore uses `.cardamom/config.yaml`,
while a redirect uses `config.yaml` in the redirect target.
Initialization excludes the database and attachment blobs from the local Git
checkout without excluding `config.yaml`.

Board selection is separate from store selection.
Use `--board` or `CARDAMOM_BOARD` for one invocation,
or persist the selection for the current checkout:

```bash
card --actor coordinator board create "Release 1.5"
card --actor coordinator board use "Release 1.5"
card --actor coordinator --json board show
```

Without an explicit selection,
`card` uses the checkout selection or the store's sole board.
When several boards are eligible and none is selected,
board-scoped commands fail instead of guessing.
Creating or selecting a board never changes the physical store.

Choose a board for the coordination boundary whose description,
issue graph,
and claims should be shared.
That may be a repository,
a release effort,
or another project-specific unit.
Do not use separate boards merely to classify issues;
labels and workstream containment handle classification within one graph.
