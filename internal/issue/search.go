package issue

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.abhg.dev/cardamom/internal/searchquery"
)

// SearchIssues returns full-text issue matches from one board.
func (q *Queries) SearchIssues(ctx context.Context, req SearchRequest) (SearchResult, error) {
	req, err := normalizeSearchRequest(req)
	if err != nil {
		return SearchResult{}, err
	}
	return q.reader.SearchIssues(ctx, req)
}

// SearchRequest selects, filters, orders, and bounds full-text issue matches.
type SearchRequest struct {
	// Query is the validated expression matched against Fields.
	Query searchquery.Query // required

	// Fields selects searchable record families. Empty selects every field.
	Fields []SearchField

	// UnderID limits results to strict containment descendants of one issue.
	UnderID string

	// Statuses matches any requested presentation status.
	Statuses []string

	// Assignee matches issues with active custody by the named actor.
	Assignee *string

	// Type matches one issue type.
	Type string

	// LabelsAll requires every requested label.
	LabelsAll []string

	// LabelsAny requires at least one requested label.
	LabelsAny []string

	// LabelsNone excludes issues carrying any requested label.
	LabelsNone []string

	// NoAssignee matches issues without active custody.
	NoAssignee bool

	// CreatedSince includes issues created at or after this instant.
	CreatedSince *time.Time

	// CreatedBefore includes issues created before this instant.
	CreatedBefore *time.Time

	// ClosedSince includes issues closed or cancelled at or after this instant.
	ClosedSince *time.Time

	// ClosedBefore includes issues closed or cancelled before this instant.
	ClosedBefore *time.Time

	// Sort selects relevance or a supported issue field. Empty uses relevance.
	Sort string

	// Reverse reverses a non-relevance order.
	Reverse bool

	// Limit is the maximum result count. Zero returns every match.
	Limit int
}

// SearchResult contains bounded matches and their count before the limit.
type SearchResult struct {
	// Total is the number of matching issues before the request limit.
	Total int

	// Matches contains issue matches in the requested order.
	Matches []SearchMatch
}

// SearchMatch combines one matching issue with its field provenance and best
// excerpt.
type SearchMatch struct {
	// Summary contains the issue metadata used for filtering and navigation.
	Summary Summary

	// MatchedFields contains every field with at least one matching document.
	MatchedFields []SearchField

	// Excerpt is the highest-scoring matching document excerpt.
	Excerpt SearchExcerpt

	// Relevance is the highest weighted document score for this issue.
	Relevance float64
}

// SearchExcerpt identifies and summarizes the highest-scoring matching
// document for one issue.
type SearchExcerpt struct {
	// Field is the issue record family that supplied Text.
	Field SearchField

	// RecordID identifies the source Log entry. Other fields leave it nil.
	RecordID *LogID

	// Text is the bounded source excerpt with matching terms marked.
	Text string
}

// SearchField identifies one issue record family included in full-text search.
type SearchField uint8

const (
	searchFieldUnknown SearchField = iota

	// SearchFieldTitle searches the issue title.
	SearchFieldTitle

	// SearchFieldSummary searches inherited stable context.
	SearchFieldSummary

	// SearchFieldDetails searches expanded stable context.
	SearchFieldDetails

	// SearchFieldState searches current mutable recovery context.
	SearchFieldState

	// SearchFieldResult searches the current durable outcome.
	SearchFieldResult

	// SearchFieldLog searches immutable posts and State snapshots.
	SearchFieldLog
)

// NewSearchField parses one public search field name.
func NewSearchField(value string) (SearchField, error) {
	switch value {
	case SearchFieldTitle.String():
		return SearchFieldTitle, nil
	case SearchFieldSummary.String():
		return SearchFieldSummary, nil
	case SearchFieldDetails.String():
		return SearchFieldDetails, nil
	case SearchFieldState.String():
		return SearchFieldState, nil
	case SearchFieldResult.String():
		return SearchFieldResult, nil
	case SearchFieldLog.String():
		return SearchFieldLog, nil
	default:
		return searchFieldUnknown, fmt.Errorf("unsupported search field %q", value)
	}
}

// AllSearchFields returns every searchable issue record family in presentation
// order.
func AllSearchFields() []SearchField {
	return []SearchField{
		SearchFieldTitle,
		SearchFieldSummary,
		SearchFieldDetails,
		SearchFieldState,
		SearchFieldResult,
		SearchFieldLog,
	}
}

// String returns the public field name.
func (f SearchField) String() string {
	switch f {
	case SearchFieldTitle:
		return "title"
	case SearchFieldSummary:
		return "summary"
	case SearchFieldDetails:
		return "details"
	case SearchFieldState:
		return "state"
	case SearchFieldResult:
		return "result"
	case SearchFieldLog:
		return "log"
	default:
		return ""
	}
}

func normalizeSearchRequest(req SearchRequest) (SearchRequest, error) {
	if req.Query.Expression() == "" {
		return SearchRequest{}, errors.New("search query is required")
	}
	if len(req.Fields) == 0 {
		req.Fields = AllSearchFields()
	} else {
		req.Fields = slices.Clone(req.Fields)
	}
	seenFields := make(map[SearchField]struct{}, len(req.Fields))
	for _, field := range req.Fields {
		if field.String() == "" {
			return SearchRequest{}, errors.New("invalid search field")
		}
		if _, ok := seenFields[field]; ok {
			return SearchRequest{}, fmt.Errorf("duplicate search field %q", field)
		}
		seenFields[field] = struct{}{}
	}
	all, anyOf, none, err := normalizeLabelGroups(
		req.LabelsAll,
		req.LabelsAny,
		req.LabelsNone,
	)
	if err != nil {
		return SearchRequest{}, err
	}
	req.LabelsAll = all
	req.LabelsAny = anyOf
	req.LabelsNone = none
	if invalidSearchTimeRange(req.CreatedSince, req.CreatedBefore) {
		return SearchRequest{}, errors.New("created-since must be before created-before")
	}
	if invalidSearchTimeRange(req.ClosedSince, req.ClosedBefore) {
		return SearchRequest{}, errors.New("closed-since must be before closed-before")
	}
	return req, nil
}

func invalidSearchTimeRange(since, before *time.Time) bool {
	return since != nil && before != nil && !since.Before(*before)
}
