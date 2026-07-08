package attachment

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
)

// IssueViewReader reads one issue and its optional inherited context.
type IssueViewReader interface {
	// ReadIssue returns the selected issue view.
	ReadIssue(context.Context, issue.ReadRequest) (issue.View, error)
}

// IssueAttachmentLister reads stable pages of attachment metadata.
type IssueAttachmentLister interface {
	// ListAttachments returns one stable attachment page.
	ListAttachments(context.Context, ListRequest) (Page, error)
}

// IssueView combines one issue view with its active originating attachments.
type IssueView struct {
	// Issue is the selected issue and its optional inherited context.
	Issue issue.View // required

	// Attachments contains every active attachment associated with Issue.
	Attachments []Attachment // required
}

// IssueInspector owns the finite issue view that includes attachment
// associations.
type IssueInspector struct {
	boardID     board.ID              // required
	issues      IssueViewReader       // required
	attachments IssueAttachmentLister // required
}

// NewIssueInspector constructs issue inspection for one selected board.
func NewIssueInspector(
	boardID board.ID,
	issues IssueViewReader,
	attachments IssueAttachmentLister,
) *IssueInspector {
	must.NotBeNilf(issues, "attachment IssueViewReader is required")
	must.NotBeNilf(attachments, "attachment IssueAttachmentLister is required")
	return &IssueInspector{
		boardID: boardID, issues: issues, attachments: attachments,
	}
}

// ReadIssue returns one issue and every active attachment associated with it.
func (i *IssueInspector) ReadIssue(
	ctx context.Context,
	request issue.ReadRequest,
) (IssueView, error) {
	view, err := i.issues.ReadIssue(ctx, request)
	if err != nil {
		return IssueView{}, err
	}
	issueID, err := issue.NewID(request.IssueID)
	if err != nil {
		return IssueView{}, fmt.Errorf("parse inspected issue ID: %w", err)
	}

	attachments := make([]Attachment, 0)
	pageToken := ""
	for {
		page, err := i.attachments.ListAttachments(ctx, ListRequest{
			BoardID: i.boardID, OriginIssueID: &issueID, PageToken: pageToken,
		})
		if err != nil {
			return IssueView{}, fmt.Errorf(
				"list attachments for issue %q: %w",
				request.IssueID,
				err,
			)
		}
		attachments = append(attachments, page.Attachments...)
		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}

	return IssueView{Issue: view, Attachments: attachments}, nil
}
