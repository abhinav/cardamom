package record

import (
	"context"
	"strings"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// SetResult replaces one issue's durable result.
type SetResult struct {
	// IssueID identifies the issue whose result changes.
	IssueID issue.ID
	// Body is the replacement Markdown result source.
	Body string
}

// ResultSet is the semantic outcome of replacing one result.
type ResultSet struct {
	// IssueID identifies the changed issue.
	IssueID issue.ID
	// Body is the normalized replacement result.
	Body string
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// SetResultResult reports the committed result.
type SetResultResult struct {
	// IssueID identifies the changed issue.
	IssueID string
	// Body is the normalized committed result.
	Body string
}

// SetResultRequest supplies caller input for replacing one result.
type SetResultRequest struct {
	// IssueID identifies the issue whose result changes.
	IssueID string
	// Body is the replacement Markdown result source.
	Body string
}

// SetResult applies result policy to the loaded snapshot.
func (p *Policy) SetResult(command SetResult) (ResultSet, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return ResultSet{}, ErrIncompleteSnapshot
	}
	body := strings.TrimSpace(command.Body)
	if body == "" {
		return ResultSet{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: result required",
		)
	}
	return ResultSet{
		IssueID: command.IssueID,
		Body:    body,
	}, nil
}

// SetResult validates caller input and replaces one result.
func (r *Recorder) SetResult(
	ctx context.Context,
	_ issue.Invocation,
	request SetResultRequest,
) (SetResultResult, error) {
	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return SetResultResult{}, err
	}
	outcome, err := r.changes.SetResult(
		ctx,
		SetResult{IssueID: id, Body: request.Body},
	)
	if err != nil {
		return SetResultResult{}, err
	}
	return SetResultResult{IssueID: outcome.IssueID.String(), Body: outcome.Body}, nil
}
