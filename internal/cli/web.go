package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	stdhttp "net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
	WebDir    string `name:"web-dir" env:"CLU_WEB_DIR" help:"Path to the web app (the TanStack Start project). Defaults to ./web/clu-web."`
}

func (c *WebCmd) Run(r *runCtx) error {
	webDir, err := resolveWebDir(c.WebDir)
	if err != nil {
		return err
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
	return cmd, nil
}

// resolveWebDir picks the web project directory. Order: explicit flag /
// env (passed in via `chosen`), then ./web/clu-web relative to cwd.
// Errors clearly when nothing's found so users know what to set.
func resolveWebDir(chosen string) (string, error) {
	if chosen != "" {
		abs, err := filepath.Abs(chosen)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "package.json")); err != nil {
			return "", fmt.Errorf("web dir %s has no package.json", abs)
		}
		return abs, nil
	}
	// Default: ./web/clu-web relative to cwd (this repo's layout).
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(cwd, "web", "clu-web")
	if _, err := os.Stat(filepath.Join(candidate, "package.json")); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("could not locate the web project; pass --web-dir or set CLU_WEB_DIR (looked in %s)", candidate)
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

// waitOrKill gives a child up to `grace` to exit on its own (it should,
// because ctx was cancelled and exec.CommandContext sends SIGKILL on
// cancellation), then force-kills if not.
func waitOrKill(cmd *exec.Cmd, grace time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(grace):
		_ = cmd.Process.Kill()
		<-done
		return nil
	}
}
