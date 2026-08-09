package attachment

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.uber.org/mock/gomock"
)

func TestIssueInspector_ReadIssue_listsEveryAssociatedAttachmentPage(t *testing.T) {
	t.Parallel()

	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	issueID, err := issue.NewID("an-1")
	require.NoError(t, err)
	first := validAttachment(t)
	firstID, err := NewID("att_" + strings.Repeat("a", 26))
	require.NoError(t, err)
	first.ID = firstID
	second := first
	secondID, err := NewID("att_" + strings.Repeat("b", 25) + "a")
	require.NoError(t, err)
	second.ID = secondID

	issueView := issue.View{Detail: issue.Detail{Issue: issue.Issue{ID: issueID.String()}}}
	readRequest := issue.ReadRequest{IssueID: issueID.String(), ContextDepth: new(2)}
	issues := NewMockIssueViewReader(gomock.NewController(t))
	issues.EXPECT().ReadIssue(gomock.Any(), readRequest).Return(issueView, nil)
	attachments := &issueAttachmentListerStub{pages: []Page{
		{Attachments: []Attachment{first}, NextPageToken: "next"},
		{Attachments: []Attachment{second}},
	}}
	inspector := NewIssueInspector(boardID, issues, attachments)

	view, err := inspector.ReadIssue(t.Context(), readRequest)

	require.NoError(t, err)
	assert.Equal(t, issueView, view.Issue)
	assert.Equal(t, []Attachment{first, second}, view.Attachments)
	require.Len(t, attachments.requests, 2)
	assert.Equal(t, ListRequest{
		BoardID: boardID, OriginIssueID: &issueID,
	}, attachments.requests[0])
	assert.Equal(t, ListRequest{
		BoardID: boardID, OriginIssueID: &issueID, PageToken: "next",
	}, attachments.requests[1])
}

func TestIssueInspector_ReadIssue_usesResolvedIssueForKeyRequest(t *testing.T) {
	t.Parallel()

	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	issueID := issue.MustID("an-1")
	issueView := issue.View{Detail: issue.Detail{Issue: issue.Issue{ID: issueID.String()}}}
	readRequest := issue.ReadRequest{Key: "source:one"}
	issues := NewMockIssueViewReader(gomock.NewController(t))
	issues.EXPECT().ReadIssue(gomock.Any(), readRequest).Return(issueView, nil)
	attachments := &issueAttachmentListerStub{pages: []Page{{Attachments: []Attachment{}}}}
	inspector := NewIssueInspector(boardID, issues, attachments)

	view, err := inspector.ReadIssue(t.Context(), readRequest)

	require.NoError(t, err)
	assert.Equal(t, issueID.String(), view.Issue.Detail.Issue.ID)
	require.Len(t, attachments.requests, 1)
	assert.Equal(t, ListRequest{
		BoardID: boardID, OriginIssueID: &issueID,
	}, attachments.requests[0])
}

type issueAttachmentListerStub struct {
	requests []ListRequest
	pages    []Page
}

func (s *issueAttachmentListerStub) ListAttachments(
	_ context.Context,
	request ListRequest,
) (Page, error) {
	s.requests = append(s.requests, request)
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}
