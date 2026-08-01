package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strconv"
	"time"
)

const developmentProbeInterval = 25 * time.Millisecond

// DevelopmentConfig describes a Vite child process served through Config's
// public listener. Connect requests stay in-process; all other requests proxy
// to Vite so browser traffic uses one origin.
type DevelopmentConfig struct {
	// Config defines the public listener and injected Connect handler.
	Config

	// WebDir is the pnpm project directory. An empty value uses web.
	WebDir string

	// Stdout receives output from the Vite child process.
	Stdout io.Writer

	// Stderr receives diagnostics from the Vite child process.
	Stderr io.Writer
}

// RunDevelopment starts Vite with hot reload and serves it beside the injected
// Connect handler on one public listener.
func RunDevelopment(ctx context.Context, cfg DevelopmentConfig) error {
	if err := validateBackendHandlers(
		cfg.HandlerPath,
		cfg.Handler,
		cfg.AttachmentContentPattern,
		cfg.AttachmentContent,
	); err != nil {
		return err
	}

	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("reserve Vite port: %w", err)
	}
	backendPort := backendListener.Addr().(*net.TCPAddr).Port
	if err := backendListener.Close(); err != nil {
		return fmt.Errorf("release reserved Vite port: %w", err)
	}

	webDir := cfg.WebDir
	if webDir == "" {
		webDir = "web"
	}
	child, err := startDevelopmentChild(webDir, backendPort, cfg.Stdout, cfg.Stderr)
	if err != nil {
		return err
	}
	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(backendPort)),
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	handler := composeApplicationHandler(
		cfg.HandlerPath,
		cfg.Handler,
		cfg.AttachmentContentPattern,
		cfg.AttachmentContent,
		proxy,
	)
	if err := waitForDevelopmentBackend(
		ctx,
		target,
		child,
		probeDevelopmentBackend,
	); err != nil {
		return err
	}
	return runSharedLifetime(ctx, cfg.Config, handler, child)
}

// waitForDevelopmentBackend keeps backend startup in the same lifetime as the
// child process. Public readiness may proceed only after a probe succeeds;
// the probe must return when its context is canceled.
func waitForDevelopmentBackend(
	ctx context.Context,
	target *url.URL,
	child developmentChild,
	probe func(context.Context, *url.URL) error,
) error {
	probeCtx, cancelProbe := context.WithCancel(ctx)
	probeDone := make(chan error, 1)
	go func() {
		probeDone <- pollDevelopmentBackend(probeCtx, target, probe)
	}()

	select {
	case err := <-probeDone:
		cancelProbe()
		if err != nil {
			return errors.Join(err, stopChild(child))
		}
		return nil
	case err := <-child.Done():
		cancelProbe()
		<-probeDone
		if err == nil {
			err = errors.New("process stopped unexpectedly")
		}
		return fmt.Errorf("web development process exited before readiness: %w", err)
	case <-ctx.Done():
		cancelProbe()
		<-probeDone
		return errors.Join(ctx.Err(), stopChild(child))
	}
}

// pollDevelopmentBackend retries connection failures until Vite accepts a
// request or startup is canceled.
func pollDevelopmentBackend(
	ctx context.Context,
	target *url.URL,
	probe func(context.Context, *url.URL) error,
) error {
	ticker := time.NewTicker(developmentProbeInterval)
	defer ticker.Stop()
	for {
		if err := probe(ctx, target); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// probeDevelopmentBackend treats any HTTP response as readiness because the
// Vite process has accepted and answered a request at that point.
func probeDevelopmentBackend(ctx context.Context, target *url.URL) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("create Vite readiness request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	return response.Body.Close()
}

// commandChild owns the one Wait call for a development process. Stop signals
// the process tree and waits on the same completion result.
type commandChild struct {
	command *exec.Cmd
	done    chan error
}

// startDevelopmentChild translates the development operation into pnpm's
// command boundary and establishes one owner for cmd.Wait.
func startDevelopmentChild(
	webDir string,
	port int,
	stdout io.Writer,
	stderr io.Writer,
) (*commandChild, error) {
	command := exec.Command(
		"pnpm", "--dir", webDir, "dev",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--strictPort",
	)
	command.Stdout = stdout
	command.Stderr = stderr
	configureDevelopmentCommand(command)
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Vite from %q: %w", webDir, err)
	}

	child := &commandChild{command: command, done: make(chan error, 1)}
	go func() {
		child.done <- command.Wait()
	}()
	return child, nil
}

func (c *commandChild) Done() <-chan error {
	return c.done
}

func (c *commandChild) Stop(ctx context.Context) error {
	select {
	case <-c.done:
		return nil
	default:
	}
	if err := terminateDevelopmentCommand(c.command); err != nil {
		return err
	}
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		killErr := killDevelopmentCommand(c.command)
		<-c.done
		return errors.Join(ctx.Err(), killErr)
	}
}
