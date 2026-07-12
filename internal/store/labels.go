package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// AddLabels attaches one or more labels to an issue. Returns the
// number actually inserted (i.e. excluding duplicates that were already
// present). Empty list → (0, nil). Returns ErrNotFound if the issue
// does not exist. Rejects empty-string labels — a label with no
// characters is almost certainly a quoting mistake and persists as a
// blank row in `label ls`.
func (s *Store) AddLabels(ctx context.Context, issueID string, labels []string) (int, error) {
	if len(labels) == 0 {
		return 0, nil
	}
	for _, l := range labels {
		if l == "" {
			return 0, fmt.Errorf("%w: label cannot be empty", ErrInvalid)
		}
	}
	if err := s.exists(ctx, issueID); err != nil {
		return 0, err
	}
	// Compute the labels that aren't already present so the audit event
	// records what actually changed, not the requested set.
	have, err := s.labelSet(ctx, issueID)
	if err != nil {
		return 0, err
	}
	var added []string
	for _, l := range labels {
		if !have[l] {
			added = append(added, l)
			have[l] = true // dedupe within this call
		}
	}
	rows := make([]IssueLabel, len(labels))
	for i, l := range labels {
		rows[i] = IssueLabel{IssueID: issueID, Label: l}
	}
	res, err := s.db.NewInsert().Model(&rows).On("CONFLICT DO NOTHING").Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if len(added) > 0 {
		s.recordEvent(ctx, issueID, "labeled", map[string]any{"added": added})
	}
	return int(n), nil
}

// labelSet returns the issue's current labels as a set for membership
// checks. The issue is assumed to exist (callers check first).
func (s *Store) labelSet(ctx context.Context, issueID string) (map[string]bool, error) {
	var labels []string
	if err := s.db.NewSelect().
		Model((*IssueLabel)(nil)).
		Column("label").
		Where("issue_id = ?", issueID).
		Scan(ctx, &labels); err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(labels))
	for _, l := range labels {
		set[l] = true
	}
	return set, nil
}

// RemoveLabels detaches labels from an issue. Returns the number
// actually removed (0 when the caller named labels that weren't on
// the issue). Empty list → (0, nil). Returns ErrNotFound if the issue
// itself doesn't exist (vs. silently no-op'ing on a typo'd ID).
func (s *Store) RemoveLabels(ctx context.Context, issueID string, labels []string) (int, error) {
	if err := s.exists(ctx, issueID); err != nil {
		return 0, err
	}
	if len(labels) == 0 {
		return 0, nil
	}
	// Record only the labels that were actually present (and thus
	// removed), not the requested set.
	have, err := s.labelSet(ctx, issueID)
	if err != nil {
		return 0, err
	}
	var removed []string
	for _, l := range labels {
		if have[l] {
			removed = append(removed, l)
			delete(have, l) // dedupe within this call
		}
	}
	res, err := s.db.NewDelete().
		Model((*IssueLabel)(nil)).
		Where("issue_id = ?", issueID).
		Where("label IN (?)", bun.List(labels)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if len(removed) > 0 {
		s.recordEvent(ctx, issueID, "unlabeled", map[string]any{"removed": removed})
	}
	return int(n), nil
}

// LabelsForIssue returns the labels on a single issue, alphabetically.
// Returns ErrNotFound if the issue doesn't exist (distinct from "issue
// exists but has no labels", which returns an empty slice).
func (s *Store) LabelsForIssue(ctx context.Context, issueID string) ([]string, error) {
	if err := s.exists(ctx, issueID); err != nil {
		return nil, err
	}
	var labels []string
	err := s.db.NewSelect().
		Model((*IssueLabel)(nil)).
		Column("label").
		Where("issue_id = ?", issueID).
		OrderExpr("label").
		Scan(ctx, &labels)
	return labels, err
}

// AllLabels returns every distinct label in the project, alphabetically.
// Includes workflow-internal labels (run:*, step:*, etc.); callers that
// only want user-managed tags should filter the result.
func (s *Store) AllLabels(ctx context.Context) ([]string, error) {
	var labels []string
	err := s.db.NewSelect().
		Model((*IssueLabel)(nil)).
		ColumnExpr("DISTINCT label").
		OrderExpr("label").
		Scan(ctx, &labels)
	return labels, err
}

// LoadLabels returns a map id -> []labels for the given issue IDs in one query.
// Used for batch display in list/show output.
func (s *Store) LoadLabels(ctx context.Context, ids []string) (map[string][]string, error) {
	out := make(map[string][]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []IssueLabel
	err := s.db.NewSelect().
		Model(&rows).
		Where("issue_id IN (?)", bun.List(ids)).
		OrderExpr("issue_id, label").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.IssueID] = append(out[r.IssueID], r.Label)
	}
	return out, nil
}
