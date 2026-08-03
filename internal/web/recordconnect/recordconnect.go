// Package recordconnect exposes issue log, state, and results through Connect.
package recordconnect

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

// BoardRecords supplies the domain operations exposed by RecordService.
type BoardRecords interface {
	// ListLogEntries returns one issue's log entries in durable order.
	ListLogEntries(context.Context, issue.LogListRequest) ([]issue.LogEntry, error)

	// AddLogEntry appends one immutable attributed log entry.
	AddLogEntry(context.Context, issue.Invocation, record.AddLogEntryRequest) (record.AddLogEntryResult, error)

	// GetState returns one issue's current mutable recovery state.
	GetState(context.Context, record.GetStateRequest) (record.GetStateResult, error)

	// SetState replaces one issue's complete mutable recovery state.
	SetState(context.Context, issue.Invocation, record.SetStateRequest) (record.StateResult, error)

	// AppendState extends one issue's mutable recovery state.
	AppendState(context.Context, issue.Invocation, record.SetStateRequest) (record.StateResult, error)

	// ClearState removes one issue's mutable recovery state.
	ClearState(context.Context, issue.Invocation, record.ClearStateRequest) (record.StateResult, error)

	// CommitState preserves changed State and applies one atomic disposition.
	CommitState(context.Context, issue.Invocation, record.CommitStateRequest) (record.CommitStateResult, error)

	// GetResult returns one issue's current durable outcome.
	GetResult(context.Context, record.GetResultRequest) (issue.Result, error)

	// SetResult replaces one issue's durable outcome.
	SetResult(context.Context, issue.Invocation, record.SetResultRequest) (record.SetResultResult, error)
}

// BoardRecordFactory opens record operations for one resolved board.
type BoardRecordFactory interface {
	// Records returns record operations constrained to boardID.
	Records(board.ID) (BoardRecords, error)
}

// Config supplies the collaborators required by RecordService.
type Config struct {
	// Scope resolves issue ownership.
	Scope *boardscope.Resolver // required

	// Records opens board-scoped record operations.
	Records BoardRecordFactory // required

	// Views renders Markdown and converts issue-domain records.
	Views *issueview.Encoder // required
}

// Service translates RecordService requests to shared domain operations.
type Service struct {
	privatev1connect.UnimplementedRecordServiceHandler
	scope   *boardscope.Resolver
	records BoardRecordFactory
	views   *issueview.Encoder
}

var _ privatev1connect.RecordServiceHandler = (*Service)(nil)

// New constructs a RecordService handler.
func New(cfg Config) *Service {
	must.NotBeNilf(cfg.Scope, "recordconnect: board scope resolver is required")
	must.NotBeNilf(cfg.Records, "recordconnect: record factory is required")
	must.NotBeNilf(cfg.Views, "recordconnect: issue view encoder is required")
	return &Service{scope: cfg.Scope, records: cfg.Records, views: cfg.Views}
}

// ListLogEntries returns immutable log entries in requested order.
func (s *Service) ListLogEntries(
	ctx context.Context,
	request *connect.Request[privatev1.ListLogEntriesRequest],
) (*connect.Response[privatev1.ListLogEntriesResponse], error) {
	boardID, records, err := s.recordsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	entries, err := records.ListLogEntries(ctx, issue.LogListRequest{
		IssueID: request.Msg.GetIssueId(),
		Reverse: request.Msg.GetDirection() ==
			privatev1.SortDirection_SORT_DIRECTION_DESCENDING,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	views := s.views.WithRoutePrefix(request.Msg.GetPresentation().GetRoutePrefix())
	converted, err := views.LogEntries(ctx, boardID, entries)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ListLogEntriesResponse{LogEntries: converted}), nil
}

// AddLogEntry appends one immutable attributed log entry.
func (s *Service) AddLogEntry(
	ctx context.Context,
	request *connect.Request[privatev1.AddLogEntryRequest],
) (*connect.Response[privatev1.AddLogEntryResponse], error) {
	boardID, records, err := s.recordsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := records.AddLogEntry(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		record.AddLogEntryRequest{
			IssueID: request.Msg.GetIssueId(), Body: request.Msg.GetBodySource(),
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.LogEntry(ctx, boardID, result.LogEntry)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.AddLogEntryResponse{LogEntry: converted}), nil
}

// GetState returns one issue's current mutable recovery state.
func (s *Service) GetState(
	ctx context.Context,
	request *connect.Request[privatev1.GetStateRequest],
) (*connect.Response[privatev1.GetStateResponse], error) {
	boardID, records, err := s.recordsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := records.GetState(ctx, record.GetStateRequest{
		IssueID: request.Msg.GetIssueId(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	state, err := s.views.StateRecord(ctx, boardID, result.State)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.GetStateResponse{
		IssueId: result.IssueID, State: state,
	}), nil
}

// CommitState preserves changed State and applies one atomic disposition.
func (s *Service) CommitState(
	ctx context.Context,
	request *connect.Request[privatev1.CommitStateRequest],
) (*connect.Response[privatev1.CommitStateResponse], error) {
	boardID, records, err := s.recordsForIssue(
		ctx,
		request.Msg.GetIssueId(),
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	domainRequest := record.CommitStateRequest{
		IssueID: request.Msg.GetIssueId(),
	}
	switch disposition := request.Msg.GetDisposition().(type) {
	case *privatev1.CommitStateRequest_Retain:
		domainRequest.Disposition = record.CommitStateRetain
	case *privatev1.CommitStateRequest_Replace:
		domainRequest.Disposition = record.CommitStateReplace
		domainRequest.Replacement = record.StateReplacement{
			Body:       disposition.Replace.GetBodySource(),
			NextAction: disposition.Replace.GetNextActionSource(),
		}
	case *privatev1.CommitStateRequest_Clear:
		domainRequest.Disposition = record.CommitStateClear
	default:
		return nil, web.FromError(errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: State commit disposition required",
		))
	}
	result, err := records.CommitState(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		domainRequest,
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	convertedIssue, err := s.stateSummary(
		boardID,
		record.StateResult{Issue: result.Issue},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	convertedState, err := s.views.StateRecord(ctx, boardID, result.State)
	if err != nil {
		return nil, web.FromError(err)
	}
	var convertedLog *privatev1.LogEntry
	if result.LogEntry != nil {
		convertedLog, err = s.views.LogEntry(
			ctx,
			boardID,
			*result.LogEntry,
		)
		if err != nil {
			return nil, web.FromError(err)
		}
	}
	return connect.NewResponse(&privatev1.CommitStateResponse{
		Issue:    convertedIssue,
		State:    convertedState,
		LogEntry: convertedLog,
	}), nil
}

// SetState replaces one issue's complete mutable recovery state.
func (s *Service) SetState(
	ctx context.Context,
	request *connect.Request[privatev1.SetStateRequest],
) (*connect.Response[privatev1.SetStateResponse], error) {
	boardID, records, err := s.recordsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := records.SetState(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		record.SetStateRequest{
			IssueID:    request.Msg.GetIssueId(),
			Text:       request.Msg.GetStateSource(),
			NextAction: request.Msg.GetNextActionSource(),
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.stateSummary(boardID, result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.SetStateResponse{Issue: converted}), nil
}

// AppendState extends one issue's mutable recovery state.
func (s *Service) AppendState(
	ctx context.Context,
	request *connect.Request[privatev1.AppendStateRequest],
) (*connect.Response[privatev1.AppendStateResponse], error) {
	boardID, records, err := s.recordsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := records.AppendState(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		record.SetStateRequest{
			IssueID: request.Msg.GetIssueId(), Text: request.Msg.GetStateSource(),
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.stateSummary(boardID, result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.AppendStateResponse{Issue: converted}), nil
}

// ClearState removes one issue's mutable recovery state.
func (s *Service) ClearState(
	ctx context.Context,
	request *connect.Request[privatev1.ClearStateRequest],
) (*connect.Response[privatev1.ClearStateResponse], error) {
	boardID, records, err := s.recordsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := records.ClearState(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		record.ClearStateRequest{IssueID: request.Msg.GetIssueId()},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.stateSummary(boardID, result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ClearStateResponse{Issue: converted}), nil
}

// GetResult returns one issue's current durable outcome.
func (s *Service) GetResult(
	ctx context.Context,
	request *connect.Request[privatev1.GetResultRequest],
) (*connect.Response[privatev1.GetResultResponse], error) {
	boardID, records, err := s.recordsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := records.GetResult(ctx, record.GetResultRequest{
		IssueID: request.Msg.GetIssueId(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.issueResult(ctx, boardID, result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.GetResultResponse{Result: converted}), nil
}

// SetResult replaces one issue's durable outcome.
func (s *Service) SetResult(
	ctx context.Context,
	request *connect.Request[privatev1.SetResultRequest],
) (*connect.Response[privatev1.SetResultResponse], error) {
	boardID, records, err := s.recordsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	_, err = records.SetResult(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		record.SetResultRequest{
			IssueID: request.Msg.GetIssueId(), Body: request.Msg.GetBodySource(),
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := records.GetResult(ctx, record.GetResultRequest{
		IssueID: request.Msg.GetIssueId(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.issueResult(ctx, boardID, result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.SetResultResponse{Result: converted}), nil
}

func (s *Service) recordsForIssue(
	ctx context.Context,
	issueID string,
) (board.ID, BoardRecords, error) {
	state, err := s.scope.BoardForIssue(ctx, issueID)
	if err != nil {
		return "", nil, err
	}
	records, err := s.records.Records(state.ID())
	if err != nil {
		return "", nil, fmt.Errorf("open board records %q: %w", state.ID(), err)
	}
	return state.ID(), records, nil
}

func (s *Service) issueResult(
	ctx context.Context,
	boardID board.ID,
	value issue.Result,
) (*privatev1.IssueResult, error) {
	body, err := s.views.Markdown(ctx, boardID, value.Body)
	if err != nil {
		return nil, err
	}
	return &privatev1.IssueResult{
		IssueId: value.IssueID, IssueTitle: value.Title, Body: body,
	}, nil
}

func (s *Service) stateSummary(
	boardID board.ID,
	result record.StateResult,
) (*privatev1.IssueSummary, error) {
	return s.views.Summary(boardID, issue.Summary{Issue: result.Issue})
}

func mutationInvocation(value *privatev1.MutationContext) issue.Invocation {
	return issue.NewInvocation(value.GetActor())
}
