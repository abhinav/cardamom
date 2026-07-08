package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func (r *Repository) replaceLabels(
	ctx context.Context,
	mutation *mutation,
	issueID issue.ID,
	labels []issue.Label,
) error {
	queries := query.New(mutation.change)
	if err := queries.BoardDeleteIssueLabels(
		ctx,
		query.BoardDeleteIssueLabelsParams{
			BoardID: r.boardID.String(),
			IssueID: issueID.String(),
		},
	); err != nil {
		return err
	}
	for _, label := range labels {
		if err := queries.BoardInsertIssueLabel(
			ctx,
			query.BoardInsertIssueLabelParams{
				BoardID: r.boardID.String(),
				IssueID: issueID.String(),
				Label:   label.String(),
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) readLabels(
	ctx context.Context,
	scope queryScope,
	id issue.ID,
) ([]string, error) {
	labels, err := query.New(scope).BoardListLabelsForIssue(
		ctx,
		query.BoardListLabelsForIssueParams{
			BoardID: r.boardID.String(),
			IssueID: id.String(),
		},
	)
	if err != nil {
		return nil, err
	}
	if labels == nil {
		labels = []string{}
	}
	return labels, nil
}

func (r *Repository) readBoardIssueLabels(
	ctx context.Context,
	scope queryScope,
) (map[issue.ID][]string, error) {
	rows, err := query.New(scope).BoardListAllIssueLabels(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	labels := make(map[issue.ID][]string)
	for _, row := range rows {
		id := issue.ID(row.IssueID)
		labels[id] = append(labels[id], row.Label)
	}
	return labels, nil
}

func labelsFromStrings(values []string) ([]issue.Label, error) {
	labels := make([]issue.Label, len(values))
	for index, value := range values {
		label, err := issue.NewLabel(value)
		if err != nil {
			return nil, err
		}
		labels[index] = label
	}
	return labels, nil
}
