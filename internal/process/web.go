package process

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/errkind"
	privatev1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/repository/store"
	"go.abhg.dev/cardamom/internal/web"
	"go.abhg.dev/cardamom/internal/web/aggregate"
	"go.abhg.dev/cardamom/internal/web/attachmentconnect"
	"go.abhg.dev/cardamom/internal/web/attachmentcontent"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/changeconnect"
	"go.abhg.dev/cardamom/internal/web/checkpointconnect"
	"go.abhg.dev/cardamom/internal/web/configurationconnect"
	"go.abhg.dev/cardamom/internal/web/dumpconnect"
	"go.abhg.dev/cardamom/internal/web/executionconnect"
	"go.abhg.dev/cardamom/internal/web/informationconnect"
	"go.abhg.dev/cardamom/internal/web/issueconnect"
	"go.abhg.dev/cardamom/internal/web/issueview"
	"go.abhg.dev/cardamom/internal/web/leaseconnect"
	"go.abhg.dev/cardamom/internal/web/mailconnect"
	"go.abhg.dev/cardamom/internal/web/planningconnect"
	"go.abhg.dev/cardamom/internal/web/projectconnect"
	"go.abhg.dev/cardamom/internal/web/recordconnect"
	"go.abhg.dev/cardamom/internal/web/server"
)

// webOperation owns the namespace for one web server invocation.
type webOperation struct{ config Config }

func provideWeb(config *Config) cli.WebOperation {
	return &webOperation{config: *config}
}

func (o *webOperation) Run(ctx context.Context, request cli.WebRequest) (err error) {
	if len(request.Sources) > 0 {
		if request.Store != "" || request.Board != "" {
			return errors.New("web: --source cannot be combined with --store or --board")
		}
		binding, err := o.openAggregate(ctx, request)
		if err != nil {
			return err
		}
		return runWebServer(ctx, request, server.Config{
			Bind: request.Bind, Port: request.Port, NoBrowser: request.NoBrowser,
			Notice: request.Notice, Diagnostic: request.Diagnostic,
			HandlerPath: binding.path, Handler: binding.handler,
			AttachmentContentPattern: binding.attachmentContentPattern,
			AttachmentContent:        binding.attachmentContent,
		})
	}
	binding, closeStore, err := o.open(ctx, request)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeStore()) }()
	return runWebServer(ctx, request, server.Config{
		Bind: request.Bind, Port: request.Port, NoBrowser: request.NoBrowser,
		Notice: request.Notice, Diagnostic: request.Diagnostic,
		HandlerPath: binding.path, Handler: binding.handler,
		AttachmentContentPattern: binding.attachmentContentPattern,
		AttachmentContent:        binding.attachmentContent,
	})
}

func (o *webOperation) openAggregate(
	ctx context.Context,
	request cli.WebRequest,
) (webHandlerBinding, error) {
	sources := make([]aggregate.SourceConfig, 0, len(request.Sources))
	for _, source := range request.Sources {
		sources = append(sources, aggregate.SourceConfig{
			Alias: source.Alias, URL: source.URL,
		})
	}
	value, err := aggregate.New(ctx, aggregate.Config{
		Sources: sources, Version: o.config.Version,
	})
	if err != nil {
		return webHandlerBinding{}, err
	}
	binding := value.Binding()
	return webHandlerBinding{
		path:                     binding.Path,
		handler:                  binding.Handler,
		attachmentContentPattern: attachmentcontent.PathPattern,
		attachmentContent:        http.NotFoundHandler(),
	}, nil
}

// webHandlerBinding identifies the route and handler for the composed web API.
type webHandlerBinding struct {
	path                     string
	handler                  http.Handler
	attachmentContentPattern string
	attachmentContent        http.Handler
}

func (o *webOperation) open(
	ctx context.Context,
	request cli.WebRequest,
) (webHandlerBinding, func() error, error) {
	runtime, err := openNamespace(ctx, o.config, request.Store)
	if err != nil {
		return webHandlerBinding{}, nil, err
	}
	selectedBoard, err := runtime.selectBoard(ctx, request.Board, nil)
	if err != nil {
		kind := errkind.Of(err)
		if request.Board != "" ||
			(kind != errkind.NotFound && kind != errkind.Conflict) {
			return webHandlerBinding{}, nil, errors.Join(err, runtime.close())
		}
		selectedBoard = nil
	}
	attachments, err := provideAttachmentService(runtime)
	if err != nil {
		return webHandlerBinding{}, nil, errors.Join(err, runtime.close())
	}
	localSource, err := localSourceRef(ctx, runtime)
	if err != nil {
		return webHandlerBinding{}, nil, errors.Join(err, runtime.close())
	}
	scope := boardscope.New(runtime.boards, runtime.locator)
	references := &markdownReferenceResolver{runtime: runtime}
	markdownRenderer := markdown.NewWithReferences(
		attachments,
		references,
		references,
	)
	views := issueview.New(markdownRenderer)
	changes := changeconnect.NewPollingSource(changeconnect.PollingConfig{
		Revisions: runtime.store, Boards: runtime.catalog,
	})
	accessMode := web.AccessModeReadWrite
	if request.ReadOnly {
		accessMode = web.AccessModeReadOnly
	}
	var defaultBoardID *board.ID
	if selectedBoard != nil {
		value := selectedBoard.ID()
		defaultBoardID = &value
	}
	projectHandler := projectconnect.New(projectconnect.Config{
		Projects:             runtime.projects,
		ProjectCreator:       runtime.projectCreator,
		Boards:               runtime.boards,
		Markdown:             markdownRenderer,
		ServerDefaultBoardID: defaultBoardID,
		SchemaVersion:        uint64(store.SchemaVersion()),
		Version:              o.config.Version,
		AccessMode:           accessMode,
		Source:               localSource,
	})
	informationHandler := informationconnect.New(runtime.informationService())
	issueHandler := issueconnect.New(issueconnect.Config{
		Scope: scope, Readers: &issueReaderFactory{runtime: runtime},
		Views: views,
	})
	planningHandler := planningconnect.New(planningconnect.Config{
		Scope: scope, Planners: &planningServiceFactory{runtime: runtime},
		Views: views,
	})
	executionHandler := executionconnect.New(executionconnect.Config{
		Scope: scope, Executors: &executionServiceFactory{runtime: runtime},
		Views: views,
	})
	checkpointHandler := checkpointconnect.New(checkpointconnect.Config{
		Scope:    scope,
		Readers:  &checkpointReaderFactory{runtime: runtime},
		Commands: &checkpointCommandFactory{runtime: runtime},
		Views:    views,
	})
	changeHandler := changeconnect.New(changeconnect.Config{
		Scope: scope, Changes: changes,
	})
	recordHandler := recordconnect.New(recordconnect.Config{
		Scope: scope, Records: &recordServiceFactory{runtime: runtime},
		Views: views,
	})
	dumpHandler := dumpconnect.New(dumpconnect.Config{
		Renderers: &dumpRendererFactory{runtime: runtime},
	})
	mailHandler := mailconnect.New(runtime.mailService())
	leaseHandler := leaseconnect.New(runtime.leaseOperations())
	attachmentHandler := attachmentconnect.New(attachments)
	configurationHandler := configurationconnect.New(runtime.configuration)
	path, handler := web.NewHandler(web.HandlerConfig{
		AccessMode: accessMode,
		Project:    projectHandler, Configuration: configurationHandler,
		Information: informationHandler,
		Issue:       issueHandler, Planning: planningHandler,
		Execution: executionHandler, Checkpoint: checkpointHandler,
		Record: recordHandler, Change: changeHandler,
		Dump: dumpHandler, Mail: mailHandler, Lease: leaseHandler,
		Attachment: attachmentHandler,
	})
	contentHandler := attachmentcontent.New(attachmentcontent.Config{
		Attachments: attachments,
		Authorizer:  attachmentContentAuthorizer{},
	})
	return webHandlerBinding{
		path: path, handler: handler,
		attachmentContentPattern: attachmentcontent.PathPattern,
		attachmentContent:        contentHandler,
	}, runtime.close, nil
}

func localSourceRef(ctx context.Context, runtime *namespaceRuntime) (*privatev1.SourceRef, error) {
	view, err := runtime.store.View(ctx)
	if err != nil {
		return nil, fmt.Errorf("open store lineage view: %w", err)
	}
	defer func() { _ = view.Done() }()
	lineage, err := view.LineageID(ctx)
	if err != nil {
		return nil, err
	}
	return &privatev1.SourceRef{StoreLineageId: lineage}, nil
}

// markdownReferenceResolver selects the repository that owns the board named
// by one Markdown rendering batch.
type markdownReferenceResolver struct {
	// runtime opens board-scoped repositories over the process-lifetime store.
	runtime *namespaceRuntime // required
}

func (r *markdownReferenceResolver) ResolveIssueReferences(
	ctx context.Context,
	boardID board.ID,
	issueIDs []issue.ID,
) ([]issue.ID, error) {
	repository, err := r.runtime.boardRepository(boardID)
	if err != nil {
		return nil, err
	}
	return repository.ResolveIssueReferences(ctx, issueIDs)
}

func (r *markdownReferenceResolver) ResolveLogReferences(
	ctx context.Context,
	boardID board.ID,
	logIDs []issue.LogID,
) ([]issue.LogReference, error) {
	repository, err := r.runtime.boardRepository(boardID)
	if err != nil {
		return nil, err
	}
	return repository.ResolveLogReferences(ctx, logIDs)
}

// attachmentContentAuthorizer keeps authorization at the raw HTTP boundary.
// Cardamom does not yet authenticate browser requests, so the current policy permits
// every attachment identity reachable through the local server.
type attachmentContentAuthorizer struct{}

func (attachmentContentAuthorizer) AuthorizeAttachmentContent(
	context.Context,
	board.ID,
	attachment.ID,
) error {
	return nil
}

// issueReaderFactory preserves IssueService's narrow board-reader return type.
type issueReaderFactory struct {
	// runtime owns the process-lifetime store used to open each board reader.
	runtime *namespaceRuntime
}

func (f *issueReaderFactory) Reader(
	boardID board.ID,
) (issueconnect.BoardReader, error) {
	return f.runtime.issueQueries(boardID)
}

// checkpointReaderFactory preserves CheckpointService's narrow board-reader
// return type.
type checkpointReaderFactory struct {
	// runtime owns the process-lifetime store used to open each board reader.
	runtime *namespaceRuntime
}

func (f *checkpointReaderFactory) Reader(
	boardID board.ID,
) (checkpointconnect.BoardReader, error) {
	queries, err := f.runtime.issueQueries(boardID)
	if err != nil {
		return nil, err
	}
	executor, err := f.runtime.issueExecutor(boardID)
	if err != nil {
		return nil, err
	}
	return &checkpointServiceReader{Queries: queries, executor: executor}, nil
}

// checkpointServiceReader combines issue detail and actionable checkpoint
// reads required by CheckpointService.
type checkpointServiceReader struct {
	*issue.Queries
	executor *execution.Executor
}

func (r *checkpointServiceReader) ListActionableCheckpoints(
	ctx context.Context,
) ([]issue.CheckpointView, error) {
	return r.executor.ListActionableCheckpoints(ctx)
}

// checkpointCommandFactory preserves CheckpointService's narrow command return
// type.
type checkpointCommandFactory struct {
	// runtime owns the process-lifetime store used to open each command service.
	runtime *namespaceRuntime
}

func (f *checkpointCommandFactory) Commands(
	boardID board.ID,
) (checkpointconnect.BoardCommands, error) {
	return f.runtime.issueExecutor(boardID)
}

// dumpRendererFactory opens concrete board-scoped renderers for DumpService.
type dumpRendererFactory struct {
	// runtime owns the process-lifetime store used to open each renderer.
	runtime *namespaceRuntime
}

func (f *dumpRendererFactory) Renderer(
	ctx context.Context,
	boardID board.ID,
) (dumpconnect.Renderer, error) {
	return f.runtime.dumpRenderer(ctx, boardID)
}

// planningServiceFactory opens concrete Planners for PlanningService.
type planningServiceFactory struct{ runtime *namespaceRuntime }

func (f *planningServiceFactory) Planner(
	boardID board.ID,
) (planningconnect.BoardPlanner, error) {
	return f.runtime.issuePlanner(boardID)
}

// executionServiceFactory opens concrete Executors for ExecutionService.
type executionServiceFactory struct{ runtime *namespaceRuntime }

func (f *executionServiceFactory) Executor(
	boardID board.ID,
) (executionconnect.BoardExecutor, error) {
	return f.runtime.issueExecutor(boardID)
}

// recordServiceFactory opens concrete Recorders for RecordService.
type recordServiceFactory struct{ runtime *namespaceRuntime }

func (f *recordServiceFactory) Records(
	boardID board.ID,
) (recordconnect.BoardRecords, error) {
	return f.runtime.issueRecorder(boardID)
}
