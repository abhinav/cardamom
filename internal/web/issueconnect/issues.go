package issueconnect

import (
	"context"
	"slices"

	"go.abhg.dev/cardamom/internal/board"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/web"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

// ListIssues returns one stable page from a board-scoped or all-board collection.
func (s *Service) ListIssues(
	ctx context.Context,
	request *connect.Request[privatev1.ListIssuesRequest],
) (*connect.Response[privatev1.ListIssuesResponse], error) {
	readers, err := s.scopedReaders(ctx, request.Msg.GetScope())
	if err != nil {
		return nil, web.FromError(err)
	}
	facetReaders := readers
	domainRequest, filters, err := listIssueRequest(request.Msg)
	if err != nil {
		return nil, web.FromError(err)
	}
	if request.Msg.AncestorId != nil {
		readers, err = s.constrainToAncestor(
			ctx,
			request.Msg.GetScope(),
			readers,
			request.Msg.GetAncestorId(),
		)
		if err != nil {
			return nil, web.FromError(err)
		}
	}
	page, err := newIssuePage(request.Msg)
	if err != nil {
		return nil, web.FromError(err)
	}

	values := make([]issue.BoardIssueSummary, 0)
	revisions := make([]issuePageRevision, 0, len(readers))
	totalCount := 0
	boardRequest := domainRequest
	boardRequest.Limit = page.offset + page.size + 1
	for _, scoped := range readers {
		snapshot, err := scoped.reader.ListIssuesSnapshot(ctx, boardRequest)
		if err != nil {
			return nil, web.FromError(err)
		}
		revisions = append(revisions, issuePageRevision{
			BoardID: scoped.board.ID().String(), Revision: snapshot.Cursor.Revision,
		})
		totalCount += snapshot.Total
		for _, value := range snapshot.Issues {
			if filters.matches(value) {
				values = append(values, issue.BoardIssueSummary{
					BoardID: scoped.board.ID().String(), Summary: value,
				})
			}
		}
	}
	if err := page.setRevisions(revisions); err != nil {
		return nil, web.FromError(err)
	}
	orderedRequest := domainRequest
	orderedRequest.Limit = 0
	values = issue.OrderSummaries(orderedRequest, values)
	if page.offset > len(values) {
		return nil, web.FromError(errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: page token offset is outside the issue collection",
		))
	}
	pageEnd := min(page.offset+page.size, len(values))
	pageValues := values[page.offset:pageEnd]
	hasNextPage := pageEnd < len(values)

	response := &privatev1.ListIssuesResponse{
		Issues:      make([]*privatev1.IssueSummary, 0, len(pageValues)),
		LabelFacets: make([]*privatev1.LabelFacet, 0),
		Truncated:   hasNextPage,
		TotalCount:  uint32(totalCount),
	}
	for _, value := range pageValues {
		converted, err := s.views.Summary(board.ID(value.BoardID), value.Summary)
		if err != nil {
			return nil, web.FromError(err)
		}
		response.Issues = append(response.Issues, converted)
	}
	if hasNextPage {
		nextPageToken, err := page.nextToken(pageEnd)
		if err != nil {
			return nil, web.FromError(err)
		}
		response.NextPageToken = &nextPageToken
	}
	response.LabelFacets, err = labelFacets(ctx, facetReaders)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(response), nil
}

// issueCollectionFilters applies repeated protocol dimensions after each
// board reader has evaluated the shared domain filters.
type issueCollectionFilters struct {
	lifecycles map[string]struct{}
	statuses   map[string]struct{}
	types      map[string]struct{}
}

func (f *issueCollectionFilters) matches(value issue.Summary) bool {
	if len(f.lifecycles) > 0 {
		if _, ok := f.lifecycles[value.Issue.Lifecycle]; !ok {
			return false
		}
	}
	if len(f.statuses) > 0 {
		if _, ok := f.statuses[issueview.PresentationStatus(value.Issue, value.Blocked)]; !ok {
			return false
		}
	}
	if len(f.types) > 0 {
		if _, ok := f.types[value.Issue.Type]; !ok {
			return false
		}
	}
	return true
}

// listIssueRequest delegates shared collection filters to the domain request
// and retains only repeated protocol dimensions for semantic-field matching.
func listIssueRequest(
	request *privatev1.ListIssuesRequest,
) (issue.ListRequest, issueCollectionFilters, error) {
	result := issue.ListRequest{
		LabelsAll:     slices.Clone(request.GetLabelsAll()),
		LabelsAny:     slices.Clone(request.GetLabelsAny()),
		LabelsNone:    slices.Clone(request.GetLabelsNone()),
		TitleContains: request.GetTitleQuery(),
	}
	if request.AncestorId != nil {
		id, err := issue.NewID(request.GetAncestorId())
		if err != nil {
			return result, issueCollectionFilters{}, err
		}
		result.UnderID = id.String()
	}
	if request.Actor != nil {
		actor := issue.NewActor(request.GetActor())
		if actor == "" {
			return result, issueCollectionFilters{}, errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: actor is required",
			)
		}
		value := actor.String()
		result.Assignee = &value
	}
	for _, values := range [][]string{
		result.LabelsAll,
		result.LabelsAny,
		result.LabelsNone,
	} {
		for index, value := range values {
			label, err := issue.NewLabel(value)
			if err != nil {
				return result, issueCollectionFilters{}, err
			}
			values[index] = label.String()
		}
	}
	var err error
	result.Sort, err = issueSort(request.GetSort())
	if err != nil {
		return result, issueCollectionFilters{}, err
	}
	result.Reverse, err = sortDirection(request.GetDirection())
	if err != nil {
		return result, issueCollectionFilters{}, err
	}
	filters := issueCollectionFilters{
		lifecycles: make(map[string]struct{}),
		statuses:   make(map[string]struct{}),
		types:      make(map[string]struct{}),
	}
	for _, value := range request.GetLifecycles() {
		name, err := requestLifecycle(value)
		if err != nil {
			return result, issueCollectionFilters{}, err
		}
		filters.lifecycles[name] = struct{}{}
		result.Lifecycles = append(result.Lifecycles, name)
	}
	for _, value := range request.GetStatuses() {
		name, err := requestStatus(value)
		if err != nil {
			return result, issueCollectionFilters{}, err
		}
		filters.statuses[name] = struct{}{}
		result.Statuses = append(result.Statuses, name)
	}
	for _, value := range request.GetTypes() {
		name, err := requestType(value)
		if err != nil {
			return result, issueCollectionFilters{}, err
		}
		filters.types[name] = struct{}{}
		result.Types = append(result.Types, name)
	}
	return result, filters, nil
}

func issueSort(value privatev1.IssueSort) (string, error) {
	switch value {
	case privatev1.IssueSort_ISSUE_SORT_UNSPECIFIED:
		return "", nil
	case privatev1.IssueSort_ISSUE_SORT_PRIORITY:
		return "priority", nil
	case privatev1.IssueSort_ISSUE_SORT_UPDATED_AT:
		return "updated", nil
	case privatev1.IssueSort_ISSUE_SORT_CREATED_AT:
		return "created", nil
	case privatev1.IssueSort_ISSUE_SORT_TITLE:
		return "title", nil
	default:
		return "", errkind.Errorf(errkind.InvalidInput, "invalid input: unknown issue sort %d", value)
	}
}

func sortDirection(value privatev1.SortDirection) (bool, error) {
	switch value {
	case privatev1.SortDirection_SORT_DIRECTION_UNSPECIFIED,
		privatev1.SortDirection_SORT_DIRECTION_ASCENDING:
		return false, nil
	case privatev1.SortDirection_SORT_DIRECTION_DESCENDING:
		return true, nil
	default:
		return false, errkind.Errorf(errkind.InvalidInput, "invalid input: unknown sort direction %d", value)
	}
}

func requestLifecycle(value privatev1.IssueLifecycle) (string, error) {
	switch value {
	case privatev1.IssueLifecycle_ISSUE_LIFECYCLE_OPEN:
		return "open", nil
	case privatev1.IssueLifecycle_ISSUE_LIFECYCLE_CLOSED:
		return "closed", nil
	case privatev1.IssueLifecycle_ISSUE_LIFECYCLE_CANCELLED:
		return "cancelled", nil
	default:
		return "", errkind.Errorf(errkind.InvalidInput, "invalid input: unknown issue lifecycle %d", value)
	}
}

func requestStatus(value privatev1.IssueStatus) (string, error) {
	switch value {
	case privatev1.IssueStatus_ISSUE_STATUS_READY:
		return "ready", nil
	case privatev1.IssueStatus_ISSUE_STATUS_BLOCKED:
		return "blocked", nil
	case privatev1.IssueStatus_ISSUE_STATUS_IN_PROGRESS:
		return "in_progress", nil
	case privatev1.IssueStatus_ISSUE_STATUS_WAITING:
		return "waiting", nil
	case privatev1.IssueStatus_ISSUE_STATUS_CLOSED:
		return "closed", nil
	case privatev1.IssueStatus_ISSUE_STATUS_CANCELLED:
		return "cancelled", nil
	default:
		return "", errkind.Errorf(errkind.InvalidInput, "invalid input: unknown issue status %d", value)
	}
}

func requestType(value privatev1.IssueType) (string, error) {
	switch value {
	case privatev1.IssueType_ISSUE_TYPE_WORKSTREAM:
		return "workstream", nil
	case privatev1.IssueType_ISSUE_TYPE_TASK:
		return "task", nil
	case privatev1.IssueType_ISSUE_TYPE_CHECKPOINT:
		return "checkpoint", nil
	case privatev1.IssueType_ISSUE_TYPE_ROUTINE:
		return "routine", nil
	default:
		return "", errkind.Errorf(errkind.InvalidInput, "invalid input: unknown issue type %d", value)
	}
}

func (s *Service) constrainToAncestor(
	ctx context.Context,
	scope *privatev1.BoardScope,
	readers []scopedBoardReader,
	ancestorID string,
) ([]scopedBoardReader, error) {
	owner, err := s.readerForIssue(ctx, ancestorID)
	if err != nil {
		return nil, err
	}
	if _, oneBoard := scope.Selection.(*privatev1.BoardScope_BoardId); oneBoard {
		if len(readers) != 1 || readers[0].board.ID() != owner.board.ID() {
			return nil, errkind.Errorf(
				errkind.Conflict,
				"issue belongs to another board: ancestor %q belongs to board %q",
				ancestorID,
				owner.board.ID(),
			)
		}
		return readers, nil
	}
	return []scopedBoardReader{owner}, nil
}

func labelFacets(
	ctx context.Context,
	readers []scopedBoardReader,
) ([]*privatev1.LabelFacet, error) {
	counts := make(map[string]uint32)
	for _, scoped := range readers {
		values, err := scoped.reader.ListIssues(ctx, issue.ListRequest{})
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			for _, label := range value.Labels {
				counts[label]++
			}
		}
	}
	labels := make([]string, 0, len(counts))
	for label := range counts {
		labels = append(labels, label)
	}
	slices.Sort(labels)
	result := make([]*privatev1.LabelFacet, 0, len(labels))
	for _, label := range labels {
		result = append(result, &privatev1.LabelFacet{Label: label, IssueCount: counts[label]})
	}
	return result, nil
}

// GetIssue returns one issue's primary record, inherited context, and current
// relationship projections without loading its log entry bodies.
func (s *Service) GetIssue(
	ctx context.Context,
	request *connect.Request[privatev1.GetIssueRequest],
) (*connect.Response[privatev1.GetIssueResponse], error) {
	scoped, err := s.readerForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	depth := 0
	view, err := scoped.reader.ReadIssue(ctx, issue.ReadRequest{
		IssueID: request.Msg.GetIssueId(), ContextDepth: &depth,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.Detail(ctx, scoped.board.ID(), view)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.GetIssueResponse{Issue: converted}), nil
}
