// Package aggregate composes the storeless web server over configured
// read-only Cardamom sources.
package aggregate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/web"
	"google.golang.org/protobuf/proto"
)

// SourceConfig identifies one upstream web server.
type SourceConfig struct {
	// Alias is the stable internal routing name for the source.
	Alias string // required

	// URL is the upstream Cardamom web endpoint.
	URL *url.URL // required
}

// Config supplies aggregate startup and source configuration.
type Config struct {
	// Sources identifies the configured upstream servers.
	Sources []SourceConfig

	// HTTPClient supplies requests to upstream sources. Nil uses a bounded
	// client that does not follow redirects.
	HTTPClient connect.HTTPClient

	// Version is the aggregate process version advertised during bootstrap.
	Version string
}

// Binding contains the existing Connect handler mount for an aggregate server.
type Binding struct {
	// Path is the common generated Connect route prefix.
	Path string

	// Handler serves the aggregate Connect procedures.
	Handler http.Handler
}

// Server owns the immutable source catalog and canonical board routes for one
// aggregate web invocation.
type Server struct {
	sources    []source
	boards     map[string]boardRoute
	projects   []*v1.Project
	boardList  []*v1.BoardSummary
	version    string
	capability []string
	cursorsMu  sync.Mutex
	cursors    map[string]*issueCursor
}

type source struct {
	config      SourceConfig
	project     privatev1connect.ProjectServiceClient
	issues      privatev1connect.IssueServiceClient
	records     privatev1connect.RecordServiceClient
	checkpoints privatev1connect.CheckpointServiceClient
	execution   privatev1connect.ExecutionServiceClient
	changes     privatev1connect.ChangeServiceClient
	bootstrap   *v1.GetBootstrapResponse
	entry       *v1.SourceCatalogEntry
}

type boardRoute struct {
	source source
	ref    *v1.BoardRef
}

// New probes configured sources and builds a storeless aggregate server.
// Unavailable or incompatible sources remain visible in bootstrap diagnostics
// but do not contribute project or board catalog entries.
func New(ctx context.Context, cfg Config) (*Server, error) {
	if ctx == nil {
		return nil, errors.New("aggregate: context is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	configured := make([]source, len(cfg.Sources))
	aliases := make(map[string]struct{}, len(cfg.Sources))
	for index, value := range cfg.Sources {
		if value.Alias == "" || strings.IndexFunc(value.Alias, func(r rune) bool {
			return r == '/' || r == '\\' || r == '?' || r == '#'
		}) >= 0 {
			return nil, fmt.Errorf("aggregate source alias %q is invalid", value.Alias)
		}
		if _, ok := aliases[value.Alias]; ok {
			return nil, fmt.Errorf("duplicate aggregate source alias %q", value.Alias)
		}
		aliases[value.Alias] = struct{}{}
		if value.URL == nil || value.URL.Host == "" {
			return nil, fmt.Errorf("aggregate source %q URL is required", value.Alias)
		}
		baseURL := value.URL.String()
		configured[index] = source{
			config:      value,
			project:     privatev1connect.NewProjectServiceClient(client, baseURL),
			issues:      privatev1connect.NewIssueServiceClient(client, baseURL),
			records:     privatev1connect.NewRecordServiceClient(client, baseURL),
			checkpoints: privatev1connect.NewCheckpointServiceClient(client, baseURL),
			execution:   privatev1connect.NewExecutionServiceClient(client, baseURL),
			changes:     privatev1connect.NewChangeServiceClient(client, baseURL),
		}
	}

	var wait sync.WaitGroup
	for index := range configured {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			probeSource(ctx, &configured[index])
		}(index)
	}
	wait.Wait()
	slices.SortFunc(configured, func(left, right source) int {
		return strings.Compare(left.config.Alias, right.config.Alias)
	})

	server := &Server{
		sources:    configured,
		boards:     make(map[string]boardRoute),
		cursors:    make(map[string]*issueCursor),
		version:    cfg.Version,
		capability: web.ReadCapabilities(),
	}
	if err := server.buildCatalog(); err != nil {
		return nil, err
	}
	return server, nil
}

func probeSource(ctx context.Context, value *source) {
	response, err := value.project.GetBootstrap(
		ctx, connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	if err != nil {
		value.entry = unavailableEntry(value.config.Alias, "source unavailable")
		return
	}
	if response == nil {
		value.entry = unavailableEntry(value.config.Alias, "source returned no bootstrap")
		return
	}
	bootstrap := response.Msg
	if bootstrap == nil {
		value.entry = unavailableEntry(value.config.Alias, "source returned no bootstrap")
		return
	}
	entry := healthyEntry(value.config.Alias, bootstrap)
	if bootstrap.GetProtocolVersion() != web.ProtocolVersion {
		entry.Health = v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE
		entry.Diagnostic = "unsupported source protocol"
		value.entry = entry
		return
	}
	if bootstrap.GetAccessMode() != v1.AccessMode_ACCESS_MODE_READ_ONLY {
		entry.Health = v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE
		entry.Diagnostic = "source is not read-only"
		value.entry = entry
		return
	}
	if len(bootstrap.GetSources()) == 0 || !bootstrap.GetSources()[0].GetReadOnly() {
		entry.Health = v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE
		entry.Diagnostic = "source does not advertise read-only mode"
		value.entry = entry
		return
	}
	for _, required := range web.ReadCapabilities() {
		if !slices.Contains(bootstrap.GetCapabilities(), required) {
			entry.Health = v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE
			entry.Diagnostic = "source lacks required read capability"
			value.entry = entry
			return
		}
	}
	entry.ReadOnly = true
	value.bootstrap = bootstrap
	value.entry = entry
}

func healthyEntry(alias string, bootstrap *v1.GetBootstrapResponse) *v1.SourceCatalogEntry {
	ref := sourceRef(alias, bootstrap)
	return &v1.SourceCatalogEntry{
		Source:          ref,
		Health:          v1.SourceHealth_SOURCE_HEALTH_HEALTHY,
		Version:         bootstrap.GetVersion(),
		SchemaVersion:   bootstrap.GetSchemaVersion(),
		ProtocolVersion: bootstrap.GetProtocolVersion(),
		Capabilities:    slices.Clone(bootstrap.GetCapabilities()),
		ReadOnly:        bootstrap.GetAccessMode() == v1.AccessMode_ACCESS_MODE_READ_ONLY,
	}
}

func unavailableEntry(alias, diagnostic string) *v1.SourceCatalogEntry {
	return &v1.SourceCatalogEntry{
		Source:     &v1.SourceRef{SourceId: alias},
		Health:     v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE,
		Diagnostic: diagnostic,
		ReadOnly:   false,
	}
}

func sourceRef(alias string, bootstrap *v1.GetBootstrapResponse) *v1.SourceRef {
	if sources := bootstrap.GetSources(); len(sources) > 0 && sources[0].GetSource() != nil {
		ref := proto.Clone(sources[0].GetSource()).(*v1.SourceRef)
		ref.SourceId = alias
		return ref
	}
	return &v1.SourceRef{SourceId: alias}
}

func (s *Server) buildCatalog() error {
	lineages := make(map[string]string)
	for _, value := range s.sources {
		if value.bootstrap == nil {
			continue
		}
		ref := value.entry.GetSource()
		if lineage := ref.GetStoreLineageId(); lineage != "" {
			if prior, ok := lineages[lineage]; ok && prior != ref.GetSourceId() {
				return fmt.Errorf("store lineage %q is configured under sources %q and %q", lineage, prior, ref.GetSourceId())
			}
			lineages[lineage] = ref.GetSourceId()
		}
		for _, project := range value.bootstrap.GetProjects() {
			projectCopy := proto.Clone(project).(*v1.Project)
			projectCopy.Ref = &v1.ProjectRef{Source: proto.Clone(ref).(*v1.SourceRef), ProjectId: project.GetId()}
			s.projects = append(s.projects, projectCopy)
		}
		for _, board := range value.bootstrap.GetBoards() {
			if board.GetId() == "" {
				return fmt.Errorf("source %q returned a board without an ID", ref.GetSourceId())
			}
			boardRef := &v1.BoardRef{Source: proto.Clone(ref).(*v1.SourceRef), BoardId: board.GetId()}
			if _, exists := s.boards[board.GetId()]; exists {
				return fmt.Errorf("duplicate board ID %q across aggregate sources", board.GetId())
			}
			boardCopy := proto.Clone(board).(*v1.BoardSummary)
			boardCopy.Ref = proto.Clone(boardRef).(*v1.BoardRef)
			s.boardList = append(s.boardList, boardCopy)
			s.boards[board.GetId()] = boardRoute{source: value, ref: boardRef}
		}
	}
	slices.SortFunc(s.projects, func(left, right *v1.Project) int {
		if result := strings.Compare(left.GetRef().GetSource().GetSourceId(), right.GetRef().GetSource().GetSourceId()); result != 0 {
			return result
		}
		return strings.Compare(left.GetId(), right.GetId())
	})
	slices.SortFunc(s.boardList, func(left, right *v1.BoardSummary) int {
		if result := strings.Compare(left.GetRef().GetSource().GetSourceId(), right.GetRef().GetSource().GetSourceId()); result != 0 {
			return result
		}
		return strings.Compare(left.GetId(), right.GetId())
	})
	return nil
}

// Binding mounts the aggregate service through the existing web handler and
// its descriptor-driven read-only interceptor.
func (s *Server) Binding() Binding {
	path, handler := web.NewHandler(web.HandlerConfig{
		AccessMode:    web.AccessModeReadOnly,
		Project:       &projectService{server: s},
		Configuration: new(privatev1connect.UnimplementedConfigurationServiceHandler),
		Information:   new(privatev1connect.UnimplementedInformationServiceHandler),
		Issue:         &issueService{server: s},
		Planning:      new(privatev1connect.UnimplementedPlanningServiceHandler),
		Execution:     &executionService{server: s},
		Checkpoint:    &checkpointService{server: s},
		Record:        &recordService{server: s},
		Change:        &changeService{server: s},
		Dump:          new(privatev1connect.UnimplementedDumpServiceHandler),
		Mail:          new(privatev1connect.UnimplementedMailServiceHandler),
		Lease:         new(privatev1connect.UnimplementedLeaseServiceHandler),
		Attachment:    new(privatev1connect.UnimplementedAttachmentServiceHandler),
	})
	return Binding{Path: path, Handler: handler}
}

type projectService struct {
	privatev1connect.UnimplementedProjectServiceHandler
	server *Server
}

func (p *projectService) ListProjects(
	context.Context,
	*connect.Request[v1.ListProjectsRequest],
) (*connect.Response[v1.ListProjectsResponse], error) {
	return connect.NewResponse(&v1.ListProjectsResponse{
		Projects: cloneProjects(p.server.projects),
	}), nil
}

func (p *projectService) ListBoards(
	context.Context,
	*connect.Request[v1.ListBoardsRequest],
) (*connect.Response[v1.ListBoardsResponse], error) {
	return connect.NewResponse(&v1.ListBoardsResponse{
		Boards: cloneBoards(p.server.boardList),
	}), nil
}

func (p *projectService) GetBootstrap(
	context.Context,
	*connect.Request[v1.GetBootstrapRequest],
) (*connect.Response[v1.GetBootstrapResponse], error) {
	entries := make([]*v1.SourceCatalogEntry, 0, len(p.server.sources))
	complete := true
	problems := make([]*v1.SourceProblem, 0)
	for _, source := range p.server.sources {
		entries = append(entries, proto.Clone(source.entry).(*v1.SourceCatalogEntry))
		if source.entry.GetHealth() != v1.SourceHealth_SOURCE_HEALTH_HEALTHY {
			complete = false
			problems = append(problems, &v1.SourceProblem{
				SourceId: source.config.Alias,
				Summary:  source.entry.GetDiagnostic(),
			})
		}
	}
	return connect.NewResponse(&v1.GetBootstrapResponse{
		Projects:        cloneProjects(p.server.projects),
		Boards:          cloneBoards(p.server.boardList),
		IssueTypes:      aggregateIssueTypes(),
		IssueStatuses:   aggregateIssueStatuses(),
		Version:         p.server.version,
		AccessMode:      v1.AccessMode_ACCESS_MODE_READ_ONLY,
		Sources:         entries,
		AggregateStatus: &v1.AggregateStatus{Complete: complete, Problems: problems},
		ProtocolVersion: web.ProtocolVersion,
		Capabilities:    slices.Clone(p.server.capability),
		DefaultScope: &v1.BoardScope{Selection: &v1.BoardScope_AllSources{
			AllSources: &v1.AllSources{},
		}},
	}), nil
}

func (p *projectService) GetBoard(
	ctx context.Context,
	request *connect.Request[v1.GetBoardRequest],
) (*connect.Response[v1.GetBoardResponse], error) {
	route, err := p.server.resolveBoard(request.Msg)
	if err != nil {
		return nil, err
	}
	response, err := route.source.project.GetBoard(ctx, connect.NewRequest(
		&v1.GetBoardRequest{BoardId: route.ref.GetBoardId()},
	))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil || response.Msg.GetBoard() == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no board"))
	}
	board := proto.Clone(response.Msg.GetBoard()).(*v1.Board)
	board.Ref = proto.Clone(route.ref).(*v1.BoardRef)
	return connect.NewResponse(&v1.GetBoardResponse{Board: board}), nil
}

func (s *Server) resolveBoard(request *v1.GetBoardRequest) (boardRoute, error) {
	if request == nil {
		return boardRoute{}, connect.NewError(connect.CodeInvalidArgument, errors.New("board request is required"))
	}
	boardID := request.GetBoardId()
	if request.GetBoard() != nil {
		ref := request.GetBoard()
		if ref.GetSource() == nil || ref.GetSource().GetSourceId() == "" || ref.GetBoardId() == "" {
			return boardRoute{}, connect.NewError(connect.CodeInvalidArgument, errors.New("board reference is incomplete"))
		}
		boardID = ref.GetBoardId()
	}
	route, ok := s.boards[boardID]
	if !ok {
		return boardRoute{}, connect.NewError(connect.CodeNotFound, errors.New("board not found"))
	}
	if ref := request.GetBoard(); ref != nil {
		if ref.GetSource().GetSourceId() != route.ref.GetSource().GetSourceId() ||
			ref.GetSource().GetStoreLineageId() != route.ref.GetSource().GetStoreLineageId() {
			return boardRoute{}, connect.NewError(connect.CodeNotFound, errors.New("board not found"))
		}
	}
	return route, nil
}

func cloneProjects(values []*v1.Project) []*v1.Project {
	result := make([]*v1.Project, 0, len(values))
	for _, value := range values {
		result = append(result, proto.Clone(value).(*v1.Project))
	}
	return result
}

func cloneBoards(values []*v1.BoardSummary) []*v1.BoardSummary {
	result := make([]*v1.BoardSummary, 0, len(values))
	for _, value := range values {
		result = append(result, proto.Clone(value).(*v1.BoardSummary))
	}
	return result
}

func aggregateIssueTypes() []v1.IssueType {
	return []v1.IssueType{
		v1.IssueType_ISSUE_TYPE_WORKSTREAM,
		v1.IssueType_ISSUE_TYPE_TASK,
		v1.IssueType_ISSUE_TYPE_CHECKPOINT,
		v1.IssueType_ISSUE_TYPE_ROUTINE,
	}
}

func aggregateIssueStatuses() []v1.IssueStatus {
	return []v1.IssueStatus{
		v1.IssueStatus_ISSUE_STATUS_READY,
		v1.IssueStatus_ISSUE_STATUS_BLOCKED,
		v1.IssueStatus_ISSUE_STATUS_IN_PROGRESS,
		v1.IssueStatus_ISSUE_STATUS_WAITING,
		v1.IssueStatus_ISSUE_STATUS_CLOSED,
		v1.IssueStatus_ISSUE_STATUS_CANCELLED,
	}
}
