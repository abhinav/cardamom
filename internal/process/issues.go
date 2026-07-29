package process

import (
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
)

func provideIssueRecorder(
	runtime *namespaceRuntime,
	selected *board.State,
) (*record.Recorder, error) {
	return runtime.issueRecorder(selected.ID())
}

func provideIssueQueries(
	runtime *namespaceRuntime,
	selected *board.State,
) (*issue.Queries, error) {
	return runtime.issueQueries(selected.ID())
}

func provideIssuePlanner(
	runtime *namespaceRuntime,
	selected *board.State,
) (*planning.Planner, error) {
	return runtime.issuePlanner(selected.ID())
}

func provideIssueExecutor(
	runtime *namespaceRuntime,
	selected *board.State,
) (*execution.Executor, error) {
	return runtime.issueExecutor(selected.ID())
}

func provideCreateIssueOperation(
	planner *planning.Planner,
) cli.CreateIssueOperation {
	return planner
}

func provideApplyDocumentOperation(
	planner *planning.Planner,
) cli.ApplyDocumentOperation {
	return planner
}

func provideEditIssueOperation(
	planner *planning.Planner,
) cli.EditIssueOperation {
	return planner
}

func provideListIssuesOperation(
	queries *issue.Queries,
) cli.ListIssuesOperation {
	return queries
}

func provideListReadyIssuesOperation(
	executor *execution.Executor,
) cli.ListReadyIssuesOperation {
	return executor
}

func provideListBlockedIssuesOperation(
	executor *execution.Executor,
) cli.ListBlockedIssuesOperation {
	return executor
}

func provideReadIssueOperation(
	inspector *domainattachment.IssueInspector,
) cli.ReadIssueOperation {
	return inspector
}

func provideIssueInspector(
	selected *board.State,
	queries *issue.Queries,
	attachments *domainattachment.Service,
) *domainattachment.IssueInspector {
	return domainattachment.NewIssueInspector(selected.ID(), queries, attachments)
}

func provideClaimOperations(executor *execution.Executor) cli.ClaimOperations {
	return executor
}

func provideReleaseOperations(executor *execution.Executor) cli.ReleaseOperations {
	return executor
}

func provideCloseOperations(executor *execution.Executor) cli.CloseOperations {
	return executor
}

func provideCancelOperations(executor *execution.Executor) cli.CancelOperations {
	return executor
}

func provideReopenOperations(executor *execution.Executor) cli.ReopenOperations {
	return executor
}

func provideCheckpointOperations(
	executor *execution.Executor,
) cli.CheckpointOperations {
	return executor
}

func provideLogWriteOperations(
	recorder *record.Recorder,
) cli.LogEntryWriteOperations {
	return recorder
}

func provideLogReadOperations(
	recorder *record.Recorder,
) cli.LogEntryReadOperations {
	return recorder
}

func provideStateWriteOperations(
	recorder *record.Recorder,
) cli.StateWriteOperations {
	return recorder
}

func provideStateReadOperations(
	recorder *record.Recorder,
) cli.StateReadOperations {
	return recorder
}

func provideStateCommitOperations(
	recorder *record.Recorder,
) cli.StateCommitOperations {
	return recorder
}

func provideResultWriteOperations(
	recorder *record.Recorder,
) cli.ResultWriteOperations {
	return recorder
}

func provideResultReadOperations(
	recorder *record.Recorder,
) cli.ResultReadOperations {
	return recorder
}
