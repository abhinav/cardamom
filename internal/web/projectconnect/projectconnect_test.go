package projectconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.abhg.dev/cardamom/internal/board"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/project"
	projectcreation "go.abhg.dev/cardamom/internal/project/creation"
	"go.uber.org/mock/gomock"
)

func TestServiceBootstrapAndBoard(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	projectTwo := testProject(t, "project-2", "Project Two")
	unsafeDescription := "<script>alert('unsafe')</script>\n\n**Safe board context**"
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", &unsafeDescription)
	boardTwo := testBoard(t, "board-2", projectTwo.ID(), "Board Two", nil)
	defaultBoard := boardTwo.ID()
	catalog := &testCatalog{
		projects: []*project.State{projectOne, projectTwo},
		boards:   []*board.State{boardOne, boardTwo},
	}
	client := newTestClient(t, Config{
		Projects:       project.NewService(catalog),
		ProjectCreator: NewMockProjectCreator(gomock.NewController(t)),
		Boards:         board.NewService(catalog, catalog),
		Markdown:       markdown.New(), ServerDefaultBoardID: &defaultBoard,
		SchemaVersion: 20260718164341,
		Version:       "v1.2.3",
	})

	projects, err := client.ListProjects(
		t.Context(),
		connect.NewRequest(&privatev1.ListProjectsRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, projects.Msg.GetProjects(), 2)
	assert.Equal(t, "project-1", projects.Msg.GetProjects()[0].GetId())

	boards, err := client.ListBoards(
		t.Context(),
		connect.NewRequest(&privatev1.ListBoardsRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, boards.Msg.GetBoards(), 2)
	assert.Equal(t, "board-1", boards.Msg.GetBoards()[0].GetId())

	bootstrap, err := client.GetBootstrap(
		t.Context(),
		connect.NewRequest(&privatev1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, bootstrap.Msg.GetProjects(), 2)
	assert.Equal(t, "project-1", bootstrap.Msg.GetProjects()[0].GetId())
	require.Len(t, bootstrap.Msg.GetBoards(), 2)
	assert.Equal(t, "board-2", bootstrap.Msg.GetServerDefaultBoardId())
	assert.Equal(t, uint64(20260718164341), bootstrap.Msg.GetSchemaVersion())
	assert.Equal(t, "v1.2.3", bootstrap.Msg.GetVersion())
	assert.Len(t, bootstrap.Msg.GetIssueTypes(), 4)
	assert.Len(t, bootstrap.Msg.GetIssueStatuses(), 6)

	response, err := client.GetBoard(
		t.Context(),
		connect.NewRequest(&privatev1.GetBoardRequest{BoardId: "board-1"}),
	)
	require.NoError(t, err)
	assert.Equal(t, "project-1", response.Msg.GetBoard().GetProjectId())
	assert.Equal(t, unsafeDescription, response.Msg.GetBoard().GetDescription().GetSource())
	assert.NotContains(t, response.Msg.GetBoard().GetDescription().GetRenderedHtml(), "<script")
	assert.Contains(t, response.Msg.GetBoard().GetDescription().GetRenderedHtml(), "<strong>Safe board context</strong>")

	_, err = client.GetBoard(t.Context(), connect.NewRequest(&privatev1.GetBoardRequest{}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = client.GetBoard(
		t.Context(),
		connect.NewRequest(&privatev1.GetBoardRequest{BoardId: "missing"}),
	)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestServiceRendersBoardDescriptionInBoardScope(t *testing.T) {
	projectState := testProject(t, "project-1", "Project One")
	description := "[diagram](attachment:att_aaaaaaaaaaaaaaaaaaaaaaaaaa)"
	boardState := testBoard(t, "board-1", projectState.ID(), "Board One", &description)
	catalog := &testCatalog{
		projects: []*project.State{projectState}, boards: []*board.State{boardState},
	}
	renderer := NewMockMarkdownRenderer(gomock.NewController(t))
	renderer.EXPECT().RenderBoard(
		gomock.Any(), board.ID("board-1"), "", []string{description},
	).Return([]string{description}, nil)
	client := newTestClient(t, Config{
		Projects:       project.NewService(catalog),
		ProjectCreator: NewMockProjectCreator(gomock.NewController(t)),
		Boards:         board.NewService(catalog, catalog),
		Markdown:       renderer,
	})

	response, err := client.GetBoard(
		t.Context(),
		connect.NewRequest(&privatev1.GetBoardRequest{BoardId: "board-1"}),
	)
	require.NoError(t, err)

	assert.Equal(t, description, response.Msg.GetBoard().GetDescription().GetSource())
}

func TestServiceBoardMutations(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	description := "Initial **context**"
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", &description)
	catalog := &testCatalog{
		projects: []*project.State{projectOne},
		boards:   []*board.State{boardOne},
	}
	client := newTestClient(t, Config{
		Projects:       project.NewService(catalog),
		ProjectCreator: NewMockProjectCreator(gomock.NewController(t)),
		Boards:         board.NewService(catalog, catalog),
		Markdown:       markdown.New(),
	})

	created, err := client.CreateBoard(t.Context(), connect.NewRequest(&privatev1.CreateBoardRequest{
		ProjectId: "project-1", Name: "New Board",
		DescriptionSource: new("New **description**"),
		Context:           mutationContext("captain"),
	}))
	require.NoError(t, err)
	require.Len(t, catalog.created, 1)
	assert.Equal(t, projectOne.ID().String(), catalog.created[0].ProjectID)
	assert.Equal(t, "created-board", created.Msg.GetBoard().GetId())
	assert.Contains(t, created.Msg.GetBoard().GetDescription().GetRenderedHtml(), "<strong>description</strong>")

	updated, err := client.UpdateBoard(t.Context(), connect.NewRequest(&privatev1.UpdateBoardRequest{
		BoardId: "board-1", Name: new("Renamed"), DescriptionSource: new(""),
		Context: mutationContext("engineer"),
	}))
	require.NoError(t, err)
	require.Len(t, catalog.edited, 1)
	assert.Equal(t, "Renamed", updated.Msg.GetBoard().GetName())
	assert.Nil(t, updated.Msg.GetBoard().GetDescription())

	_, err = client.UpdateBoard(t.Context(), connect.NewRequest(&privatev1.UpdateBoardRequest{
		BoardId: "board-1",
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestService_CreateProject(t *testing.T) {
	created := testProject(t, "project-created", "Mission Control")
	projectCreator := NewMockProjectCreator(gomock.NewController(t))
	catalog := &testCatalog{}
	prefix := "mission-"
	projectCreator.EXPECT().CreateProject(
		gomock.Any(),
		projectcreation.NewInvocation("captain"),
		projectcreation.Request{Name: "Mission Control", Prefix: &prefix},
	).Return(created, nil)
	client := newTestClient(t, Config{
		Projects:       project.NewService(catalog),
		ProjectCreator: projectCreator,
		Boards:         board.NewService(catalog, catalog),
		Markdown:       markdown.New(),
	})
	response, err := client.CreateProject(
		t.Context(),
		connect.NewRequest(&privatev1.CreateProjectRequest{
			Name:    "Mission Control",
			Prefix:  &prefix,
			Context: mutationContext("captain"),
		}),
	)
	require.NoError(t, err)

	assert.Equal(t, "project-created", response.Msg.GetProject().GetId())
	assert.Equal(t, "Mission Control", response.Msg.GetProject().GetName())
}

func newTestClient(t *testing.T, cfg Config) privatev1connect.ProjectServiceClient {
	t.Helper()
	_, handler := privatev1connect.NewProjectServiceHandler(New(cfg))
	httpClient := &http.Client{Transport: &testHandlerTransport{handler: handler}}
	return privatev1connect.NewProjectServiceClient(httpClient, "http://cardamom.test")
}

type testHandlerTransport struct {
	handler http.Handler
}

func (t *testHandlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

type testCatalog struct {
	projects []*project.State
	boards   []*board.State
	created  []board.CreateRequest
	edited   []board.EditRequest
}

func (c *testCatalog) ListProjects(context.Context) ([]*project.State, error) {
	return c.projects, nil
}

func (c *testCatalog) ListAllBoards(context.Context) ([]*board.State, error) {
	return c.boards, nil
}

func (c *testCatalog) Board(_ context.Context, id board.ID) (*board.State, error) {
	for _, board := range c.boards {
		if board.ID() == id {
			return board, nil
		}
	}
	return nil, errkind.Errorf(errkind.NotFound, "board not found")
}

func (c *testCatalog) SoleBoard(context.Context) (*board.State, error) {
	if len(c.boards) == 1 {
		return c.boards[0], nil
	}
	return nil, errkind.Errorf(errkind.Conflict, "board selection is ambiguous")
}

func (c *testCatalog) CreateBoard(
	_ context.Context,
	request board.CreateRequest,
) (*board.State, error) {
	c.created = append(c.created, request)
	value, err := board.Load(board.Snapshot{
		ID: "created-board", ProjectID: request.ProjectID,
		Name: request.Name, Description: request.Description,
		Created: time.Unix(3, 0).UTC(),
	})
	if err != nil {
		return nil, err
	}
	c.boards = append(c.boards, value)
	return value, nil
}

func (c *testCatalog) EditBoardSettings(
	_ context.Context,
	request board.EditRequest,
) (*board.State, error) {
	c.edited = append(c.edited, request)
	for index, state := range c.boards {
		if state.ID() != request.BoardID {
			continue
		}
		edited, err := state.EditSettings(request.Settings)
		if err != nil {
			return nil, err
		}
		c.boards[index] = edited
		return edited, nil
	}
	return nil, errkind.Errorf(errkind.NotFound, "board not found")
}

func testProject(t *testing.T, id, name string) *project.State {
	t.Helper()
	value, err := project.Load(project.Snapshot{
		ID: project.ID(id), Name: name, Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return value
}

func testBoard(
	t *testing.T,
	id string,
	projectID project.ID,
	name string,
	description *string,
) *board.State {
	t.Helper()
	value, err := board.Load(board.Snapshot{
		ID: board.ID(id), ProjectID: projectID.String(), Name: name,
		Description: description, Created: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	return value
}

func mutationContext(actor string) *privatev1.MutationContext {
	return &privatev1.MutationContext{Actor: &actor}
}
