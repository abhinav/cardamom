package aggregate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/web"
)

func TestNewBuildsBootstrapAndReadOnlyHandler(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-primary", ProjectId: "project-primary", Name: "Primary"}
	server := newSourceServer(t, sourceBootstrap(board, true), &v1.Board{
		Id: board.GetId(), ProjectId: board.GetProjectId(), Name: board.GetName(),
	})

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "primary", URL: mustURL(t, server.URL)}},
		Version: "aggregate-test",
	})
	require.NoError(t, err)

	client := newAggregateClient(t, aggregate)
	bootstrap, err := client.GetBootstrap(
		t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, bootstrap.Msg.GetSources(), 1)
	assert.Equal(t, v1.SourceHealth_SOURCE_HEALTH_HEALTHY, bootstrap.Msg.GetSources()[0].GetHealth())
	assert.Equal(t, "primary", bootstrap.Msg.GetSources()[0].GetSource().GetSourceId())
	assert.Equal(t, "lineage-primary", bootstrap.Msg.GetSources()[0].GetSource().GetStoreLineageId())
	assert.Equal(t, v1.AccessMode_ACCESS_MODE_READ_ONLY, bootstrap.Msg.GetAccessMode())
	assert.Equal(t, web.ProtocolVersion, bootstrap.Msg.GetProtocolVersion())
	assert.Equal(t, []string{web.CapabilityBoardCatalog, web.CapabilityBoardRead}, bootstrap.Msg.GetCapabilities())
	assert.Equal(t, "board-primary", bootstrap.Msg.GetBoards()[0].GetId())
	assert.Equal(t, "primary", bootstrap.Msg.GetBoards()[0].GetRef().GetSource().GetSourceId())
	_, ok := bootstrap.Msg.GetDefaultScope().GetSelection().(*v1.BoardScope_AllSources)
	assert.True(t, ok)

	_, err = client.CreateBoard(t.Context(), connect.NewRequest(&v1.CreateBoardRequest{}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestNewRejectsSourcesWithoutRequiredCapabilities(t *testing.T) {
	for _, test := range []struct {
		name       string
		bootstrap  *v1.GetBootstrapResponse
		diagnostic string
	}{
		{
			name: "MissingCapability",
			bootstrap: func() *v1.GetBootstrapResponse {
				value := sourceBootstrap(nil, true)
				value.Capabilities = []string{web.CapabilityBoardCatalog}
				return value
			}(),
			diagnostic: "source lacks required read capability",
		},
		{
			name: "WritableSource",
			bootstrap: func() *v1.GetBootstrapResponse {
				value := sourceBootstrap(nil, false)
				value.Capabilities = web.ReadCapabilities()
				return value
			}(),
			diagnostic: "source is not read-only",
		},
		{
			name: "UnsupportedProtocol",
			bootstrap: func() *v1.GetBootstrapResponse {
				value := sourceBootstrap(nil, true)
				value.ProtocolVersion++
				return value
			}(),
			diagnostic: "unsupported source protocol",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newSourceServer(t, test.bootstrap, nil)
			aggregate, err := New(t.Context(), Config{
				Sources: []SourceConfig{{Alias: "source", URL: mustURL(t, server.URL)}},
			})
			require.NoError(t, err)

			bootstrap, err := newAggregateClient(t, aggregate).GetBootstrap(
				t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
			)
			require.NoError(t, err)
			require.Len(t, bootstrap.Msg.GetSources(), 1)
			assert.Equal(t, v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE, bootstrap.Msg.GetSources()[0].GetHealth())
			assert.Equal(t, test.diagnostic, bootstrap.Msg.GetSources()[0].GetDiagnostic())
			assert.False(t, bootstrap.Msg.GetAggregateStatus().GetComplete())
		})
	}
}

func TestNewRejectsDuplicateBoardIDs(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-duplicate", ProjectId: "project", Name: "Duplicate"}
	first := newSourceServer(t, sourceBootstrap(board, true), nil)
	secondBootstrap := sourceBootstrap(board, true)
	secondBootstrap.Sources[0].Source.StoreLineageId = "lineage-second"
	second := newSourceServer(t, secondBootstrap, nil)

	_, err := New(t.Context(), Config{Sources: []SourceConfig{
		{Alias: "first", URL: mustURL(t, first.URL)},
		{Alias: "second", URL: mustURL(t, second.URL)},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate board ID "board-duplicate"`)
}

func TestNewKeepsUnavailableSourceInBootstrap(t *testing.T) {
	server := newSourceServer(t, sourceBootstrap(nil, true), nil)
	server.Close()

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "offline", URL: mustURL(t, server.URL)}},
	})
	require.NoError(t, err)

	bootstrap, err := newAggregateClient(t, aggregate).GetBootstrap(
		t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, bootstrap.Msg.GetSources(), 1)
	assert.Equal(t, "offline", bootstrap.Msg.GetSources()[0].GetSource().GetSourceId())
	assert.Equal(t, v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE, bootstrap.Msg.GetSources()[0].GetHealth())
	assert.False(t, bootstrap.Msg.GetAggregateStatus().GetComplete())
	assert.Equal(t, "source unavailable", bootstrap.Msg.GetAggregateStatus().GetProblems()[0].GetSummary())
}

func TestProjectServiceGetBoardUsesCanonicalBoardRoute(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-canonical", ProjectId: "project", Name: "Canonical"}
	source := newSourceServer(t, sourceBootstrap(board, true), &v1.Board{
		Id: board.GetId(), ProjectId: board.GetProjectId(), Name: board.GetName(),
	})

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "laptop", URL: mustURL(t, source.URL)}},
	})
	require.NoError(t, err)
	response, err := newAggregateClient(t, aggregate).GetBoard(
		t.Context(), connect.NewRequest(&v1.GetBoardRequest{BoardId: board.GetId()}),
	)
	require.NoError(t, err)
	assert.Equal(t, board.GetId(), response.Msg.GetBoard().GetId())
	assert.Equal(t, "laptop", response.Msg.GetBoard().GetRef().GetSource().GetSourceId())
	assert.Equal(t, board.GetId(), response.Msg.GetBoard().GetRef().GetBoardId())
}

type sourceHandler struct {
	privatev1connect.UnimplementedProjectServiceHandler
	bootstrap *v1.GetBootstrapResponse
	board     *v1.Board
}

func (s *sourceHandler) GetBootstrap(
	context.Context,
	*connect.Request[v1.GetBootstrapRequest],
) (*connect.Response[v1.GetBootstrapResponse], error) {
	return connect.NewResponse(s.bootstrap), nil
}

func (s *sourceHandler) GetBoard(
	_ context.Context,
	request *connect.Request[v1.GetBoardRequest],
) (*connect.Response[v1.GetBoardResponse], error) {
	if s.board == nil || request.Msg.GetBoardId() != s.board.GetId() {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&v1.GetBoardResponse{Board: s.board}), nil
}

func newSourceServer(
	t *testing.T,
	bootstrap *v1.GetBootstrapResponse,
	board *v1.Board,
) *httptest.Server {
	t.Helper()
	_, handler := privatev1connect.NewProjectServiceHandler(&sourceHandler{
		bootstrap: bootstrap, board: board,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func sourceBootstrap(board *v1.BoardSummary, readOnly bool) *v1.GetBootstrapResponse {
	value := &v1.GetBootstrapResponse{
		AccessMode:      v1.AccessMode_ACCESS_MODE_READ_ONLY,
		ProtocolVersion: web.ProtocolVersion,
		Capabilities:    web.ReadCapabilities(),
		Sources: []*v1.SourceCatalogEntry{{
			Source:   &v1.SourceRef{StoreLineageId: "lineage-primary"},
			ReadOnly: readOnly,
		}},
	}
	if !readOnly {
		value.AccessMode = v1.AccessMode_ACCESS_MODE_READ_WRITE
	}
	if board != nil {
		value.Boards = []*v1.BoardSummary{board}
		value.Projects = []*v1.Project{{Id: board.GetProjectId(), Name: "Project"}}
	}
	return value
}

func newAggregateClient(t *testing.T, aggregate *Server) privatev1connect.ProjectServiceClient {
	t.Helper()
	binding := aggregate.Binding()
	client := &http.Client{Transport: handlerTransport{handler: binding.Handler}}
	return privatev1connect.NewProjectServiceClient(client, "http://aggregate.test")
}

type handlerTransport struct {
	handler http.Handler
}

func (t handlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	require.NoError(t, err)
	return parsed
}
