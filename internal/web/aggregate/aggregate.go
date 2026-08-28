// Package aggregate composes the storeless web server over configured
// read-only Cardamom sources.
package aggregate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
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

	// AttachmentContentPattern is the canonical raw attachment route.
	AttachmentContentPattern string

	// AttachmentContent streams source-authorized attachment bytes.
	AttachmentContent http.Handler
}

// Server owns source monitoring and source-qualified routing for one aggregate
// web invocation.
type Server struct {
	catalog    *catalog
	changes    *changeHub
	version    string
	httpClient connect.HTTPClient
	cursorsMu  sync.Mutex
	cursors    map[string]*issueCursor
}

type source struct {
	config      SourceConfig
	project     privatev1connect.ProjectServiceClient
	issues      privatev1connect.IssueServiceClient
	records     privatev1connect.RecordServiceClient
	attachments privatev1connect.AttachmentServiceClient
	checkpoints privatev1connect.CheckpointServiceClient
	execution   privatev1connect.ExecutionServiceClient
	changes     privatev1connect.ChangeServiceClient
}

var sourceAliasPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// New probes configured sources and builds a storeless aggregate server.
// Unavailable or incompatible sources remain visible in bootstrap diagnostics
// but do not contribute project or board catalog entries.
func New(ctx context.Context, cfg Config) (*Server, error) {
	if ctx == nil {
		return nil, errors.New("aggregate: context is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
		transport.ResponseHeaderTimeout = 10 * time.Second
		client = &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	changeClient := sourceChangeHTTPClient(client)

	configured := make([]source, len(cfg.Sources))
	aliases := make(map[string]struct{}, len(cfg.Sources))
	for index, value := range cfg.Sources {
		if !sourceAliasPattern.MatchString(value.Alias) {
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
			attachments: privatev1connect.NewAttachmentServiceClient(client, baseURL),
			checkpoints: privatev1connect.NewCheckpointServiceClient(client, baseURL),
			execution:   privatev1connect.NewExecutionServiceClient(client, baseURL),
			changes:     privatev1connect.NewChangeServiceClient(changeClient, baseURL),
		}
	}

	slices.SortFunc(configured, func(left, right source) int {
		return strings.Compare(left.config.Alias, right.config.Alias)
	})
	states := make([]sourceState, len(configured))
	var wait sync.WaitGroup
	for index := range configured {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			states[index] = probeSource(ctx, &configured[index])
		}(index)
	}
	wait.Wait()

	catalog, err := newCatalog(configured, states)
	if err != nil {
		return nil, err
	}
	server := &Server{
		catalog:    catalog,
		changes:    newChangeHub(),
		cursors:    make(map[string]*issueCursor),
		version:    cfg.Version,
		httpClient: client,
	}
	for index := range configured {
		go server.monitorSource(ctx, index)
	}
	return server, nil
}

// sourceChangeHTTPClient removes request and response-header deadlines from
// source subscriptions because a healthy empty store may not send its first
// change for an arbitrarily long time. Process cancellation still owns the
// stream lifetime, while unary source reads retain their bounded client.
func sourceChangeHTTPClient(client connect.HTTPClient) connect.HTTPClient {
	value, ok := client.(*http.Client)
	if !ok {
		return client
	}
	streamClient := *value
	streamClient.Timeout = 0
	if transport, ok := value.Transport.(*http.Transport); ok {
		streamTransport := transport.Clone()
		streamTransport.ResponseHeaderTimeout = 0
		streamClient.Transport = streamTransport
	}
	return &streamClient
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
		Attachment:    &attachmentService{server: s},
	})
	return Binding{
		Path: path, Handler: handler,
		AttachmentContentPattern: "/source/{sourceID}/board/{boardID}/attachment/{attachmentID}",
		AttachmentContent:        http.HandlerFunc(s.serveAttachmentContent),
	}
}

// aggregatePresentation makes source-rendered entity links retain the route
// needed to distinguish equal board IDs from different stores.
func aggregatePresentation(value *source) *v1.PresentationContext {
	return &v1.PresentationContext{
		RoutePrefix: "/source/" + value.config.Alias + "/board",
	}
}

func (s *Server) serveAttachmentContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	boardID, err := board.NewID(r.PathValue("boardID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	attachmentID, err := attachment.NewID(r.PathValue("attachmentID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ref := &v1.SourceRef{
		SourceId: r.PathValue("sourceID"),
	}
	route, err := s.routeForBoard(boardID.String(), ref)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	target := *route.source.config.URL
	target = *target.JoinPath(
		"board", boardID.String(), "attachment", attachmentID.String(),
	)
	target.RawQuery = ""
	target.Fragment = ""
	request, err := http.NewRequestWithContext(
		r.Context(), r.Method, target.String(), nil,
	)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	copyAttachmentRequestHeaders(request.Header, r.Header)
	response, err := s.httpClient.Do(request)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	defer func() { _ = response.Body.Close() }()
	copyAttachmentResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, response.Body)
	}
}

func copyAttachmentRequestHeaders(dst, src http.Header) {
	for _, name := range []string{
		"Accept", "Range", "If-Range", "If-Match", "If-None-Match",
		"If-Modified-Since", "If-Unmodified-Since",
	} {
		for _, value := range src.Values(name) {
			dst.Add(name, value)
		}
	}
}

func copyAttachmentResponseHeaders(dst, src http.Header) {
	for name, values := range src {
		if isAttachmentHopByHopHeader(name) || name == "Location" ||
			name == "Set-Cookie" || name == "Www-Authenticate" ||
			name == "Proxy-Authenticate" {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

func isAttachmentHopByHopHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

type projectService struct {
	privatev1connect.UnimplementedProjectServiceHandler
	server *Server
}

func (p *projectService) ListProjects(
	context.Context,
	*connect.Request[v1.ListProjectsRequest],
) (*connect.Response[v1.ListProjectsResponse], error) {
	snapshot := p.server.catalog.snapshot()
	return connect.NewResponse(&v1.ListProjectsResponse{
		Projects: cloneProjects(snapshot.projects),
	}), nil
}

func (p *projectService) ListBoards(
	context.Context,
	*connect.Request[v1.ListBoardsRequest],
) (*connect.Response[v1.ListBoardsResponse], error) {
	snapshot := p.server.catalog.snapshot()
	return connect.NewResponse(&v1.ListBoardsResponse{
		Boards: cloneBoards(snapshot.boards),
	}), nil
}

func (p *projectService) GetBootstrap(
	context.Context,
	*connect.Request[v1.GetBootstrapRequest],
) (*connect.Response[v1.GetBootstrapResponse], error) {
	snapshot := p.server.catalog.snapshot()
	entries := make([]*v1.SourceCatalogEntry, 0, len(snapshot.sources))
	complete := true
	problems := make([]*v1.SourceProblem, 0)
	for _, source := range snapshot.sources {
		entries = append(entries, proto.Clone(source.entry).(*v1.SourceCatalogEntry))
		if source.entry.GetHealth() != v1.SourceHealth_SOURCE_HEALTH_HEALTHY {
			complete = false
			problems = append(problems, &v1.SourceProblem{
				SourceId: source.entry.GetSource().GetSourceId(),
				Summary:  source.entry.GetDiagnostic(),
			})
		}
	}
	return connect.NewResponse(&v1.GetBootstrapResponse{
		Projects:        cloneProjects(snapshot.projects),
		Boards:          cloneBoards(snapshot.boards),
		IssueTypes:      aggregateIssueTypes(),
		IssueStatuses:   aggregateIssueStatuses(),
		Version:         p.server.version,
		AccessMode:      v1.AccessMode_ACCESS_MODE_READ_ONLY,
		Sources:         entries,
		AggregateStatus: &v1.AggregateStatus{Complete: complete, Problems: problems},
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
		&v1.GetBoardRequest{
			BoardId: route.boardID, Presentation: aggregatePresentation(route.source),
		},
	))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil || response.Msg.GetBoard() == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no board"))
	}
	board := proto.Clone(response.Msg.GetBoard()).(*v1.Board)
	board.Source = proto.Clone(route.ref).(*v1.SourceRef)
	return connect.NewResponse(&v1.GetBoardResponse{Board: board}), nil
}

func (s *Server) resolveBoard(request *v1.GetBoardRequest) (boardRoute, error) {
	if request == nil {
		return boardRoute{}, connect.NewError(connect.CodeInvalidArgument, errors.New("board request is required"))
	}
	return s.routeForBoard(request.GetBoardId(), request.GetSource())
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
