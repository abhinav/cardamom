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
	args := []string{"worktree", "add"}
	if c.Branch != "" {
		args = append(args, "-b", c.Branch)
	}
	args = append(args, c.Path)
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
	dest, err := filepath.Abs(c.Path)
	if err != nil {
		return err
	}
	return runBootstrap(r, dest)
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
	dest, err := filepath.Abs(c.Path)
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
