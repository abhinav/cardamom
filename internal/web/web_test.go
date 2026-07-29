package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
)

func TestFromErrorClassifiesErrors(t *testing.T) {
	t.Parallel()

	passthrough := connect.NewError(connect.CodeAborted, errors.New("already Connect"))
	tests := []struct {
		name        string
		give        error
		wantCode    connect.Code
		wantMessage string
	}{
		{name: "Passthrough", give: passthrough, wantCode: connect.CodeAborted, wantMessage: "already Connect"},
		{name: "Canceled", give: context.Canceled, wantCode: connect.CodeCanceled, wantMessage: "context canceled"},
		{name: "Deadline", give: context.DeadlineExceeded, wantCode: connect.CodeDeadlineExceeded, wantMessage: "context deadline exceeded"},
		{name: "InvalidInput", give: errkind.Errorf(errkind.InvalidInput, "invalid input: title required"), wantCode: connect.CodeInvalidArgument, wantMessage: "invalid input: title required"},
		{name: "NotFound", give: errkind.Errorf(errkind.NotFound, "issue not found: an-missing"), wantCode: connect.CodeNotFound, wantMessage: "issue not found: an-missing"},
		{name: "Conflict", give: errkind.Errorf(errkind.Conflict, "close requires lifecycle open or cancelled; issue lifecycle is closed"), wantCode: connect.CodeFailedPrecondition, wantMessage: "close requires lifecycle open or cancelled; issue lifecycle is closed"},
		{name: "Internal", give: errors.New("database unavailable"), wantCode: connect.CodeInternal, wantMessage: "internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := FromError(tt.give)
			assert.Equal(t, tt.wantCode, connect.CodeOf(err))
			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Equal(t, tt.wantMessage, connectErr.Message())
		})
	}
	assert.NoError(t, FromError(nil))
}

func TestNewHandlerMountsGeneratedServices(t *testing.T) {
	t.Parallel()

	path, handler := NewHandler(HandlerConfig{
		Project:       unimplementedProjectService{},
		Configuration: unimplementedConfigurationService{},
		Information:   unimplementedInformationService{},
		Attachment:    unimplementedAttachmentService{},
		Issue:         unimplementedIssueService{},
		Planning:      unimplementedPlanningService{},
		Execution:     unimplementedExecutionService{},
		Checkpoint:    unimplementedCheckpointService{},
		Record:        unimplementedRecordService{},
		Change:        unimplementedChangeService{},
		Dump:          unimplementedDumpService{},
		Mail:          unimplementedMailService{},
		Lease:         unimplementedLeaseService{},
	})
	assert.Equal(t, "/cardamom.private.v1.", path)

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
	_, projectErr := privatev1connect.NewProjectServiceClient(client, "http://cardamom.test").GetBootstrap(
		t.Context(),
		connect.NewRequest(&privatev1.GetBootstrapRequest{}),
	)
	_, configurationErr := privatev1connect.NewConfigurationServiceClient(
		client,
		"http://cardamom.test",
	).GetConfiguration(
		t.Context(),
		connect.NewRequest(&privatev1.GetConfigurationRequest{}),
	)
	_, informationErr := privatev1connect.NewInformationServiceClient(client, "http://cardamom.test").GetInformation(
		t.Context(),
		connect.NewRequest(&privatev1.GetInformationRequest{}),
	)
	_, issueErr := privatev1connect.NewIssueServiceClient(client, "http://cardamom.test").GetIssue(
		t.Context(),
		connect.NewRequest(&privatev1.GetIssueRequest{}),
	)
	_, planningErr := privatev1connect.NewPlanningServiceClient(client, "http://cardamom.test").EditIssue(
		t.Context(),
		connect.NewRequest(&privatev1.EditIssueRequest{}),
	)
	_, executionErr := privatev1connect.NewExecutionServiceClient(client, "http://cardamom.test").ClaimIssue(
		t.Context(),
		connect.NewRequest(&privatev1.ClaimIssueRequest{}),
	)
	_, checkpointErr := privatev1connect.NewCheckpointServiceClient(client, "http://cardamom.test").ListActionableCheckpoints(
		t.Context(),
		connect.NewRequest(&privatev1.ListActionableCheckpointsRequest{}),
	)
	_, recordErr := privatev1connect.NewRecordServiceClient(client, "http://cardamom.test").GetState(
		t.Context(),
		connect.NewRequest(&privatev1.GetStateRequest{}),
	)
	changeStream, changeErr := privatev1connect.NewChangeServiceClient(client, "http://cardamom.test").WatchChanges(
		t.Context(),
		connect.NewRequest(&privatev1.WatchChangesRequest{}),
	)
	require.NoError(t, changeErr)
	assert.False(t, changeStream.Receive())
	dumpStream, dumpErr := privatev1connect.NewDumpServiceClient(client, "http://cardamom.test").RenderDump(
		t.Context(),
		connect.NewRequest(&privatev1.RenderDumpRequest{}),
	)
	require.NoError(t, dumpErr)
	assert.False(t, dumpStream.Receive())
	_, mailErr := privatev1connect.NewMailServiceClient(client, "http://cardamom.test").ListSubscriptions(
		t.Context(),
		connect.NewRequest(&privatev1.ListSubscriptionsRequest{}),
	)
	_, leaseErr := privatev1connect.NewLeaseServiceClient(client, "http://cardamom.test").ListLeases(
		t.Context(),
		connect.NewRequest(&privatev1.ListLeasesRequest{}),
	)
	_, attachmentErr := privatev1connect.NewAttachmentServiceClient(client, "http://cardamom.test").GetAttachment(
		t.Context(),
		connect.NewRequest(&privatev1.GetAttachmentRequest{}),
	)

	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(projectErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(configurationErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(informationErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(attachmentErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(issueErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(planningErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(executionErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(checkpointErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(recordErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(changeStream.Err()))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(dumpStream.Err()))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(mailErr))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(leaseErr))
}

type unimplementedProjectService struct {
	privatev1connect.UnimplementedProjectServiceHandler
}

type unimplementedConfigurationService struct {
	privatev1connect.UnimplementedConfigurationServiceHandler
}

type unimplementedInformationService struct {
	privatev1connect.UnimplementedInformationServiceHandler
}

type unimplementedAttachmentService struct {
	privatev1connect.UnimplementedAttachmentServiceHandler
}

type unimplementedIssueService struct {
	privatev1connect.UnimplementedIssueServiceHandler
}

type unimplementedPlanningService struct {
	privatev1connect.UnimplementedPlanningServiceHandler
}

type unimplementedExecutionService struct {
	privatev1connect.UnimplementedExecutionServiceHandler
}

type unimplementedCheckpointService struct {
	privatev1connect.UnimplementedCheckpointServiceHandler
}

type unimplementedRecordService struct {
	privatev1connect.UnimplementedRecordServiceHandler
}

type unimplementedChangeService struct {
	privatev1connect.UnimplementedChangeServiceHandler
}

type unimplementedDumpService struct {
	privatev1connect.UnimplementedDumpServiceHandler
}

type unimplementedMailService struct {
	privatev1connect.UnimplementedMailServiceHandler
}

type unimplementedLeaseService struct {
	privatev1connect.UnimplementedLeaseServiceHandler
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

var (
	_ http.Handler                                 = (*http.ServeMux)(nil)
	_ privatev1connect.ProjectServiceHandler       = unimplementedProjectService{}
	_ privatev1connect.ConfigurationServiceHandler = unimplementedConfigurationService{}
	_ privatev1connect.InformationServiceHandler   = unimplementedInformationService{}
	_ privatev1connect.AttachmentServiceHandler    = unimplementedAttachmentService{}
	_ privatev1connect.IssueServiceHandler         = unimplementedIssueService{}
	_ privatev1connect.PlanningServiceHandler      = unimplementedPlanningService{}
	_ privatev1connect.ExecutionServiceHandler     = unimplementedExecutionService{}
	_ privatev1connect.CheckpointServiceHandler    = unimplementedCheckpointService{}
	_ privatev1connect.RecordServiceHandler        = unimplementedRecordService{}
	_ privatev1connect.ChangeServiceHandler        = unimplementedChangeService{}
	_ privatev1connect.DumpServiceHandler          = unimplementedDumpService{}
	_ privatev1connect.MailServiceHandler          = unimplementedMailService{}
	_ privatev1connect.LeaseServiceHandler         = unimplementedLeaseService{}
)
