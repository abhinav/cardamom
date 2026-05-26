package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	stdhttp "net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	httpsrv "github.com/rovak/clu/internal/http"
	"github.com/rovak/clu/internal/store"
)

// WebCmd boots the local web UI: starts the REST API in-process and
// spawns the TanStack Start server as a child. The two together give
// the user a clickable kanban / list / detail UI at a localhost URL.
//
// In-process API rather than spawning `clu http` separately so we only
// manage one external child (Node) and don't need a port-discovery dance.
type WebCmd struct {
	APIPort   int    `name:"api-port" default:"7777" help:"Port for the REST API. 0 picks a free port."`
	WebPort   int    `name:"web-port" default:"5757" help:"Port for the web server (0 picks a free port). Default 5757 avoids colliding with common dev servers (3000/5173/8080)."`
	Bind      string `name:"bind" default:"127.0.0.1" help:"Interface to bind both servers."`
	Dev       bool   `name:"dev" help:"Use 'pnpm dev' (HMR) instead of the built output. Default tries built output first, falls back to dev."`
	NoBrowser bool   `name:"no-browser" help:"Don't try to open a browser window."`
	WebDir    string `name:"web-dir" env:"CLU_WEB_DIR" help:"Path to the web app. Default search: $CLU_WEB_DIR → ~/.local/share/clu/web → ./web/clu-web."`
	Install   bool   `name:"install" help:"Build the web app (pnpm install + pnpm build) and copy .output/ to ~/.local/share/clu/web. Run once after 'go install'; afterwards bare 'clu web' works from any directory."`
}

func (c *WebCmd) Run(r *runCtx) error {
	if c.Install {
		return installWebApp(r, c.WebDir)
	}
	webDir, err := resolveWebDir(c.WebDir)
	if err != nil {
		return err
	}
	if c.Dev && !hasPackageJSON(webDir) {
		return fmt.Errorf("--dev requires the web source (with package.json); %s is a built-only install. Pass --web-dir to the source tree or omit --dev", webDir)
	}

	// 1. Bring up the API in-process. Listen on the requested port; if
	// it's 0 the OS picks one and we plumb the chosen URL through to
	// the web child via env.
	apiAddr := fmt.Sprintf("%s:%d", c.Bind, c.APIPort)
	apiLn, err := net.Listen("tcp", apiAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", apiAddr, err)
	}
	apiURL := "http://" + apiLn.Addr().String()
	r.notice("api  → %s\n", apiURL)

	// 2. Build a context that cancels when the parent does or when
	// either child exits unexpectedly, so we can tear everything down
	// in one place.
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	// 3. Start the API HTTP server in a goroutine.
	apiErrCh := make(chan error, 1)
	go func() {
		apiErrCh <- runAPIServer(ctx, r, apiLn)
	}()

	// 4. Decide how to start the web app:
	//    - If --dev OR the built output is missing, run `pnpm dev`
	//    - Else run `node .output/server/index.mjs` (production build)
	cmd, err := webCommand(ctx, c, webDir, apiURL)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start web: %w", err)
	}
	r.notice("web  → http://%s:%d  (pid %d)\n", c.Bind, c.WebPort, cmd.Process.Pid)

	webErrCh := make(chan error, 1)
	go func() { webErrCh <- cmd.Wait() }()

	// 5. Wait for the web server to be reachable, then maybe open a
	// browser. waitForHTTP polls /; bound by a 30s ceiling to surface
	// startup failures rather than hang forever.
	webURL := fmt.Sprintf("http://%s:%d", c.Bind, c.WebPort)
	if !c.NoBrowser {
		go func() {
			if waitForHTTP(ctx, webURL, 30*time.Second) {
				openBrowser(webURL)
			}
		}()
	}

	// 6. Block on whichever finishes first (or the user hitting ^C).
	// Whoever does, we cancel the context to drain the other.
	//
	// Cmd.Wait reports "signal: killed/interrupted" when the child
	// dies from a signal we (or the OS process-group SIGINT) sent.
	// That isn't an error to surface — the user asked to quit.
	select {
	case err := <-apiErrCh:
		cancel()
		_ = waitOrKill(cmd, 5*time.Second)
		if err != nil && !shutdownError(err) {
			return fmt.Errorf("api: %w", err)
		}
		return nil
	case err := <-webErrCh:
		cancel()
		<-apiErrCh
		// Web child exiting first usually means SIGINT propagated
		// through the process group; treat the signal exit as normal.
		if err != nil && !shutdownError(err) && !contextDone(ctx) {
			return fmt.Errorf("web: %w", err)
		}
		return nil
	case <-ctx.Done():
		_ = waitOrKill(cmd, 5*time.Second)
		<-apiErrCh
		return ctx.Err()
	}
}

// shutdownError reports whether err is signal-induced child-exit
// noise (SIGINT/SIGKILL/SIGTERM) — these come from us tearing the
// process group down on quit, not a real crash. A non-zero exit
// without a signal is still a real error worth surfacing.
func shutdownError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		// On unix, ProcessState.Sys() is a syscall.WaitStatus; if
		// .Signaled() it died from a signal. We don't reach for the
		// syscall package directly to stay portable — but ExitError's
		// string representation reliably starts with "signal: " in
		// that case across platforms Go supports.
		s := err.Error()
		if len(s) > 8 && s[:8] == "signal: " {
			return true
		}
	}
	return false
}

func contextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// runAPIServer opens the store and serves the REST API on ln until ctx
// is cancelled. Shared shape with HTTPCmd but in-process — no `clu
// http` subprocess to manage.
func runAPIServer(ctx context.Context, r *runCtx, ln net.Listener) error {
	return withStore(r, func(s *store.Store) error {
		srv := httpsrv.New(s)
		httpServer := &stdhttp.Server{
			Handler:           srv.Mux(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			<-ctx.Done()
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(sctx)
		}()
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			return err
		}
		return nil
	})
}

// webCommand builds the *exec.Cmd that runs the frontend. Prefers the
// built output (`node .output/server/index.mjs`) when present; falls
// back to `pnpm dev` otherwise (or always with --dev).
func webCommand(ctx context.Context, c *WebCmd, webDir, apiURL string) (*exec.Cmd, error) {
	env := append(os.Environ(),
		"VITE_CLU_API_URL="+apiURL,
		"PORT="+strconv.Itoa(c.WebPort),
		"HOST="+c.Bind,
	)

	builtEntry := filepath.Join(webDir, ".output", "server", "index.mjs")
	useBuilt := !c.Dev
	if useBuilt {
		if _, err := os.Stat(builtEntry); err != nil {
			useBuilt = false
		}
	}

	var cmd *exec.Cmd
	if useBuilt {
		cmd = exec.CommandContext(ctx, "node", builtEntry)
	} else {
		// pnpm dev. If pnpm isn't on PATH, surface a clear error rather
		// than a confusing fork failure.
		if _, err := exec.LookPath("pnpm"); err != nil {
			return nil, fmt.Errorf("pnpm not found on PATH; either install it or run `pnpm build` and re-run clu web")
		}
		// `--host` forces vite to bind the requested interface. Without
		// it, vite defaults to ::1 only on some setups, which makes
		// http://127.0.0.1:<port> mysteriously fail even though
		// http://localhost:<port> works.
		cmd = exec.CommandContext(ctx, "pnpm", "dev",
			"--port", strconv.Itoa(c.WebPort),
			"--host", c.Bind,
		)
	}
	cmd.Dir = webDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Put the child in its own process group so a single signal can
	// take down the whole tree (pnpm → node → vite workers). Without
	// this, killing the immediate child leaves orphaned grandchildren
	// holding the port — the bug the user hit ("kill 57928 failed:
	// no such process" while clu still won't exit).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

// resolveWebDir picks the web project directory. Search order:
//
//  1. Explicit flag (--web-dir) or env (CLU_WEB_DIR), passed in as `chosen`.
//     Must contain either package.json (source) or .output/server/index.mjs
//     (installed build).
//  2. The installed location (~/.local/share/clu/web or $XDG_DATA_HOME/clu/web).
//     This is what `clu web --install` populates, so a one-time install
//     makes `clu web` work from any directory.
//  3. ./web/clu-web relative to cwd (the dev convenience for running
//     out of a repo checkout).
//
// Errors clearly when nothing's found, pointing the user at --install.
func resolveWebDir(chosen string) (string, error) {
	if chosen != "" {
		abs, err := filepath.Abs(chosen)
		if err != nil {
			return "", err
		}
		if !looksLikeWebDir(abs) {
			return "", fmt.Errorf("web dir %s has neither package.json nor .output/server/index.mjs", abs)
		}
		return abs, nil
	}
	if installed := installedWebDir(); looksLikeWebDir(installed) {
		return installed, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(cwd, "web", "clu-web")
	if looksLikeWebDir(candidate) {
		return candidate, nil
	}
	return "", fmt.Errorf(
		"could not locate the web project. Tried:\n"+
			"  - %s (installed)\n"+
			"  - %s (repo checkout)\n"+
			"Fix: run `clu web --install` inside the agents-clu repo, "+
			"or pass --web-dir / set CLU_WEB_DIR.",
		installedWebDir(), candidate,
	)
}

// installedWebDir returns the path where `clu web --install` stages the
// built web bundle. Honors $XDG_DATA_HOME if set; falls back to
// ~/.local/share/clu/web on every platform (Windows users with this
// directory missing will get a clear error from the install step).
func installedWebDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "clu", "web")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "clu", "web")
}

// looksLikeWebDir reports whether path holds either a source tree
// (package.json) or a built install (.output/server/index.mjs).
func looksLikeWebDir(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, ".output", "server", "index.mjs")); err == nil {
		return true
	}
	return false
}

// hasPackageJSON reports whether path is a web source tree (vs. a
// built-only install dir). Used to gate --dev on having pnpm-able sources.
func hasPackageJSON(path string) bool {
	_, err := os.Stat(filepath.Join(path, "package.json"))
	return err == nil
}

// installWebApp builds the web app (pnpm install + pnpm build) in the
// source tree and copies .output/ to the installed location so that
// future `clu web` invocations work from any directory.
//
// Source tree resolution: --web-dir / CLU_WEB_DIR if set, else
// ./web/clu-web. Requires pnpm on PATH.
func installWebApp(r *runCtx, chosen string) error {
	// Resolve source: must be a source tree (package.json), not an
	// installed bundle. We can't install from a built-only dir.
	src, err := resolveWebSource(chosen)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("pnpm"); err != nil {
		return fmt.Errorf("pnpm not found on PATH; install pnpm first (https://pnpm.io/installation)")
	}

	dest := installedWebDir()
	if dest == "" {
		return errors.New("could not determine install destination: $HOME unset and $XDG_DATA_HOME unset")
	}

	r.notice("source: %s\n", src)
	r.notice("dest:   %s\n", dest)

	// Build steps run in the source tree, inheriting stdio so the user
	// sees pnpm's progress directly.
	for _, step := range [][]string{
		{"pnpm", "install", "--frozen-lockfile"},
		{"pnpm", "build"},
	} {
		r.notice("\n› %s\n", strings.Join(step, " "))
		cmd := exec.CommandContext(r.ctx, step[0], step[1:]...)
		cmd.Dir = src
		cmd.Stdout = r.stdout
		cmd.Stderr = r.stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(step, " "), err)
		}
	}

	output := filepath.Join(src, ".output")
	if _, err := os.Stat(output); err != nil {
		return fmt.Errorf("expected build artefact %s missing after `pnpm build`: %w", output, err)
	}

	// Replace the destination atomically-enough: build under a sibling
	// tmp dir, then rename over. Avoids leaving a half-copied bundle
	// in place if the copy fails partway through.
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}
	staging, err := os.MkdirTemp(filepath.Dir(dest), ".clu-web-install-")
	if err != nil {
		return fmt.Errorf("mkdir staging: %w", err)
	}
	defer os.RemoveAll(staging) // no-op if the rename below succeeds
	if err := copyTree(output, filepath.Join(staging, ".output")); err != nil {
		return fmt.Errorf("copy .output: %w", err)
	}
	_ = os.RemoveAll(dest)
	if err := os.Rename(staging, dest); err != nil {
		return fmt.Errorf("install rename: %w", err)
	}

	r.notice("\n✓ installed clu web at %s\n", dest)
	r.notice("you can now run `clu web` from any directory.\n")
	return nil
}

// resolveWebSource finds the web app source tree (must have
// package.json — a built-only install dir is rejected). Used by
// --install to know where to invoke pnpm.
func resolveWebSource(chosen string) (string, error) {
	if chosen != "" {
		abs, err := filepath.Abs(chosen)
		if err != nil {
			return "", err
		}
		if !hasPackageJSON(abs) {
			return "", fmt.Errorf("--install needs the web source (package.json), got %s", abs)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(cwd, "web", "clu-web")
	if hasPackageJSON(candidate) {
		return candidate, nil
	}
	return "", fmt.Errorf("could not find web source; pass --web-dir or run from inside the agents-clu repo (looked in %s)", candidate)
}

// copyTree recursively copies src → dst, preserving file modes. Both
// must be absolute. Used by --install to stage the built bundle into
// ~/.local/share/clu/web.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// waitForHTTP polls url every 200ms until it returns any HTTP response
// or ctx/timeout expires. Reports true on success.
func waitForHTTP(ctx context.Context, url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &stdhttp.Client{Timeout: 500 * time.Millisecond}
	for {
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		default:
		}
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// openBrowser shells out to the platform's "open" command. Best-effort:
// silently no-ops on unsupported platforms or when the command isn't
// installed.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// waitOrKill tears down the child process group: SIGTERM the whole
// group, give it `grace` to exit cleanly, then SIGKILL the group as a
// last resort. Signaling the negative PID hits every process in the
// group (since we set Setpgid on launch), so pnpm + node + vite all
// go down together. Without this, killing only the immediate child
// leaves grandchildren holding the port and `cmd.Wait()` blocked.
func waitOrKill(cmd *exec.Cmd, grace time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid := cmd.Process.Pid
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// First try: polite TERM to the whole group. node + pnpm usually
	// shut down cleanly on TERM.
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	select {
	case err := <-done:
		return err
	case <-time.After(grace):
		// Hard stop. KILL the group so grandchildren that ignored TERM
		// (or were too busy) also die. cmd.Process.Kill() only hits
		// the immediate child, which is why we send to -pgid.
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
		return nil
	}
}
