package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rovak/beadsv2/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(cli.Run(ctx, os.Stdout, os.Stderr, os.Args[1:]))
}
