package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/arjia-labs/clu/internal/cli"
)

func main() {
	// First ^C / SIGTERM cancels ctx so commands can clean up
	// (e.g. `clu web` tearing down its child process group). A second
	// signal force-exits — covers the case where shutdown hangs and
	// the user is mashing Ctrl+C trying to get out.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop() // reset the handler so subsequent signals go to default behavior
		hard := make(chan os.Signal, 1)
		signal.Notify(hard, os.Interrupt, syscall.SIGTERM)
		<-hard
		fmt.Fprintln(os.Stderr, "\nforcing exit (shutdown didn't complete)")
		os.Exit(130)
	}()
	os.Exit(cli.Run(ctx, os.Stdout, os.Stderr, os.Args[1:]))
}
