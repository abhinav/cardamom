package cli

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
	"go.uber.org/mock/gomock"
)

func TestListCommandPassesEveryFilterAndEmitsJSONLines(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := []issue.Summary{
		{
			Issue: issue.Issue{
				ID: "an-1", Title: "First", Type: "task", Lifecycle: "open",
				Status: "ready", Priority: 1, Created: 10, Updated: 11, Revision: 4,
			},
			Labels: []string{"area:cli"},
		},
		{
			Issue: issue.Issue{
				ID: "an-2", Title: "Second", Type: "checkpoint", Lifecycle: "open",
				Status: "blocked", Priority: 2, Created: 12, Updated: 13, Revision: 4,
			},
			Blocked: true,
		},
	}
	request := issue.ListRequest{
		UnderID: "an-root", Statuses: []string{"ready", "closed", "waiting"},
		Assignee: new("worker"), Type: "task",
		LabelsAll: []string{"area:cli"}, LabelsAny: []string{"phase:a", "phase:b"},
		LabelsNone: []string{"archived"},
		NoAssignee: true, TitleRegexp: regexp.MustCompile("(?i)adapter"),
		Sort: "updated", Reverse: true, Limit: 9,
	}
	operation := NewMockListIssuesOperation(gomock.NewController(t))
	operation.EXPECT().ListIssues(gomock.Any(), request).Return(result, nil)
	app := newInspectionApplication(
		t,
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*ListIssuesOperation)(nil)),
	)

	exitCode := app.Run(t.Context(), []string{
		"--json", "list", "--under", "an-root",
		"--status", "ready,closed", "--status", "waiting",
		"--assignee", "worker", "--type", "task",
		"--label", "+area:cli", "--label", "-archived",
		"--label-any", "phase:a", "--label-any", "phase:b",
		"--no-assignee", "--title-regexp", "(?i)adapter",
		"--sort", "updated", "--reverse", "--limit", "9",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(
		t,
		"{\"id\":\"an-1\",\"title\":\"First\",\"type\":\"task\",\"lifecycle\":\"open\",\"status\":\"ready\",\"priority\":1,\"active_claim\":null,\"created\":10,\"updated\":11,\"revision\":4,\"labels\":[\"area:cli\"],\"blocked\":false}\n"+
			"{\"id\":\"an-2\",\"title\":\"Second\",\"type\":\"checkpoint\",\"lifecycle\":\"open\",\"status\":\"blocked\",\"priority\":2,\"active_claim\":null,\"created\":12,\"updated\":13,\"revision\":4,\"labels\":[],\"blocked\":true}\n",
		stdout.String(),
	)
	assert.Empty(t, stderr.String())
}

func TestListCommandDefaultsToNonTerminalStatuses(t *testing.T) {
	var stdout, stderr bytes.Buffer
	request := issue.ListRequest{
		Statuses: []string{"ready", "blocked", "in_progress", "waiting"},
	}
	operation := NewMockListIssuesOperation(gomock.NewController(t))
	operation.EXPECT().ListIssues(gomock.Any(), request).Return(nil, nil)
	app := newInspectionApplication(
		t,
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*ListIssuesOperation)(nil)),
	)

	exitCode := app.Run(t.Context(), []string{"list"})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Empty(t, stderr.String())
}

func TestListCommandRejectsInvalidTitleRegexp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := NewMockListIssuesOperation(gomock.NewController(t))
	app := newInspectionApplication(
		t,
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*ListIssuesOperation)(nil)),
	)

	exitCode := app.Run(t.Context(), []string{"list", "--title-regexp", "["})

	assert.Equal(t, ExitUsage, exitCode)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), `invalid --title-regexp "["`)
}

func TestReadyAndBlockedCommandsPassDomainLimits(t *testing.T) {
	var readyOut, readyErr bytes.Buffer
	readyOperation := NewMockListReadyIssuesOperation(gomock.NewController(t))
	readyOperation.EXPECT().ListReadyIssues(
		gomock.Any(),
		issue.ListReadyRequest{Limit: 7},
	).Return(nil, nil)
	app := newInspectionApplication(
		t,
		testConfig(&readyOut, &readyErr),
		kong.BindTo(readyOperation, (*ListReadyIssuesOperation)(nil)),
	)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"ready", "--limit", "7"}))
	assert.Equal(t, "ID  PRI  STATUS  TYPE  TITLE\n", readyOut.String())
	assert.Empty(t, readyErr.String())

	var blockedOut, blockedErr bytes.Buffer
	blockedOperation := NewMockListBlockedIssuesOperation(gomock.NewController(t))
	blockedOperation.EXPECT().ListBlockedIssues(
		gomock.Any(),
		issue.ListBlockedRequest{Limit: 8},
	).Return(nil, nil)
	app = newInspectionApplication(
		t,
		testConfig(&blockedOut, &blockedErr),
		kong.BindTo(blockedOperation, (*ListBlockedIssuesOperation)(nil)),
	)
	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"blocked", "--limit", "8"}))
	assert.Equal(t, "ID  PRI  STATUS  TYPE  TITLE\n", blockedOut.String())
	assert.Empty(t, blockedErr.String())
}

func TestShowCommandRequestsInheritedContextAndEmitsOneObject(t *testing.T) {
	var stdout, stderr bytes.Buffer
	boardDescription := "Shared board context"
	latestLogID := issue.LogID("cmt_77777777777777777777777777777777")
	summary := "Current summary"
	result := issue.View{
		Detail: issue.Detail{
			Issue: issue.Issue{
				ID: "an-current", Title: "Current", Type: "task", Lifecycle: "open",
				Status: "ready", Priority: 2, Created: 20, Updated: 21,
				Summary: &summary, Revision: 6,
			},
			Labels: []string{"area:cli"}, DependsOn: []issue.Reference{{ID: "an-dep"}},
			Keys:     []string{"source:a", "source:z"},
			ParentID: new("an-parent"), Blocked: true,
			LogSummary: issue.LogSummary{Count: 2, LatestID: &latestLogID},
		},
		Context: &issue.Context{
			Board: issue.BoardDescription{Description: &boardDescription},
			Ancestors: []issue.ContextEntry{{
				Issue: issue.Issue{
					ID: "an-parent", Title: "Parent", Type: "workstream",
					Lifecycle: "open", Status: "ready", Priority: 1,
					Created: 10, Updated: 11, Revision: 6,
				},
				LogSummary: issue.LogSummary{Count: 1},
			}},
			DependencyResults: []issue.DependencyResult{{
				Issue: issue.Reference{ID: "an-dep", Title: "Dependency"}, Body: "Completed",
			}},
		},
	}
	request := issue.ReadRequest{IssueID: "an-current", ContextDepth: new(2)}
	operation := NewMockReadIssueOperation(gomock.NewController(t))
	operation.EXPECT().ReadIssue(gomock.Any(), request).Return(attachment.IssueView{
		Issue: result, Attachments: []attachment.Attachment{},
	}, nil)
	app := newInspectionApplication(
		t,
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*ReadIssueOperation)(nil)),
	)

	exitCode := app.Run(t.Context(), []string{
		"--json", "show", "an-current", "--context", "--context-depth", "2",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.JSONEq(t, `{
		"board":{"description":"Shared board context"},
		"context":[{
			"id":"an-parent","title":"Parent","type":"workstream",
			"lifecycle":"open","status":"ready","priority":1,
			"active_claim":null,"created":10,"updated":11,"revision":6,
			"log_count":1
		}],
		"dependency_results":[{
			"issue_id":"an-dep","title":"Dependency","body":"Completed"
		}],
		"issue":{
			"id":"an-current","title":"Current","type":"task",
			"lifecycle":"open","status":"ready","priority":2,
			"active_claim":null,"created":20,"updated":21,
			"summary":"Current summary","revision":6,
			"keys":["source:a","source:z"],
			"labels":["area:cli"],"depends_on":["an-dep"],"blocks":[],"attachments":[],
			"log_count":2,"latest_log_id":"cmt_77777777777777777777777777777777","parent_id":"an-parent",
			"blocked":true
		}
	}`, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestShowCommandTreatsPositionalArgumentAsKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := issue.View{
		Detail: issue.Detail{
			Issue: issue.Issue{
				ID: "an-current", Title: "Current", Type: "task", Lifecycle: "open",
				Status: "ready", Priority: 2, Created: 20, Updated: 21, Revision: 6,
			},
			Keys: []string{"source:current"},
		},
	}
	request := issue.ReadRequest{Key: "source:current"}
	operation := NewMockReadIssueOperation(gomock.NewController(t))
	operation.EXPECT().ReadIssue(gomock.Any(), request).Return(attachment.IssueView{
		Issue: result, Attachments: []attachment.Attachment{},
	}, nil)
	app := newInspectionApplication(
		t,
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*ReadIssueOperation)(nil)),
	)

	exitCode := app.Run(t.Context(), []string{"show", "--key", "source:current"})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Empty(t, (&showCommand{ID: "source:current", Key: true}).referencedIssueIDs())
	assert.Contains(t, stdout.String(), "Keys: source:current\n")
	assert.Empty(t, stderr.String())
}

func newInspectionApplication(t *testing.T, config Config, options ...kong.Option) *Application {
	t.Helper()
	app, err := New(config, options...)
	require.NoError(t, err)
	return app
}
