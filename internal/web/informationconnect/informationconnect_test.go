package informationconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/information"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/project"
)

func TestServiceReturnsStoreInformation(t *testing.T) {
	projectID, err := project.NewID("project-one")
	require.NoError(t, err)
	selectedProject, err := project.Load(project.Snapshot{
		ID: projectID, Name: "Mission", Created: time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)
	boardID, err := board.NewID("board-one")
	require.NoError(t, err)
	selectedBoard, err := board.Load(board.Snapshot{
		ID: boardID, ProjectID: projectID.String(), Name: "Operations",
		Created: time.Unix(20, 0).UTC(),
	})
	require.NoError(t, err)
	reader := &fakeReader{report: information.Report{
		Store: information.Store{
			Directory:    "/stores/mission",
			DatabasePath: "/stores/mission/board.sqlite3",
		},
		Project:       selectedProject,
		Board:         selectedBoard,
		Schema:        information.Schema{DatabaseVersion: 12, CodeVersion: 13},
		Configuration: configuration.Defaults(),
		Revision:      information.Revision{Current: 42},
		Issues: information.IssueInventory{
			Total: 3,
			ByStatus: []information.IssueStatusCount{
				{Status: issue.StatusReady, Count: 2},
				{Status: issue.StatusClosed, Count: 1},
			},
		},
	}}
	client := newTestClient(t, reader)

	response, err := client.GetInformation(
		t.Context(),
		connect.NewRequest(&privatev1.GetInformationRequest{
			BoardId: boardID.String(),
		}),
	)
	require.NoError(t, err)

	assert.Equal(t, information.Request{BoardID: boardID}, reader.request)
	assert.Equal(t, "/stores/mission", response.Msg.GetStore().GetDirectory())
	assert.Equal(t, "project-one", response.Msg.GetProject().GetId())
	assert.Equal(t, "board-one", response.Msg.GetBoard().GetId())
	assert.Equal(t, uint64(12), response.Msg.GetSchema().GetDatabaseVersion())
	assert.Equal(t, "cm-", response.Msg.GetConfiguration().GetIssue().GetId().GetPrefix())
	assert.Equal(t, uint64(42), response.Msg.GetRevision().GetCurrent())
	assert.Equal(t, uint64(3), response.Msg.GetIssues().GetTotal())
	assert.Equal(t, privatev1.IssueStatus_ISSUE_STATUS_READY, response.Msg.GetIssues().GetByStatus()[0].GetStatus())
	assert.Equal(t, uint64(2), response.Msg.GetIssues().GetByStatus()[0].GetCount())
}

func TestServiceRejectsMissingBoard(t *testing.T) {
	client := newTestClient(t, new(fakeReader))

	_, err := client.GetInformation(
		t.Context(),
		connect.NewRequest(&privatev1.GetInformationRequest{}),
	)

	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func newTestClient(
	t *testing.T,
	reader Reader,
) privatev1connect.InformationServiceClient {
	t.Helper()
	_, handler := privatev1connect.NewInformationServiceHandler(New(reader))
	server := &http.Server{Handler: handler}
	transport := &localRoundTripper{server: server}
	return privatev1connect.NewInformationServiceClient(
		&http.Client{Transport: transport},
		"http://cardamom.test",
	)
}

type fakeReader struct {
	report  information.Report
	request information.Request
}

func (f *fakeReader) Read(
	_ context.Context,
	request information.Request,
) (information.Report, error) {
	f.request = request
	return f.report, nil
}

// localRoundTripper serves Connect requests without opening a listener.
type localRoundTripper struct {
	server *http.Server
}

func (r *localRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	r.server.Handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}
