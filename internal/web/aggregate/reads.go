package aggregate

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"slices"
	"sort"
	"strings"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	aggregateIssuePageSize   = 250
	defaultAggregatePageSize = 100
	maximumAggregateCursors  = 128
)

type readTarget struct {
	source  *source
	boardID string
}

// issueCursor retains one bounded upstream page per source between browser pages.
type issueCursor struct {
	fingerprint string
	request     *v1.ListIssuesRequest
	sort        v1.IssueSort
	descending  bool
	sources     []*issueCursorSource
	total       uint32
	facets      map[string]uint32
	problems    map[string]string
}

// issueCursorSource owns one source-local continuation and its queued records.
type issueCursorSource struct {
	target    readTarget
	values    []*v1.IssueSummary
	nextPage  string
	exhausted bool
}

func (s *Server) listIssues(
	ctx context.Context,
	request *connect.Request[v1.ListIssuesRequest],
) (*connect.Response[v1.ListIssuesResponse], error) {
	if request == nil || request.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("issue request is required"))
	}
	fingerprint, err := issueRequestFingerprint(request.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal server error"))
	}
	var cursor *issueCursor
	if request.Msg.GetPageToken() != "" {
		cursor = s.issueCursor(request.Msg.GetPageToken())
		if cursor == nil || cursor.fingerprint != fingerprint {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid input: invalid aggregate page token"))
		}
	} else {
		cursor, err = s.newIssueCursor(ctx, request.Msg, fingerprint)
		if err != nil {
			return nil, err
		}
	}

	limit := aggregatePageSize(request.Msg.GetLimit())
	values, err := cursor.next(ctx, limit)
	if err != nil {
		return nil, err
	}
	response := &v1.ListIssuesResponse{
		Issues:          values,
		TotalCount:      cursor.total,
		LabelFacets:     issueFacets(cursor.facets),
		AggregateStatus: aggregateStatus(cursor.problems),
	}
	if cursor.hasMore() {
		token, err := s.storeIssueCursor(cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal server error"))
		}
		response.Truncated = true
		response.NextPageToken = &token
	}
	return connect.NewResponse(response), nil
}

func (s *Server) newIssueCursor(
	ctx context.Context,
	request *v1.ListIssuesRequest,
	fingerprint string,
) (*issueCursor, error) {
	targets, err := s.targets(request.GetScope())
	if err != nil {
		return nil, err
	}
	cursor := &issueCursor{
		fingerprint: fingerprint,
		request:     proto.Clone(request).(*v1.ListIssuesRequest),
		sort:        request.GetSort(),
		descending:  request.GetDirection() == v1.SortDirection_SORT_DIRECTION_DESCENDING,
		facets:      make(map[string]uint32),
		problems:    make(map[string]string),
	}
	cursor.request.PageToken = nil
	for _, target := range targets {
		response, err := target.source.issues.ListIssues(ctx, connect.NewRequest(
			sourceIssueRequest(cursor.request, target),
		))
		if err != nil {
			cursor.addProblem(target.source.config.Alias, "source unavailable")
			continue
		}
		if response == nil || response.Msg == nil {
			cursor.addProblem(target.source.config.Alias, "source returned no issue collection")
			continue
		}
		value := &issueCursorSource{target: target}
		value.setPage(response.Msg, s)
		cursor.sources = append(cursor.sources, value)
		cursor.total += response.Msg.GetTotalCount()
		for _, facet := range response.Msg.GetLabelFacets() {
			cursor.facets[facet.GetLabel()] += facet.GetIssueCount()
		}
	}
	if len(targets) > 0 && len(cursor.sources) == 0 {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("no aggregate sources are available"))
	}
	return cursor, nil
}

func sourceIssueRequest(request *v1.ListIssuesRequest, target readTarget) *v1.ListIssuesRequest {
	result := proto.Clone(request).(*v1.ListIssuesRequest)
	result.Limit = uint32(aggregateIssuePageSize)
	result.PageToken = nil
	if target.boardID == "" {
		result.Scope = &v1.BoardScope{Selection: &v1.BoardScope_AllBoards{AllBoards: &v1.AllBoards{}}}
	} else {
		result.Scope = &v1.BoardScope{Selection: &v1.BoardScope_BoardId{BoardId: target.boardID}}
	}
	return result
}

func (s *issueCursorSource) setPage(response *v1.ListIssuesResponse, server *Server) {
	s.values = s.values[:0]
	for _, value := range response.GetIssues() {
		if s.target.boardID != "" && value.GetBoardId() != s.target.boardID {
			continue
		}
		s.values = append(s.values, qualifySummary(server, s.target, value))
	}
	s.nextPage = response.GetNextPageToken()
	s.exhausted = !response.GetTruncated() && s.nextPage == ""
}

func (s *issueCursor) next(ctx context.Context, limit int) ([]*v1.IssueSummary, error) {
	result := make([]*v1.IssueSummary, 0, limit)
	for len(result) < limit {
		best := -1
		for index, value := range s.sources {
			if len(value.values) == 0 && !value.exhausted {
				if err := s.loadNext(ctx, value); err != nil {
					return nil, err
				}
			}
			if len(value.values) == 0 {
				continue
			}
			if best < 0 || compareIssueSummaryFor(value.values[0], s.sources[best].values[0], s.sort, s.descending) < 0 {
				best = index
			}
		}
		if best < 0 {
			break
		}
		result = append(result, s.sources[best].values[0])
		s.sources[best].values = s.sources[best].values[1:]
	}
	return result, nil
}

func (s *issueCursor) loadNext(ctx context.Context, value *issueCursorSource) error {
	if value.nextPage == "" {
		value.exhausted = true
		return nil
	}
	request := sourceIssueRequest(s.request, value.target)
	request.PageToken = new(value.nextPage)
	response, err := value.target.source.issues.ListIssues(ctx, connect.NewRequest(request))
	if err != nil {
		return connect.NewError(connect.CodeUnavailable, errors.New("source page unavailable"))
	}
	if response == nil || response.Msg == nil {
		return connect.NewError(connect.CodeInternal, errors.New("source returned no issue page"))
	}
	value.setPage(response.Msg, nil)
	for index, issue := range value.values {
		value.values[index] = qualifySummary(nil, value.target, issue)
	}
	value.exhausted = !response.Msg.GetTruncated() && value.nextPage == ""
	return nil
}

func (s *issueCursor) hasMore() bool {
	for _, value := range s.sources {
		if len(value.values) > 0 || !value.exhausted {
			return true
		}
	}
	return false
}

func (s *issueCursor) addProblem(sourceID, summary string) {
	if _, exists := s.problems[sourceID]; !exists {
		s.problems[sourceID] = summary
	}
}

func issueRequestFingerprint(request *v1.ListIssuesRequest) (string, error) {
	value := proto.Clone(request).(*v1.ListIssuesRequest)
	value.Limit = 0
	value.PageToken = nil
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func aggregatePageSize(value uint32) int {
	if value == 0 {
		return defaultAggregatePageSize
	}
	return min(int(value), aggregateIssuePageSize)
}

func (s *Server) issueCursor(token string) *issueCursor {
	s.cursorsMu.Lock()
	defer s.cursorsMu.Unlock()
	return s.cursors[token]
}

func (s *Server) storeIssueCursor(cursor *issueCursor) (string, error) {
	data := make([]byte, 18)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(data)
	s.cursorsMu.Lock()
	defer s.cursorsMu.Unlock()
	if len(s.cursors) >= maximumAggregateCursors {
		for key := range s.cursors {
			delete(s.cursors, key)
			break
		}
	}
	s.cursors[token] = cursor
	return token, nil
}

func issueFacets(values map[string]uint32) []*v1.LabelFacet {
	labels := make([]string, 0, len(values))
	for label := range values {
		labels = append(labels, label)
	}
	slices.Sort(labels)
	result := make([]*v1.LabelFacet, 0, len(labels))
	for _, label := range labels {
		result = append(result, &v1.LabelFacet{Label: label, IssueCount: values[label]})
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

func (s *Server) targets(scope *v1.BoardScope) ([]readTarget, error) {
	if scope == nil || scope.Selection == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid input: board scope is required"))
	}
	switch selection := scope.Selection.(type) {
	case *v1.BoardScope_BoardId:
		route, ok := s.boards[selection.BoardId]
		if !ok {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("board not found"))
		}
		return []readTarget{{source: &route.source, boardID: selection.BoardId}}, nil
	case *v1.BoardScope_Board:
		route, err := s.routeForBoardRef(selection.Board)
		if err != nil {
			return nil, err
		}
		return []readTarget{{source: &route.source, boardID: route.ref.BoardId}}, nil
	case *v1.BoardScope_Source:
		source, err := s.sourceForRef(selection.Source)
		if err != nil {
			return nil, err
		}
		return []readTarget{{source: source}}, nil
	case *v1.BoardScope_Project:
		source, err := s.sourceForRef(selection.Project.GetSource())
		if err != nil {
			return nil, err
		}
		var result []readTarget
		for _, board := range s.boardList {
			ref := board.GetRef()
			if ref.GetSource().GetSourceId() == source.config.Alias &&
				board.GetProjectId() == selection.Project.GetProjectId() {
				result = append(result, readTarget{source: source, boardID: board.GetId()})
			}
		}
		return result, nil
	case *v1.BoardScope_AllBoards, *v1.BoardScope_AllSources:
		result := make([]readTarget, 0, len(s.sources))
		for index := range s.sources {
			result = append(result, readTarget{source: &s.sources[index]})
		}
		return result, nil
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid input: unknown board scope"))
	}
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

func (s *Server) routeForBoardRef(ref *v1.BoardRef) (boardRoute, error) {
	if ref == nil || ref.GetBoardId() == "" {
		return boardRoute{}, connect.NewError(connect.CodeInvalidArgument, errors.New("board reference is required"))
	}
	route, ok := s.boards[ref.GetBoardId()]
	if !ok || ref.GetSource().GetSourceId() != route.ref.GetSource().GetSourceId() ||
		(ref.GetSource().GetStoreLineageId() != "" && ref.GetSource().GetStoreLineageId() != route.ref.GetSource().GetStoreLineageId()) {
		return boardRoute{}, connect.NewError(connect.CodeNotFound, errors.New("board not found"))
	}
	return route, nil
}

func qualifySummary(server *Server, target readTarget, value *v1.IssueSummary) *v1.IssueSummary {
	result := proto.Clone(value).(*v1.IssueSummary)
	boardID := result.GetBoardId()
	if target.boardID != "" {
		boardID = target.boardID
	}
	ref := &v1.BoardRef{Source: sourceRefFromEntry(target.source), BoardId: boardID}
	if server != nil {
		if route, ok := server.boards[boardID]; ok && route.source.config.Alias == target.source.config.Alias {
			ref = proto.Clone(route.ref).(*v1.BoardRef)
		}
	}
	result.Ref = &v1.IssueRef{Board: ref, IssueId: result.GetId()}
	return result
}

func sourceRefFromEntry(value *source) *v1.SourceRef {
	if value.entry != nil && value.entry.GetSource() != nil {
		return proto.Clone(value.entry.GetSource()).(*v1.SourceRef)
	}
	return &v1.SourceRef{SourceId: value.config.Alias}
}

func compareIssueSummary(left, right *v1.IssueSummary) int {
	if result := cmp.Compare(left.GetPriority(), right.GetPriority()); result != 0 {
		return result
	}
	if result := compareTimestamp(left.GetUpdatedAt(), right.GetUpdatedAt()); result != 0 {
		return result
	}
	if result := strings.Compare(left.GetTitle(), right.GetTitle()); result != 0 {
		return result
	}
	leftSource, leftBoard := issueSourceBoard(left)
	rightSource, rightBoard := issueSourceBoard(right)
	if result := strings.Compare(leftSource, rightSource); result != 0 {
		return result
	}
	if result := strings.Compare(leftBoard, rightBoard); result != 0 {
		return result
	}
	return strings.Compare(left.GetId(), right.GetId())
}

func issueSourceBoard(value *v1.IssueSummary) (string, string) {
	return value.GetRef().GetBoard().GetSource().GetSourceId(), value.GetBoardId()
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
	case v1.IssueSort_ISSUE_SORT_UPDATED_AT:
		result = compareTimestamp(left.GetUpdatedAt(), right.GetUpdatedAt())
	case v1.IssueSort_ISSUE_SORT_CREATED_AT:
		result = compareTimestamp(left.GetCreatedAt(), right.GetCreatedAt())
	case v1.IssueSort_ISSUE_SORT_TITLE:
		result = strings.Compare(left.GetTitle(), right.GetTitle())
	default:
		result = cmp.Compare(left.GetPriority(), right.GetPriority())
	}
	if descending {
		result = -result
	}
	if result != 0 {
		return result
	}
	return compareIssueSummaryTieBreakers(left, right)
}

func compareIssueSummaryTieBreakers(left, right *v1.IssueSummary) int {
	leftSource, leftBoard := issueSourceBoard(left)
	rightSource, rightBoard := issueSourceBoard(right)
	if result := strings.Compare(leftSource, rightSource); result != 0 {
		return result
	}
	if result := strings.Compare(leftBoard, rightBoard); result != 0 {
		return result
	}
	return strings.Compare(left.GetId(), right.GetId())
}

func (s *Server) getIssue(
	ctx context.Context,
	request *connect.Request[v1.GetIssueRequest],
) (*connect.Response[v1.GetIssueResponse], error) {
	if request == nil || request.Msg == nil || request.Msg.GetIssueId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("issue ID is required"))
	}
	route, err := s.issueRoute(ctx, request.Msg.GetIssueId(), request.Msg.GetIssue())
	if err != nil {
		return nil, err
	}
	response, err := route.source.issues.GetIssue(ctx, connect.NewRequest(&v1.GetIssueRequest{IssueId: request.Msg.GetIssueId()}))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil || response.Msg.GetIssue() == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no issue"))
	}
	return connect.NewResponse(&v1.GetIssueResponse{Issue: qualifyDetail(s, route, response.Msg.GetIssue())}), nil
}

func (s *Server) issueRoute(ctx context.Context, issueID string, ref *v1.IssueRef) (boardRoute, error) {
	if ref != nil {
		if ref.GetIssueId() != "" && ref.GetIssueId() != issueID {
			return boardRoute{}, connect.NewError(connect.CodeInvalidArgument, errors.New("issue reference does not match issue ID"))
		}
		return s.routeForBoardRef(ref.GetBoard())
	}
	var found boardRoute
	foundCount := 0
	unavailable := false
	for index := range s.sources {
		source := &s.sources[index]
		response, err := source.issues.GetIssue(ctx, connect.NewRequest(&v1.GetIssueRequest{IssueId: issueID}))
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				continue
			}
			unavailable = true
			continue
		}
		if response == nil || response.Msg == nil || response.Msg.GetIssue() == nil {
			continue
		}
		boardID := response.Msg.GetIssue().GetIssue().GetBoardId()
		route, ok := s.boards[boardID]
		if !ok || route.source.config.Alias != source.config.Alias {
			continue
		}
		found, foundCount = route, foundCount+1
	}
	if foundCount == 1 {
		return found, nil
	}
	if foundCount > 1 {
		return boardRoute{}, connect.NewError(connect.CodeFailedPrecondition, errors.New("source-qualified issue reference is required"))
	}
	if unavailable {
		return boardRoute{}, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	return boardRoute{}, connect.NewError(connect.CodeNotFound, errors.New("issue not found"))
}

func qualifyDetail(server *Server, route boardRoute, value *v1.IssueDetail) *v1.IssueDetail {
	result := proto.Clone(value).(*v1.IssueDetail)
	if result.Issue != nil {
		result.Issue = qualifySummary(server, readTarget{source: &route.source, boardID: route.ref.GetBoardId()}, result.Issue)
	}
	for _, ancestor := range result.GetContext().GetAncestors() {
		ancestor.Issue = qualifyRelated(route, ancestor.GetIssue())
	}
	for _, dependency := range result.GetContext().GetDependencyResults() {
		dependency.Issue = qualifyRelated(route, dependency.GetIssue())
	}
	for _, node := range result.GetContainment().GetNodes() {
		node.Issue = qualifyRelated(route, node.GetIssue())
	}
	for index, related := range result.GetPrerequisites() {
		result.Prerequisites[index] = qualifyRelated(route, related)
	}
	for index, related := range result.GetDependents() {
		result.Dependents[index] = qualifyRelated(route, related)
	}
	return result
}

func qualifyRelated(route boardRoute, value *v1.RelatedIssue) *v1.RelatedIssue {
	if value == nil {
		return nil
	}
	result := proto.Clone(value).(*v1.RelatedIssue)
	result.Ref = &v1.IssueRef{Board: proto.Clone(route.ref).(*v1.BoardRef), IssueId: result.GetId()}
	return result
}

func (s *Server) listLogs(
	ctx context.Context,
	request *connect.Request[v1.ListLogEntriesRequest],
) (*connect.Response[v1.ListLogEntriesResponse], error) {
	if request == nil || request.Msg == nil || request.Msg.GetIssueId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("issue ID is required"))
	}
	route, err := s.issueRoute(ctx, request.Msg.GetIssueId(), request.Msg.GetIssue())
	if err != nil {
		return nil, err
	}
	response, err := route.source.records.ListLogEntries(ctx, connect.NewRequest(&v1.ListLogEntriesRequest{
		IssueId: request.Msg.GetIssueId(), Direction: request.Msg.GetDirection(),
	}))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no logs"))
	}
	issueRef := &v1.IssueRef{Board: proto.Clone(route.ref).(*v1.BoardRef), IssueId: request.Msg.GetIssueId()}
	entries := make([]*v1.LogEntry, 0, len(response.Msg.GetLogEntries()))
	for _, entry := range response.Msg.GetLogEntries() {
		value := proto.Clone(entry).(*v1.LogEntry)
		value.Ref = &v1.LogRef{Issue: proto.Clone(issueRef).(*v1.IssueRef), LogId: value.GetId()}
		entries = append(entries, value)
	}
	return connect.NewResponse(&v1.ListLogEntriesResponse{
		LogEntries:      entries,
		AggregateStatus: &v1.AggregateStatus{Complete: true},
	}), nil
}

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
			&v1.ListActionableCheckpointsRequest{Scope: targetScope(target)},
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
		return compareIssueSummary(checkpoints[left].GetCheckpoint(), checkpoints[right].GetCheckpoint()) < 0
	})
	return connect.NewResponse(&v1.ListActionableCheckpointsResponse{
		Checkpoints: checkpoints, AggregateStatus: aggregateStatus(problems),
	}), nil
}

func qualifyCheckpoint(server *Server, target readTarget, value *v1.ActionableCheckpoint) *v1.ActionableCheckpoint {
	result := proto.Clone(value).(*v1.ActionableCheckpoint)
	if result.Checkpoint != nil {
		result.Checkpoint = qualifySummary(server, target, result.Checkpoint)
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
	route := boardRoute{source: *target.source, ref: &v1.BoardRef{Source: sourceRefFromEntry(target.source), BoardId: value.GetBoardId()}}
	if server != nil {
		if known, ok := server.boards[value.GetBoardId()]; ok && known.source.config.Alias == target.source.config.Alias {
			route = known
		}
	}
	return qualifyRelated(route, value)
}

func targetScope(target readTarget) *v1.BoardScope {
	if target.boardID == "" {
		return &v1.BoardScope{Selection: &v1.BoardScope_AllBoards{AllBoards: &v1.AllBoards{}}}
	}
	return &v1.BoardScope{Selection: &v1.BoardScope_BoardId{BoardId: target.boardID}}
}

// executionReadMode selects one of the read-only eligibility collections.
type executionReadMode uint8

const (
	readyExecutionRead executionReadMode = iota
	blockedExecutionRead
)

func (s *Server) listExecution(
	ctx context.Context,
	scope *v1.BoardScope,
	limit uint32,
	mode executionReadMode,
) ([]*v1.IssueSummary, *v1.AggregateStatus, error) {
	targets, err := s.targets(scope)
	if err != nil {
		return nil, nil, err
	}
	problems := make(map[string]string)
	successes := 0
	var values []*v1.IssueSummary
	for _, target := range targets {
		if mode == blockedExecutionRead {
			var blockedResponse *connect.Response[v1.ListBlockedIssuesResponse]
			blockedResponse, err := target.source.execution.ListBlockedIssues(ctx, connect.NewRequest(&v1.ListBlockedIssuesRequest{Scope: targetScope(target), Limit: limit}))
			if err != nil || blockedResponse == nil || blockedResponse.Msg == nil {
				problems[target.source.config.Alias] = "source unavailable"
				continue
			}
			successes++
			for _, value := range blockedResponse.Msg.GetIssues() {
				values = append(values, qualifySummary(s, target, value))
			}
			continue
		}
		response, err := target.source.execution.ListReadyIssues(ctx, connect.NewRequest(&v1.ListReadyIssuesRequest{Scope: targetScope(target), Limit: limit}))
		if err != nil || response == nil || response.Msg == nil {
			problems[target.source.config.Alias] = "source unavailable"
			continue
		}
		successes++
		for _, value := range response.Msg.GetIssues() {
			values = append(values, qualifySummary(s, target, value))
		}
	}
	sort.SliceStable(values, func(left, right int) bool { return compareIssueSummary(values[left], values[right]) < 0 })
	if pageSize := aggregatePageSize(limit); len(values) > pageSize {
		values = values[:pageSize]
	}
	if len(targets) > 0 && successes == 0 {
		return nil, nil, connect.NewError(connect.CodeUnavailable, errors.New("no aggregate sources are available"))
	}
	return values, aggregateStatus(problems), nil
}

type issueService struct {
	privatev1connect.UnimplementedIssueServiceHandler
	server *Server
}

func (s *issueService) ListIssues(
	ctx context.Context,
	request *connect.Request[v1.ListIssuesRequest],
) (*connect.Response[v1.ListIssuesResponse], error) {
	return s.server.listIssues(ctx, request)
}

func (s *issueService) GetIssue(
	ctx context.Context,
	request *connect.Request[v1.GetIssueRequest],
) (*connect.Response[v1.GetIssueResponse], error) {
	return s.server.getIssue(ctx, request)
}

type recordService struct {
	privatev1connect.UnimplementedRecordServiceHandler
	server *Server
}

func (s *recordService) ListLogEntries(
	ctx context.Context,
	request *connect.Request[v1.ListLogEntriesRequest],
) (*connect.Response[v1.ListLogEntriesResponse], error) {
	return s.server.listLogs(ctx, request)
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

type executionService struct {
	privatev1connect.UnimplementedExecutionServiceHandler
	server *Server
}

func (s *executionService) ListReadyIssues(
	ctx context.Context,
	request *connect.Request[v1.ListReadyIssuesRequest],
) (*connect.Response[v1.ListReadyIssuesResponse], error) {
	if request == nil || request.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("execution request is required"))
	}
	issues, status, err := s.server.listExecution(
		ctx, request.Msg.GetScope(), request.Msg.GetLimit(), readyExecutionRead,
	)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ListReadyIssuesResponse{
		Issues: issues, AggregateStatus: status,
	}), nil
}

func (s *executionService) ListBlockedIssues(
	ctx context.Context,
	request *connect.Request[v1.ListBlockedIssuesRequest],
) (*connect.Response[v1.ListBlockedIssuesResponse], error) {
	if request == nil || request.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("execution request is required"))
	}
	issues, status, err := s.server.listExecution(
		ctx, request.Msg.GetScope(), request.Msg.GetLimit(), blockedExecutionRead,
	)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ListBlockedIssuesResponse{
		Issues: issues, AggregateStatus: status,
	}), nil
}
