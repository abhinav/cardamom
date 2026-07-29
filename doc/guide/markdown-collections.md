# Markdown collections

`card dump` writes a one-way Markdown view of one board.
The generated files are designed for browsing or committing to a repository;
they are not an interchange or import format.

Dump the whole board to create or update a collection:

```bash
card --actor publisher dump docs/board
```

A whole-board dump writes `README.md` and one canonical page for every issue,
including open, in-progress, closed, and cancelled issues.
All issue pages live at `issues/<id>.md`.

Select one or more issues with repeatable `--issue` flags.
Each flag also accepts comma-separated IDs:

```bash
card --actor publisher dump docs/board \
  --issue cardamom-release \
  --issue cardamom-docs,cardamom-review
```

Each selected issue includes all of its containment descendants by default.
Use `--no-descendants` to include only the issue IDs named by `--issue`:

```bash
card --actor publisher dump docs/board \
  --issue cardamom-release \
  --no-descendants
```

An issue-selected dump updates only the selected canonical pages.
It does not write `README.md`,
and it preserves existing pages for issues outside the selection.
This makes repeated commands an additive collection for the same board.

Every generated file carries private YAML frontmatter under `cardamom`.
The metadata records the owning board,
the generated file identity,
and a digest of the Markdown body so `card` can recognize later modifications.
Unowned files in the destination remain outside the collection.

By default,
`card dump` refuses to replace a recognized generated file whose body changed
after generation.
Use `--force` to repair that modified generated file from the board:

```bash
card --actor publisher dump docs/board \
  --issue cardamom-release \
  --force
```

`--force` does not override ownership or path safety.
It cannot replace unowned files,
files generated from another board,
or files at unsafe or noncanonical paths.
