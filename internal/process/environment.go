package process

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	"go.abhg.dev/cardamom/internal/cli"
)

// SystemConfig reads process-owned environment, stream, identity, and clock
// inputs for one invocation.
func SystemConfig(args []string) (Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("get current directory: %w", err)
	}
	currentUser, err := user.Current()
	if err != nil {
		return Config{}, fmt.Errorf("get current user: %w", err)
	}
	clock, err := environmentClock(os.Getenv("CARDAMOM_NOW"))
	if err != nil {
		return Config{}, err
	}
	stdinIsTTY := false
	if info, statErr := os.Stdin.Stat(); statErr == nil {
		stdinIsTTY = info.Mode()&os.ModeCharDevice != 0
	}
	return Config{
		Version: cli.Version, Args: args, CWD: cwd,
		DefaultActor: currentUser.Username,
		Stdin:        os.Stdin, StdinIsTTY: stdinIsTTY,
		Stdout: os.Stdout, Stderr: os.Stderr,
		DisableGitIgnore: os.Getenv("CARDAMOM_NO_GITIGNORE") != "",
		Clock:            clock,
	}, nil
}

// Main runs the operating-system process with interrupt and termination
// cancellation.
func Main() int {
	cfg, err := SystemConfig(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	return Execute(ctx, cfg)
}

func environmentClock(value string) (Clock, error) {
	if value == "" {
		return systemClock{}, nil
	}
	instant, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("CARDAMOM_NOW %q: %w", value, err)
	}
	return fixedEnvironmentClock{instant: instant}, nil
}

type fixedEnvironmentClock struct{ instant time.Time }

func (c fixedEnvironmentClock) Now() time.Time { return c.instant }
