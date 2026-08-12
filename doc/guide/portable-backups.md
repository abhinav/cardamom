# Portable backups

`card board copy` preserves the board's pin order and effective pin limit,
and rewrites each pin to the copied issue ID.

`card backup` writes complete boards to one portable archive.
Each included board carries its complete board data
and every committed attachment file,
including its pin order and effective pin limit.

## Choose boards to back up

By default,
`card backup` uses the selected board:

```bash
card --actor operator backup cardamom-backup
```

Repeat `--include-board` to select complete boards by stable ID
or exact name:

```bash
card --actor operator backup release-backup \
  --include-board "Release planning" \
  --include-board board_74b8c15f
```

Use `--all` to include every board in the selected store:

```bash
card --actor operator backup full-store-backup --all
```

## Restore an archive

`card restore` loads every board in the archive.
Use `--store` to select an existing destination store
or a path for a new store:

```bash
card --actor operator \
  --store "$HOME/.local/share/cardamom/restored" \
  restore full-store-backup
```

Restore retains each archived project's identity.
Restored boards retain the archived pin order and pin limit.
Cardamom adds the archived projects and boards
without replacing unrelated data already in the destination.
If the destination has the same project identity with incompatible metadata,
the restore fails instead of combining the projects.

## Retry a restore

Restores are safe to retry.
Run the same `card restore` command again after an interruption.
Cardamom resumes with boards that remain to be restored
and reports boards that were restored previously,
without creating duplicate copies.
