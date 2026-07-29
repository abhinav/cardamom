package cli

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestListCommandPassesEveryFilterAndEmitsJSONLines(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &inspectionOperation{listResult: []issue.Summary{
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
	}}
	app := newInspectionApplication(t, testConfig(&stdout, &stderr), operation)

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
	assert.Equal(t, issue.ListRequest{
		UnderID: "an-root", Statuses: []string{"ready", "closed", "waiting"},
		Assignee: new("worker"), Type: "task",
		LabelsAll: []string{"area:cli"}, LabelsAny: []string{"phase:a", "phase:b"},
		LabelsNone: []string{"archived"},
		NoAssignee: true, TitleRegexp: regexp.MustCompile("(?i)adapter"),
		Sort: "updated", Reverse: true, Limit: 9,
	}, operation.listRequest)
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
	operation := new(inspectionOperation)
	app := newInspectionApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{"list"})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(t, []string{
		"ready", "blocked", "in_progress", "waiting",
	}, operation.listRequest.Statuses)
	assert.Equal(t, 1, operation.listCalls)
	assert.Empty(t, stderr.String())
}

func TestListCommandRejectsInvalidTitleRegexp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := new(inspectionOperation)
	app := newInspectionApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{"list", "--title-regexp", "["})

	assert.Equal(t, ExitUsage, exitCode)
	assert.Zero(t, operation.listCalls)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), `invalid --title-regexp "["`)
}

func TestReadyAndBlockedCommandsPassDomainLimits(t *testing.T) {
	var readyOut, readyErr bytes.Buffer
	operation := new(inspectionOperation)
	app := newInspectionApplication(t, testConfig(&readyOut, &readyErr), operation)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"ready", "--limit", "7"}))
	assert.Equal(t, issue.ListReadyRequest{Limit: 7}, operation.readyRequest)
	assert.Equal(t, "ID  PRI  STATUS  TYPE  TITLE\n", readyOut.String())
	assert.Empty(t, readyErr.String())

	var blockedOut, blockedErr bytes.Buffer
	app = newInspectionApplication(t, testConfig(&blockedOut, &blockedErr), operation)
	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"blocked", "--limit", "8"}))
	assert.Equal(t, issue.ListBlockedRequest{Limit: 8}, operation.blockedRequest)
	assert.Equal(t, "ID  PRI  STATUS  TYPE  TITLE\n", blockedOut.String())
	assert.Empty(t, blockedErr.String())
}

func TestShowCommandRequestsInheritedContextAndEmitsOneObject(t *testing.T) {
	var stdout, stderr bytes.Buffer
	boardDescription := "Shared board context"
	latestLogID := issue.LogID("cmt_77777777777777777777777777777777")
	summary := "Current summary"
	operation := &inspectionOperation{readResult: issue.View{
		Detail: issue.Detail{
			Issue: issue.Issue{
				ID: "an-current", Title: "Current", Type: "task", Lifecycle: "open",
				Status: "ready", Priority: 2, Created: 20, Updated: 21,
				Summary: &summary, Revision: 6,
			},
			Labels: []string{"area:cli"}, DependsOn: []issue.Reference{{ID: "an-dep"}},
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
	}}
	app := newInspectionApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{
		"--json", "show", "an-current", "--context", "--context-depth", "2",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(t, issue.ReadRequest{
		IssueID: "an-current", ContextDepth: new(2),
	}, operation.readRequest)
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
			"labels":["area:cli"],"depends_on":["an-dep"],"blocks":[],"attachments":[],
			"log_count":2,"latest_log_id":"cmt_77777777777777777777777777777777","parent_id":"an-parent",
			"blocked":true
		}
	}`, stdout.String())
	assert.Empty(t, stderr.String())
}

func newInspectionApplication(t *testing.T, config Config, operation *inspectionOperation) *Application {
	t.Helper()
	app, err := New(
		config,
		kong.BindTo(operation, (*ListIssuesOperation)(nil)),
		kong.BindTo(operation, (*ListReadyIssuesOperation)(nil)),
		kong.BindTo(operation, (*ListBlockedIssuesOperation)(nil)),
		kong.BindTo(operation, (*ReadIssueOperation)(nil)),
	)
	require.NoError(t, err)
	return app
}

type inspectionOperation struct {
	listRequest    issue.ListRequest
	listResult     []issue.Summary
	listCalls      int
	readyRequest   issue.ListReadyRequest
	readyResult    []issue.Summary
	readyCalls     int
	blockedRequest issue.ListBlockedRequest
	blockedResult  []issue.Summary
	blockedCalls   int
	readRequest    issue.ReadRequest
	readResult     issue.View
}

func (o *inspectionOperation) ListIssues(
	_ context.Context,
	request issue.ListRequest,
) ([]issue.Summary, error) {
	o.listCalls++
	o.listRequest = request
	return o.listResult, nil
}

func (o *inspectionOperation) ListReadyIssues(
	_ context.Context,
	request issue.ListReadyRequest,
) ([]issue.Summary, error) {
	o.readyCalls++
	o.readyRequest = request
	return o.readyResult, nil
}

func (o *inspectionOperation) ListBlockedIssues(
	_ context.Context,
	request issue.ListBlockedRequest,
) ([]issue.Summary, error) {
	o.blockedCalls++
	o.blockedRequest = request
	return o.blockedResult, nil
}

func (o *inspectionOperation) ReadIssue(
	_ context.Context,
	request issue.ReadRequest,
) (attachment.IssueView, error) {
	o.readRequest = request
	return attachment.IssueView{
		Issue: o.readResult, Attachments: []attachment.Attachment{},
	}, nil
}
