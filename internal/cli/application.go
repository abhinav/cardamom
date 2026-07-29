// Package cli declares Cardamom's command grammar and adapts command invocations to
// domain operations and process output.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"

	"github.com/alecthomas/kong"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/komplete"
)

// Version is the build version reported by the version command.
// Release builds replace it through the linker.
var Version = "dev " + runtime.Version()

const (
	// ExitSuccess reports successful command completion.
	ExitSuccess = 0

	// ExitOperation reports a command operation failure.
	ExitOperation = 1

	// ExitUsage reports invalid command-line syntax or input.
	ExitUsage = 2

	// ExitCanceled reports process cancellation.
	ExitCanceled = 130
)

// Config supplies process-owned values to an Application.
//
// The executable composition root resolves terminal state and the default
// actor before constructing the CLI so command adapters do not inspect process
// globals independently.
type Config struct {
	// Version is printed by the version command.
	Version string // required

	// DefaultActor is used when CARDAMOM_ACTOR and --actor are absent.
	DefaultActor string // required

	// Stdin supplies shared Markdown command input.
	Stdin io.Reader // required

	// StdinIsTerminal distinguishes omitted input from piped input.
	StdinIsTerminal bool

	// Stdout receives requested results and human status notices.
	Stdout io.Writer // required

	// Stderr receives diagnostics.
	Stderr io.Writer // required

	// CompletionOptions install lazy predictors for values owned by process or
	// domain composition. Static commands, flags, and enum values need no
	// explicit predictor.
	CompletionOptions []komplete.Option
}

// Application owns the process-stable CLI configuration and Kong dependency
// providers.
//
// A fresh grammar and parser are built for every invocation. Kong options may
// bind values or singleton providers for command Run methods; parser-owned
// name, writers, help, and exit behavior remain fixed by Application.
type Application struct {
	config  Config
	options []kong.Option
}

// New constructs an Application and validates its grammar and provider
// declarations without resolving invocation dependencies.
func New(config Config, options ...kong.Option) (*Application, error) {
	if config.Version == "" {
		return nil, errors.New("version is required")
	}
	if config.DefaultActor == "" {
		return nil, errors.New("default actor is required")
	}
	if config.Stdin == nil {
		return nil, errors.New("standard input is required")
	}
	if config.Stdout == nil {
		return nil, errors.New("standard output is required")
	}
	if config.Stderr == nil {
		return nil, errors.New("standard error is required")
	}
	config.CompletionOptions = append(
		[]komplete.Option(nil),
		config.CompletionOptions...,
	)

	app := &Application{
		config:  config,
		options: append([]kong.Option(nil), options...),
	}
	if _, err := app.newParser(&commandTree{
		Version: versionCommand{value: config.Version},
	}); err != nil {
		return nil, fmt.Errorf("build command grammar: %w", err)
	}
	return app, nil
}

// Invocation carries values whose lifetime is one parsed command.
//
// Command adapters receive Invocation through Kong injection and pass its
// context and scope selectors to narrow domain operations.
type Invocation struct {
	// Context is canceled when the process requests graceful shutdown.
	Context context.Context // required

	// Actor owns custody changes and record attribution.
	Actor string

	// Store is the explicit or environment-selected physical store.
	// An empty value requests automatic discovery by process composition.
	Store string

	// Board is the explicit or environment-selected board.
	// An empty value requests normal board selection by process composition.
	Board string

	// BoardIssueIDs are selectors whose owning board determines implicit scope.
	BoardIssueIDs []string

	// Output owns the invocation's stream and structured-output contracts.
	Output *Output // required

	// Stdin supplies non-Markdown command input such as an apply document.
	Stdin io.Reader // required

	// StdinIsTerminal distinguishes omitted input from piped input.
	StdinIsTerminal bool

	// Markdown selects argument or standard-input Markdown for commands that
	// accept prose.
	Markdown MarkdownInput
}

type parsedInvocation struct {
	Context    *kong.Context
	Invocation *Invocation
}

// Run parses and executes one command invocation and returns its process exit
// status without terminating the process.
func (a *Application) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		args = []string{"--help"}
	}

	parsed, err := a.parse(ctx, args)
	if err != nil {
		var requested requestedExitError
		if errors.As(err, &requested) {
			return requested.code
		}
		return a.reportError(err)
	}

	err = parsed.Context.Run(
		parsed.Invocation,
		parsed.Invocation.Output,
		&parsed.Invocation.Markdown,
	)
	return a.reportError(err)
}

func (a *Application) parse(ctx context.Context, args []string) (
	parsed *parsedInvocation,
	err error,
) {
	if ctx == nil {
		return nil, errors.New("invocation context is required")
	}

	// Kong's help hook calls Exit before command validation or dependency
	// resolution. Convert that callback into a returned status so Application
	// never terminates its host process.
	defer func() {
		if recovered := recover(); recovered != nil {
			requested, ok := recovered.(requestedExitError)
			if !ok {
				panic(recovered)
			}
			err = requested
		}
	}()

	grammar := &commandTree{
		Version: versionCommand{value: a.config.Version},
	}
	parser, err := a.newParser(grammar)
	if err != nil {
		return nil, fmt.Errorf("build command grammar: %w", err)
	}
	komplete.Run(parser, a.config.CompletionOptions...)
	parseContext, err := parser.Parse(args)
	if err != nil {
		return nil, err
	}
	if grammar.Actor == "" {
		grammar.Actor = a.config.DefaultActor
	}
	boardIssueIDs := parsedBoardIssueIDs(parseContext)

	output := newOutput(
		a.config.Stdout,
		a.config.Stderr,
		grammar.JSON,
		grammar.Quiet,
	)
	invocation := &Invocation{
		Context:         ctx,
		Actor:           grammar.Actor,
		Store:           grammar.Store,
		Board:           grammar.BoardSelector,
		BoardIssueIDs:   boardIssueIDs,
		Output:          output,
		Stdin:           a.config.Stdin,
		StdinIsTerminal: a.config.StdinIsTerminal,
		Markdown: MarkdownInput{
			Context:    ctx,
			Stdin:      a.config.Stdin,
			IsTerminal: a.config.StdinIsTerminal,
		},
	}

	return &parsedInvocation{
		Context:    parseContext,
		Invocation: invocation,
	}, nil
}

// issueReferencingCommand exposes issue selectors that determine the owning
// board before Kong invokes the command.
type issueReferencingCommand interface {
	referencedIssueIDs() []string
}

// parsedBoardIssueIDs reads the deepest selected command because parent command
// nodes may also satisfy unrelated Kong interfaces.
func parsedBoardIssueIDs(context *kong.Context) []string {
	for _, path := range slices.Backward(context.Path) {
		command := path.Command
		if command == nil || !command.Target.IsValid() {
			continue
		}
		if command.Target.CanInterface() {
			if source, ok := command.Target.Interface().(issueReferencingCommand); ok {
				return source.referencedIssueIDs()
			}
		}
		if command.Target.CanAddr() && command.Target.Addr().CanInterface() {
			if source, ok := command.Target.Addr().Interface().(issueReferencingCommand); ok {
				return source.referencedIssueIDs()
			}
		}
	}
	return nil
}

func (a *Application) newParser(grammar *commandTree) (*kong.Kong, error) {
	options := append([]kong.Option(nil), a.options...)
	options = append(options,
		kong.Name("card"),
		kong.Description("card - a local issue tracker for project boards."),
		kong.Vars{
			"default_actor": a.config.DefaultActor,
			"version":       a.config.Version,
		},
		commandGroups(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:   true,
			FlagsLast: true,
		}),
		kong.Writers(a.config.Stdout, a.config.Stderr),
		kong.Exit(func(code int) {
			panic(requestedExitError{code: code})
		}),
	)
	return kong.New(grammar, options...)
}

func (a *Application) reportError(err error) int {
	code := ExitCode(err)
	if code == ExitSuccess || code == ExitCanceled {
		return code
	}
	_, _ = fmt.Fprintf(a.config.Stderr, "error: %s\n", err)
	return code
}

// ExitCode classifies command errors according to Cardamom's process contract.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, context.Canceled) {
		return ExitCanceled
	}

	var usage usageError
	if errors.As(err, &usage) {
		return ExitUsage
	}
	var parseError *kong.ParseError
	if errors.As(err, &parseError) {
		return ExitUsage
	}
	return ExitOperation
}

// usageError marks a caller-correctable failure without coupling command
// adapters to Kong's parser error representation.
type usageError struct {
	err error
}

func (e usageError) Error() string { return e.err.Error() }

func (e usageError) Unwrap() error { return e.err }

// UsageErrorf reports a caller-correctable command usage failure.
func UsageErrorf(format string, args ...any) error {
	return errkind.Wrap(
		errkind.InvalidInput,
		usageError{err: fmt.Errorf(format, args...)},
	)
}

// requestedExitError carries a Kong help exit through its callback.
type requestedExitError struct {
	code int
}

func (e requestedExitError) Error() string {
	return fmt.Sprintf("exit %d requested", e.code)
}
