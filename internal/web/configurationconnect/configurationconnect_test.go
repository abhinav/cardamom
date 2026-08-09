package configurationconnect

import (
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
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestServiceReadsOrderedConfigurationView(t *testing.T) {
	view := testView(t)
	operations := NewMockConfigurations(gomock.NewController(t))
	operations.EXPECT().Resolve(gomock.Any(), board.ID("board-1")).Return(view, nil)
	client := newTestClient(t, operations)

	response, err := client.GetConfiguration(
		t.Context(),
		connect.NewRequest(&privatev1.GetConfigurationRequest{BoardId: "board-1"}),
	)
	require.NoError(t, err)

	responseView := response.Msg.GetView()
	require.Len(t, responseView.GetLayers(), 4)
	assert.Equal(t, []privatev1.ConfigurationScope{
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_BUILT_IN,
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_STORE,
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_PROJECT,
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_BOARD,
	}, []privatev1.ConfigurationScope{
		responseView.GetLayers()[0].GetSource().GetScope(),
		responseView.GetLayers()[1].GetSource().GetScope(),
		responseView.GetLayers()[2].GetSource().GetScope(),
		responseView.GetLayers()[3].GetSource().GetScope(),
	})
	assert.Equal(t, "cm-", responseView.GetLayers()[0].GetOverrides().GetIssue().GetId().GetPrefix())
	assert.Equal(
		t,
		privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL,
		responseView.GetLayers()[1].GetOverrides().GetIssue().GetId().GetStrategy(),
	)
	assert.Equal(t, "project-1", responseView.GetLayers()[2].GetSource().GetIdentity())
	assert.Equal(t, "board-1", responseView.GetLayers()[3].GetSource().GetIdentity())
	assert.Equal(t, "project-", responseView.GetEffective().GetIssue().GetId().GetPrefix())
	assert.Equal(
		t,
		privatev1.ConfigurationScope_CONFIGURATION_SCOPE_PROJECT,
		responseView.GetOrigins().GetIssue().GetId().GetPrefix().GetScope(),
	)
}

func TestServiceUpdatesTypedConfigurationPatch(t *testing.T) {
	prefix := "next-"
	parsedPrefix, err := configuration.NewPrefix(prefix)
	require.NoError(t, err)
	operations := NewMockConfigurations(gomock.NewController(t))
	operations.EXPECT().Update(
		gomock.Any(),
		configuration.NewInvocation("engineer"),
		configuration.UpdateRequest{
			BoardID: board.ID("board-1"),
			Scope:   configuration.ScopeProject,
			Patch: configuration.Patch{
				Fields: []configuration.Field{
					configuration.FieldIssueIDPrefix,
					configuration.FieldAttachmentMaxBytes,
				},
				Overrides: configuration.Overrides{
					Issue: configuration.IssueOverrides{
						ID: configuration.IssueIDOverrides{Prefix: &parsedPrefix},
					},
				},
			},
		},
	).Return(testView(t), nil)
	client := newTestClient(t, operations)

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
}

func TestServiceRejectsInvalidConfigurationPatch(t *testing.T) {
	operations := NewMockConfigurations(gomock.NewController(t))
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
