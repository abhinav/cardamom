package process

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestConfigureGitIgnoreCanonicalizesStorePath(t *testing.T) {
	projectDirectory := t.TempDir()
	command := exec.Command("git", "init", "-q", "-b", "main", projectDirectory)
	require.NoError(t, command.Run())
	storeDirectory := filepath.Join(projectDirectory, ".cardamom")
	require.NoError(t, os.Mkdir(storeDirectory, 0o755))

	assert.Equal(t, cli.InitIgnoreStoragePatternsAdded, configureGitIgnore(
		projectDirectory,
		storeDirectory,
	))
	body, err := os.ReadFile(filepath.Join(projectDirectory, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.Contains(t, string(body), ".cardamom/board.sqlite3*")
	assert.Contains(t, string(body), ".cardamom/blobs/")
	assert.Equal(t, cli.InitIgnoreUnchanged, configureGitIgnore(
		projectDirectory,
		storeDirectory,
	))
}

func TestInitializeAddsBlobsToLegacyExcludes(t *testing.T) {
	projectDirectory := t.TempDir()
	command := exec.Command("git", "init", "-q", "-b", "main", projectDirectory)
	require.NoError(t, command.Run())
	cfg := testConfig(t)
	cfg.CWD = projectDirectory
	cfg.DisableGitIgnore = true
	initialized := execute(t, cfg, "--json", "init")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)

	excludePath := filepath.Join(projectDirectory, ".git", "info", "exclude")
	require.NoError(t, os.WriteFile(excludePath, []byte(".cardamom/board.sqlite3*\n"), 0o644))
	cfg.DisableGitIgnore = false

	reinitialized := execute(t, cfg, "--json", "init")
	require.Equal(t, cli.ExitSuccess, reinitialized.code, reinitialized.stderr)
	var result cli.InitResult
	require.NoError(t, json.Unmarshal([]byte(reinitialized.stdout), &result))
	assert.True(t, result.AlreadyInitialized)
	body, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.Equal(t, ".cardamom/board.sqlite3*\n.cardamom/blobs/\n", string(body))
}

func TestProviderOptionsComposeAttachmentService(t *testing.T) {
	cfg := testConfig(t)
	initialized := execute(t, cfg, "--json", "init", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)
	var namespace cli.InitResult
	require.NoError(t, json.Unmarshal([]byte(initialized.stdout), &namespace))
	require.NotNil(t, namespace.BoardID)

	grammar := new(attachmentProviderGrammar)
	cleanup := new(selectedNamespaceCleanup)
	t.Cleanup(func() { assert.NoError(t, cleanup.close()) })
	options := []kong.Option{kong.Bind(&cfg), kong.Bind(cleanup)}
	options = append(options, providerOptions()...)
	parser, err := kong.New(grammar, options...)
	require.NoError(t, err)
	parsed, err := parser.Parse([]string{"probe"})
	require.NoError(t, err)
	invocation := new(cli.Invocation)
	invocation.Context = t.Context()
	invocation.Store = filepath.Join(cfg.CWD, ".cardamom")
	require.NoError(t, parsed.Run(invocation))
	require.NotNil(t, grammar.Probe.service)
	assert.True(t, grammar.Probe.singleton)

	boardID, err := board.NewID(*namespace.BoardID)
	require.NoError(t, err)
	association, err := attachment.NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := attachment.NewFilename("provider.txt")
	require.NoError(t, err)
	_, err = grammar.Probe.service.BeginUpload(t.Context(), attachment.BeginUploadRequest{
		Invocation:  attachment.NewInvocation("captain"),
		Association: association,
		Filename:    filename,
	})
	require.NoError(t, err)
	staging, err := filepath.Glob(filepath.Join(cfg.CWD, ".cardamom", "blobs", "staging", "*"))
	require.NoError(t, err)
	assert.Len(t, staging, 1)
}

func TestProviderOptionsPassEntropyToMailPersistence(t *testing.T) {
	cfg := testConfig(t)
	cfg.Entropy = bytes.NewReader(bytes.Repeat([]byte{0x5a}, 16))
	initialized := execute(t, cfg, "init", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)

	sent := execute(t, cfg, "--actor", "scotty", "--json", "mail", "send", "kirk", "Status")
	require.Equal(t, cli.ExitSuccess, sent.code, sent.stderr)
	assert.JSONEq(t, `{
		"id":"mail_5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
		"sender":"scotty",
		"recipient":"kirk",
		"source_topic":null,
		"body":"Status",
		"created":"2026-07-18T12:00:00Z",
		"expires":"2026-07-25T12:00:00Z",
		"read_at":null
	}`, sent.stdout)
}

func TestNamespaceRuntimeResolvesConfigurationForEachIssueOperation(t *testing.T) {
	cfg := testConfig(t)
	initialized := execute(t, cfg, "--json", "init", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)
	var namespace cli.InitResult
	require.NoError(t, json.Unmarshal([]byte(initialized.stdout), &namespace))
	require.NotNil(t, namespace.BoardID)
	boardID, err := board.NewID(*namespace.BoardID)
	require.NoError(t, err)

	runtime, err := openNamespace(
		t.Context(),
		cfg,
		filepath.Join(cfg.CWD, ".cardamom"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, runtime.close()) })
	planner, err := runtime.issuePlanner(boardID)
	require.NoError(t, err)

	firstPrefix, err := configuration.NewPrefix("first-")
	require.NoError(t, err)
	firstSummaryLimit, err := configuration.NewByteLimit(4)
	require.NoError(t, err)
	strategy := configuration.IDStrategySequential
	require.NoError(t, writeSettings(
		settingsPath(runtime.directory),
		configuration.Overrides{
			Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{
					Prefix: &firstPrefix, Strategy: &strategy,
				},
				Summary: configuration.SummaryOverrides{MaxBytes: &firstSummaryLimit},
			},
		},
	))
	created, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("tester"),
		planning.CreateIssueRequest{Title: "First", Summary: "four"},
	)
	require.NoError(t, err)
	assert.Equal(t, "first-1", created.Issue.Issue.ID)

	secondPrefix, err := configuration.NewPrefix("second-")
	require.NoError(t, err)
	secondSummaryLimit, err := configuration.NewByteLimit(2)
	require.NoError(t, err)
	require.NoError(t, writeSettings(
		settingsPath(runtime.directory),
		configuration.Overrides{
			Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{
					Prefix: &secondPrefix, Strategy: &strategy,
				},
				Summary: configuration.SummaryOverrides{MaxBytes: &secondSummaryLimit},
			},
		},
	))
	_, err = planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("tester"),
		planning.CreateIssueRequest{Title: "Too large", Summary: "four"},
	)
	assert.ErrorContains(t, err, "summary is 4 bytes; maximum is 2 bytes")
	created, err = planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("tester"),
		planning.CreateIssueRequest{Title: "Second"},
	)
	require.NoError(t, err)
	assert.Equal(t, "second-2", created.Issue.Issue.ID)
}

func TestCompletionUsesExplicitNamespaceSelectors(t *testing.T) {
	cfg := testConfig(t)
	initialized := execute(t, cfg, "--json", "init", "--prefix", "complete-", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)
	var namespace cli.InitResult
	require.NoError(t, json.Unmarshal([]byte(initialized.stdout), &namespace))
	require.NotNil(t, namespace.BoardID)

	created := execute(t, cfg, "--board", *namespace.BoardID, "create", "Calibrate sensors")
	require.Equal(t, cli.ExitSuccess, created.code, created.stderr)
	issueID := strings.TrimSpace(created.stdout)

	assert.Equal(t, []string{issueID}, predict(&cfg, "issues", []string{
		"--store", filepath.Join(cfg.CWD, ".cardamom"),
		"--board", *namespace.BoardID,
	}))
}

func TestCLIAndConnectShareIssueOperationOutcomes(t *testing.T) {
	cfg := testConfig(t)
	initialized := execute(t, cfg, "--json", "init", "--prefix", "parity-", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)
	var namespace cli.InitResult
	require.NoError(t, json.Unmarshal([]byte(initialized.stdout), &namespace))
	require.NotNil(t, namespace.BoardID)

	// Create equivalent issues through each boundary.
	cliCreated := execute(
		t,
		cfg,
		"--actor",
		"engineer",
		"--board",
		*namespace.BoardID,
		"create",
		"Original",
	)
	require.Equal(t, cli.ExitSuccess, cliCreated.code, cliCreated.stderr)
	cliIssueID := strings.TrimSpace(cliCreated.stdout)

	operation := &webOperation{config: cfg}
	binding, closeStore, err := operation.open(t.Context(), cli.WebRequest{
		Store: filepath.Join(cfg.CWD, ".cardamom"), Board: *namespace.BoardID,
	})
	require.NoError(t, err)
	closed := false
	defer func() {
		if !closed {
			require.NoError(t, closeStore())
		}
	}()
	httpClient := &http.Client{Transport: processHandlerTransport{handler: binding.handler}}
	planningClient := privatev1connect.NewPlanningServiceClient(httpClient, "http://cardamom.test")
	issueClient := privatev1connect.NewIssueServiceClient(httpClient, "http://cardamom.test")
	recordClient := privatev1connect.NewRecordServiceClient(httpClient, "http://cardamom.test")
	executionClient := privatev1connect.NewExecutionServiceClient(httpClient, "http://cardamom.test")
	mutation := &privatev1.MutationContext{Actor: new("engineer")}

	connectCreated, err := planningClient.CreateIssue(
		t.Context(),
		connect.NewRequest(&privatev1.CreateIssueRequest{
			BoardId:  *namespace.BoardID,
			Title:    "Original",
			Type:     privatev1.IssueType_ISSUE_TYPE_TASK,
			Priority: 2,
			Context:  mutation,
		}),
	)
	require.NoError(t, err)
	connectIssueID := connectCreated.Msg.GetIssue().GetId()

	// Apply equivalent planning, record, and execution transitions.
	cliEdited := execute(
		t,
		cfg,
		"--actor",
		"engineer",
		"--board",
		*namespace.BoardID,
		"edit",
		cliIssueID,
		"--title",
		"Renamed",
		"--label",
		"area:protocol",
	)
	require.Equal(t, cli.ExitSuccess, cliEdited.code, cliEdited.stderr)

	_, err = planningClient.EditIssue(t.Context(), connect.NewRequest(&privatev1.EditIssueRequest{
		IssueId: connectIssueID, Title: new("Renamed"), AddLabels: []string{"area:protocol"},
		Context: mutation,
	}))
	require.NoError(t, err)

	cliState := execute(
		t,
		cfg,
		"--actor",
		"engineer",
		"--board",
		*namespace.BoardID,
		"state",
		"set",
		cliIssueID,
		"Ready for review.",
	)
	require.Equal(t, cli.ExitSuccess, cliState.code, cliState.stderr)
	_, err = recordClient.SetState(t.Context(), connect.NewRequest(&privatev1.SetStateRequest{
		IssueId: connectIssueID, StateSource: "Ready for review.", Context: mutation,
	}))
	require.NoError(t, err)

	cliClaimed := execute(
		t,
		cfg,
		"--actor",
		"engineer",
		"--board",
		*namespace.BoardID,
		"claim",
		cliIssueID,
	)
	require.Equal(t, cli.ExitSuccess, cliClaimed.code, cliClaimed.stderr)
	_, err = executionClient.ClaimIssue(t.Context(), connect.NewRequest(&privatev1.ClaimIssueRequest{
		IssueId: connectIssueID, Context: mutation,
	}))
	require.NoError(t, err)

	cliClosed := execute(
		t,
		cfg,
		"--actor",
		"engineer",
		"--board",
		*namespace.BoardID,
		"close",
		cliIssueID,
	)
	require.Equal(t, cli.ExitSuccess, cliClosed.code, cliClosed.stderr)
	_, err = executionClient.CloseIssues(t.Context(), connect.NewRequest(&privatev1.CloseIssuesRequest{
		IssueIds: []string{connectIssueID}, Context: mutation,
	}))
	require.NoError(t, err)

	// Compare persisted domain state through Connect.
	cliRead, err := issueClient.GetIssue(t.Context(), connect.NewRequest(&privatev1.GetIssueRequest{
		IssueId: cliIssueID,
	}))
	require.NoError(t, err)
	connectRead, err := issueClient.GetIssue(t.Context(), connect.NewRequest(&privatev1.GetIssueRequest{
		IssueId: connectIssueID,
	}))
	require.NoError(t, err)
	cliSummary := cliRead.Msg.GetIssue().GetIssue()
	connectSummary := connectRead.Msg.GetIssue().GetIssue()
	assert.Equal(t, connectSummary.GetTitle(), cliSummary.GetTitle())
	assert.Equal(t, connectSummary.GetType(), cliSummary.GetType())
	assert.Equal(t, connectSummary.GetPriority(), cliSummary.GetPriority())
	assert.Equal(t, connectSummary.GetLabels(), cliSummary.GetLabels())
	assert.Equal(t, connectSummary.GetLifecycle(), cliSummary.GetLifecycle())
	assert.Equal(t, connectSummary.GetStatus(), cliSummary.GetStatus())
	assert.Equal(t, connectSummary.GetStartedAt(), cliSummary.GetStartedAt())
	assert.Equal(t, connectSummary.GetClosedAt(), cliSummary.GetClosedAt())
	assert.Nil(t, cliSummary.GetActiveClaim())
	assert.Nil(t, connectSummary.GetActiveClaim())

	cliStateRead, err := recordClient.GetState(t.Context(), connect.NewRequest(&privatev1.GetStateRequest{
		IssueId: cliIssueID,
	}))
	require.NoError(t, err)
	connectState, err := recordClient.GetState(t.Context(), connect.NewRequest(&privatev1.GetStateRequest{
		IssueId: connectIssueID,
	}))
	require.NoError(t, err)
	assert.Equal(
		t,
		connectState.Msg.GetState().GetBody().GetSource(),
		cliStateRead.Msg.GetState().GetBody().GetSource(),
	)
}

func TestCloseSelectedNamespace(t *testing.T) {
	tests := []struct {
		name       string
		giveCode   int
		wantCode   int
		wantStderr string
	}{
		{
			name:       "SuccessfulCommand",
			giveCode:   cli.ExitSuccess,
			wantCode:   cli.ExitOperation,
			wantStderr: "error: close store: close failed\n",
		},
		{
			name:     "FailedCommand",
			giveCode: cli.ExitUsage,
			wantCode: cli.ExitUsage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			cleanup := &selectedNamespaceCleanup{
				closeStore: func() error {
					calls++
					return errors.New("close failed")
				},
			}
			var stderr bytes.Buffer

			got := closeSelectedNamespace(cleanup, tt.giveCode, &stderr)

			assert.Equal(t, tt.wantCode, got)
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, 1, calls)
			assert.NoError(t, cleanup.close())
			assert.Equal(t, 1, calls)
		})
	}
}

func TestExecute_reportsCleanupBeforeCancellationCompletes(t *testing.T) {
	cfg := testConfig(t)
	initialized := execute(t, cfg, "init", "--board-name", "Bridge")
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)

	ctx, cancel := context.WithCancel(t.Context())
	clock := &notifyingClock{Clock: cfg.Clock, called: make(chan struct{}, 1)}
	stderr := newInterruptNoticeWriter()
	cfg.Args = []string{"--actor", "tester", "claim", "--watch"}
	cfg.Clock = clock
	cfg.Stderr = stderr
	result := make(chan int, 1)
	go func() { result <- Execute(ctx, cfg) }()

	<-clock.called
	cancel()
	select {
	case notice := <-stderr.started:
		assert.Equal(t, "Cleaning up...\n", notice)
	case code := <-result:
		require.Failf(t, "cleanup notice missing", "Execute returned %d before reporting cleanup", code)
	}

	select {
	case code := <-result:
		close(stderr.release)
		require.Failf(t, "cleanup notice incomplete", "Execute returned %d before the notice completed", code)
	default:
	}
	close(stderr.release)
	assert.Equal(t, cli.ExitCanceled, <-result)
}

type executionResult struct {
	code   int
	stdout string
	stderr string
}

// notifyingClock exposes the first process-clock read as a synchronization
// point without adding a production-only lifecycle hook.
type notifyingClock struct {
	Clock
	called chan struct{}
}

func (c *notifyingClock) Now() time.Time {
	select {
	case c.called <- struct{}{}:
	default:
	}
	return c.Clock.Now()
}

// interruptNoticeWriter holds the cleanup notice open so the test can verify
// that process completion waits for the user-visible response.
type interruptNoticeWriter struct {
	started chan string
	release chan struct{}
}

func newInterruptNoticeWriter() *interruptNoticeWriter {
	return &interruptNoticeWriter{
		started: make(chan string),
		release: make(chan struct{}),
	}
}

func (w *interruptNoticeWriter) Write(body []byte) (int, error) {
	w.started <- string(body)
	<-w.release
	return len(body), nil
}

type attachmentProviderGrammar struct {
	Probe attachmentProviderProbe `cmd:""`
}

type attachmentProviderProbe struct {
	service   *attachment.Service
	singleton bool
}

func (p *attachmentProviderProbe) Run(first, second *attachment.Service) error {
	p.service = first
	p.singleton = first == second
	return nil
}

func execute(t *testing.T, cfg Config, args ...string) executionResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cfg.Args = args
	cfg.Stdout = &stdout
	cfg.Stderr = &stderr
	code := Execute(t.Context(), cfg)
	return executionResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Version:      "test",
		CWD:          t.TempDir(),
		DefaultActor: "tester",
		Stdin:        bytes.NewReader(nil),
		Clock:        fixedClock{instant: time.Unix(1784376000, 0).UTC()},
		ProjectIDs:   &incrementingIDs{},
	}
}

type fixedClock struct{ instant time.Time }

func (c fixedClock) Now() time.Time { return c.instant }

type incrementingIDs struct{ next int }

func (s *incrementingIDs) NewID(kind string) (string, error) {
	s.next++
	return kind + "-test-" + time.Unix(int64(s.next), 0).UTC().Format("150405"), nil
}

type processHandlerTransport struct{ handler http.Handler }

func (t processHandlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}
