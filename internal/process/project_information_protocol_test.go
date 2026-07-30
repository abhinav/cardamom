package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
)

func TestProjectAndInformationProtocolUsesFreshStoreOperations(t *testing.T) {
	cfg := testConfig(t)
	initialized := execute(
		t,
		cfg,
		"--json",
		"init",
		"--prefix",
		"mission-",
		"--board-name",
		"Primary",
	)
	require.Equal(t, cli.ExitSuccess, initialized.code, initialized.stderr)
	var namespace cli.InitResult
	require.NoError(t, json.Unmarshal([]byte(initialized.stdout), &namespace))
	require.NotNil(t, namespace.ProjectID)
	require.NotNil(t, namespace.BoardID)

	operation := &webOperation{config: cfg}
	binding, closeStore, err := operation.open(t.Context(), cli.WebRequest{
		Store: filepath.Join(cfg.CWD, ".cardamom"),
		Board: *namespace.BoardID,
	})
	require.NoError(t, err)
	storeOpen := true
	t.Cleanup(func() {
		if storeOpen {
			assert.NoError(t, closeStore())
		}
	})
	client := &http.Client{Transport: protocolRoundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			binding.handler.ServeHTTP(recorder, request)
			return recorder.Result(), nil
		},
	)}
	projects := privatev1connect.NewProjectServiceClient(client, "http://cardamom.test")
	storeInformation := privatev1connect.NewInformationServiceClient(client, "http://cardamom.test")

	projectList, err := projects.ListProjects(
		t.Context(),
		connect.NewRequest(&privatev1.ListProjectsRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, projectList.Msg.GetProjects(), 1)
	assert.Equal(t, *namespace.ProjectID, projectList.Msg.GetProjects()[0].GetId())

	boardList, err := projects.ListBoards(
		t.Context(),
		connect.NewRequest(&privatev1.ListBoardsRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, boardList.Msg.GetBoards(), 1)
	assert.Equal(t, *namespace.BoardID, boardList.Msg.GetBoards()[0].GetId())

	invalidPrefix := "INVALID"
	_, err = projects.CreateProject(
		t.Context(),
		connect.NewRequest(&privatev1.CreateProjectRequest{
			Name:   "Invalid Project",
			Prefix: &invalidPrefix,
		}),
	)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	projectPrefix := "secondary-"
	createdProject, err := projects.CreateProject(
		t.Context(),
		connect.NewRequest(&privatev1.CreateProjectRequest{
			Name:    "Secondary Project",
			Prefix:  &projectPrefix,
			Context: &privatev1.MutationContext{Actor: new("protocol-engineer")},
		}),
	)
	require.NoError(t, err)

	projectList, err = projects.ListProjects(
		t.Context(),
		connect.NewRequest(&privatev1.ListProjectsRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, projectList.Msg.GetProjects(), 2)
	assert.Equal(t, createdProject.Msg.GetProject().GetId(), projectList.Msg.GetProjects()[1].GetId())
	boardList, err = projects.ListBoards(
		t.Context(),
		connect.NewRequest(&privatev1.ListBoardsRequest{}),
	)
	require.NoError(t, err)
	assert.Len(t, boardList.Msg.GetBoards(), 1)

	createdBoard, err := projects.CreateBoard(
		t.Context(),
		connect.NewRequest(&privatev1.CreateBoardRequest{
			ProjectId: createdProject.Msg.GetProject().GetId(),
			Name:      "Secondary",
			Context:   &privatev1.MutationContext{Actor: new("protocol-engineer")},
		}),
	)
	require.NoError(t, err)
	createdBoardID := createdBoard.Msg.GetBoard().GetId()

	updated, err := projects.UpdateBoard(
		t.Context(),
		connect.NewRequest(&privatev1.UpdateBoardRequest{
			BoardId: createdBoardID,
			Name:    new("Renamed"),
			Context: &privatev1.MutationContext{Actor: new("protocol-engineer")},
		}),
	)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.Msg.GetBoard().GetName())
	retrieved, err := projects.GetBoard(
		t.Context(),
		connect.NewRequest(&privatev1.GetBoardRequest{BoardId: createdBoardID}),
	)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", retrieved.Msg.GetBoard().GetName())

	informationResponse, err := storeInformation.GetInformation(
		t.Context(),
		connect.NewRequest(&privatev1.GetInformationRequest{BoardId: createdBoardID}),
	)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cfg.CWD, ".cardamom"), informationResponse.Msg.GetStore().GetDirectory())
	assert.Equal(t, createdProject.Msg.GetProject().GetId(), informationResponse.Msg.GetProject().GetId())
	assert.Equal(t, createdBoardID, informationResponse.Msg.GetBoard().GetId())
	assert.Equal(t, "secondary-", informationResponse.Msg.GetConfiguration().GetIssue().GetId().GetPrefix())
	assert.Equal(t, uint64(4), informationResponse.Msg.GetRevision().GetCurrent())
	assert.Zero(t, informationResponse.Msg.GetIssues().GetTotal())

	require.NoError(t, closeStore())
	storeOpen = false
	shown := execute(
		t,
		cfg,
		"--store",
		filepath.Join(cfg.CWD, ".cardamom"),
		"--json",
		"board",
		"show",
		createdBoardID,
	)
	require.Equal(t, cli.ExitSuccess, shown.code, shown.stderr)
	assert.JSONEq(t, `{
		"id":"`+createdBoardID+`",
		"project_id":"`+createdProject.Msg.GetProject().GetId()+`",
		"name":"Renamed",
		"created":1784376000,
		"description":null
	}`, shown.stdout)
}

type protocolRoundTripFunc func(*http.Request) (*http.Response, error)

func (f protocolRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
