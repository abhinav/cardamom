// Package storelocation resolves the filesystem directory that owns a
// Cardamom store.
package storelocation

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	storeName    = ".cardamom"
	databaseName = "board.sqlite3"
)

var errNotGitRepository = errors.New("not inside a supported Git repository")

// Resolve selects an existing store from an explicit path, ancestor .cardamom
// entries, or the current Git repository's shared worktree locations.
// Relative explicit paths use cwd as their base.
// In a linked worktree, an entry below the worktree root remains authoritative,
// while the checked-out root entry yields to an existing common-directory or
// primary-worktree store. Outside that case, the nearest ancestor entry
// remains authoritative.
func Resolve(explicit, cwd string) (string, error) {
	if explicit != "" {
		return resolveStorePath(explicit, cwd)
	}

	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve current directory %q: %w", cwd, err)
	}
	ancestor, err := findAncestorStore(absCWD)
	if err != nil {
		return "", err
	}

	git, err := inspectGitWorktree(absCWD)
	if err != nil && !errors.Is(err, errNotGitRepository) {
		return "", fmt.Errorf("inspect Git worktree: %w", err)
	}
	if ancestor != "" && (err != nil || !git.linked || storeBelowWorktreeRoot(ancestor, git.root)) {
		return resolveStorePath(ancestor, filepath.Dir(ancestor))
	}
	if err == nil {
		commonStore := filepath.Join(git.commonDir, storeName)
		if _, statErr := os.Lstat(commonStore); statErr == nil {
			return resolveStorePath(commonStore, git.commonDir)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect Git common-directory store %q: %w", commonStore, statErr)
		}
		if git.linked {
			primaryStore := filepath.Join(git.primaryRoot, storeName)
			if _, statErr := os.Lstat(primaryStore); statErr == nil {
				return resolveStorePath(primaryStore, git.primaryRoot)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return "", fmt.Errorf("inspect primary-worktree store %q: %w", primaryStore, statErr)
			}
		}
	}
	if ancestor != "" {
		return resolveStorePath(ancestor, filepath.Dir(ancestor))
	}
	return "", fmt.Errorf("no Cardamom store found from %q", absCWD)
}

func findAncestorStore(cwd string) (string, error) {
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, storeName)
		if _, err := os.Lstat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect store path %q: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
	}
}

func storeBelowWorktreeRoot(store, root string) bool {
	relative, err := filepath.Rel(root, filepath.Dir(store))
	if err != nil || relative == "." || relative == ".." {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// InitTarget selects the store directory that init creates. An explicit store
// wins over the optional project path; otherwise init creates PATH/.cardamom.
func InitTarget(explicit, projectPath string) (string, error) {
	if explicit != "" {
		return filepath.Clean(explicit), nil
	}
	if projectPath == "" {
		return storeName, nil
	}
	return filepath.Join(filepath.Clean(projectPath), storeName), nil
}

// DatabasePath returns the canonical backing-file path inside a store.
func DatabasePath(storeDir string) string {
	return filepath.Join(storeDir, databaseName)
}

// resolveStorePath accepts either a store directory or a redirect file that
// contains one path.
// Relative redirect targets use the redirect file's directory as their base.
func resolveStorePath(path, base string) (string, error) {
	path, err := absoluteFrom(path, base)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("store %q does not exist", path)
		}
		return "", fmt.Errorf("inspect Cardamom store %q: %w", path, err)
	}
	if info.IsDir() {
		return path, nil
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("store path %q is neither a directory nor a redirect file", path)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Cardamom store redirect %q: %w", path, err)
	}
	target := strings.TrimSpace(string(body))
	if target == "" {
		return "", fmt.Errorf("store redirect %q is empty", path)
	}
	if strings.ContainsAny(target, "\r\n") {
		return "", fmt.Errorf("store redirect %q must contain one path", path)
	}
	target, err = absoluteFrom(target, filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("resolve Cardamom store redirect %q: %w", path, err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("store redirect %q points to missing directory %q", path, target)
		}
		return "", fmt.Errorf("inspect Cardamom store redirect target %q: %w", target, err)
	}
	if !targetInfo.IsDir() {
		return "", fmt.Errorf("store redirect %q points to non-directory %q", path, target)
	}
	return target, nil
}

func absoluteFrom(path, base string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	return filepath.Clean(path), nil
}

type gitWorktree struct {
	gitDir      string
	commonDir   string
	primaryRoot string
	root        string
	linked      bool
}

func inspectGitWorktree(cwd string) (gitWorktree, error) {
	cmd := exec.Command(
		"git", "-C", cwd, "rev-parse", "--path-format=absolute",
		"--git-dir", "--git-common-dir", "--show-toplevel",
	)
	out, err := cmd.Output()
	if err != nil {
		return gitWorktree{}, errNotGitRepository
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3 {
		return gitWorktree{}, fmt.Errorf("git returned %d worktree paths, want 3", len(lines))
	}
	gitDir := filepath.Clean(lines[0])
	commonDir := filepath.Clean(lines[1])
	root := filepath.Clean(lines[2])
	linked := gitDir != commonDir
	primaryRoot := root
	if linked {
		primaryRoot, err = gitPrimaryWorktree(cwd)
		if err != nil {
			return gitWorktree{}, err
		}
	}
	return gitWorktree{
		gitDir:      gitDir,
		commonDir:   commonDir,
		primaryRoot: primaryRoot,
		root:        root,
		linked:      linked,
	}, nil
}

// BoardBindingPath returns the checkout-private file used for board selection.
// Git checkouts use the current worktree's private administrative directory.
// Non-Git checkouts use a sidecar beside the nearest discovered .cardamom entry, or
// the current directory when no local entry exists.
func BoardBindingPath(cwd string) (string, error) {
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve current directory %q: %w", cwd, err)
	}
	git, err := inspectGitWorktree(absCWD)
	if err == nil {
		return filepath.Join(git.gitDir, "cardamom-board"), nil
	}
	if !errors.Is(err, errNotGitRepository) {
		return "", fmt.Errorf("inspect Git worktree: %w", err)
	}
	store, err := findAncestorStore(absCWD)
	if err != nil {
		return "", err
	}
	if store != "" {
		return filepath.Join(filepath.Dir(store), ".cardamom-board"), nil
	}
	return filepath.Join(absCWD, ".cardamom-board"), nil
}

func gitPrimaryWorktree(cwd string) (string, error) {
	cmd := exec.Command("git", "-C", cwd, "worktree", "list", "--porcelain", "-z")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	const prefix = "worktree "
	field, _, _ := strings.Cut(string(out), "\x00")
	if !strings.HasPrefix(field, prefix) || len(field) == len(prefix) {
		return "", errors.New("git returned no primary worktree")
	}
	return filepath.Clean(strings.TrimPrefix(field, prefix)), nil
}
