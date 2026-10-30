// Package conflictconnect translates reviewed conflict operations to Connect.
package conflictconnect

import (
	"context"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/errkind"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/web"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

// Boards opens conflict operations for the selected board.
type Boards interface {
	Conflicts(board.ID) (*boardconflict.Service, error)
}

// Service exposes bounded metadata reads, individual values, and reviewed repair.
// Scope and Boards share the namespace opened by process composition.
type Service struct {
	Scope  *boardscope.Resolver // required
	Boards Boards               // required
	Views  *issueview.Encoder   // required
}

var _ privatev1connect.ConflictServiceHandler = (*Service)(nil)

// ListConflicts returns one compact page without reading retained bodies.
func (s *Service) ListConflicts(ctx context.Context, req *connect.Request[v1.ListConflictsRequest]) (*connect.Response[v1.ListConflictsResponse], error) {
	id, service, err := s.board(ctx, req.Msg.GetScope())
	if err != nil {
		return nil, web.FromError(err)
	}
	page, err := service.List(ctx, boardconflict.ListRequest{IssueID: req.Msg.GetIssueId(), After: req.Msg.GetAfter(), Limit: int(req.Msg.GetLimit())})
	if err != nil {
		return nil, web.FromError(err)
	}
	out := &v1.ListConflictsResponse{Next: page.Next}
	for _, value := range page.Conflicts {
		summary, err := s.summary(id, value)
		if err != nil {
			return nil, web.FromError(err)
		}
		out.Conflicts = append(out.Conflicts, summary)
	}
	return connect.NewResponse(out), nil
}

// GetConflict reads review tokens and provenance without alternative bodies.
func (s *Service) GetConflict(ctx context.Context, req *connect.Request[v1.GetConflictRequest]) (*connect.Response[v1.GetConflictResponse], error) {
	id, service, err := s.board(ctx, req.Msg.GetScope())
	if err != nil {
		return nil, web.FromError(err)
	}
	value, err := service.Read(ctx, req.Msg.GetConflictId())
	if err != nil {
		return nil, web.FromError(err)
	}
	out, err := s.inspection(id, value)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&v1.GetConflictResponse{Conflict: out}), nil
}

// GetConflictAlternative loads one value selected from an inspected conflict.
func (s *Service) GetConflictAlternative(ctx context.Context, req *connect.Request[v1.GetConflictAlternativeRequest]) (*connect.Response[v1.GetConflictAlternativeResponse], error) {
	id, service, err := s.board(ctx, req.Msg.GetScope())
	if err != nil {
		return nil, web.FromError(err)
	}
	value, err := service.ReadAlternative(ctx, req.Msg.GetConflictId(), req.Msg.GetAlternativeId())
	if err != nil {
		return nil, web.FromError(err)
	}
	out, err := encodeValue(value.Payload)
	if err != nil {
		return nil, web.FromError(err)
	}
	out.Issues, err = s.references(id, value.Issues)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&v1.GetConflictAlternativeResponse{Value: out}), nil
}

// ResolveConflict preserves the submitted review identity through domain dispatch.
func (s *Service) ResolveConflict(ctx context.Context, req *connect.Request[v1.ResolveConflictRequest]) (*connect.Response[v1.ResolveConflictResponse], error) {
	_, service, err := s.board(ctx, req.Msg.GetScope())
	if err != nil {
		return nil, web.FromError(err)
	}
	var result boardconflict.Resolution
	switch choice := req.Msg.GetChoice().(type) {
	case *v1.ResolveConflictRequest_Take:
		result, err = service.Resolve(ctx, boardconflict.ResolveRequest{ConflictID: req.Msg.ConflictId, Choice: boardconflict.TakeAlternative{ID: choice.Take}})
	case *v1.ResolveConflictRequest_Text:
		result, err = service.Resolve(ctx, boardconflict.ResolveRequest{ConflictID: req.Msg.ConflictId, Choice: boardconflict.ReplaceText{Body: choice.Text}})
	case *v1.ResolveConflictRequest_Relations:
		var request boardconflict.RelationsRequest
		request, err = relationRequest(req.Msg.ConflictId, choice.Relations)
		if err == nil {
			result, err = service.RepairRelations(ctx, request)
		}
	case *v1.ResolveConflictRequest_Execution:
		var request boardconflict.ExecutionRequest
		request, err = executionRequest(req.Msg.ConflictId, choice.Execution)
		if err == nil {
			result, err = service.RepairExecution(ctx, request)
		}
	default:
		err = errkind.Errorf(errkind.InvalidInput, "choose an alternative, text, or constraint repair")
	}
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&v1.ResolveConflictResponse{ConflictId: result.ConflictID, Revision: result.Revision}), nil
}

func (s *Service) board(ctx context.Context, scope *v1.ConflictScope) (board.ID, *boardconflict.Service, error) {
	if scope == nil {
		return board.ID(""), nil, errkind.Errorf(errkind.InvalidInput, "board scope required")
	}
	states, err := s.Scope.Boards(ctx, &v1.BoardScope{Selection: &v1.BoardScope_BoardId{BoardId: scope.BoardId}})
	if err != nil {
		return board.ID(""), nil, err
	}
	id := states[0].ID()
	service, err := s.Boards.Conflicts(id)
	return id, service, err
}

func relationRequest(id string, value *v1.ConflictRelationsRepair) (boardconflict.RelationsRequest, error) {
	out := boardconflict.RelationsRequest{ConflictID: id}
	if value == nil {
		return out, errkind.Errorf(errkind.InvalidInput, "relationship repair required")
	}
	for _, assignment := range value.Remove {
		out.Remove = append(out.Remove, boardconflict.AssignmentID(assignment))
	}
	for _, parent := range value.Parents {
		child, err := issue.NewID(parent.GetChildId())
		if err != nil {
			return out, err
		}
		selection := boardconflict.ParentSelection{ChildID: child}
		if parent.ParentId != nil {
			id, err := issue.NewID(*parent.ParentId)
			if err != nil {
				return out, err
			}
			selection.ParentID = &id
		}
		out.Parents = append(out.Parents, selection)
	}
	return out, nil
}

func executionRequest(id string, value *v1.ConflictExecutionRepair) (boardconflict.ExecutionRequest, error) {
	out := boardconflict.ExecutionRequest{ConflictID: id}
	if value == nil {
		return out, errkind.Errorf(errkind.InvalidInput, "execution repair required")
	}
	switch value.Waiting {
	case "keep":
		out.Waiting = boardconflict.KeepWaiting
	case "clear":
		out.Waiting = boardconflict.ClearWaiting
	default:
		return out, errkind.Errorf(errkind.InvalidInput, "waiting must be keep or clear")
	}
	out.WaitingAssignment = boardconflict.AssignmentID(value.WaitingAssignment)
	if value.Kind != nil {
		parsed, err := issue.NewKind(*value.Kind)
		if err != nil {
			return out, err
		}
		out.Kind = &parsed
	}
	if value.Lifecycle != nil {
		parsed, err := issue.NewLifecycle(*value.Lifecycle)
		if err != nil {
			return out, err
		}
		out.Lifecycle = &parsed
	}
	return out, nil
}
