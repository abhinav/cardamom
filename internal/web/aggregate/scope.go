package aggregate

import (
	"cmp"
	"errors"
	"slices"
	"strings"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// readTarget identifies the source and optional board for one aggregate read.
type readTarget struct {
	source  *source
	boardID string
}

func (s *Server) targets(scope *v1.BoardScope) ([]readTarget, error) {
	if scope == nil || scope.Selection == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid input: board scope is required"))
	}
	if source := scope.GetSource(); source != nil {
		value, err := s.sourceForRef(source)
		if err != nil {
			return nil, err
		}
		switch selection := scope.Selection.(type) {
		case *v1.BoardScope_BoardId:
			route, err := s.routeForBoard(selection.BoardId, value)
			if err != nil {
				return nil, err
			}
			return []readTarget{{source: route.source, boardID: route.boardID}}, nil
		case *v1.BoardScope_ProjectId:
			return s.projectTargets(value, selection.ProjectId), nil
		case *v1.BoardScope_AllBoards:
			return []readTarget{{source: value}}, nil
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid input: source scope requires a board, project, or all boards selection"))
		}
	}
	switch selection := scope.Selection.(type) {
	case *v1.BoardScope_BoardId:
		route, err := s.routeForBoard(selection.BoardId, nil)
		if err != nil {
			return nil, err
		}
		return []readTarget{{source: route.source, boardID: route.boardID}}, nil
	case *v1.BoardScope_AllBoards, *v1.BoardScope_AllSources:
		result := make([]readTarget, 0, len(s.sources))
		for index := range s.sources {
			result = append(result, readTarget{source: &s.sources[index]})
		}
		return result, nil
	case *v1.BoardScope_ProjectId:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid input: project scope requires a source"))
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid input: unknown board scope"))
	}
}

// projectTargets expands a project aggregate to active boards. Explicit board
// routes are resolved elsewhere and continue to admit archived boards for reads.
func (s *Server) projectTargets(value *source, projectID string) []readTarget {
	var result []readTarget
	for _, board := range s.boardList {
		if board.GetArchived() != nil {
			continue
		}
		if board.GetSource().GetSourceId() != value.config.Alias || board.GetProjectId() != projectID {
			continue
		}
		result = append(result, readTarget{source: value, boardID: board.GetId()})
	}
	return result
}

func aggregateStatus(problems map[string]string) *v1.AggregateStatus {
	ids := make([]string, 0, len(problems))
	for id := range problems {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	result := &v1.AggregateStatus{Complete: len(ids) == 0}
	for _, id := range ids {
		result.Problems = append(result.Problems, &v1.SourceProblem{SourceId: id, Summary: problems[id]})
	}
	return result
}

func (s *Server) sourceForRef(ref *v1.SourceRef) (*source, error) {
	if ref == nil || ref.GetSourceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("source reference is required"))
	}
	for index := range s.sources {
		value := &s.sources[index]
		if value.config.Alias == ref.GetSourceId() {
			if ref.GetStoreLineageId() != "" && ref.GetStoreLineageId() != value.entry.GetSource().GetStoreLineageId() {
				break
			}
			return value, nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, errors.New("source not found"))
}

// routeForBoard accepts an unqualified board ID only when the catalog contains
// one route for it. A supplied source always selects within that source.
func (s *Server) routeForBoard(boardID string, source *source) (boardRoute, error) {
	routes := s.boards[boardID]
	if source != nil {
		for _, route := range routes {
			if route.source == source {
				return route, nil
			}
		}
		return boardRoute{}, connect.NewError(connect.CodeNotFound, errors.New("board not found"))
	}
	switch len(routes) {
	case 0:
		return boardRoute{}, connect.NewError(connect.CodeNotFound, errors.New("board not found"))
	case 1:
		return routes[0], nil
	default:
		return boardRoute{}, connect.NewError(connect.CodeFailedPrecondition, errors.New("source-qualified board reference is required"))
	}
}

func sourceRefFromEntry(value *source) *v1.SourceRef {
	if value.entry != nil && value.entry.GetSource() != nil {
		return proto.Clone(value.entry.GetSource()).(*v1.SourceRef)
	}
	return &v1.SourceRef{SourceId: value.config.Alias}
}

func qualifySummary(target readTarget, value *v1.IssueSummary) *v1.IssueSummary {
	result := proto.Clone(value).(*v1.IssueSummary)
	result.Source = sourceRefFromEntry(target.source)
	if target.boardID != "" {
		result.BoardId = target.boardID
	}
	return result
}

func comparePriorityIssueSummary(left, right *v1.IssueSummary) int {
	return compareIssueSummaryFor(
		left,
		right,
		v1.IssueSort_ISSUE_SORT_PRIORITY,
		false,
	)
}

func compareTimestamp(left, right *timestamppb.Timestamp) int {
	if left == nil || right == nil {
		if left == nil && right == nil {
			return 0
		}
		if left == nil {
			return -1
		}
		return 1
	}
	if result := cmp.Compare(left.GetSeconds(), right.GetSeconds()); result != 0 {
		return result
	}
	return cmp.Compare(left.GetNanos(), right.GetNanos())
}

func compareIssueSummaryFor(left, right *v1.IssueSummary, sortValue v1.IssueSort, descending bool) int {
	result := 0
	switch sortValue {
	case v1.IssueSort_ISSUE_SORT_PRIORITY:
		result = cmp.Compare(left.GetPriority(), right.GetPriority())
	case v1.IssueSort_ISSUE_SORT_UPDATED_AT:
		result = compareTimestamp(left.GetUpdatedAt(), right.GetUpdatedAt())
	case v1.IssueSort_ISSUE_SORT_CREATED_AT:
		result = compareTimestamp(left.GetCreatedAt(), right.GetCreatedAt())
	case v1.IssueSort_ISSUE_SORT_TITLE:
		result = strings.Compare(left.GetTitle(), right.GetTitle())
	default:
		result = compareTimestamp(left.GetCreatedAt(), right.GetCreatedAt())
	}
	if descending {
		result = -result
	}
	if result != 0 {
		return result
	}
	if sortValue != v1.IssueSort_ISSUE_SORT_CREATED_AT &&
		sortValue != v1.IssueSort_ISSUE_SORT_UNSPECIFIED {
		result = compareTimestamp(left.GetCreatedAt(), right.GetCreatedAt())
		if descending {
			result = -result
		}
		if result != 0 {
			return result
		}
	}
	return compareIssueSummaryTieBreakers(left, right)
}

func compareIssueSummaryTieBreakers(left, right *v1.IssueSummary) int {
	if result := strings.Compare(left.GetSource().GetSourceId(), right.GetSource().GetSourceId()); result != 0 {
		return result
	}
	if result := strings.Compare(left.GetBoardId(), right.GetBoardId()); result != 0 {
		return result
	}
	return strings.Compare(left.GetId(), right.GetId())
}

func targetScope(target readTarget) *v1.BoardScope {
	if target.boardID == "" {
		return &v1.BoardScope{Selection: &v1.BoardScope_AllBoards{AllBoards: &v1.AllBoards{}}}
	}
	return &v1.BoardScope{Selection: &v1.BoardScope_BoardId{BoardId: target.boardID}}
}
