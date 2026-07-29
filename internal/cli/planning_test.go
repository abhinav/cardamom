package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestCreateCommandBuildsAtomicRequestAndPrintsID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &planningOperation{
		createResult: planning.CreateIssueResult{Issue: testIssueDetail("an-created", "Build adapter")},
	}
	app := newPlanningApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{
		"--actor", "planner",
		"create", " Build adapter ",
		"--type", "workstream", "--priority", "1",
		"--label", "area:cli,+phase:one", "--label", "delivery",
		"--depends-on", "an-dep", "--parent", "an-parent",
		"--summary", "Durable scope", "--key", "source:adapter",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(t, "planner", operation.createInvocation.Actor())
	assert.Equal(t, planning.CreateIssueRequest{
		Title: " Build adapter ", Type: "workstream", Priority: 1,
		Labels: []string{"area:cli", "phase:one", "delivery"}, DependsOn: []string{"an-dep"},
		Parent: "an-parent", Summary: "Durable scope", Key: new("source:adapter"),
	}, operation.createRequest)
	assert.Equal(t, "an-created\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestCreateCommandRejectsLongFlagAsLabelValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := new(planningOperation)
	app := newPlanningApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{
		"create", "Build adapter", "--label", "--priority",
	})

	assert.Equal(t, ExitUsage, exitCode)
	assert.Empty(t, operation.createRequest.Labels)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), `expected label value but got "--priority"`)
}

func TestCreateCommandRejectsNegativeLabelTerm(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := new(planningOperation)
	app := newPlanningApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{
		"create", "Build adapter", "--label", "-phase:build",
	})

	assert.Equal(t, ExitUsage, exitCode)
	assert.Empty(t, operation.createRequest.Labels)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "error: create --label does not accept removal terms\n", stderr.String())
}

func TestApplyCommandDecodesStrictProtoJSONAndRendersReceipt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &planningOperation{applyResult: planning.ApplyReceipt{
		Entries: []planning.ApplyReceiptEntry{{
			InputIndex: 0, Alias: new("gate"), ID: new("an-existing"),
			Key: new("source:gate"), Action: planning.ApplyActionUpdate,
		}},
		Counts: planning.ApplyCounts{Update: 1}, DryRun: true,
	}}
	config := testConfig(&stdout, &stderr)
	config.Stdin = strings.NewReader(`{
		"version":1,
		"on_existing":"update",
		"issues":[{
			"alias":"gate","id":"an-existing","key":"source:gate",
			"title":"Approve launch","type":"checkpoint","priority":0,
			"summary":"Inspect telemetry","labels":{"values":["phase:launch"]},
			"depends_on":{"values":[{"id":"an-dep"}]},
			"parent":{"alias":"workstream"}
		}]
	}`)
	config.StdinIsTerminal = false
	app := newPlanningApplication(t, config, operation)

	exitCode := app.Run(t.Context(), []string{
		"--json", "--actor", "planner", "apply", "--dry-run",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "planner", operation.applyInvocation.Actor())
	assert.Equal(t, planning.ApplyModeDryRun, operation.applyRequest.Mode)
	assert.Equal(t, planning.ApplyExistingUpdate, operation.applyRequest.OnExisting)
	require.Len(t, operation.applyRequest.Issues, 1)
	input := operation.applyRequest.Issues[0]
	assert.Equal(t, new("gate"), input.Alias)
	assert.Equal(t, new("an-existing"), input.ID)
	assert.Equal(t, new("source:gate"), input.Key)
	assert.Equal(t, new("Approve launch"), input.Title)
	assert.Equal(t, new("checkpoint"), input.Type)
	assert.Equal(t, new(0), input.Priority)
	assert.Equal(t, new("Inspect telemetry"), input.Summary)
	assert.Equal(t, new([]string{"phase:launch"}), input.Labels)
	require.NotNil(t, input.DependsOn)
	assert.Equal(t, []planning.ApplyIssueReference{{
		Kind: planning.ApplyReferenceID, ID: "an-dep",
	}}, *input.DependsOn)
	assert.Equal(t, planning.ParentReplace, input.Parent.Kind)
	assert.Equal(t, planning.ApplyIssueReference{
		Kind: planning.ApplyReferenceAlias, Alias: "workstream",
	}, input.Parent.Reference)
	assert.JSONEq(t, `{
		"entries":[{
			"alias":"gate","id":"an-existing","key":"source:gate",
			"action":"update"
		}],
		"counts":{"update":1},
		"dry_run":true
	}`, stdout.String())
}

func TestApplyCommandReadsDocumentFromFileAndRendersHumanReceipt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &planningOperation{applyResult: planning.ApplyReceipt{
		Entries: []planning.ApplyReceiptEntry{
			{InputIndex: 0, Alias: new("first"), ID: new("an-1"), Action: planning.ApplyActionCreate},
			{InputIndex: 1, ID: new("an-2"), Action: planning.ApplyActionCreate},
		},
		Counts: planning.ApplyCounts{Create: 2},
	}}
	path := t.TempDir() + "/document.json"
	require.NoError(t, os.WriteFile(path, []byte(`{
		"version":1,
		"issues":[
			{"alias":"first","title":"First","type":"task"},
			{"title":"Second","type":"task","depends_on":{"values":[{"alias":"first"}]}}
		]
	}`), 0o600))
	app := newPlanningApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{"apply", path})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(t, planning.ApplyExistingError, operation.applyRequest.OnExisting)
	assert.Equal(t, planning.ApplyModeCommit, operation.applyRequest.Mode)
	require.Len(t, operation.applyRequest.Issues, 2)
	assert.Equal(t, new("first"), operation.applyRequest.Issues[0].Alias)
	assert.Equal(t, "applied document: 2 create, 0 update, 0 skip, 0 no change\n0\tcreate\tan-1\n1\tcreate\tan-2\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestApplyCommandRejectsUnknownProtoJSONFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := new(planningOperation)
	config := testConfig(&stdout, &stderr)
	config.Stdin = strings.NewReader(`{
		"version":1,
		"issues":[{"title":"Gate","type":"checkpoint","group":"old"}]
	}`)
	config.StdinIsTerminal = false
	app := newPlanningApplication(t, config, operation)

	exitCode := app.Run(t.Context(), []string{"apply"})

	assert.Equal(t, ExitUsage, exitCode)
	assert.Empty(t, operation.applyRequest.Issues)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), `unknown field "group"`)
}

func TestApplyCommandRejectsUnsupportedApplyValues(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		wantError string
	}{
		{
			name: "ExistingPolicy",
			document: `{
				"version":1,
				"on_existing":"APPLY_EXISTING_POLICY_UPDATE",
				"issues":[]
			}`,
			wantError: "on_existing",
		},
		{
			name: "IssueType",
			document: `{
				"version":1,
				"issues":[{"title":"Build","type":"ISSUE_TYPE_TASK"}]
			}`,
			wantError: "issues[0].type",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			operation := new(planningOperation)
			config := testConfig(&stdout, &stderr)
			config.Stdin = strings.NewReader(test.document)
			config.StdinIsTerminal = false
			app := newPlanningApplication(t, config, operation)

			exitCode := app.Run(t.Context(), []string{"apply"})

			assert.Equal(t, ExitUsage, exitCode)
			assert.Zero(t, operation.applyCalls)
			assert.Empty(t, operation.applyRequest.Issues)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.wantError)
		})
	}
}

func TestApplyCommandRejectsInvalidReceiptAction(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &planningOperation{applyResult: planning.ApplyReceipt{
		Entries: []planning.ApplyReceiptEntry{{Action: planning.ApplyActionUnknown}},
	}}
	config := testConfig(&stdout, &stderr)
	config.Stdin = strings.NewReader(`{"version":1,"issues":[]}`)
	config.StdinIsTerminal = false
	app := newPlanningApplication(t, config, operation)

	exitCode := app.Run(t.Context(), []string{"--json", "apply"})

	assert.Equal(t, ExitOperation, exitCode)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "validate apply receipt")
}

func TestApplyCommandPreservesOmittedRelationshipFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &planningOperation{applyResult: planning.ApplyReceipt{
		Entries: []planning.ApplyReceiptEntry{{
			InputIndex: 0, ID: new("an-existing"), Action: planning.ApplyActionUpdate,
		}},
		Counts: planning.ApplyCounts{Update: 1},
	}}
	config := testConfig(&stdout, &stderr)
	config.Stdin = strings.NewReader(`{
		"version":1,
		"on_existing":"update",
		"issues":[{
			"id":"an-existing","title":"Replacement",
			"type":"task","priority":3,"summary":"Replacement summary"
		}]
	}`)
	config.StdinIsTerminal = false
	app := newPlanningApplication(t, config, operation)

	exitCode := app.Run(t.Context(), []string{"apply"})

	assert.Equal(t, ExitSuccess, exitCode)
	require.Len(t, operation.applyRequest.Issues, 1)
	input := operation.applyRequest.Issues[0]
	assert.Nil(t, input.Labels)
	assert.Nil(t, input.DependsOn)
	assert.Equal(t, planning.ParentUnchanged, input.Parent.Kind)
	assert.Empty(t, stderr.String())
}

func TestEditCommandPreservesRequestedEditDimensions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &planningOperation{
		editResult: planning.EditIssueResult{Issue: testIssueDetail("an-edit", "Changed")},
	}
	app := newPlanningApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{
		"edit", "an-edit", "--title", "Changed",
		"--type", "workstream", "--priority", "0",
		"--summary=", "--parent=",
		"--key", "source:edit",
		"--depends-on", "an-add", "--depends-on", "+an-other,-an-remove",
		"--label", "new", "--label", "-old",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(t, planning.EditIssueRequest{
		ID: "an-edit", Title: new("Changed"), Type: new("workstream"),
		Priority: new(0), Summary: new(""), SummarySet: true,
		ParentSet: true, Key: new("source:edit"),
		AddDependencies: []string{"an-add", "an-other"}, RemoveDependencies: []string{"an-remove"},
		AddLabels: []string{"new"}, RemoveLabels: []string{"old"},
	}, operation.editRequest)
	assert.Equal(t, "edited an-edit\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestEditCommandParsesSignedLabelTerms(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &planningOperation{
		editResult: planning.EditIssueResult{Issue: testIssueDetail("an-edit", "Changed")},
	}
	app := newPlanningApplication(t, testConfig(&stdout, &stderr), operation)

	exitCode := app.Run(t.Context(), []string{
		"edit", "an-edit",
		"--label", "+new,-old", "--label", "plain",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(t, []string{"new", "plain"}, operation.editRequest.AddLabels)
	assert.Equal(t, []string{"old"}, operation.editRequest.RemoveLabels)
	assert.Equal(t, "edited an-edit\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func newPlanningApplication(t *testing.T, config Config, operation *planningOperation) *Application {
	t.Helper()
	app, err := New(config,
		kong.BindTo(operation, (*CreateIssueOperation)(nil)),
		kong.BindTo(operation, (*ApplyDocumentOperation)(nil)),
		kong.BindTo(operation, (*EditIssueOperation)(nil)),
	)
	require.NoError(t, err)
	return app
}

type planningOperation struct {
	createInvocation issue.Invocation
	createRequest    planning.CreateIssueRequest
	createResult     planning.CreateIssueResult
	applyInvocation  issue.Invocation
	applyCalls       int
	applyRequest     planning.ApplyDocumentRequest
	applyResult      planning.ApplyReceipt
	editInvocation   issue.Invocation
	editRequest      planning.EditIssueRequest
	editResult       planning.EditIssueResult
}

func (o *planningOperation) CreateIssue(
	_ context.Context,
	inv issue.Invocation,
	request planning.CreateIssueRequest,
) (planning.CreateIssueResult, error) {
	o.createInvocation = inv
	o.createRequest = request
	return o.createResult, nil
}

func (o *planningOperation) ApplyDocument(
	_ context.Context,
	inv issue.Invocation,
	request planning.ApplyDocumentRequest,
) (planning.ApplyReceipt, error) {
	o.applyCalls++
	o.applyInvocation = inv
	o.applyRequest = request
	return o.applyResult, nil
}

func (o *planningOperation) EditIssue(
	_ context.Context,
	inv issue.Invocation,
	request planning.EditIssueRequest,
) (planning.EditIssueResult, error) {
	o.editInvocation = inv
	o.editRequest = request
	return o.editResult, nil
}

func testIssueDetail(id, title string) issue.Detail {
	return issue.Detail{Issue: issue.Issue{
		ID: id, Title: title, Type: "task", Lifecycle: "open", Status: "ready",
		Priority: 2, Created: 10, Updated: 10, Revision: 3,
	}}
}
