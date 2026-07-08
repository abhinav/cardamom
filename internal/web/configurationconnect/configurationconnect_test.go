package configurationconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	privatev1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestServiceReadsOrderedConfigurationView(t *testing.T) {
	operations := &configurationOperations{view: testView(t)}
	client := newTestClient(t, operations)

	response, err := client.GetConfiguration(
		t.Context(),
		connect.NewRequest(&privatev1.GetConfigurationRequest{BoardId: "board-1"}),
	)
	require.NoError(t, err)

	assert.Equal(t, board.ID("board-1"), operations.resolvedBoardID)
	view := response.Msg.GetView()
	require.Len(t, view.GetLayers(), 4)
	assert.Equal(t, []privatev1.ConfigurationScope{
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_BUILT_IN,
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_STORE,
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_PROJECT,
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_BOARD,
	}, []privatev1.ConfigurationScope{
		view.GetLayers()[0].GetSource().GetScope(),
		view.GetLayers()[1].GetSource().GetScope(),
		view.GetLayers()[2].GetSource().GetScope(),
		view.GetLayers()[3].GetSource().GetScope(),
	})
	assert.Equal(t, "cm-", view.GetLayers()[0].GetOverrides().GetIssue().GetId().GetPrefix())
	assert.Equal(
		t,
		privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL,
		view.GetLayers()[1].GetOverrides().GetIssue().GetId().GetStrategy(),
	)
	assert.Equal(t, "project-1", view.GetLayers()[2].GetSource().GetIdentity())
	assert.Equal(t, "board-1", view.GetLayers()[3].GetSource().GetIdentity())
	assert.Equal(t, "project-", view.GetEffective().GetIssue().GetId().GetPrefix())
	assert.Equal(
		t,
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_PROJECT,
		view.GetOrigins().GetIssue().GetId().GetPrefix().GetScope(),
	)
}

func TestServiceUpdatesTypedConfigurationPatch(t *testing.T) {
	operations := &configurationOperations{view: testView(t)}
	client := newTestClient(t, operations)
	prefix := "next-"

	response, err := client.UpdateConfiguration(
		t.Context(),
		connect.NewRequest(&privatev1.UpdateConfigurationRequest{
			BoardId: "board-1",
			Scope:   privatev1.ConfigurationScope_CONFIGURATION_SCOPE_PROJECT,
			Overrides: &privatev1.ConfigurationOverrides{
				Issue: &privatev1.ConfigurationIssueOverrides{
					Id: &privatev1.ConfigurationIssueIDOverrides{Prefix: &prefix},
				},
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{
				"issue.id.prefix",
				"attachment.max_bytes",
			}},
			Context: &privatev1.MutationContext{Actor: new("engineer")},
		}),
	)
	require.NoError(t, err)

	assert.Equal(t, "project-", response.Msg.GetView().GetEffective().GetIssue().GetId().GetPrefix())
	assert.Equal(t, "engineer", operations.invocation.Actor())
	assert.Equal(t, board.ID("board-1"), operations.update.BoardID)
	assert.Equal(t, configuration.ScopeProject, operations.update.Scope)
	assert.Equal(t, []configuration.Field{
		configuration.FieldIssueIDPrefix,
		configuration.FieldAttachmentMaxBytes,
	}, operations.update.Patch.Fields)
	require.NotNil(t, operations.update.Patch.Overrides.Issue.ID.Prefix)
	assert.Equal(t, "next-", operations.update.Patch.Overrides.Issue.ID.Prefix.String())
	assert.Nil(t, operations.update.Patch.Overrides.Attachment.MaxBytes)
}

func TestServiceRejectsInvalidConfigurationPatch(t *testing.T) {
	operations := &configurationOperations{view: testView(t)}
	client := newTestClient(t, operations)

	_, err := client.UpdateConfiguration(
		t.Context(),
		connect.NewRequest(&privatev1.UpdateConfigurationRequest{
			BoardId: "board-1",
			Scope:   privatev1.ConfigurationScope_CONFIGURATION_SCOPE_BOARD,
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"issue.unknown"},
			},
		}),
	)

	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Zero(t, operations.updateCalls)
}

type configurationOperations struct {
	view            configuration.View
	resolvedBoardID board.ID
	invocation      configuration.Invocation
	update          configuration.UpdateRequest
	updateCalls     int
}

func (o *configurationOperations) Resolve(
	_ context.Context,
	boardID board.ID,
) (configuration.View, error) {
	o.resolvedBoardID = boardID
	return o.view, nil
}

func (o *configurationOperations) Update(
	_ context.Context,
	invocation configuration.Invocation,
	request configuration.UpdateRequest,
) (configuration.View, error) {
	o.invocation = invocation
	o.update = request
	o.updateCalls++
	return o.view, nil
}

func newTestClient(
	t *testing.T,
	operations Configurations,
) privatev1connect.ConfigurationServiceClient {
	t.Helper()
	_, handler := privatev1connect.NewConfigurationServiceHandler(New(operations))
	httpClient := &http.Client{Transport: configurationRoundTripper{handler: handler}}
	return privatev1connect.NewConfigurationServiceClient(httpClient, "http://cardamom.test")
}

type configurationRoundTripper struct{ handler http.Handler }

func (t configurationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

func testView(t *testing.T) configuration.View {
	t.Helper()
	builtIn := configuration.Defaults()
	sequential := configuration.IDStrategySequential
	projectPrefix, err := configuration.NewPrefix("project-")
	require.NoError(t, err)
	summaryLimit, err := configuration.NewByteLimit(4096)
	require.NoError(t, err)
	effective := builtIn
	effective.Issue.ID.Prefix = projectPrefix
	effective.Issue.ID.Strategy = sequential
	effective.Issue.Summary.MaxBytes = summaryLimit
	builtInSource := configuration.Source{
		Scope: configuration.ScopeBuiltIn, Identity: "built-in",
	}
	storeSource := configuration.Source{
		Scope: configuration.ScopeStore, Identity: "/tmp/store",
	}
	projectSource := configuration.Source{
		Scope: configuration.ScopeProject, Identity: "project-1",
	}
	boardSource := configuration.Source{
		Scope: configuration.ScopeBoard, Identity: "board-1",
	}
	return configuration.View{
		BuiltIn: builtIn,
		Store: configuration.Layer{
			Source: storeSource,
			Overrides: configuration.Overrides{
				Issue: configuration.IssueOverrides{
					ID: configuration.IssueIDOverrides{Strategy: &sequential},
				},
			},
		},
		Project: configuration.Layer{
			Source: projectSource,
			Overrides: configuration.Overrides{
				Issue: configuration.IssueOverrides{
					ID: configuration.IssueIDOverrides{Prefix: &projectPrefix},
				},
			},
		},
		Board: configuration.Layer{
			Source: boardSource,
			Overrides: configuration.Overrides{
				Issue: configuration.IssueOverrides{
					Summary: configuration.SummaryOverrides{MaxBytes: &summaryLimit},
				},
			},
		},
		Effective: effective,
		Origins: configuration.Origins{
			Issue: configuration.IssueOrigins{
				ID: configuration.IssueIDOrigins{
					Prefix: projectSource, Strategy: storeSource,
				},
				Summary: configuration.SummaryOrigins{MaxBytes: boardSource},
			},
			Attachment: configuration.AttachmentOrigins{MaxBytes: builtInSource},
		},
	}
}
