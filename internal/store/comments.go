package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AddComment appends a new comment to an issue. Validates the issue
// exists. Returns the inserted Comment with its allocated ID and
// `created` timestamp.
func (s *Store) AddComment(ctx context.Context, issueID, author, body string) (Comment, error) {
	if author == "" {
		return Comment{}, errors.New("author required")
	}
	if body == "" {
		return Comment{}, errors.New("body required")
	}
	if err := s.exists(ctx, issueID); err != nil {
		return Comment{}, err
	}
	c := Comment{IssueID: issueID, Author: author, Body: body, Created: now()}
	if _, err := s.db.NewInsert().Model(&c).Exec(ctx); err != nil {
		return Comment{}, err
	}
	s.recordEvent(ctx, issueID, "commented", map[string]any{"comment_id": c.ID})
	return c, nil
}

// Comments returns all comments on an issue in chronological order.
// Returns ErrNotFound if the issue itself doesn't exist (distinct from
// "no comments yet", which returns an empty slice).
func (s *Store) Comments(ctx context.Context, issueID string) ([]Comment, error) {
	if err := s.exists(ctx, issueID); err != nil {
		return nil, err
	}
	var cs []Comment
	err := s.db.NewSelect().
		Model(&cs).
		Where("issue_id = ?", issueID).
		OrderExpr("id ASC").
		Scan(ctx)
	return cs, err
}

// RemoveComment deletes a comment by its numeric ID. Returns
// ErrCommentNotFound if no such comment exists — distinct from
// ErrNotFound (which is for issues) so the CLI can say "comment not
// found" instead of "issue not found".
func (s *Store) RemoveComment(ctx context.Context, commentID int64) error {
	// Look up the owning issue first so the audit event can be scoped to
	// it (the delete itself only knows the comment id).
	var issueID string
	err := s.db.NewSelect().Model((*Comment)(nil)).
		Column("issue_id").Where("id = ?", commentID).Scan(ctx, &issueID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCommentNotFound
	}
	if err != nil {
		return err
	}
	res, err := s.db.NewDelete().
		Model((*Comment)(nil)).
		Where("id = ?", commentID).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCommentNotFound
	}
	s.recordEvent(ctx, issueID, "comment_removed", map[string]any{"comment_id": commentID})
	return nil
}

// EditComment replaces the body of an existing comment. Preserves
// author and created so issue-history references stay stable. Returns
// ErrCommentNotFound if no row matches.
func (s *Store) EditComment(ctx context.Context, commentID int64, body string) (Comment, error) {
	if body == "" {
		return Comment{}, fmt.Errorf("%w: body required", ErrInvalid)
	}
	res, err := s.db.NewUpdate().
		Model((*Comment)(nil)).
		Set("body = ?", body).
		Where("id = ?", commentID).
		Exec(ctx)
	if err != nil {
		return Comment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Comment{}, ErrCommentNotFound
	}
	var cm Comment
	if err := s.db.NewSelect().Model(&cm).Where("id = ?", commentID).Scan(ctx); err != nil {
		return Comment{}, err
	}
	s.recordEvent(ctx, cm.IssueID, "comment_edited", map[string]any{"comment_id": commentID})
	return cm, nil
}

// UpsertComment inserts a comment with an explicit ID (used by import).
// On conflict, updates every field.
func (s *Store) UpsertComment(ctx context.Context, c Comment) error {
	return UpsertCommentTx(ctx, s.db, c)
}

// AllComments returns every comment, ordered deterministically for export.
func (s *Store) AllComments(ctx context.Context) ([]Comment, error) {
	var cs []Comment
	err := s.db.NewSelect().Model(&cs).OrderExpr("id ASC").Scan(ctx)
	return cs, err
}
