// Package process composes the Cardamom command process from domain operations and
// repository implementations.
package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/repository/project"
)

// Clock supplies one process-wide wall clock to every time-sensitive
// operation.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// Config contains process-owned inputs and dependency overrides.
type Config struct {
	// Version is the product version reported by the version command.
	Version string

	// BuildTime is the optional UTC RFC 3339 timestamp reported by the version
	// command.
	BuildTime string

	// Revision is the optional full Git revision reported by the version command.
	Revision string

	// Modified reports whether Revision was built from a dirty worktree.
	Modified bool

	// Args contains command arguments without the executable name.
	Args []string

	// CWD is the working directory used for store and project discovery.
	CWD string

	// DefaultActor identifies the caller when no actor flag or environment
	// override is present.
	DefaultActor string

	// Stdin supplies command input.
	Stdin io.Reader

	// StdinIsTTY distinguishes omitted input from piped input.
	StdinIsTTY bool

	// Stdout receives command results.
	Stdout io.Writer

	// Stderr receives diagnostics.
	Stderr io.Writer

	// DisableGitIgnore prevents initialization from changing Git's local
	// exclude file.
	DisableGitIgnore bool

	// Clock supplies time to process and domain operations.
	Clock Clock

	// ProjectIDs overrides namespace identities in deterministic tests.
	ProjectIDs project.IDSource

	// Entropy overrides repository identities in deterministic tests.
	Entropy io.Reader
}

// Execute composes and runs one CLI invocation without terminating the host
// process.
func Execute(ctx context.Context, cfg Config) int {
	cfg = withConfigDefaults(cfg)
	finishCancellationReporting := reportCleanupOnCancellation(ctx, cfg.Stderr)
	defer finishCancellationReporting()
	cleanup := new(selectedNamespaceCleanup)
	defer func() { _ = cleanup.close() }()
	options := []kong.Option{kong.Bind(&cfg), kong.Bind(cleanup)}
	options = append(options, providerOptions()...)

	app, err := cli.New(cli.Config{
		Version:           cfg.Version,
		BuildTime:         cfg.BuildTime,
		Revision:          cfg.Revision,
		Modified:          cfg.Modified,
		DefaultActor:      cfg.DefaultActor,
		Stdin:             cfg.Stdin,
		StdinIsTerminal:   cfg.StdinIsTTY,
		Stdout:            cfg.Stdout,
		Stderr:            cfg.Stderr,
		CompletionOptions: completionOptions(&cfg),
	}, options...)
	if err != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "error: %s\n", err)
		return cli.ExitOperation
	}
	code := app.Run(ctx, cfg.Args)
	return closeSelectedNamespace(cleanup, code, cfg.Stderr)
}

// reportCleanupOnCancellation reports cancellation before process cleanup can
// delay completion. The returned function stops an unused callback or waits
// for an active write.
func reportCleanupOnCancellation(ctx context.Context, stderr io.Writer) func() {
	reportFinished := make(chan struct{})
	stopReport := context.AfterFunc(ctx, func() {
		defer close(reportFinished)
		_, _ = io.WriteString(stderr, "Cleaning up...\n")
	})
	return func() {
		if stopReport() {
			close(reportFinished)
		}
		<-reportFinished
	}
}

// providerOptions declares the lazy process dependency graph consumed by Kong.
func providerOptions() []kong.Option {
	return []kong.Option{
		kong.BindSingletonProvider(provideInitializer),
		kong.BindSingletonProvider(provideInfo),
		kong.BindSingletonProvider(provideSkillInstallOperation),
		kong.BindSingletonProvider(provideWeb),
		kong.BindSingletonProvider(provideNamespace),
		kong.BindSingletonProvider(provideConfigurationService),
		kong.BindSingletonProvider(provideAttachmentService),
		kong.BindSingletonProvider(provideProjectService),
		kong.BindSingletonProvider(provideProjectCreationService),
		kong.BindSingletonProvider(provideProjectInspectionService),
		kong.BindSingletonProvider(provideBoardResolver),
		kong.BindSingletonProvider(provideBoardService),
		kong.BindSingletonProvider(provideBoardCatalog),
		kong.BindSingletonProvider(provideBoardCopyOperation),
		kong.BindSingletonProvider(provideBackupOperation),
		kong.BindSingletonProvider(provideRestoreOperation),
		kong.BindSingletonProvider(provideSelectedBoard),
		kong.BindSingletonProvider(provideBoardRepository),
		kong.BindSingletonProvider(provideIssueQueries),
		kong.BindSingletonProvider(provideIssueRecorder),
		kong.BindSingletonProvider(provideIssuePlanner),
		kong.BindSingletonProvider(provideIssueExecutor),
		kong.BindSingletonProvider(provideCreateIssueOperation),
		kong.BindSingletonProvider(provideApplyDocumentOperation),
		kong.BindSingletonProvider(provideEditIssueOperation),
		kong.BindSingletonProvider(provideListIssuesOperation),
		kong.BindSingletonProvider(provideListReadyIssuesOperation),
		kong.BindSingletonProvider(provideListBlockedIssuesOperation),
		kong.BindSingletonProvider(provideIssueInspector),
		kong.BindSingletonProvider(provideReadIssueOperation),
		kong.BindSingletonProvider(provideClaimOperations),
		kong.BindSingletonProvider(provideReleaseOperations),
		kong.BindSingletonProvider(provideCloseOperations),
		kong.BindSingletonProvider(provideCancelOperations),
		kong.BindSingletonProvider(provideReopenOperations),
		kong.BindSingletonProvider(provideCheckpointOperations),
		kong.BindSingletonProvider(provideLogWriteOperations),
		kong.BindSingletonProvider(provideLogReadOperations),
		kong.BindSingletonProvider(provideStateWriteOperations),
		kong.BindSingletonProvider(provideStateReadOperations),
		kong.BindSingletonProvider(provideStateCommitOperations),
		kong.BindSingletonProvider(provideResultWriteOperations),
		kong.BindSingletonProvider(provideResultReadOperations),
		kong.BindSingletonProvider(provideMailService),
		kong.BindSingletonProvider(provideMailOperations),
		kong.BindSingletonProvider(provideLeaseOperations),
		kong.BindSingletonProvider(provideDumpPublicationService),
	}
}

// closeSelectedNamespace preserves an existing command failure when namespace
// cleanup also fails.
func closeSelectedNamespace(
	cleanup *selectedNamespaceCleanup,
	code int,
	stderr io.Writer,
) int {
	if err := cleanup.close(); err != nil && code == cli.ExitSuccess {
		_, _ = fmt.Fprintf(stderr, "error: close store: %s\n", err)
		return cli.ExitOperation
	}
	return code
}

func withConfigDefaults(cfg Config) Config {
	if cfg.Version == "" {
		cfg.Version = cli.Version
	}
	if cfg.CWD == "" {
		cfg.CWD, _ = os.Getwd()
	}
	if cfg.DefaultActor == "" {
		cfg.DefaultActor = "unknown"
	}
	if cfg.Stdin == nil {
		cfg.Stdin = strings.NewReader("")
	}
	if cfg.Stdout == nil {
		cfg.Stdout = io.Discard
	}
	if cfg.Stderr == nil {
		cfg.Stderr = io.Discard
	}
	if cfg.Clock == nil {
		cfg.Clock = systemClock{}
	}
	return cfg
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// selectedNamespaceCleanup owns cleanup for the namespace opened by a
// board-scoped command.
type selectedNamespaceCleanup struct {
	// closeStore is nil until namespace selection opens a store successfully.
	closeStore func() error
}

// register records the selected namespace immediately after it opens.
func (c *selectedNamespaceCleanup) register(runtime *namespaceRuntime) {
	c.closeStore = runtime.close
}

// close closes the selected namespace at most once.
func (c *selectedNamespaceCleanup) close() error {
	if c.closeStore == nil {
		return nil
	}
	closeStore := c.closeStore
	c.closeStore = nil
	return closeStore()
}
