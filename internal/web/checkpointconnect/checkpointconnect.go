// Package checkpointconnect exposes human checkpoint decisions through Connect.
package checkpointconnect

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

//go:generate go tool mockgen -destination mocks_test.go -package checkpointconnect -typed -write_package_comment=false . BoardReader,BoardReaderFactory,BoardCommands,BoardCommandFactory

// BoardReader supplies the checkpoint reads exposed by CheckpointService.
type BoardReader interface {
	// ReadIssue returns one checkpoint detail and its requested inherited context.
	ReadIssue(context.Context, issue.ReadRequest) (issue.View, error)

	// ListActionableCheckpoints returns ready human decisions in domain order.
	ListActionableCheckpoints(context.Context) ([]issue.CheckpointView, error)
}

// BoardReaderFactory opens checkpoint reads for one explicitly resolved board.
type BoardReaderFactory interface {
	// Reader returns checkpoint reads constrained to boardID.
	Reader(board.ID) (BoardReader, error)
}

// BoardCommands supplies the checkpoint decisions exposed by CheckpointService.
type BoardCommands interface {
	// ApproveCheckpoint approves one checkpoint and returns the committed decision.
	ApproveCheckpoint(context.Context, issue.Invocation, execution.CheckpointRequest) (execution.ResolveCheckpointResult, error)

	// DenyCheckpoint denies one checkpoint and returns the committed cascade.
	DenyCheckpoint(context.Context, issue.Invocation, execution.CheckpointRequest) (execution.ResolveCheckpointResult, error)
}

// BoardCommandFactory opens checkpoint decisions for one explicitly resolved board.
type BoardCommandFactory interface {
	// Commands returns checkpoint decisions constrained to boardID.
	Commands(board.ID) (BoardCommands, error)
}

// Config supplies the collaborators required by CheckpointService.
type Config struct {
	// Scope resolves protocol scopes and store-global checkpoint ownership.
	Scope *boardscope.Resolver // required

	// Readers opens board-scoped checkpoint reads.
	Readers BoardReaderFactory // required

	// Commands opens board-scoped checkpoint decisions.
	Commands BoardCommandFactory // required

	// Views converts issue-domain records to generated protocol messages.
	Views *issueview.Encoder // required
}

// Service adapts human decisions to generated CheckpointService RPCs.
type Service struct {
	privatev1connect.UnimplementedCheckpointServiceHandler
	scope    *boardscope.Resolver
	readers  BoardReaderFactory
	commands BoardCommandFactory
	views    *issueview.Encoder
}

var _ privatev1connect.CheckpointServiceHandler = (*Service)(nil)

// New constructs a CheckpointService handler from board-scoped domain operations.
func New(cfg Config) *Service {
	must.NotBeNilf(cfg.Scope, "checkpointconnect: board scope resolver is required")
	must.NotBeNilf(cfg.Readers, "checkpointconnect: board reader factory is required")
	must.NotBeNilf(cfg.Commands, "checkpointconnect: board command factory is required")
	must.NotBeNilf(cfg.Views, "checkpointconnect: issue view encoder is required")
	return &Service{
		scope: cfg.Scope, readers: cfg.Readers,
		commands: cfg.Commands, views: cfg.Views,
	}
}

// scopedBoardReader pairs one resolved board with checkpoint reads constrained to it.
type scopedBoardReader struct {
	board  *board.State
	reader BoardReader
}

// scopedBoardCommands pairs one resolved board and reader with its decisions.
type scopedBoardCommands struct {
	scopedBoardReader
	commands BoardCommands
}

// ListActionableCheckpoints returns ready human decisions in one global domain
// order across the explicitly selected board scope.
func (s *Service) ListActionableCheckpoints(
	ctx context.Context,
	request *connect.Request[privatev1.ListActionableCheckpointsRequest],
) (*connect.Response[privatev1.ListActionableCheckpointsResponse], error) {
	readers, err := s.scopedReaders(ctx, request.Msg.GetScope())
	if err != nil {
		return nil, web.FromError(err)
	}
	type checkpointRecord struct {
		scoped scopedBoardReader
		value  issue.CheckpointView
	}
	records := make(map[string]checkpointRecord)
	ordered := make([]issue.BoardIssueSummary, 0)
	for _, scoped := range readers {
		values, err := scoped.reader.ListActionableCheckpoints(ctx)
		if err != nil {
			return nil, web.FromError(err)
		}
		for _, value := range values {
			key := scoped.board.ID().String() + "\x00" + value.ID
			records[key] = checkpointRecord{scoped: scoped, value: value}
			ordered = append(ordered, issue.BoardIssueSummary{
				BoardID: scoped.board.ID().String(),
				Summary: issue.Summary{
					Issue: value.Issue, Labels: value.Labels,
				},
			})
		}
	}
	ordered = issue.OrderSummaries(issue.ListRequest{Sort: "priority"}, ordered)
	response := &privatev1.ListActionableCheckpointsResponse{
		Checkpoints: make([]*privatev1.ActionableCheckpoint, 0, len(ordered)),
	}
	views := s.views.WithRoutePrefix(request.Msg.GetPresentation().GetRoutePrefix())
	for _, orderedValue := range ordered {
		key := orderedValue.BoardID + "\x00" + orderedValue.Summary.Issue.ID
		record := records[key]
		converted, err := s.actionableCheckpoint(
			ctx, record.scoped, record.value, views,
		)
		if err != nil {
			return nil, web.FromError(err)
		}
		response.Checkpoints = append(response.Checkpoints, converted)
	}
	return connect.NewResponse(response), nil
}

// ResolveCheckpoint applies the selected human decision and preserves the
// domain-provided denial cascade when building authoritative issue summaries.
func (s *Service) ResolveCheckpoint(
	ctx context.Context,
	request *connect.Request[privatev1.ResolveCheckpointRequest],
) (*connect.Response[privatev1.ResolveCheckpointResponse], error) {
	scoped, err := s.commandsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	domainRequest := execution.CheckpointRequest{
		IssueID: request.Msg.GetIssueId(), Reason: request.Msg.GetReason(),
	}
	invocation := mutationInvocation(request.Msg.GetContext())
	var result execution.ResolveCheckpointResult
	switch request.Msg.GetOutcome() {
	case privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_APPROVED:
		result, err = scoped.commands.ApproveCheckpoint(ctx, invocation, domainRequest)
	case privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_DENIED:
		result, err = scoped.commands.DenyCheckpoint(ctx, invocation, domainRequest)
	default:
		return nil, web.FromError(errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: unknown checkpoint decision %d",
			request.Msg.GetOutcome(),
		))
	}
	if err != nil {
		return nil, web.FromError(err)
	}
	expectedOutcome := "denied"
	if request.Msg.GetOutcome() == privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_APPROVED {
		expectedOutcome = "approved"
	}
	if result.Decision.Outcome != expectedOutcome {
		return nil, web.FromError(fmt.Errorf(
			"checkpoint result outcome %s does not match request %s",
			result.Decision.Outcome,
			expectedOutcome,
		))
	}
	checkpointView, err := scoped.reader.ReadIssue(ctx, issue.ReadRequest{
		IssueID: request.Msg.GetIssueId(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	convertedDetail, err := s.views.Detail(ctx, scoped.board.ID(), checkpointView)
	if err != nil {
		return nil, web.FromError(err)
	}
	cancelledIDs := make([]string, 0, len(result.Cancelled))
	for _, cancelled := range result.Cancelled {
		if cancelled.ID == request.Msg.GetIssueId() {
			continue
		}
		cancelledIDs = append(cancelledIDs, cancelled.ID)
	}
	cancelled, err := s.readIssueSummaries(ctx, scoped.scopedBoardReader, cancelledIDs)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ResolveCheckpointResponse{
		Decision:            convertedDetail.GetCheckpointDecision(),
		Checkpoint:          convertedDetail.GetIssue(),
		CancelledDependents: cancelled,
	}), nil
}

func (s *Service) actionableCheckpoint(
	ctx context.Context,
	scoped scopedBoardReader,
	checkpoint issue.CheckpointView,
	views *issueview.Encoder,
) (*privatev1.ActionableCheckpoint, error) {
	view, err := scoped.reader.ReadIssue(ctx, issue.ReadRequest{
		IssueID:      checkpoint.ID,
		ContextDepth: new(0),
	})
	if err != nil {
		return nil, err
	}
	summary, err := s.views.Summary(scoped.board.ID(), issue.Summary{
		Issue: checkpoint.Issue, Labels: checkpoint.Labels,
	})
	if err != nil {
		return nil, err
	}
	detail, err := views.Detail(ctx, scoped.board.ID(), view)
	if err != nil {
		return nil, err
	}
	blocked, err := s.views.References(scoped.board.ID(), checkpoint.Blocks)
	if err != nil {
		return nil, err
	}
	return &privatev1.ActionableCheckpoint{
		Checkpoint: summary, Summary: detail.GetSummary(),
		Context: detail.GetContext(), BlockedIssues: blocked,
	}, nil
}

func (s *Service) scopedReaders(
	ctx context.Context,
	scopeValue *privatev1.BoardScope,
) ([]scopedBoardReader, error) {
	boards, err := s.scope.Boards(ctx, scopeValue)
	if err != nil {
		return nil, err
	}
	readers := make([]scopedBoardReader, 0, len(boards))
	for _, board := range boards {
		reader, err := s.readers.Reader(board.ID())
		if err != nil {
			return nil, fmt.Errorf("open board %q: %w", board.ID(), err)
		}
		readers = append(readers, scopedBoardReader{board: board, reader: reader})
	}
	return readers, nil
}

func (s *Service) commandsForIssue(
	ctx context.Context,
	issueID string,
) (scopedBoardCommands, error) {
	board, err := s.scope.BoardForIssue(ctx, issueID)
	if err != nil {
		return scopedBoardCommands{}, err
	}
	reader, err := s.readers.Reader(board.ID())
	if err != nil {
		return scopedBoardCommands{}, fmt.Errorf("open board %q: %w", board.ID(), err)
	}
	commands, err := s.commands.Commands(board.ID())
	if err != nil {
		return scopedBoardCommands{}, fmt.Errorf("open board commands %q: %w", board.ID(), err)
	}
	return scopedBoardCommands{
		scopedBoardReader: scopedBoardReader{board: board, reader: reader},
		commands:          commands,
	}, nil
}

func (s *Service) readIssueSummary(
	ctx context.Context,
	scoped scopedBoardReader,
	issueID string,
) (issue.Summary, error) {
	view, err := scoped.reader.ReadIssue(ctx, issue.ReadRequest{IssueID: issueID})
	if err != nil {
		return issue.Summary{}, err
	}
	return issue.Summary{
		Issue: view.Detail.Issue, Labels: view.Detail.Labels,
		Blocked: view.Detail.Blocked,
	}, nil
}

func (s *Service) readIssueSummaries(
	ctx context.Context,
	scoped scopedBoardReader,
	issueIDs []string,
) ([]*privatev1.IssueSummary, error) {
	result := make([]*privatev1.IssueSummary, 0, len(issueIDs))
	for _, issueID := range issueIDs {
		summary, err := s.readIssueSummary(ctx, scoped, issueID)
		if err != nil {
			return nil, err
		}
		converted, err := s.views.Summary(scoped.board.ID(), summary)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func mutationInvocation(value *privatev1.MutationContext) issue.Invocation {
	return issue.NewInvocation(value.GetActor())
}
