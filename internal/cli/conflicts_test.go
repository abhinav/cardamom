package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.uber.org/mock/gomock"
)

func TestConflictsListCommands(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		issueID issue.ID
	}{
		{name: "Default", args: []string{"conflicts"}},
		{name: "List", args: []string{"conflicts", "list"}},
		{name: "Issue", args: []string{"conflicts", "cm-task"}, issueID: "cm-task"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			controller := gomock.NewController(t)
			repository := NewMockRepository(controller)
			configuration := NewMockConfigurationResolver(controller)
			request := boardconflict.ListRequest{Limit: 50, IssueID: test.issueID.String()}
			repository.EXPECT().ListConflicts(gomock.Any(), request).Return(boardconflict.Page{}, nil)
			app, err := New(testConfig(&stdout, &stderr), kong.Bind(&boardconflict.Service{Repository: repository, Configuration: configuration}))
			require.NoError(t, err)
			assert.Equal(t, ExitSuccess, app.Run(t.Context(), test.args))
			assert.Contains(t, stdout.String(), "No unresolved conflicts.")
			assert.Empty(t, stderr.String())
		})
	}
}

func TestResolveRelationsPreservesCompoundChoice(t *testing.T) {
	for _, test := range []struct{ name, parent, unparent string }{
		{name: "BareIDs", parent: "cm-child:cm-parent", unparent: "cm-other"},
		{name: "ReferenceTokens", parent: "%cm-child:%cm-parent", unparent: "%cm-other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			controller := gomock.NewController(t)
			repository := NewMockRepository(controller)
			configuration := NewMockConfigurationResolver(controller)
			parent := issue.ID("cm-parent")
			repository.EXPECT().RepairRelations(gomock.Any(), boardconflict.RelationsRequest{
				ConflictID: "reviewed-token", Remove: []boardconflict.AssignmentID{"assignment-a"},
				Parents: []boardconflict.ParentSelection{{ChildID: "cm-child", ParentID: &parent}, {ChildID: "cm-other"}},
			}).Return(boardconflict.Resolution{ConflictID: "reviewed-token", Revision: 12}, nil)
			app, err := New(testConfig(&stdout, &stderr), kong.Bind(&boardconflict.Service{Repository: repository, Configuration: configuration}))
			require.NoError(t, err)
			assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"resolve", "reviewed-token", "--remove-assignment", "assignment-a", "--parent", test.parent, "--unparent", test.unparent}))
			assert.Contains(t, stdout.String(), "Resolved reviewed-token at revision 12")
			assert.Empty(t, stderr.String())
		})
	}
}

func TestResolveExecutionHasNoImplicitReplacement(t *testing.T) {
	var stdout, stderr bytes.Buffer
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	configuration := NewMockConfigurationResolver(controller)
	repository.EXPECT().RepairExecution(gomock.Any(), boardconflict.ExecutionRequest{
		ConflictID: "reviewed-token", Waiting: boardconflict.ClearWaiting,
	}).Return(boardconflict.Resolution{ConflictID: "reviewed-token"}, nil)
	app, err := New(testConfig(&stdout, &stderr), kong.Bind(&boardconflict.Service{Repository: repository, Configuration: configuration}))
	require.NoError(t, err)
	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"resolve", "reviewed-token", "--waiting", "clear"}))
	assert.Empty(t, stderr.String())
}

func TestResolveRejectsMissingOrMixedChoices(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "Missing", args: []string{"resolve", "token"}},
		{name: "IncompleteExecutionChoice", args: []string{"resolve", "token", "--kind", "task"}},
		{name: "Mixed", args: []string{"resolve", "token", "--take", "alt", "--waiting", "clear"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app, err := New(testConfig(&stdout, &stderr))
			require.NoError(t, err)
			assert.Equal(t, ExitUsage, app.Run(t.Context(), test.args))
			assert.Empty(t, stdout.String())
		})
	}
}

func TestResolveStaleTokenPreservesDiagnosticAndCleanJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	configuration := NewMockConfigurationResolver(controller)
	repository.EXPECT().ResolveConflict(gomock.Any(), boardconflict.ResolveRequest{ConflictID: "reviewed-token", Choice: boardconflict.TakeAlternative{ID: "alt"}}).
		Return(boardconflict.Resolution{}, errkind.Errorf(errkind.Conflict, "conflict changed; inspect the current alternatives"))
	app, err := New(testConfig(&stdout, &stderr), kong.Bind(&boardconflict.Service{Repository: repository, Configuration: configuration}))
	require.NoError(t, err)
	assert.Equal(t, ExitOperation, app.Run(t.Context(), []string{"--json", "resolve", "reviewed-token", "--take", "alt"}))
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "inspect the current alternatives")
}

func TestConflictsShowRetrievesOnlySelectedBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	configuration := NewMockConfigurationResolver(controller)
	uid, err := issue.ParseUID([]byte("conflict-subject"))
	require.NoError(t, err)
	repository.EXPECT().ReadAlternative(gomock.Any(), "token", "alt").Return(boardconflict.Value{Payload: &boardhistory.IssueSummarySet{IssueUID: uid, Summary: new("Selected body")}}, nil)
	app, err := New(testConfig(&stdout, &stderr), kong.Bind(&boardconflict.Service{Repository: repository, Configuration: configuration}))
	require.NoError(t, err)
	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"conflicts", "show", "token", "--alternative", "alt"}))
	assert.Equal(t, "Selected body\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestResolveReadsReplacementFromStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	configuration := NewMockConfigurationResolver(controller)
	conflict := boardconflict.Load(boardconflict.Snapshot{ID: "token", BoardID: "board-test", Unit: boardconflict.Unit{Kind: boardhistory.EventBoardDescriptionSet}, Alternatives: nil})
	repository.EXPECT().ReadConflict(gomock.Any(), "token").Return(boardconflict.Inspection{Conflict: conflict}, nil)
	repository.EXPECT().ResolveConflict(gomock.Any(), boardconflict.ResolveRequest{ConflictID: "token", Choice: boardconflict.ReplaceText{Body: "Composed text\n"}}).Return(boardconflict.Resolution{ConflictID: "token"}, nil)
	config := testConfig(&stdout, &stderr)
	config.Stdin = strings.NewReader("Composed text\n")
	config.StdinIsTerminal = false
	app, err := New(config, kong.Bind(&boardconflict.Service{Repository: repository, Configuration: configuration}))
	require.NoError(t, err)
	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"resolve", "token", "--file", "-"}))
	assert.Empty(t, stderr.String())
}

func TestResolveRevalidatesReviewedRelationships(t *testing.T) {
	var stdout, stderr bytes.Buffer
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	configuration := NewMockConfigurationResolver(controller)
	repository.EXPECT().RepairRelations(gomock.Any(), boardconflict.RelationsRequest{ConflictID: "reviewed"}).Return(boardconflict.Resolution{ConflictID: "reviewed"}, nil)
	app, err := New(testConfig(&stdout, &stderr), kong.Bind(&boardconflict.Service{Repository: repository, Configuration: configuration}))
	require.NoError(t, err)
	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"resolve", "reviewed", "--relationships"}))
	assert.Empty(t, stderr.String())
}

func TestConflictsJSONPreservesIdentityAndTypedAbsence(t *testing.T) {
	uid, err := issue.ParseUID([]byte("conflict-subject"))
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	configuration := NewMockConfigurationResolver(controller)
	repository.EXPECT().ReadAlternative(gomock.Any(), "token", "alt").Return(boardconflict.Value{
		Payload: &boardhistory.IssueSummarySet{IssueUID: uid},
		Issues:  []boardconflict.IssueReference{{UID: uid, Reference: issue.Reference{ID: "cm-child", Status: "unresolved"}}},
	}, nil)
	app, err := New(testConfig(&stdout, &stderr), kong.Bind(&boardconflict.Service{Repository: repository, Configuration: configuration}))
	require.NoError(t, err)
	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"--json", "conflicts", "show", "token", "--alternative", "alt"}))
	assert.JSONEq(t, `{"unit":{"kind":"issue.summary.set","issue_uid":"636f6e666c6963742d7375626a656374"},"value":null,"issues":[{"uid":"636f6e666c6963742d7375626a656374","issue":{"id":"cm-child","title":null,"type":null,"status":"unresolved","priority":null}}]}`, stdout.String())
	assert.Empty(t, stderr.String())
}
