// Package planningconnect exposes issue planning operations through Connect.
package planningconnect

import (
	"context"
	"fmt"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	cardamomv1 "go.abhg.dev/cardamom/internal/gen/cardamom/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

// BoardPlanner supplies the domain operations exposed by PlanningService.
type BoardPlanner interface {
	// CreateIssue creates one issue and its initial relationships.
	CreateIssue(context.Context, issue.Invocation, planning.CreateIssueRequest) (planning.CreateIssueResult, error)

	// EditIssue atomically changes issue metadata and relationships.
	EditIssue(context.Context, issue.Invocation, planning.EditIssueRequest) (planning.EditIssueResult, error)

	// ApplyDocument validates or atomically applies one canonical document.
	ApplyDocument(context.Context, issue.Invocation, planning.ApplyDocumentRequest) (planning.ApplyReceipt, error)
}

// BoardPlannerFactory opens planning operations for one explicitly resolved board.
type BoardPlannerFactory interface {
	// Planner returns planning operations constrained to boardID.
	Planner(board.ID) (BoardPlanner, error)
}

// Config supplies the collaborators required by PlanningService.
type Config struct {
	// Scope resolves explicit board identities and issue ownership.
	Scope *boardscope.Resolver // required

	// Planners opens board-scoped planning operations.
	Planners BoardPlannerFactory // required

	// Views converts issue-domain records to generated messages.
	Views *issueview.Encoder // required
}

// Service translates PlanningService requests to shared domain operations.
type Service struct {
	privatev1connect.UnimplementedPlanningServiceHandler
	scope    *boardscope.Resolver
	planners BoardPlannerFactory
	views    *issueview.Encoder
}

var _ privatev1connect.PlanningServiceHandler = (*Service)(nil)

// New constructs a PlanningService handler.
func New(cfg Config) *Service {
	must.NotBeNilf(cfg.Scope, "planningconnect: board scope resolver is required")
	must.NotBeNilf(cfg.Planners, "planningconnect: planner factory is required")
	must.NotBeNilf(cfg.Views, "planningconnect: issue view encoder is required")
	return &Service{scope: cfg.Scope, planners: cfg.Planners, views: cfg.Views}
}

// CreateIssue creates one issue and its initial relationships.
func (s *Service) CreateIssue(
	ctx context.Context,
	request *connect.Request[privatev1.CreateIssueRequest],
) (*connect.Response[privatev1.CreateIssueResponse], error) {
	boardID, planner, err := s.plannerForBoard(ctx, request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	domainRequest, err := createIssueRequest(request.Msg)
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := planner.CreateIssue(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		domainRequest,
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.Summary(boardID, issue.Summary{
		Issue: result.Issue.Issue, Labels: result.Issue.Labels,
		Blocked: result.Issue.Blocked,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.CreateIssueResponse{Issue: converted}), nil
}

// EditIssue atomically changes issue metadata and relationships.
func (s *Service) EditIssue(
	ctx context.Context,
	request *connect.Request[privatev1.EditIssueRequest],
) (*connect.Response[privatev1.EditIssueResponse], error) {
	boardID, planner, err := s.plannerForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	domainRequest, err := editIssueRequest(request.Msg)
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := planner.EditIssue(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		domainRequest,
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.Detail(ctx, boardID, issue.View{Detail: result.Issue})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.EditIssueResponse{Issue: converted}), nil
}

// ApplyDocument validates or atomically applies one canonical document.
func (s *Service) ApplyDocument(
	ctx context.Context,
	request *connect.Request[privatev1.ApplyDocumentRequest],
) (*connect.Response[privatev1.ApplyDocumentResponse], error) {
	_, planner, err := s.plannerForBoard(ctx, request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	domainRequest, err := applyDocumentRequest(request.Msg.GetDocument(), request.Msg.GetDryRun())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := planner.ApplyDocument(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		domainRequest,
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	receipt := applyReceipt(result)
	if err := protovalidate.Validate(receipt); err != nil {
		return nil, web.FromError(fmt.Errorf("validate apply receipt: %w", err))
	}
	return connect.NewResponse(&privatev1.ApplyDocumentResponse{
		Receipt: receipt,
	}), nil
}

func (s *Service) plannerForBoard(
	ctx context.Context,
	value string,
) (board.ID, BoardPlanner, error) {
	boardID, err := board.NewID(value)
	if err != nil {
		return "", nil, err
	}
	boards, err := s.scope.Boards(ctx, &privatev1.BoardScope{
		Selection: &privatev1.BoardScope_BoardId{BoardId: boardID.String()},
	})
	if err != nil {
		return "", nil, err
	}
	if len(boards) != 1 || boards[0].ID() != boardID {
		return "", nil, fmt.Errorf("resolve board %q", boardID)
	}
	planner, err := s.planners.Planner(boardID)
	if err != nil {
		return "", nil, fmt.Errorf("open board planner %q: %w", boardID, err)
	}
	return boardID, planner, nil
}

func (s *Service) plannerForIssue(
	ctx context.Context,
	issueID string,
) (board.ID, BoardPlanner, error) {
	state, err := s.scope.BoardForIssue(ctx, issueID)
	if err != nil {
		return "", nil, err
	}
	planner, err := s.planners.Planner(state.ID())
	if err != nil {
		return "", nil, fmt.Errorf("open board planner %q: %w", state.ID(), err)
	}
	return state.ID(), planner, nil
}

func createIssueRequest(value *privatev1.CreateIssueRequest) (planning.CreateIssueRequest, error) {
	typeValue, err := issueType(value.GetType(), true)
	if err != nil {
		return planning.CreateIssueRequest{}, err
	}
	request := planning.CreateIssueRequest{
		Title: value.GetTitle(), Type: typeValue, Priority: int(value.GetPriority()),
		Labels: value.GetLabels(), DependsOn: value.GetPrerequisiteIds(),
		Parent: value.GetParentId(), Summary: value.GetSummarySource(),
		Details: value.GetDetailsSource(),
	}
	return request, nil
}

func editIssueRequest(value *privatev1.EditIssueRequest) (planning.EditIssueRequest, error) {
	request := planning.EditIssueRequest{
		ID: value.GetIssueId(), Title: value.Title,
		Summary:            value.SummarySource,
		SummarySet:         value.SummarySource != nil,
		Details:            value.DetailsSource,
		DetailsSet:         value.DetailsSource != nil,
		AddDependencies:    value.GetAddPrerequisiteIds(),
		RemoveDependencies: value.GetRemovePrerequisiteIds(),
		AddLabels:          value.GetAddLabels(), RemoveLabels: value.GetRemoveLabels(),
	}
	if value.Type != nil {
		converted, err := issueType(value.GetType(), false)
		if err != nil {
			return planning.EditIssueRequest{}, err
		}
		request.Type = &converted
	}
	if value.Priority != nil {
		converted := int(value.GetPriority())
		request.Priority = &converted
	}
	if value.ParentId != nil {
		request.ParentSet = true
		if value.GetParentId() != "" {
			request.Parent = new(value.GetParentId())
		}
	}
	if value.Labels != nil {
		labels := value.GetLabels().GetValues()
		request.Labels = &labels
	}
	return request, nil
}

func applyDocumentRequest(
	value *cardamomv1.ApplyDocument,
	dryRun bool,
) (planning.ApplyDocumentRequest, error) {
	if value == nil {
		return planning.ApplyDocumentRequest{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: apply document required",
		)
	}
	if err := protovalidate.Validate(value); err != nil {
		return planning.ApplyDocumentRequest{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: validate apply document: %v",
			err,
		)
	}
	existing, err := planning.NewApplyExistingPolicy(value.GetOnExisting())
	if err != nil {
		return planning.ApplyDocumentRequest{}, err
	}
	request := planning.ApplyDocumentRequest{
		Version: int(value.GetVersion()), Issues: make([]planning.ApplyIssue, 0, len(value.GetIssues())),
		OnExisting: existing, Mode: planning.ApplyModeCommit,
	}
	if dryRun {
		request.Mode = planning.ApplyModeDryRun
	}
	for index, entry := range value.GetIssues() {
		converted, err := applyIssue(entry)
		if err != nil {
			return planning.ApplyDocumentRequest{}, fmt.Errorf("issue %d: %w", index+1, err)
		}
		request.Issues = append(request.Issues, converted)
	}
	return request, nil
}

func applyIssue(value *cardamomv1.ApplyIssue) (planning.ApplyIssue, error) {
	if value == nil {
		return planning.ApplyIssue{}, errkind.Errorf(errkind.InvalidInput, "invalid input: issue required")
	}
	entry := planning.ApplyIssue{
		Alias: value.Alias, ID: value.Id, Key: value.Key,
		Title: value.Title, Summary: value.Summary, Details: value.Details,
	}
	if value.Type != nil {
		kind, err := issue.NewKind(value.GetType())
		if err != nil {
			return planning.ApplyIssue{}, err
		}
		converted := kind.String()
		entry.Type = &converted
	}
	if value.Priority != nil {
		priority := int(value.GetPriority())
		entry.Priority = &priority
	}
	if value.Labels != nil {
		labels := value.Labels.GetValues()
		entry.Labels = &labels
	}
	switch parent := value.GetParentChange().(type) {
	case nil:
	case *cardamomv1.ApplyIssue_Parent:
		converted, err := applyReference(parent.Parent)
		if err != nil {
			return planning.ApplyIssue{}, err
		}
		entry.Parent = planning.ApplyParentChange{
			Kind: planning.ParentReplace, Reference: converted,
		}
	case *cardamomv1.ApplyIssue_ClearParent:
		if parent.ClearParent == nil {
			return planning.ApplyIssue{}, errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: clear_parent value required",
			)
		}
		entry.Parent = planning.ApplyParentChange{Kind: planning.ParentClear}
	default:
		return planning.ApplyIssue{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: unknown parent change",
		)
	}
	if value.DependsOn != nil {
		dependencies := make([]planning.ApplyIssueReference, 0, len(value.DependsOn.GetValues()))
		for _, reference := range value.DependsOn.GetValues() {
			converted, err := applyReference(reference)
			if err != nil {
				return planning.ApplyIssue{}, err
			}
			dependencies = append(dependencies, converted)
		}
		entry.DependsOn = &dependencies
	}
	return entry, nil
}

func applyReference(value *cardamomv1.IssueReference) (planning.ApplyIssueReference, error) {
	if value == nil {
		return planning.ApplyIssueReference{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: issue reference required",
		)
	}
	switch target := value.GetTarget().(type) {
	case *cardamomv1.IssueReference_Alias:
		return planning.ApplyIssueReference{
			Kind: planning.ApplyReferenceAlias, Alias: target.Alias,
		}, nil
	case *cardamomv1.IssueReference_Id:
		return planning.ApplyIssueReference{
			Kind: planning.ApplyReferenceID, ID: target.Id,
		}, nil
	case *cardamomv1.IssueReference_Key:
		return planning.ApplyIssueReference{
			Kind: planning.ApplyReferenceKey, Key: target.Key,
		}, nil
	default:
		return planning.ApplyIssueReference{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: issue reference target required",
		)
	}
}

func applyReceipt(value planning.ApplyReceipt) *cardamomv1.ApplyReceipt {
	entries := make([]*cardamomv1.ApplyReceiptEntry, len(value.Entries))
	for index, entry := range value.Entries {
		entries[index] = &cardamomv1.ApplyReceiptEntry{
			InputIndex: uint32(entry.InputIndex), Alias: entry.Alias, Id: entry.ID,
			Key: entry.Key, Action: entry.Action.String(),
		}
	}
	return &cardamomv1.ApplyReceipt{
		Entries: entries,
		Counts: &cardamomv1.ApplyCounts{
			Create: uint32(value.Counts.Create), Update: uint32(value.Counts.Update),
			Skip: uint32(value.Counts.Skip), NoChange: uint32(value.Counts.NoChange),
		},
		Revision: value.Revision, DryRun: value.DryRun,
	}
}

func issueType(value privatev1.IssueType, allowDefault bool) (string, error) {
	switch value {
	case privatev1.IssueType_ISSUE_TYPE_UNSPECIFIED:
		if allowDefault {
			return "", nil
		}
	case privatev1.IssueType_ISSUE_TYPE_WORKSTREAM:
		return "workstream", nil
	case privatev1.IssueType_ISSUE_TYPE_TASK:
		return "task", nil
	case privatev1.IssueType_ISSUE_TYPE_CHECKPOINT:
		return "checkpoint", nil
	case privatev1.IssueType_ISSUE_TYPE_ROUTINE:
		return "routine", nil
	}
	return "", errkind.Errorf(errkind.InvalidInput, "invalid input: unknown issue type %d", value)
}

func mutationInvocation(value *privatev1.MutationContext) issue.Invocation {
	return issue.NewInvocation(value.GetActor())
}
