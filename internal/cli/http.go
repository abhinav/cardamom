package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	stdhttp "net/http"
	"time"

	httpsrv "github.com/rovak/clu/internal/http"
	"github.com/rovak/clu/internal/store"
	"github.com/rovak/clu/internal/workflow"
)

// HTTPCmd starts a REST API server backed by the project's store.
// Used by `clu web` (which spawns it as a child process) and is also
// usable on its own for scripting or for pointing a separate frontend
// dev server at a live tracker.
type HTTPCmd struct {
	Bind string `name:"bind" default:"127.0.0.1" help:"Interface to bind. Defaults to loopback; use 0.0.0.0 to share across the LAN."`
	Port int    `name:"port" default:"7777" help:"TCP port. 0 picks a free port and prints it on startup."`
}

func (c *HTTPCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		srv := httpsrv.New(s).WithTemplatesDir(workflow.TemplatesPath(r.dir))
		srv.Start(r.ctx)
		addr := fmt.Sprintf("%s:%d", c.Bind, c.Port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("listen %s: %w", addr, err)
		}
		actual := ln.Addr().String()
		r.notice("clu http listening on http://%s\n", actual)
		// In --json mode the listener detail is the only thing on stdout
		// so a parent process (clu web) can read one line and learn the
		// chosen port (useful when --port=0).
		if r.json {
			_ = r.emitJSON(map[string]any{"addr": actual})
		}
		httpServer := &stdhttp.Server{
			Handler:           srv.Mux(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		// Graceful shutdown: when the run context is cancelled (e.g.
		// SIGINT in main), tell http.Server to drain and stop accepting.
		go func() {
			<-r.ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx)
		}()
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			return err
		}
		return nil
	})
}
