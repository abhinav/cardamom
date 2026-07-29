package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

// Config describes one public web listener and its injected Connect handler.
type Config struct {
	// Bind is the network address on which the public listener accepts requests.
	// An empty value binds to 127.0.0.1.
	Bind string

	// Port is the public listener port. Zero selects an available port.
	Port int

	// NoBrowser prevents Run from opening the application after readiness.
	NoBrowser bool

	// Notice receives the public application address after the listener binds.
	Notice io.Writer

	// HandlerPath is the common prefix of every Connect procedure route.
	HandlerPath string // required

	// Handler dispatches generated Connect procedures under HandlerPath.
	Handler http.Handler // required

	// AttachmentContentPath is the raw attachment route prefix.
	AttachmentContentPath string // required

	// AttachmentContent dispatches raw attachment GET and HEAD requests under
	// AttachmentContentPath.
	AttachmentContent http.Handler // required
}

// Run serves the embedded browser application and the injected Connect handler
// on one listener until cancellation or an unexpected server exit.
func Run(ctx context.Context, cfg Config) error {
	if len(embeddedApplicationArchive) == 0 {
		return errors.New("web assets not found; run an asset-bearing build")
	}
	handler, err := newApplicationHandler(
		embeddedApplicationArchive,
		cfg.HandlerPath,
		cfg.Handler,
		cfg.AttachmentContentPath,
		cfg.AttachmentContent,
	)
	if err != nil {
		return err
	}
	return runSharedLifetime(ctx, cfg, handler, nil)
}

// developmentChild represents the Vite process participating in the same
// lifetime as the public HTTP listener. Done has one consumer: the runtime.
type developmentChild interface {
	Done() <-chan error
	Stop(context.Context) error
}

// runSharedLifetime binds before reporting readiness, then treats the server,
// optional child, and caller context as one lifetime. The first exit signal
// shuts down the remaining participant before the operation returns.
func runSharedLifetime(
	ctx context.Context,
	cfg Config,
	handler http.Handler,
	child developmentChild,
) error {
	bind := cfg.Bind
	if bind == "" {
		bind = "127.0.0.1"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(cfg.Port)))
	if err != nil {
		return errors.Join(
			fmt.Errorf("listen on %s: %w", net.JoinHostPort(bind, strconv.Itoa(cfg.Port)), err),
			stopChild(child),
		)
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serverDone := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverDone <- err
	}()

	address := publicAddress(bind, listener.Addr())
	notice := cfg.Notice
	if notice == nil {
		notice = io.Discard
	}
	if _, err := fmt.Fprintf(notice, "web application available at %s\n", address); err != nil {
		return errors.Join(fmt.Errorf("write readiness notice: %w", err), stopLifetime(server, child))
	}
	if !cfg.NoBrowser {
		if err := openBrowser(address); err != nil {
			return errors.Join(err, stopLifetime(server, child))
		}
	}

	var childDone <-chan error
	if child != nil {
		childDone = child.Done()
	}
	select {
	case <-ctx.Done():
		return errors.Join(ctx.Err(), stopLifetime(server, child))
	case err := <-serverDone:
		if err == nil {
			err = errors.New("server stopped unexpectedly")
		}
		return errors.Join(fmt.Errorf("web server exited: %w", err), stopChild(child))
	case err := <-childDone:
		if err == nil {
			err = errors.New("process stopped unexpectedly")
		}
		return errors.Join(
			fmt.Errorf("web development process exited: %w", err),
			stopServer(server),
		)
	}
}

func publicAddress(bind string, address net.Addr) string {
	host := bind
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	port := address.(*net.TCPAddr).Port
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}).String()
}

func stopLifetime(server *http.Server, child developmentChild) error {
	return errors.Join(stopServer(server), stopChild(child))
}

func stopServer(server *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return errors.Join(err, server.Close())
	}
	return nil
}

func stopChild(child developmentChild) error {
	if child == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return child.Stop(ctx)
}

func openBrowser(address string) error {
	executable := "xdg-open"
	arguments := []string{address}
	switch runtime.GOOS {
	case "darwin":
		executable = "open"
	case "windows":
		executable = "rundll32"
		arguments = []string{"url.dll,FileProtocolHandler", address}
	}
	command := exec.Command(executable, arguments...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("open browser at %q: %w", address, err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release browser process: %w", err)
	}
	return nil
}
