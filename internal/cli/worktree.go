package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rovak/clu/internal/config"
)

// WorktreeCmd groups worktree lifecycle subcommands. clu wraps `git
// worktree` so the project's bootstrap recipe (config.yaml's
// `worktree:` section) runs in lock-step with the git operation —
// otherwise every fresh worktree starts as a half-broken filesystem
// tree missing .env, install state, and any per-checkout init.
type WorktreeCmd struct {
	Add       WorktreeAddCmd       `cmd:"" help:"Create a new git worktree (and optionally run the bootstrap recipe)."`
	Bootstrap WorktreeBootstrapCmd `cmd:"" help:"Run the worktree.copy + worktree.commands recipe against an existing worktree."`
	Remove    WorktreeRemoveCmd    `cmd:"" aliases:"rm" help:"Remove a git worktree after checking for uncommitted/unpushed work."`
}

// WorktreeAddCmd wraps `git worktree add`. With --bootstrap it then
// runs the project's `worktree:` recipe so a single command goes from
// "I want a worktree for feat/foo" to "ready to work."
type WorktreeAddCmd struct {
	Path      string `arg:"" help:"Path to create the worktree at (relative or absolute)."`
	Ref       string `arg:"" optional:"" help:"Branch or commit-ish to check out (default: current HEAD)."`
	Branch    string `name:"branch" short:"b" help:"Create a new branch at <ref> for this worktree (passes -b to git worktree add)."`
	Bootstrap bool   `name:"bootstrap" help:"After creating the worktree, run config.yaml's worktree.copy + worktree.commands."`
}

func (c *WorktreeAddCmd) Run(r *runCtx) error {
	if r.json {
		return errors.New("worktree add streams subprocess output; --json is not supported")
	}
	cfg, err := config.Load(r.dir)
	if err != nil {
		return err
	}
	worktreeDir := cfg.Worktree.Dir
	if worktreeDir == "" {
		worktreeDir = config.DefaultWorktreeDir
	}
	dest, usedDefault, err := resolveWorktreePath(r, c.Path, worktreeDir)
	if err != nil {
		return err
	}
	// When the default `<worktreeDir>/<name>` location is used, make
	// sure `<worktreeDir>/` is excluded so the new tree doesn't show
	// up as untracked in `git status`. Writing to `.git/info/exclude`
	// (rather than `.gitignore`) keeps this per-clone — no tracked-file
	// diff to surprise the team, but the entry still survives across
	// secondary worktrees since info/exclude lives in the common .git.
	if usedDefault {
		if added, err := ensureGitExclude(r, worktreeDir+"/"); err != nil {
			r.notice("warning: could not update .git/info/exclude: %v (add `%s/` manually)\n", err, worktreeDir)
		} else if added {
			r.notice("added `%s/` to .git/info/exclude (per-clone; not committed)\n", worktreeDir)
		}
	}
	// Anchor `git worktree add` to the main worktree so it works when
	// the user runs clu from outside the repo via --dir.
	gitDir := mainFromDir(r.dir)
	if gitDir == "" {
		if m, err := mainWorktreePath(); err == nil {
			gitDir = m
		}
	}
	args := []string{"worktree", "add"}
	if gitDir != "" {
		args = append([]string{"-C", gitDir}, args...)
	}
	if c.Branch != "" {
		args = append(args, "-b", c.Branch)
	}
	args = append(args, dest)
	if c.Ref != "" {
		args = append(args, c.Ref)
	}
	r.notice("$ git %s\n", strings.Join(args, " "))
	cmd := exec.CommandContext(r.ctx, "git", args...)
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	if !c.Bootstrap {
		return nil
	}
	return runBootstrap(r, dest)
}

// resolveWorktreePath turns the user's --path argument into an
// absolute path, returning whether the default `<worktreeDir>/<name>`
// location was used (so the caller knows to ensure .gitignore covers
// it).
//
//   - Absolute path → used as-is. usedDefault=false.
//   - Path starting with "./" / "../" or containing a separator →
//     resolved against cwd. usedDefault=false. (User is being explicit.)
//   - Bare name ("test1", "feat-foo") → placed at
//     `<main-worktree>/<worktreeDir>/<name>`. usedDefault=true.
//
// The bare-name default keeps worktrees inside a single, gitignored
// folder so they don't clutter sibling directories or accidentally
// land inside the working tree as untracked files.
func resolveWorktreePath(r *runCtx, raw, worktreeDir string) (string, bool, error) {
	if raw == "" {
		return "", false, errors.New("path required")
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), false, nil
	}
	if strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") || strings.ContainsRune(raw, filepath.Separator) {
		abs, err := filepath.Abs(raw)
		if err != nil {
			return "", false, err
		}
		return abs, false, nil
	}
	// Bare name: place under <main>/<worktreeDir>/<name>. Prefer the
	// main worktree implied by r.dir (which resolveCluDir already
	// resolved to <main>/.clu, including when --dir was passed
	// explicitly). Fall back to a fresh git rev-parse from cwd when
	// r.dir doesn't look like .clu inside a worktree — this covers the
	// "no .clu yet, brand-new project" case.
	if main := mainFromDir(r.dir); main != "" {
		return filepath.Join(main, worktreeDir, raw), true, nil
	}
	if main, err := mainWorktreePath(); err == nil {
		return filepath.Join(main, worktreeDir, raw), true, nil
	}
	// Last resort: cwd-relative without the worktreeDir prefix.
	abs, ferr := filepath.Abs(raw)
	return abs, false, ferr
}

// mainFromDir returns the absolute path of the main worktree implied
// by the given clu directory (which is expected to be `<main>/.clu`).
// Returns "" if r.dir is the bare default ".clu" — that means
// resolveCluDir couldn't find a real .clu and we should fall back to
// the git-driven lookup.
func mainFromDir(dir string) string {
	if dir == "" || dir == ".clu" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return filepath.Dir(abs)
}

// resolveExistingWorktreePath is the bootstrap/remove counterpart to
// resolveWorktreePath. Bare names map to `<main>/<worktreeDir>/<name>`
// — but only if that directory actually exists, so we don't surprise
// users who passed a bare path expecting cwd-relative. If the bare
// candidate is missing, fall back to cwd-relative so the resulting
// error message names the path the user actually typed.
func resolveExistingWorktreePath(r *runCtx, raw string) (string, error) {
	if raw == "" {
		return "", errors.New("path required")
	}
	if filepath.IsAbs(raw) {
		return canonical(filepath.Clean(raw)), nil
	}
	if strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") || strings.ContainsRune(raw, filepath.Separator) {
		abs, err := filepath.Abs(raw)
		if err != nil {
			return "", err
		}
		return canonical(abs), nil
	}
	cfg, err := config.Load(r.dir)
	if err != nil {
		return "", err
	}
	worktreeDir := cfg.Worktree.Dir
	if worktreeDir == "" {
		worktreeDir = config.DefaultWorktreeDir
	}
	// Same dir-driven preference as resolveWorktreePath — honors --dir
	// from outside the repo. Falls back to git lookup for the
	// brand-new project case.
	var main string
	if m := mainFromDir(r.dir); m != "" {
		main = m
	} else if m, err := mainWorktreePath(); err == nil {
		main = m
	}
	if main != "" {
		candidate := filepath.Join(main, worktreeDir, raw)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return canonical(candidate), nil
		}
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	return canonical(abs), nil
}

// canonical resolves symlinks if possible, otherwise returns the
// cleaned input. Used in the resolve-existing path so /tmp/foo and
// /private/tmp/foo (macOS) compare equal downstream.
func canonical(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// ensureGitExclude appends `entry` to the repo's `.git/info/exclude`
// if not already present (literal-line match — globs and comments are
// treated as opaque). The file is per-clone, untracked, and shared
// across secondary worktrees (it lives in the common .git directory,
// found via `git rev-parse --git-common-dir`). Creates the file if it
// doesn't exist. Returns true when the file was actually written to.
func ensureGitExclude(r *runCtx, entry string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if m := mainFromDir(r.dir); m != "" {
		cmd.Dir = m
	}
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git rev-parse: %w", err)
	}
	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" {
		return false, errors.New("git did not return a common-dir path")
	}
	path := filepath.Join(gitDir, "info", "exclude")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == entry {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	body := string(existing)
	if len(body) > 0 && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += entry + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// WorktreeBootstrapCmd applies the config.yaml `worktree:` recipe to an
// already-created worktree. Idempotent enough to re-run after editing
// the recipe — copies overwrite, commands run again.
type WorktreeBootstrapCmd struct {
	Path string `arg:"" help:"Path of the worktree to bootstrap. Must be a git worktree."`
}

func (c *WorktreeBootstrapCmd) Run(r *runCtx) error {
	if r.json {
		return errors.New("worktree bootstrap streams subprocess output; --json is not supported")
	}
	dest, err := resolveExistingWorktreePath(r, c.Path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dest); err != nil {
		return fmt.Errorf("%s: %w", dest, err)
	} else if !info.IsDir() {
		return fmt.Errorf("%s: not a directory", dest)
	}
	if !isGitWorktree(dest) {
		return fmt.Errorf("%s: not a git worktree (run `git worktree add` first, or `clu worktree add --bootstrap` to do both)", dest)
	}
	return runBootstrap(r, dest)
}

// WorktreeRemoveCmd wraps `git worktree remove` with safety checks
// git itself doesn't perform: unpushed commits (the load-bearing one)
// plus a stash warning. Git already refuses to remove a worktree with
// uncommitted changes, but we run that check explicitly so the error
// message is uniform with the others.
type WorktreeRemoveCmd struct {
	Path  string `arg:"" help:"Path of the worktree to remove."`
	Force bool   `name:"force" short:"f" help:"Skip the safety checks and pass --force to git worktree remove."`
}

func (c *WorktreeRemoveCmd) Run(r *runCtx) error {
	if r.json {
		return errors.New("worktree remove streams subprocess output; --json is not supported")
	}
	dest, err := resolveExistingWorktreePath(r, c.Path)
	if err != nil {
		return err
	}
	if !isGitWorktree(dest) {
		return fmt.Errorf("%s: not a git worktree", dest)
	}
	// Refuse to remove the main worktree — git refuses too, but we
	// surface it earlier with a clearer message.
	if main, err := mainWorktreePath(); err == nil && absEqual(main, dest) {
		return fmt.Errorf("%s is the main worktree; can't remove it with `clu worktree remove`", dest)
	}

	if !c.Force {
		if err := worktreeSafetyChecks(r, dest); err != nil {
			return err
		}
	}

	// Run git from inside the target worktree (or the main worktree
	// as fallback) so `git worktree remove` works even when the user
	// invokes clu from outside the repo via --dir.
	gitDir := dest
	if !isGitWorktree(gitDir) {
		if m := mainFromDir(r.dir); m != "" {
			gitDir = m
		}
	}
	args := []string{"-C", gitDir, "worktree", "remove"}
	if c.Force {
		args = append(args, "--force")
	}
	args = append(args, dest)
	r.notice("$ git %s\n", strings.Join(args, " "))
	cmd := exec.CommandContext(r.ctx, "git", args...)
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}
	r.notice("removed %s\n", dest)
	return nil
}

// worktreeSafetyChecks runs the pre-remove safety net.
//
// Only **uncommitted changes** block — those genuinely die with the
// working tree. The branch ref, its commits, and stashes all live in
// the shared .git/ and survive `git worktree remove`, so "no upstream"
// and "unpushed commits" are notices, not errors. Users routinely
// remove throwaway worktrees on never-pushed branches; blocking that
// is friction without payoff.
func worktreeSafetyChecks(r *runCtx, dest string) error {
	// Uncommitted changes (the one blocking check).
	out, err := exec.Command("git", "-C", dest, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("git status in %s: %w", dest, err)
	}
	if len(bytesTrimSpace(out)) > 0 {
		return fmt.Errorf("%s has uncommitted changes:\n%s\ncommit or stash them, or pass --force", dest, indentLines(string(out), "  "))
	}

	// Upstream state — informational. The branch ref survives the
	// worktree removal either way; we just tell the user what's on it
	// so they don't accidentally forget a branch with unpushed work.
	branchOut, _ := exec.Command("git", "-C", dest, "rev-parse", "--abbrev-ref", "HEAD").Output()
	branch := strings.TrimSpace(string(branchOut))
	if _, upErr := exec.Command("git", "-C", dest, "rev-parse", "--symbolic-full-name", "@{u}").Output(); upErr != nil {
		r.notice("note: branch %q has no upstream — the branch ref survives, but it's only on this machine\n", branch)
	} else if logOut, err := exec.Command("git", "-C", dest, "log", "@{u}..HEAD", "--oneline").Output(); err == nil && len(bytesTrimSpace(logOut)) > 0 {
		r.notice("note: branch %q has unpushed commits (survive in .git/, but not yet on the remote):\n%s", branch, indentLines(string(logOut), "  "))
	}

	// Stashes are repo-global (stored in the main .git/), so we can't
	// say "your stash" vs "someone else's stash" reliably. Notice only.
	stashOut, _ := exec.Command("git", "-C", dest, "stash", "list").Output()
	if len(bytesTrimSpace(stashOut)) > 0 {
		r.notice("note: repo has stashes (shared across worktrees; they survive worktree removal):\n%s", indentLines(string(stashOut), "  "))
	}
	return nil
}

// bytesTrimSpace is strings.TrimSpace for []byte without an alloc-y
// conversion when checking for "is this empty?"
func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// indentLines prefixes every non-empty line in s with pad. Used for
// error messages that embed git output.
func indentLines(s, pad string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, ln := range lines {
		if ln == "" {
			continue
		}
		lines[i] = pad + ln
	}
	return strings.Join(lines, "\n") + "\n"
}

// runBootstrap is the body shared by `add --bootstrap` and `bootstrap`.
// Resolves the main worktree (= source for copies), loads the config,
// then steps through copy → commands fail-fast.
func runBootstrap(r *runCtx, dest string) error {
	source, err := mainWorktreePath()
	if err != nil {
		return fmt.Errorf("locate main worktree: %w", err)
	}
	// Refuse to bootstrap a worktree onto itself — would copy a file
	// over itself and run commands twice in the same dir.
	if absEqual(source, dest) {
		return errors.New("source and destination resolve to the same path — bootstrap must target a different worktree")
	}
	cfg, err := config.Load(r.dir)
	if err != nil {
		return err
	}
	if len(cfg.Worktree.Copy) == 0 && len(cfg.Worktree.Commands) == 0 {
		r.notice("no worktree.copy or worktree.commands configured in %s; nothing to do\n", config.Path(r.dir))
		return nil
	}
	r.notice("bootstrapping %s (source: %s)\n", dest, source)
	for _, p := range cfg.Worktree.Copy {
		src := filepath.Join(source, p)
		dst := filepath.Join(dest, p)
		if err := copyWorktreeFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", p, err)
		}
		r.notice("  copied %s\n", p)
	}
	for _, cmdLine := range cfg.Worktree.Commands {
		r.notice("  $ %s\n", cmdLine)
		c := exec.CommandContext(r.ctx, "sh", "-c", cmdLine)
		c.Dir = dest
		c.Stdout = r.stdout
		c.Stderr = r.stderr
		c.Stdin = os.Stdin
		if err := c.Run(); err != nil {
			return fmt.Errorf("command %q failed: %w", cmdLine, err)
		}
	}
	r.notice("bootstrap complete\n")
	return nil
}

// resolveCluDir picks the project directory for this invocation.
//
// Order:
//  1. If --dir / CLU_DIR resolved to something other than the bare
//     default ".clu", trust the user.
//  2. If "./.clu" exists in cwd, use it (fast path — cwd IS the main
//     worktree, or the user has the project in cwd directly).
//  3. Else try git: if cwd is inside a git repo and the main worktree
//     has a "./.clu", use <main-worktree>/.clu. This is the path that
//     makes secondary worktrees share state automatically.
//  4. Fall back to the original default so `clu init` in a brand-new
//     project still works.
//
// The git step only fires when "./.clu" is missing, so the common case
// (running from the main worktree) doesn't pay the rev-parse cost.
func resolveCluDir(dir string) string {
	if dir != ".clu" || os.Getenv("CLU_DIR") != "" {
		return dir
	}
	if info, err := os.Stat(".clu"); err == nil && info.IsDir() {
		return dir
	}
	main, err := mainWorktreePath()
	if err != nil {
		return dir
	}
	candidate := filepath.Join(main, ".clu")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return dir
}

// mainWorktreePath returns the absolute path to the *main* worktree of
// the repo containing cwd. For non-bare repos, that's the parent of
// the common .git directory.
func mainWorktreePath() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" {
		return "", errors.New("git did not return a common-dir path")
	}
	return filepath.Dir(gitDir), nil
}

// isGitWorktree reports whether dest is the root of a git worktree (any
// kind — primary or secondary). Cheap: `git rev-parse --is-inside-work-tree`
// run with -C <dest>.
func isGitWorktree(dest string) bool {
	cmd := exec.Command("git", "-C", dest, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// copyWorktreeFile copies src → dst, creating parent dirs, preserving
// the file mode. Overwrites dst if present (the recipe is "make sure
// this file is here," so freshening from source is fine). Symlinks in
// the source are dereferenced.
func copyWorktreeFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s: directory copies are not supported — list files individually", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// absEqual reports whether two paths resolve to the same filesystem
// location after cleaning + EvalSymlinks (best-effort — falls back to
// Clean comparison if EvalSymlinks fails on either side).
func absEqual(a, b string) bool {
	ra, ea := filepath.EvalSymlinks(a)
	rb, eb := filepath.EvalSymlinks(b)
	if ea == nil && eb == nil {
		return ra == rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
