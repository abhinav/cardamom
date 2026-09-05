package board

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"go.abhg.dev/cardamom/internal/issue"
)

// SearchIssues returns one result per matching issue from a coherent board
// snapshot.
func (r *Repository) SearchIssues(
	ctx context.Context,
	request issue.SearchRequest,
) (out issue.SearchResult, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	index, err := r.readBoardIssueIndex(ctx, view)
	if err != nil {
		return out, err
	}
	descendants, err := r.descendantSet(ctx, view, request.UnderID)
	if err != nil {
		return out, err
	}
	matches := make(map[issue.ID]*issue.SearchMatch)
	matchedFields := make(map[issue.ID]map[issue.SearchField]struct{})
	err = r.visitIssueSearchDocuments(ctx, view, request, func(document searchDocument) error {
		id := issue.ID(document.issueID)
		if _, ok := index.states[id]; !ok {
			return fmt.Errorf("search document references absent issue %q", id)
		}
		projected := index.summary(id)
		if !matchesSearchRequest(request, projected, descendants) {
			return nil
		}
		fields := matchedFields[id]
		if fields == nil {
			fields = make(map[issue.SearchField]struct{})
			matchedFields[id] = fields
		}
		fields[document.field] = struct{}{}
		if _, ok := matches[id]; ok {
			return nil
		}
		matches[id] = &issue.SearchMatch{
			Summary:   projected,
			Relevance: document.relevance,
			Excerpt: issue.SearchExcerpt{
				Field: document.field, RecordID: document.recordID,
				Text: document.excerpt,
			},
		}
		return nil
	})
	if err != nil {
		return out, err
	}

	out.Matches = make([]issue.SearchMatch, 0, len(matches))
	for id, match := range matches {
		for _, field := range issue.AllSearchFields() {
			if _, ok := matchedFields[id][field]; ok {
				match.MatchedFields = append(match.MatchedFields, field)
			}
		}
		out.Matches = append(out.Matches, *match)
	}
	out.Total = len(out.Matches)
	orderSearchMatches(request, r.boardID.String(), out.Matches)
	if request.Limit > 0 && len(out.Matches) > request.Limit {
		out.Matches = out.Matches[:request.Limit]
	}
	return out, nil
}

type searchDocument struct {
	issueID   string
	field     issue.SearchField
	recordID  *issue.LogID
	relevance float64
	excerpt   string
}

// sqlc's SQLite parser does not accept the FTS5 MATCH operator. Keep this
// stable statement here so the board repository still owns the only runtime
// query that uses the virtual table directly.
const searchIssueDocumentsSQL = `
SELECT
    document.issue_id,
    document.field,
    document.record_id,
    CAST(
        -bm25(issue_search_fts) * CASE document.field
            WHEN 'title' THEN 8.0
            WHEN 'summary' THEN 4.0
            WHEN 'details' THEN 2.0
            WHEN 'result' THEN 2.0
            WHEN 'state' THEN 1.0
            WHEN 'log' THEN 1.0
        END
        AS REAL
    ) AS relevance,
    CAST(
        snippet(issue_search_fts, 0, '[', ']', ' … ', 24)
        AS TEXT
    ) AS excerpt
FROM issue_search_fts
JOIN issue_search_documents AS document
    ON document.rowid = issue_search_fts.rowid
WHERE issue_search_fts.body MATCH ?
    AND document.board_id = ?
    AND document.field IN (?, ?, ?, ?, ?, ?)
ORDER BY
    relevance DESC,
    document.issue_id,
    CASE document.field
        WHEN 'title' THEN 1
        WHEN 'summary' THEN 2
        WHEN 'details' THEN 3
        WHEN 'result' THEN 4
        WHEN 'state' THEN 5
        WHEN 'log' THEN 6
    END,
    document.record_id
`

func (r *Repository) visitIssueSearchDocuments(
	ctx context.Context,
	scope queryScope,
	request issue.SearchRequest,
	visit func(searchDocument) error,
) (err error) {
	fields := make([]any, len(issue.AllSearchFields()))
	for index, field := range request.Fields {
		fields[index] = field.String()
	}
	rows, err := scope.QueryContext(
		ctx,
		searchIssueDocumentsSQL,
		request.Query.Expression(),
		r.boardID.String(),
		fields[0],
		fields[1],
		fields[2],
		fields[3],
		fields[4],
		fields[5],
	)
	if err != nil {
		return fmt.Errorf("search issue documents: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		var issueID, fieldValue, recordID, excerpt string
		var relevance float64
		if err := rows.Scan(
			&issueID,
			&fieldValue,
			&recordID,
			&relevance,
			&excerpt,
		); err != nil {
			return fmt.Errorf("scan issue search document: %w", err)
		}
		field, err := issue.NewSearchField(fieldValue)
		if err != nil {
			return err
		}
		var parsedRecordID *issue.LogID
		if recordID != "" {
			id, err := issue.NewLogID(recordID)
			if err != nil {
				return err
			}
			parsedRecordID = &id
		}
		if err := visit(searchDocument{
			issueID: issueID, field: field, recordID: parsedRecordID,
			relevance: relevance, excerpt: excerpt,
		}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read issue search documents: %w", err)
	}
	return nil
}

func matchesSearchRequest(
	request issue.SearchRequest,
	summary issue.Summary,
	descendants map[issue.ID]struct{},
) bool {
	if !matchesIssueRequest(issue.ListRequest{
		UnderID: request.UnderID, Statuses: request.Statuses,
		Assignee: request.Assignee, Type: request.Type,
		LabelsAll: request.LabelsAll, LabelsAny: request.LabelsAny,
		LabelsNone: request.LabelsNone, NoAssignee: request.NoAssignee,
	}, summary, descendants) {
		return false
	}
	value := summary.Issue
	if request.CreatedSince != nil && value.Created < request.CreatedSince.Unix() {
		return false
	}
	if request.CreatedBefore != nil && value.Created >= request.CreatedBefore.Unix() {
		return false
	}
	if request.ClosedSince != nil &&
		(value.Closed == nil || *value.Closed < request.ClosedSince.Unix()) {
		return false
	}
	if request.ClosedBefore != nil &&
		(value.Closed == nil || *value.Closed >= request.ClosedBefore.Unix()) {
		return false
	}
	return true
}

func orderSearchMatches(
	request issue.SearchRequest,
	boardID string,
	matches []issue.SearchMatch,
) {
	if request.Sort == "" || request.Sort == "relevance" {
		slices.SortStableFunc(matches, func(left, right issue.SearchMatch) int {
			if order := cmp.Compare(right.Relevance, left.Relevance); order != 0 {
				return order
			}
			return cmp.Compare(left.Summary.Issue.ID, right.Summary.Issue.ID)
		})
		return
	}

	byID := make(map[string]issue.SearchMatch, len(matches))
	values := make([]issue.BoardIssueSummary, 0, len(matches))
	for _, match := range matches {
		byID[match.Summary.Issue.ID] = match
		values = append(values, issue.BoardIssueSummary{
			BoardID: boardID,
			Summary: match.Summary,
		})
	}
	ordered := issue.OrderSummaries(issue.ListRequest{
		Sort: request.Sort, Reverse: request.Reverse,
	}, values)
	for index, value := range ordered {
		matches[index] = byID[value.Summary.Issue.ID]
	}
}
