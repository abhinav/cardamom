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

## Multiple projects

Add another repository or product namespace to the selected store explicitly:

```bash
card --actor coordinator --json project list
card --actor coordinator --json project create \
  --prefix payments- "Payments"
```

`project create` returns the new project ID.
Inspect the project before creating or selecting a board:

```bash
card --actor coordinator --json project show <project-id>
```

`project show` reports the project metadata,
effective configuration through the project layer,
and the project's boards in stable order.
It does not require or change board selection.

Inspect or update the same project configuration directly when no board has
been created or selected:

```bash
card --actor coordinator --json config show --project <project-id>
card --actor coordinator --json config set \
  --project <project-id> --scope project issue.id.strategy sequential
card --actor coordinator --json config unset \
  --project <project-id> --scope project issue.id.strategy
```

Direct project configuration resolves the project by stable ID or exact name.
It ignores ambient board selection,
but cannot be combined with an explicit `--board` target.
`config set` and `config unset` require `--scope project` in this mode.

Use that ID when creating the project's first board:

```bash
card --actor coordinator --json board create \
  --project <project-id> "Payments delivery"
```

Rename an existing project by stable ID:

```bash
card --actor coordinator --json project edit \
  <project-id> --name "Payments platform"
```

`project edit` returns the updated project state.
Renaming preserves the project ID, boards, configuration, and creation time.

Project creation does not create or select a board.
Project names need not be unique,
so use the stable project ID when a name is ambiguous.
Without `--prefix`,
the new project inherits an active store prefix
or stores a prefix inferred from its name.

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

Pin a short ordered set of issues to the selected board,
remove issues from that order,
or inspect the current order:

```bash
card --actor coordinator board pin <issue-id>
card --actor coordinator board unpin <issue-id>
card --actor coordinator board pins
```

`board pin` appends an issue to the order.
Both mutation commands accept `--key`
when the positional argument is an exact producer key instead of an issue ID.

The `board.pins.max_count` configuration key limits the number of pinned
issues and defaults to `8`.
Like other configuration values,
it inherits through built-in, store, project, and board scopes,
with the most specific configured value taking precedence.
Set a board-specific limit with:

```bash
card --actor coordinator config set \
  --scope board board.pins.max_count 12
```

`config unset` at a scope restores inheritance from the next less-specific
configured scope.

Choose a board for the coordination boundary whose description,
issue graph,
and claims should be shared.
That may be a repository,
a release effort,
or another project-specific unit.
Do not use separate boards merely to classify issues;
labels and workstream containment handle classification within one graph.
