package planningconnect

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	cardamomv1 "go.abhg.dev/cardamom/internal/gen/cardamom/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

func TestServiceMapsAtomicEditAndTypedApplyReferences(t *testing.T) {
	t.Parallel()

	boardState := planningTestBoard(t)
	planner := new(planningStub)
	service := New(Config{
		Scope: boardscope.New(
			planningCatalog{boardState},
			planningLocator{"an-1": boardState.ID()},
		),
		Planners: planningFactory{boardState.ID(): planner},
		Views:    issueview.New(markdown.New()),
	})

	labels := &privatev1.IssueLabels{Values: []string{"replacement"}}
	_, err := service.EditIssue(t.Context(), connect.NewRequest(&privatev1.EditIssueRequest{
		IssueId: "an-1", Title: new("Renamed"), ParentId: new(""),
		AddPrerequisiteIds: []string{"an-dep"}, RemoveLabels: []string{"old"},
		Labels: labels, Context: &privatev1.MutationContext{Actor: new("editor")},
	}))
	require.NoError(t, err)
	assert.Equal(t, "editor", planner.invocation.Actor())
	assert.Equal(t, "Renamed", *planner.edit.Title)
	assert.True(t, planner.edit.ParentSet)
	assert.Nil(t, planner.edit.Parent)
	assert.Equal(t, []string{"an-dep"}, planner.edit.AddDependencies)
	assert.Equal(t, []string{"replacement"}, *planner.edit.Labels)

	response, err := service.ApplyDocument(t.Context(), connect.NewRequest(&privatev1.ApplyDocumentRequest{
		BoardId: boardState.ID().String(),
		Document: &cardamomv1.ApplyDocument{
			Version: 1,
			Issues: []*cardamomv1.ApplyIssue{
				{
					Alias: new("build"), Title: new("Build"), Type: new("task"),
				},
				{
					Alias: new("test"), Title: new("Test"), Type: new("task"),
					DependsOn: &cardamomv1.IssueReferences{Values: []*cardamomv1.IssueReference{{
						Target: &cardamomv1.IssueReference_Alias{Alias: "build"},
					}}},
					ParentChange: &cardamomv1.ApplyIssue_Parent{Parent: &cardamomv1.IssueReference{
						Target: &cardamomv1.IssueReference_Id{Id: "an-parent"},
					}},
					Key: new("source:test"),
					Labels: &cardamomv1.IssueLabels{
						Values: []string{"backend"},
					},
				},
			},
			OnExisting: "update",
		},
		Context: &privatev1.MutationContext{Actor: new("planner")},
	}))
	require.NoError(t, err)
	require.Len(t, planner.apply.Issues, 2)
	require.NotNil(t, planner.apply.Issues[1].DependsOn)
	assert.Equal(t, []planning.ApplyIssueReference{{
		Kind: planning.ApplyReferenceAlias, Alias: "build",
	}}, *planner.apply.Issues[1].DependsOn)
	assert.Equal(t, planning.ApplyIssueReference{
		Kind: planning.ApplyReferenceID, ID: "an-parent",
	}, planner.apply.Issues[1].Parent.Reference)
	assert.Equal(t, planning.ApplyExistingUpdate, planner.apply.OnExisting)
	assert.Equal(t, "planner", planner.invocation.Actor())
	assert.Equal(t, "create", response.Msg.GetReceipt().GetEntries()[0].GetAction())
}

func TestServiceRejectsUnsupportedApplyValues(t *testing.T) {
	t.Parallel()

	boardState := planningTestBoard(t)
	planner := new(planningStub)
	service := New(Config{
		Scope: boardscope.New(
			planningCatalog{boardState},
			planningLocator{},
		),
		Planners: planningFactory{boardState.ID(): planner},
		Views:    issueview.New(markdown.New()),
	})
	tests := []struct {
		name      string
		document  *cardamomv1.ApplyDocument
		wantError string
	}{
		{
			name: "ExistingPolicy",
			document: &cardamomv1.ApplyDocument{
				Version: 1, OnExisting: "APPLY_EXISTING_POLICY_UPDATE",
			},
			wantError: "on_existing",
		},
		{
			name: "IssueType",
			document: &cardamomv1.ApplyDocument{
				Version: 1,
				Issues: []*cardamomv1.ApplyIssue{{
					Title: new("Build"), Type: new("ISSUE_TYPE_TASK"),
				}},
			},
			wantError: "issues[0].type",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.ApplyDocument(t.Context(), connect.NewRequest(&privatev1.ApplyDocumentRequest{
				BoardId: boardState.ID().String(), Document: test.document,
			}))

			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.ErrorContains(t, err, test.wantError)
			assert.Zero(t, planner.applyCalls)
			assert.Empty(t, planner.apply.Issues)
		})
	}
}

func TestServiceRejectsInvalidApplyReceiptAction(t *testing.T) {
	t.Parallel()

	boardState := planningTestBoard(t)
	planner := &planningStub{applyResult: &planning.ApplyReceipt{
		Entries: []planning.ApplyReceiptEntry{{Action: planning.ApplyActionUnknown}},
	}}
	service := New(Config{
		Scope:    boardscope.New(planningCatalog{boardState}, planningLocator{}),
		Planners: planningFactory{boardState.ID(): planner},
		Views:    issueview.New(markdown.New()),
	})

	response, err := service.ApplyDocument(t.Context(), connect.NewRequest(&privatev1.ApplyDocumentRequest{
		BoardId:  boardState.ID().String(),
		Document: &cardamomv1.ApplyDocument{Version: 1},
	}))

	assert.Nil(t, response)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

type planningStub struct {
	invocation  issue.Invocation
	edit        planning.EditIssueRequest
	applyCalls  int
	apply       planning.ApplyDocumentRequest
	applyResult *planning.ApplyReceipt
}

func (*planningStub) CreateIssue(
	context.Context,
	issue.Invocation,
	planning.CreateIssueRequest,
) (planning.CreateIssueResult, error) {
	return planning.CreateIssueResult{}, nil
}

func (s *planningStub) EditIssue(
	_ context.Context,
	invocation issue.Invocation,
	request planning.EditIssueRequest,
) (planning.EditIssueResult, error) {
	s.invocation = invocation
	s.edit = request
	return planning.EditIssueResult{Issue: issue.Detail{Issue: planningTestIssue("an-1")}}, nil
}

func (s *planningStub) ApplyDocument(
	_ context.Context,
	invocation issue.Invocation,
	request planning.ApplyDocumentRequest,
) (planning.ApplyReceipt, error) {
	s.applyCalls++
	s.invocation = invocation
	s.apply = request
	if s.applyResult != nil {
		return *s.applyResult, nil
	}
	return planning.ApplyReceipt{
		Entries: []planning.ApplyReceiptEntry{{
			InputIndex: 0, ID: new("an-build"), Action: planning.ApplyActionCreate,
		}},
		Counts: planning.ApplyCounts{Create: 1},
	}, nil
}

type planningFactory map[board.ID]BoardPlanner

func (f planningFactory) Planner(boardID board.ID) (BoardPlanner, error) {
	planner, ok := f[boardID]
	if !ok {
		return nil, assert.AnError
	}
	return planner, nil
}

type planningCatalog struct{ board *board.State }

func (c planningCatalog) ListAllBoards(context.Context) ([]*board.State, error) {
	return []*board.State{c.board}, nil
}

func (c planningCatalog) Board(_ context.Context, id board.ID) (*board.State, error) {
	if c.board.ID() != id {
		return nil, errkind.Errorf(errkind.NotFound, "board not found")
	}
	return c.board, nil
}

func (c planningCatalog) Get(ctx context.Context, id board.ID) (*board.State, error) {
	return c.Board(ctx, id)
}

func (c planningCatalog) List(ctx context.Context) ([]*board.State, error) {
	return c.ListAllBoards(ctx)
}

type planningLocator map[string]board.ID

func (l planningLocator) BoardForIssue(_ context.Context, issueID string) (board.ID, error) {
	boardID, ok := l[issueID]
	if !ok {
		return "", errkind.Errorf(errkind.NotFound, "issue not found")
	}
	return boardID, nil
}

func planningTestBoard(t *testing.T) *board.State {
	t.Helper()
	state, err := board.Load(board.Snapshot{
		ID: "board-1", ProjectID: project.ID("project-1").String(),
		Name: "Board", Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}

func planningTestIssue(id string) issue.Issue {
	return issue.Issue{
		ID: id, Title: "Issue", Type: "task", Lifecycle: "open",
		Status: "ready", Priority: 2, Created: 1, Updated: 1,
	}
}
