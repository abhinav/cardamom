package aggregate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"slices"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"google.golang.org/protobuf/proto"
)

const (
	aggregateIssuePageSize   = 250
	defaultAggregatePageSize = 100
	maximumAggregateCursors  = 128
)

// issueCursor retains one upstream page per source between browser pages.
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
		value.setPage(response.Msg)
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

func (s *issueCursorSource) setPage(response *v1.ListIssuesResponse) {
	s.values = s.values[:0]
	for _, value := range response.GetIssues() {
		if s.target.boardID != "" && value.GetBoardId() != s.target.boardID {
			continue
		}
		s.values = append(s.values, qualifySummary(s.target, value))
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
	value.setPage(response.Msg)
	for index, issue := range value.values {
		value.values[index] = qualifySummary(value.target, issue)
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
