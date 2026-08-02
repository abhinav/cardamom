// Package projectconnect exposes project and board endpoints through Connect.
package projectconnect

import (
	"context"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/board"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
	projectcreation "go.abhg.dev/cardamom/internal/project/creation"
	"go.abhg.dev/cardamom/internal/web"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Projects supplies project catalog reads exposed by ProjectService.
type Projects interface {
	// List returns every current project in catalog order.
	List(context.Context) ([]*project.State, error)
}

// ProjectCreator establishes projects through shared domain policy.
type ProjectCreator interface {
	// CreateProject establishes one project in the current store.
	CreateProject(
		context.Context,
		projectcreation.Invocation,
		projectcreation.Request,
	) (*project.State, error)
}

// Boards supplies board catalog reads and mutations exposed by ProjectService.
type Boards interface {
	// List returns every current board in catalog order.
	List(context.Context) ([]*board.State, error)

	// Get returns the current board with the requested stable identity.
	Get(context.Context, board.ID) (*board.State, error)

	// Create establishes one board with canonical invocation metadata.
	Create(context.Context, board.Invocation, board.CreateRequest) (*board.State, error)

	// EditSettings changes one board's settings with canonical invocation metadata.
	EditSettings(context.Context, board.Invocation, board.EditRequest) (*board.State, error)
}

// MarkdownRenderer converts stored board Markdown to trusted presentation HTML.
type MarkdownRenderer interface {
	// RenderBoard converts one board response's Markdown sources to safe HTML.
	RenderBoard(context.Context, board.ID, string, []string) ([]string, error)
}

// Config supplies ProjectService collaborators and immutable bootstrap data.
type Config struct {
	// Projects reads the project catalog.
	Projects Projects // required

	// ProjectCreator establishes projects through shared domain policy.
	ProjectCreator ProjectCreator // required

	// Boards reads and changes boards.
	Boards Boards // required

	// Markdown renders board descriptions for browser presentation.
	Markdown MarkdownRenderer // required

	// ServerDefaultBoardID identifies the board selected at server startup.
	ServerDefaultBoardID *board.ID

	// SchemaVersion reports the store schema served by this process.
	SchemaVersion uint64

	// Version reports the running Cardamom binary version.
	Version string

	// AccessMode reports the server-side write policy for this web invocation.
	AccessMode web.AccessMode

	// Source identifies the local store when it is published as an aggregate
	// source. The source ID is empty for ordinary local web serving.
	Source *privatev1.SourceRef
}

// Service adapts project catalog operations to generated ProjectService RPCs.
type Service struct {
	privatev1connect.UnimplementedProjectServiceHandler
	projects             Projects
	projectCreator       ProjectCreator
	boards               Boards
	markdown             MarkdownRenderer
	serverDefaultBoardID *board.ID
	schemaVersion        uint64
	version              string
	accessMode           web.AccessMode
	source               *privatev1.SourceRef
}

var _ privatev1connect.ProjectServiceHandler = (*Service)(nil)

// New constructs a ProjectService handler from its catalog and presentation
// collaborators.
func New(cfg Config) *Service {
	must.NotBeNilf(cfg.Projects, "projectconnect: projects are required")
	must.NotBeNilf(
		cfg.ProjectCreator,
		"projectconnect: project creator is required",
	)
	must.NotBeNilf(cfg.Boards, "projectconnect: board service is required")
	must.NotBeNilf(cfg.Markdown, "projectconnect: Markdown renderer is required")
	return &Service{
		projects:             cfg.Projects,
		projectCreator:       cfg.ProjectCreator,
		boards:               cfg.Boards,
		markdown:             cfg.Markdown,
		serverDefaultBoardID: cloneBoardID(cfg.ServerDefaultBoardID),
		schemaVersion:        cfg.SchemaVersion,
		version:              cfg.Version,
		accessMode:           cfg.AccessMode,
		source:               cloneSourceRef(cfg.Source),
	}
}

// ListProjects returns every project in catalog order.
func (s *Service) ListProjects(
	ctx context.Context,
	_ *connect.Request[privatev1.ListProjectsRequest],
) (*connect.Response[privatev1.ListProjectsResponse], error) {
	projects, err := s.listProjects(ctx)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ListProjectsResponse{
		Projects: projects,
	}), nil
}

// ListBoards returns every board in catalog order.
func (s *Service) ListBoards(
	ctx context.Context,
	_ *connect.Request[privatev1.ListBoardsRequest],
) (*connect.Response[privatev1.ListBoardsResponse], error) {
	boards, err := s.listBoards(ctx)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ListBoardsResponse{
		Boards: boards,
	}), nil
}

// GetBootstrap returns the project and board catalog plus immutable server
// metadata needed before the browser selects a collection scope.
func (s *Service) GetBootstrap(
	ctx context.Context,
	_ *connect.Request[privatev1.GetBootstrapRequest],
) (*connect.Response[privatev1.GetBootstrapResponse], error) {
	projects, err := s.listProjects(ctx)
	if err != nil {
		return nil, web.FromError(err)
	}
	boards, err := s.listBoards(ctx)
	if err != nil {
		return nil, web.FromError(err)
	}
	protocol := web.BrowserProtocol()
	response := &privatev1.GetBootstrapResponse{
		Projects:      projects,
		Boards:        boards,
		IssueTypes:    validIssueTypes(),
		IssueStatuses: validIssueStatuses(),
		SchemaVersion: s.schemaVersion,
		Version:       s.version,
		AccessMode:    bootstrapAccessMode(s.accessMode),
		Protocol:      protocol,
	}
	if s.source != nil {
		response.Sources = []*privatev1.SourceCatalogEntry{{
			Source:        cloneSourceRef(s.source),
			Health:        privatev1.SourceHealth_SOURCE_HEALTH_HEALTHY,
			Version:       s.version,
			SchemaVersion: s.schemaVersion,
			Protocol:      proto.Clone(protocol).(*privatev1.WebProtocol),
			ReadOnly:      s.accessMode == web.AccessModeReadOnly,
		}}
	}
	if s.serverDefaultBoardID != nil {
		value := s.serverDefaultBoardID.String()
		response.ServerDefaultBoardId = &value
	}
	return connect.NewResponse(response), nil
}

func bootstrapAccessMode(mode web.AccessMode) privatev1.AccessMode {
	switch mode {
	case web.AccessModeReadWrite:
		return privatev1.AccessMode_ACCESS_MODE_READ_WRITE
	case web.AccessModeReadOnly:
		return privatev1.AccessMode_ACCESS_MODE_READ_ONLY
	default:
		panic(fmt.Sprintf("projectconnect: unsupported access mode %d", mode))
	}
}

func (s *Service) listProjects(ctx context.Context) ([]*privatev1.Project, error) {
	projects, err := s.projects.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*privatev1.Project, 0, len(projects))
	for _, value := range projects {
		result = append(result, projectMessage(value))
	}
	return result, nil
}

func (s *Service) listBoards(ctx context.Context) ([]*privatev1.BoardSummary, error) {
	boards, err := s.boards.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*privatev1.BoardSummary, 0, len(boards))
	for _, value := range boards {
		result = append(result, boardSummary(value))
	}
	return result, nil
}

// GetBoard returns one explicitly identified board and rendered context.
func (s *Service) GetBoard(
	ctx context.Context,
	request *connect.Request[privatev1.GetBoardRequest],
) (*connect.Response[privatev1.GetBoardResponse], error) {
	boardID, err := board.NewID(request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	board, err := s.boards.Get(ctx, boardID)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.board(ctx, board, request.Msg.GetPresentation().GetRoutePrefix())
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.GetBoardResponse{Board: converted}), nil
}

// CreateProject establishes one project in the current store.
func (s *Service) CreateProject(
	ctx context.Context,
	request *connect.Request[privatev1.CreateProjectRequest],
) (*connect.Response[privatev1.CreateProjectResponse], error) {
	created, err := s.projectCreator.CreateProject(
		ctx,
		projectcreation.NewInvocation(request.Msg.GetContext().GetActor()),
		projectcreation.Request{
			Name:   request.Msg.GetName(),
			Prefix: cloneString(request.Msg.Prefix),
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.CreateProjectResponse{
		Project: projectMessage(created),
	}), nil
}

// CreateBoard establishes one board in an explicitly identified project.
func (s *Service) CreateBoard(
	ctx context.Context,
	request *connect.Request[privatev1.CreateBoardRequest],
) (*connect.Response[privatev1.CreateBoardResponse], error) {
	projectID, err := project.NewID(request.Msg.GetProjectId())
	if err != nil {
		return nil, web.FromError(err)
	}
	created, err := s.boards.Create(ctx, mutationInvocation(request.Msg.GetContext()), board.CreateRequest{
		ProjectID:   projectID.String(),
		Name:        request.Msg.GetName(),
		Description: cloneString(request.Msg.DescriptionSource),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.board(ctx, created, "")
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.CreateBoardResponse{Board: converted}), nil
}

// UpdateBoard applies all supplied settings in one board-domain operation.
func (s *Service) UpdateBoard(
	ctx context.Context,
	request *connect.Request[privatev1.UpdateBoardRequest],
) (*connect.Response[privatev1.UpdateBoardResponse], error) {
	boardID, err := board.NewID(request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	settings := board.SettingsEdit{Name: cloneString(request.Msg.Name)}
	if request.Msg.DescriptionSource != nil {
		// Presence selects replacement; an empty source clears the optional
		// description because the project domain does not retain blank Markdown.
		var description *string
		if value := request.Msg.GetDescriptionSource(); value != "" {
			description = &value
		}
		settings.Description = board.ReplaceDescription(description)
	}
	if settings.Name == nil && settings.Description == nil {
		return nil, web.FromError(errkind.Errorf(
			errkind.InvalidInput,
			"invalid project namespace: board setting required",
		))
	}
	edited, err := s.boards.EditSettings(ctx, mutationInvocation(request.Msg.GetContext()), board.EditRequest{
		BoardID:  boardID,
		Settings: settings,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.board(ctx, edited, "")
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.UpdateBoardResponse{Board: converted}), nil
}

func (s *Service) board(
	ctx context.Context,
	value *board.State,
	routePrefix string,
) (*privatev1.Board, error) {
	description, err := s.optionalMarkdown(
		ctx, value.ID(), routePrefix, value.Description(),
	)
	if err != nil {
		return nil, err
	}
	return &privatev1.Board{
		Id: value.ID().String(), ProjectId: value.ProjectID(),
		Name: value.Name(), Description: description,
		CreatedAt: timestamp(value.Created().Unix()),
	}, nil
}

func (s *Service) optionalMarkdown(
	ctx context.Context,
	boardID board.ID,
	routePrefix string,
	source *string,
) (*privatev1.MarkdownContent, error) {
	if source == nil {
		return nil, nil
	}
	rendered, err := s.markdown.RenderBoard(
		ctx, boardID, routePrefix, []string{*source},
	)
	if err != nil {
		return nil, fmt.Errorf("render Markdown: %w", err)
	}
	if len(rendered) != 1 {
		return nil, fmt.Errorf("render Markdown: got %d results for 1 source", len(rendered))
	}
	return &privatev1.MarkdownContent{Source: *source, RenderedHtml: rendered[0]}, nil
}

func validIssueTypes() []privatev1.IssueType {
	return []privatev1.IssueType{
		privatev1.IssueType_ISSUE_TYPE_WORKSTREAM,
		privatev1.IssueType_ISSUE_TYPE_TASK,
		privatev1.IssueType_ISSUE_TYPE_CHECKPOINT,
		privatev1.IssueType_ISSUE_TYPE_ROUTINE,
	}
}

func validIssueStatuses() []privatev1.IssueStatus {
	return []privatev1.IssueStatus{
		privatev1.IssueStatus_ISSUE_STATUS_READY,
		privatev1.IssueStatus_ISSUE_STATUS_BLOCKED,
		privatev1.IssueStatus_ISSUE_STATUS_IN_PROGRESS,
		privatev1.IssueStatus_ISSUE_STATUS_WAITING,
		privatev1.IssueStatus_ISSUE_STATUS_CLOSED,
		privatev1.IssueStatus_ISSUE_STATUS_CANCELLED,
	}
}

func boardSummary(value *board.State) *privatev1.BoardSummary {
	return &privatev1.BoardSummary{
		Id: value.ID().String(), ProjectId: value.ProjectID(),
		Name: value.Name(),
	}
}

func projectMessage(value *project.State) *privatev1.Project {
	return &privatev1.Project{
		Id:   value.ID().String(),
		Name: value.Name(),
	}
}

func timestamp(seconds int64) *timestamppb.Timestamp {
	return timestamppb.New(time.Unix(seconds, 0).UTC())
}

func cloneBoardID(value *board.ID) *board.ID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneSourceRef(value *privatev1.SourceRef) *privatev1.SourceRef {
	if value == nil {
		return nil
	}
	return proto.Clone(value).(*privatev1.SourceRef)
}

func mutationInvocation(value *privatev1.MutationContext) board.Invocation {
	return board.NewInvocation(value.GetActor())
}
