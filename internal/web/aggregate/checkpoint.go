package aggregate

import (
	"context"
	"errors"
	"sort"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"google.golang.org/protobuf/proto"
)

func (s *Server) listApprovals(
	ctx context.Context,
	request *connect.Request[v1.ListActionableCheckpointsRequest],
) (*connect.Response[v1.ListActionableCheckpointsResponse], error) {
	if request == nil || request.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("checkpoint request is required"))
	}
	targets, err := s.targets(request.Msg.GetScope())
	if err != nil {
		return nil, err
	}
	problems := make(map[string]string)
	successes := 0
	var checkpoints []*v1.ActionableCheckpoint
	for _, target := range targets {
		response, err := target.source.checkpoints.ListActionableCheckpoints(ctx, connect.NewRequest(
			&v1.ListActionableCheckpointsRequest{
				Scope: targetScope(target), Presentation: aggregatePresentation(target.source),
			},
		))
		if err != nil || response == nil || response.Msg == nil {
			problems[target.source.config.Alias] = "source unavailable"
			continue
		}
		successes++
		for _, checkpoint := range response.Msg.GetCheckpoints() {
			if target.boardID != "" && checkpoint.GetCheckpoint().GetBoardId() != target.boardID {
				continue
			}
			checkpoints = append(checkpoints, qualifyCheckpoint(s, target, checkpoint))
		}
	}
	if len(targets) > 0 && successes == 0 {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("no aggregate sources are available"))
	}
	sort.SliceStable(checkpoints, func(left, right int) bool {
		return comparePriorityIssueSummary(
			checkpoints[left].GetCheckpoint(),
			checkpoints[right].GetCheckpoint(),
		) < 0
	})
	return connect.NewResponse(&v1.ListActionableCheckpointsResponse{
		Checkpoints: checkpoints, AggregateStatus: aggregateStatus(problems),
	}), nil
}

func qualifyCheckpoint(server *Server, target readTarget, value *v1.ActionableCheckpoint) *v1.ActionableCheckpoint {
	result := proto.Clone(value).(*v1.ActionableCheckpoint)
	if result.Checkpoint != nil {
		result.Checkpoint = qualifySummary(target, result.Checkpoint)
	}
	for _, ancestor := range result.GetContext().GetAncestors() {
		ancestor.Issue = qualifyRelatedForTarget(server, target, ancestor.GetIssue())
	}
	for _, dependency := range result.GetContext().GetDependencyResults() {
		dependency.Issue = qualifyRelatedForTarget(server, target, dependency.GetIssue())
	}
	for index, blocked := range result.GetBlockedIssues() {
		result.BlockedIssues[index] = qualifyRelatedForTarget(server, target, blocked)
	}
	return result
}

func qualifyRelatedForTarget(server *Server, target readTarget, value *v1.RelatedIssue) *v1.RelatedIssue {
	if value == nil {
		return nil
	}
	route := boardRoute{
		source: target.source, ref: target.ref, boardID: value.GetBoardId(),
	}
	if server != nil {
		if known, err := server.routeForBoard(value.GetBoardId(), target.ref); err == nil {
			route = known
		}
	}
	return qualifyRelated(route, value)
}

type checkpointService struct {
	privatev1connect.UnimplementedCheckpointServiceHandler
	server *Server
}

func (s *checkpointService) ListActionableCheckpoints(
	ctx context.Context,
	request *connect.Request[v1.ListActionableCheckpointsRequest],
) (*connect.Response[v1.ListActionableCheckpointsResponse], error) {
	return s.server.listApprovals(ctx, request)
}
