# Git workflow

Use this guide when changing Git branches, commits, stacks, rebases, pushes,
or pull requests.

## Preserve user state

Do not treat incidental repository state as part of the task.
Dirty files, staged hunks, untracked files,
and mixed staged or unstaged paths may be intentional.

Inspect only the state needed for the requested operation.
Stage only intended files.
Report unrelated state instead of rearranging it.

## Branches and commits

Use `git-spice` for branch, stack, and commit operations.
Do not create commits with raw `git commit`.
Pass `--no-prompt` to every `git-spice` command.

Create a branch and its first commit with
`git-spice branch create --no-prompt <branch> -m <message>`.
Create another commit on a tracked non-trunk branch with
`git-spice commit create --no-prompt -m <message>`.

Every commit message has an imperative subject of at most 72 characters
and a non-empty body.
Keep every body line within 72 characters and use semantic line breaks.

Before committing, confirm the current branch.
Do not commit new work directly on trunk
unless the user explicitly names trunk as the target.

After a branch or commit operation,
verify the current branch, latest commit, and stack state.

## Publishing

A local commit and a pull request update are separate operations.
Do not push, publish, submit, or update a pull request
unless the user requests that action.

If the user requests only a local commit, leave remote state unchanged.

## Recovery

If a `git-spice` operation is interrupted,
inspect the current branch, rebase state, and stack state before continuing.
Use `git-spice ls --no-prompt` to reconcile stack metadata.
Continue a resolved `git-spice` rebase with
`git-spice rebase continue --no-prompt --no-edit`.

Do not use destructive Git commands
unless the user explicitly requested the destructive operation
or approved the recovery step.
