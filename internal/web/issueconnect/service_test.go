package issueconnect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/board/selection"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

func TestServiceListIssuesAppliesOneGlobalOrderAndLimit(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	boardTwo := testBoard(t, "board-2", projectOne.ID(), "Board Two", nil)
	readerOne := &testBoardReader{issues: []issue.Summary{
		testIssueSummary("issue-z", "Needle later priority", "task", "open", "ready", 1, 20, []string{"blue"}, false),
		testIssueSummary("issue-a", "Needle alpha", "task", "open", "ready", 0, 30, []string{"alpha"}, false),
	}}
	readerTwo := &testBoardReader{issues: []issue.Summary{
		testIssueSummary("issue-b", "Needle beta", "checkpoint", "open", "ready", 0, 10, []string{"blue", "green"}, true),
	}}
	client := newTestClient(t, testConfig{
		Catalog:      &testCatalog{boards: []*board.State{boardOne, boardTwo}},
		IssueBoards:  testIssueLocator{},
		BoardReaders: testBoardReaders{"board-1": readerOne, "board-2": readerTwo},
		Markdown:     markdown.New(),
	})

	response, err := client.ListIssues(
		t.Context(),
		connect.NewRequest(&privatev1.ListIssuesRequest{
			Scope: &privatev1.BoardScope{
				Selection: &privatev1.BoardScope_AllBoards{AllBoards: &privatev1.AllBoards{}},
			},
			TitleQuery: new("needle"),
			Limit:      2,
		}),
	)
	require.NoError(t, err)
	require.Len(t, response.Msg.GetIssues(), 2)
	assert.Equal(t, uint32(3), response.Msg.GetTotalCount())
	assert.Equal(t, "issue-b", response.Msg.GetIssues()[0].GetId())
	assert.Equal(t, "board-2", response.Msg.GetIssues()[0].GetBoardId())
	assert.Equal(t, privatev1.IssueStatus_ISSUE_STATUS_BLOCKED, response.Msg.GetIssues()[0].GetStatus())
	assert.Equal(t, "issue-a", response.Msg.GetIssues()[1].GetId())
	assert.True(t, response.Msg.GetTruncated())
	assert.Equal(t, []*privatev1.LabelFacet{
		{Label: "alpha", IssueCount: 1},
		{Label: "blue", IssueCount: 2},
		{Label: "green", IssueCount: 1},
	}, response.Msg.GetLabelFacets())
	for _, reader := range []*testBoardReader{readerOne, readerTwo} {
		require.Len(t, reader.requests, 2)
		assert.Equal(t, 3, reader.requests[0].Limit)
		assert.Zero(t, reader.requests[1].Limit)
	}
}

func TestListIssueRequestMapsLabelSelectors(t *testing.T) {
	t.Parallel()

	request, filters, err := listIssueRequest(&privatev1.ListIssuesRequest{
		LabelsAll:  []string{"area:protocol", "ready"},
		LabelsAny:  []string{"phase:a", "phase:b"},
		LabelsNone: []string{"paused"},
	})
	require.NoError(t, err)
	assert.Equal(t, issue.ListRequest{
		LabelsAll:  []string{"area:protocol", "ready"},
		LabelsAny:  []string{"phase:a", "phase:b"},
		LabelsNone: []string{"paused"},
	}, request)
	assert.Empty(t, filters.lifecycles)
	assert.Empty(t, filters.statuses)
	assert.Empty(t, filters.types)
}

func TestListIssueRequestRejectsInvalidLabelSelectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give *privatev1.ListIssuesRequest
		want string
	}{
		{
			name: "LabelsAll",
			give: &privatev1.ListIssuesRequest{LabelsAll: []string{"+required"}},
			want: "label cannot start with + or -",
		},
		{
			name: "LabelsAny",
			give: &privatev1.ListIssuesRequest{LabelsAny: []string{" "}},
			want: "label cannot be empty",
		},
		{
			name: "LabelsNone",
			give: &privatev1.ListIssuesRequest{LabelsNone: []string{"-paused"}},
			want: "label cannot start with + or -",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := listIssueRequest(tt.give)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestServiceListIssuesContinuesStablePage(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	reader := &testBoardReader{
		revision: 7,
		issues: []issue.Summary{
			testIssueSummary("issue-d", "Delta", "task", "open", "ready", 0, 40, nil, false),
			testIssueSummary("issue-b", "Bravo", "task", "open", "ready", 0, 20, nil, false),
			testIssueSummary("issue-c", "Charlie", "task", "open", "ready", 0, 30, nil, false),
			testIssueSummary("issue-a", "Alpha", "task", "open", "ready", 0, 10, nil, false),
		},
	}
	client := newTestClient(t, testConfig{
		Catalog:      &testCatalog{boards: []*board.State{boardOne}},
		IssueBoards:  testIssueLocator{},
		BoardReaders: testBoardReaders{"board-1": reader},
		Markdown:     markdown.New(),
	})
	request := &privatev1.ListIssuesRequest{
		Scope: &privatev1.BoardScope{
			Selection: &privatev1.BoardScope_BoardId{BoardId: "board-1"},
		},
		Sort:  privatev1.IssueSort_ISSUE_SORT_TITLE,
		Limit: 2,
	}

	first, err := client.ListIssues(t.Context(), connect.NewRequest(request))
	require.NoError(t, err)
	assert.Equal(t, []string{"issue-a", "issue-b"}, issueIDs(first.Msg.GetIssues()))
	assert.Equal(t, uint32(4), first.Msg.GetTotalCount())
	require.NotEmpty(t, first.Msg.GetNextPageToken())
	assert.True(t, first.Msg.GetTruncated())

	request.PageToken = new(first.Msg.GetNextPageToken())
	second, err := client.ListIssues(t.Context(), connect.NewRequest(request))
	require.NoError(t, err)
	assert.Equal(t, []string{"issue-c", "issue-d"}, issueIDs(second.Msg.GetIssues()))
	assert.Equal(t, uint32(4), second.Msg.GetTotalCount())
	assert.Empty(t, second.Msg.GetNextPageToken())
	assert.False(t, second.Msg.GetTruncated())
}

func TestServiceListIssuesRejectsInvalidContinuation(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	reader := &testBoardReader{
		revision: 3,
		issues: []issue.Summary{
			testIssueSummary("issue-a", "Alpha", "task", "open", "ready", 0, 10, nil, false),
			testIssueSummary("issue-b", "Bravo", "task", "open", "ready", 0, 20, nil, false),
		},
	}
	client := newTestClient(t, testConfig{
		Catalog:      &testCatalog{boards: []*board.State{boardOne}},
		IssueBoards:  testIssueLocator{},
		BoardReaders: testBoardReaders{"board-1": reader},
		Markdown:     markdown.New(),
	})
	request := &privatev1.ListIssuesRequest{
		Scope: &privatev1.BoardScope{
			Selection: &privatev1.BoardScope_BoardId{BoardId: "board-1"},
		},
		Sort:  privatev1.IssueSort_ISSUE_SORT_TITLE,
		Limit: 1,
	}
	first, err := client.ListIssues(t.Context(), connect.NewRequest(request))
	require.NoError(t, err)
	token := first.Msg.GetNextPageToken()
	require.NotEmpty(t, token)

	changedQuery := &privatev1.ListIssuesRequest{
		Scope: request.Scope, Sort: privatev1.IssueSort_ISSUE_SORT_UPDATED_AT,
		Limit: request.Limit, PageToken: &token,
	}
	_, err = client.ListIssues(t.Context(), connect.NewRequest(changedQuery))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	reader.revision++
	request.PageToken = &token
	_, err = client.ListIssues(t.Context(), connect.NewRequest(request))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	malformed := "not-a-page-token"
	request.PageToken = &malformed
	_, err = client.ListIssues(t.Context(), connect.NewRequest(request))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestServiceListIssuesFiltersProtocolDimensions(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	reader := &testBoardReader{issues: []issue.Summary{
		testIssueSummary("blocked-checkpoint", "Blocked gate", "checkpoint", "open", "ready", 0, 10, nil, true),
		testIssueSummary("open-checkpoint", "Open gate", "checkpoint", "open", "ready", 0, 20, nil, false),
		testIssueSummary("blocked-task", "Blocked task", "task", "open", "ready", 0, 30, nil, true),
	}, filteredTotal: new(1)}
	client := newTestClient(t, testConfig{
		Catalog:      &testCatalog{boards: []*board.State{boardOne}},
		IssueBoards:  testIssueLocator{},
		BoardReaders: testBoardReaders{"board-1": reader},
		Markdown:     markdown.New(),
	})

	response, err := client.ListIssues(
		t.Context(),
		connect.NewRequest(&privatev1.ListIssuesRequest{
			Scope: &privatev1.BoardScope{
				Selection: &privatev1.BoardScope_BoardId{BoardId: "board-1"},
			},
			Lifecycles: []privatev1.IssueLifecycle{privatev1.IssueLifecycle_ISSUE_LIFECYCLE_OPEN},
			Statuses:   []privatev1.IssueStatus{privatev1.IssueStatus_ISSUE_STATUS_BLOCKED},
			Types:      []privatev1.IssueType{privatev1.IssueType_ISSUE_TYPE_CHECKPOINT},
		}),
	)
	require.NoError(t, err)
	require.Len(t, response.Msg.GetIssues(), 1)
	assert.Equal(t, uint32(1), response.Msg.GetTotalCount())
	assert.Equal(t, "blocked-checkpoint", response.Msg.GetIssues()[0].GetId())
}

func TestServiceListIssuesReturnsPresentEmptyCollections(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	service := newTestService(t, testConfig{
		Catalog:      &testCatalog{boards: []*board.State{boardOne}},
		IssueBoards:  testIssueLocator{},
		BoardReaders: testBoardReaders{"board-1": {}},
		Markdown:     markdown.New(),
	})
	client := newTestClientForService(t, service)
	request := &privatev1.ListIssuesRequest{
		Scope: &privatev1.BoardScope{
			Selection: &privatev1.BoardScope_BoardId{BoardId: "board-1"},
		},
	}

	response, err := client.ListIssues(
		t.Context(),
		connect.NewRequest(request),
	)
	require.NoError(t, err)
	assert.Empty(t, response.Msg.GetIssues())
	assert.Empty(t, response.Msg.GetLabelFacets())

	direct, err := service.ListIssues(t.Context(), connect.NewRequest(request))
	require.NoError(t, err)
	assert.NotNil(t, direct.Msg.Issues)
	assert.NotNil(t, direct.Msg.LabelFacets)
}

func TestServiceGetIssue(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardDescription := "# Shared context"
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", &boardDescription)
	rootID := "root"
	selectedID := "selected"
	reader := &testBoardReader{
		views: map[string]issue.View{
			"selected": {
				Detail: issue.Detail{
					Issue: issue.Issue{
						ID: "selected", Title: "Selected issue", Type: "task",
						Lifecycle: "open", Status: "ready", Priority: 1,
						Created: 20, Updated: 30,
						Summary: new("**Summary**"), Details: new("Expanded details"),
						State: new("_Current state_"),
					},
					Keys:    []string{" producer:z ", "producer:a"},
					Labels:  []string{"area:web"},
					Blocked: true,
					DependsOn: []issue.Reference{
						testIssueReference("closed-dep", "Closed dependency", "task", "closed", 3),
						testIssueReference("open-dep", "Open dependency", "task", "ready", 0),
					},
					Blocks: []issue.Reference{
						testIssueReference("dependent", "Dependent", "task", "ready", 2),
					},
					CurrentResult: &issue.Result{
						IssueID: "selected", Title: "Selected issue", Body: "Result body",
					},
					LogSummary: issue.LogSummary{Count: 2, LatestID: new(issue.LogID("cmt_12121212121212121212121212121212"))},
					Story: issue.Story{
						Containment: []issue.ContainmentNode{
							{Reference: testIssueReference("root", "Root", "workstream", "ready", 0)},
							{Reference: testIssueReference("selected", "Selected issue", "task", "blocked", 1), ParentID: &rootID},
							{Reference: testIssueReference("child", "Child", "task", "ready", 2), ParentID: &selectedID},
							{Reference: testIssueReference("sibling", "Sibling", "task", "ready", 2), ParentID: &rootID},
						},
						DependsOn: []issue.Reference{testIssueReference("open-dep", "Open dependency", "task", "ready", 0)},
						Blocks:    []issue.Reference{testIssueReference("dependent", "Dependent", "task", "ready", 2)},
					},
				},
				Context: &issue.Context{
					Board: issue.BoardDescription{Description: &boardDescription},
					Ancestors: []issue.ContextEntry{{
						Issue: issue.Issue{
							ID: "root", Title: "Root", Type: "workstream",
							Lifecycle: "open", Status: "ready", Priority: 0,
							Created: 1, Updated: 2,
							Summary: new("Root summary"), State: new("Root state"),
						},
						LogSummary: issue.LogSummary{Count: 3, LatestID: new(issue.LogID("cmt_99999999999999999999999999999999"))},
					}},
					DependencyResults: []issue.DependencyResult{{
						Issue: testIssueReference("closed-dep", "Closed dependency", "task", "closed", 3),
						Body:  "**Done**",
					}},
				},
			},
			"closed-dep": {
				Detail: issue.Detail{Issue: issue.Issue{
					ID: "closed-dep", Title: "Closed dependency", Type: "task",
					Lifecycle: "closed", Status: "closed", Priority: 3,
					Created: 3, Updated: 4, Closed: new(int64(4)),
				}},
			},
			"open-dep": {
				Detail: issue.Detail{Issue: testIssue("open-dep", "Open dependency", "task", "open", "ready", 0, 5)},
			},
			"dependent": {
				Detail: issue.Detail{Issue: testIssue("dependent", "Dependent", "task", "open", "ready", 2, 6)},
			},
		},
	}
	client := newTestClient(t, testConfig{
		Catalog:      &testCatalog{boards: []*board.State{boardOne}},
		IssueBoards:  testIssueLocator{"selected": "board-1"},
		BoardReaders: testBoardReaders{"board-1": reader},
		Markdown:     markdown.New(),
	})

	response, err := client.GetIssue(
		t.Context(),
		connect.NewRequest(&privatev1.GetIssueRequest{IssueId: "selected"}),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"selected"}, reader.readIssueIDs)
	detail := response.Msg.GetIssue()
	assert.Equal(t, "board-1", detail.GetIssue().GetBoardId())
	assert.Equal(t, privatev1.IssueStatus_ISSUE_STATUS_BLOCKED, detail.GetIssue().GetStatus())
	assert.Equal(t, []string{" producer:z ", "producer:a"}, detail.GetExternalKeys())
	assert.Contains(t, detail.GetSummary().GetRenderedHtml(), "<strong>Summary</strong>")
	assert.Equal(t, "Expanded details", detail.GetDetails().GetSource())
	assert.Contains(t, detail.GetState().GetRenderedHtml(), "<em>Current state</em>")
	assert.Equal(t, "Result body", detail.GetResult().GetSource())
	require.Len(t, detail.GetContext().GetAncestors(), 1)
	assert.Equal(t, uint32(3), detail.GetContext().GetAncestors()[0].GetLogCount())
	require.Len(t, detail.GetContext().GetDependencyResults(), 1)
	assert.Equal(t, privatev1.IssueType_ISSUE_TYPE_TASK, detail.GetContext().GetDependencyResults()[0].GetIssue().GetType())
	assert.Contains(t, detail.GetContext().GetDependencyResults()[0].GetResult().GetRenderedHtml(), "<strong>Done</strong>")
	require.Len(t, detail.GetContainment().GetNodes(), 4)
	assert.Equal(t, uint32(0), detail.GetContainment().GetNodes()[0].GetDepth())
	assert.True(t, detail.GetContainment().GetNodes()[0].GetSelectedPath())
	assert.Equal(t, uint32(1), detail.GetContainment().GetNodes()[1].GetDepth())
	assert.True(t, detail.GetContainment().GetNodes()[1].GetSelectedPath())
	assert.Equal(t, uint32(2), detail.GetContainment().GetNodes()[2].GetDepth())
	assert.False(t, detail.GetContainment().GetNodes()[2].GetSelectedPath())
	assert.Equal(t, uint32(2), detail.GetLogCount())
	assert.Equal(t, "cmt_12121212121212121212121212121212", detail.GetLatestLogId())
	require.Len(t, detail.GetPrerequisites(), 2)
	assert.Equal(t, []string{"closed-dep", "open-dep"}, []string{
		detail.GetPrerequisites()[0].GetId(), detail.GetPrerequisites()[1].GetId(),
	})
	require.Len(t, detail.GetDependents(), 1)
	assert.Equal(t, "dependent", detail.GetDependents()[0].GetId())
}

type testConfig struct {
	Catalog      boardscope.Catalog
	IssueBoards  boardscope.IssueLocator
	BoardReaders BoardReaderFactory
	Markdown     issueview.MarkdownRenderer
}

func newTestService(t *testing.T, cfg testConfig) *Service {
	t.Helper()
	return New(Config{
		Scope:   boardscope.New(cfg.Catalog, cfg.IssueBoards),
		Readers: cfg.BoardReaders, Views: issueview.New(cfg.Markdown),
	})
}

func newTestClient(t *testing.T, cfg testConfig) privatev1connect.IssueServiceClient {
	t.Helper()
	return newTestClientForService(t, newTestService(t, cfg))
}

func newTestClientForService(t *testing.T, service *Service) privatev1connect.IssueServiceClient {
	t.Helper()
	_, handler := privatev1connect.NewIssueServiceHandler(service)
	httpClient := &http.Client{Transport: &testHandlerTransport{handler: handler}}
	return privatev1connect.NewIssueServiceClient(httpClient, "http://cardamom.test")
}

// testHandlerTransport keeps generated-client tests at the HTTP boundary
// without requiring a listener from the test environment.
type testHandlerTransport struct {
	handler http.Handler
}

func (t *testHandlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

type testCatalog struct {
	boards []*board.State
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

func (c *testCatalog) Get(ctx context.Context, id board.ID) (*board.State, error) {
	return c.Board(ctx, id)
}

func (c *testCatalog) List(ctx context.Context) ([]*board.State, error) {
	return c.ListAllBoards(ctx)
}

type testIssueLocator map[string]board.ID

func (l testIssueLocator) BoardForIssue(_ context.Context, issueID string) (board.ID, error) {
	boardID, ok := l[issueID]
	if !ok {
		return "", selection.ErrIssueNotFound
	}
	return boardID, nil
}

type testBoardReaders map[board.ID]*testBoardReader

func (f testBoardReaders) Reader(boardID board.ID) (BoardReader, error) {
	reader, ok := f[boardID]
	if !ok || reader == nil {
		return nil, errors.New("test board reader not configured")
	}
	return reader, nil
}

type testBoardReader struct {
	issues        []issue.Summary
	views         map[string]issue.View
	requests      []issue.ListRequest
	readIssueIDs  []string
	listErr       error
	revision      uint64
	filteredTotal *int
}

func (r *testBoardReader) ListIssues(_ context.Context, request issue.ListRequest) ([]issue.Summary, error) {
	r.requests = append(r.requests, request)
	if r.listErr != nil {
		return nil, r.listErr
	}
	if request.TitleContains == "" {
		return r.issues, nil
	}
	var out []issue.Summary
	for _, value := range r.issues {
		if request.TitleContains == "needle" &&
			(value.Issue.ID == "issue-a" || value.Issue.ID == "issue-b" || value.Issue.ID == "issue-z") {
			out = append(out, value)
		}
	}
	return out, nil
}

func (r *testBoardReader) ListIssuesSnapshot(
	ctx context.Context,
	request issue.ListRequest,
) (issue.ListSnapshot, error) {
	issues, err := r.ListIssues(ctx, request)
	total := len(issues)
	if r.filteredTotal != nil {
		total = *r.filteredTotal
	}
	return issue.ListSnapshot{
		Issues: issues, Total: total,
		Cursor: issue.ChangeCursor{Revision: int64(r.revision)},
	}, err
}

func (r *testBoardReader) ReadIssue(_ context.Context, request issue.ReadRequest) (issue.View, error) {
	r.readIssueIDs = append(r.readIssueIDs, request.IssueID)
	view, ok := r.views[request.IssueID]
	if !ok {
		return issue.View{}, errkind.Errorf(errkind.NotFound, "issue not found")
	}
	return view, nil
}

func issueIDs(values []*privatev1.IssueSummary) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.GetId()
	}
	return result
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

func testIssueSummary(
	id, title, kind, lifecycle, status string,
	priority int,
	created int64,
	labels []string,
	blocked bool,
) issue.Summary {
	return issue.Summary{
		Issue:  testIssue(id, title, kind, lifecycle, status, priority, created),
		Labels: labels, Blocked: blocked,
	}
}

func testIssue(
	id, title, kind, lifecycle, status string,
	priority int,
	created int64,
) issue.Issue {
	return issue.Issue{
		ID: id, Title: title, Type: kind, Lifecycle: lifecycle, Status: status,
		Priority: priority, Created: created, Updated: created,
	}
}

func testIssueReference(id, title, kind, status string, priority int) issue.Reference {
	return issue.Reference{
		ID: id, Title: title, Type: kind, Status: status, Priority: priority,
	}
}
