package markdown_test

import (
	"context"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/markdown"
)

func TestRenderer_RenderBoardResolvesTypedReferencesAcrossSources(t *testing.T) {
	logID, err := issue.NewLogID("log_0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	attachmentID := testAttachmentID('a')
	resolver := &testBoardReferenceResolver{
		issueReferences: []issue.ID{"known-1"},
		logReferences: []issue.LogReference{{
			LogID: logID, IssueID: "owner-1",
		}},
		attachmentResolutions: []attachment.Resolution{
			testResolution(
				attachmentID,
				attachment.ResolutionActive,
				"diagnostic report.pdf",
				"application/pdf",
				attachment.BlobAvailabilityVerified,
			),
		},
	}
	renderer := markdown.NewWithReferences(resolver, resolver, resolver)
	sources := []string{
		"See %known-1, %" + logID.String() + " and %" + attachmentID.String() + ".",
		"Again %" + logID.String() + ", %" + attachmentID.String() +
			", %known-1, and [download](attachment:" + attachmentID.String() + ").",
	}

	rendered, err := renderer.RenderBoard(t.Context(), "board-1", "", sources)
	require.NoError(t, err)

	require.Len(t, resolver.issueRequests, 1)
	assert.Equal(t, board.ID("board-1"), resolver.issueRequests[0].boardID)
	assert.Equal(t, []issue.ID{"known-1"}, resolver.issueRequests[0].ids)
	require.Len(t, resolver.logRequests, 1)
	assert.Equal(t, board.ID("board-1"), resolver.logRequests[0].boardID)
	assert.Equal(t, []issue.LogID{logID}, resolver.logRequests[0].ids)
	require.Len(t, resolver.attachmentRequests, 1)
	assert.Equal(t, []attachment.ID{attachmentID},
		resolver.attachmentRequests[0].AttachmentIDs)
	assert.Contains(t, rendered[0],
		`<a href="/board/board-1/issue/known-1">%known-1</a>`)
	assert.Contains(t, rendered[0],
		`<a href="/board/board-1/issue/owner-1#`+logID.String()+`">%`+logID.String()+`</a>`)
	assert.Contains(t, rendered[0],
		`<a href="/board/board-1/attachment/`+attachmentID.String()+
			`">diagnostic report.pdf</a>`)
	assert.Contains(t, rendered[1],
		`<a href="/board/board-1/attachment/`+attachmentID.String()+
			`">download</a>`)
	assert.Contains(t, rendered[0], `data-cardamom-reference="log"`)
	assert.Contains(t, rendered[0], `data-cardamom-reference="attachment"`)
}

func TestRenderer_RenderBoardUsesRoutePrefixForReferences(t *testing.T) {
	resolver := &testBoardReferenceResolver{
		issueReferences: []issue.ID{"known-1"},
	}
	renderer := markdown.NewWithReferences(resolver, resolver, resolver)

	rendered, err := renderer.RenderBoard(
		t.Context(), "board-1", "/board", []string{"Open %known-1."},
	)
	require.NoError(t, err)
	assert.Contains(t, rendered[0], `href="/board/board-1/issue/known-1"`)
}

func TestRenderer_RenderBoardLeavesUnavailableTypedReferencesAsText(t *testing.T) {
	logID, err := issue.NewLogID("log_0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	attachmentID := testAttachmentID('a')
	resolver := &testBoardReferenceResolver{
		attachmentResolutions: []attachment.Resolution{{
			AttachmentID: attachmentID,
			State:        attachment.ResolutionUnknown,
		}},
	}
	renderer := markdown.NewWithReferences(resolver, resolver, resolver)
	source := "Keep %" + logID.String() + ", %" + attachmentID.String() +
		", %log_0123456789abcdef0123456789abcdeg, and " +
		"%att_aaaaaaaaaaaaaaaaaaaaaaaaa0 readable."

	rendered, err := renderer.RenderBoard(t.Context(), "board-1", "", []string{source})
	require.NoError(t, err)

	assert.Equal(t, "<p>"+source+"</p>\n", rendered[0])
	assert.NotContains(t, rendered[0], "<a ")
	assert.NotContains(t, rendered[0], "data-cardamom-reference")
}

func TestRenderer_RenderBoardEscapesBoardIdentityAsOneRouteSegment(t *testing.T) {
	resolver := &testBoardReferenceResolver{
		issueReferences: []issue.ID{"known-1"},
	}
	renderer := markdown.NewWithReferences(resolver, resolver, resolver)

	rendered, err := renderer.RenderBoard(
		t.Context(),
		"board/one",
		"",
		[]string{"Open %known-1."},
	)
	require.NoError(t, err)

	assert.Contains(
		t,
		rendered[0],
		`<a href="/board/board%2Fone/issue/known-1">%known-1</a>`,
	)
}

func TestRenderer_RenderBoardLeavesUnavailableIssueReferencesAsText(t *testing.T) {
	resolver := &testBoardReferenceResolver{
		issueReferences: []issue.ID{"known-1"},
	}
	renderer := markdown.NewWithReferences(resolver, resolver, resolver)
	source := "Open %known-1; keep %other-board-issue readable."

	rendered, err := renderer.RenderBoard(t.Context(), "board-1", "", []string{source})
	require.NoError(t, err)

	require.Len(t, resolver.issueRequests, 1)
	assert.Equal(t,
		[]issue.ID{"known-1", "other-board-issue"},
		resolver.issueRequests[0].ids,
	)
	assert.Contains(t, rendered[0],
		`<a href="/board/board-1/issue/known-1">%known-1</a>`)
	assert.Contains(t, rendered[0], "; keep %other-board-issue readable.")
	assert.NotContains(t, rendered[0], `/issue/other-board-issue`)
}

func TestRenderer_RenderBoardBoundsIssueReferenceBatches(t *testing.T) {
	resolver := &testBoardReferenceResolver{}
	var source strings.Builder
	for index := range 1001 {
		id := issue.ID(fmt.Sprintf("issue-%04d", index))
		if index > 0 {
			source.WriteByte(' ')
		}
		source.WriteByte('%')
		source.WriteString(id.String())
	}
	renderer := markdown.NewWithReferences(resolver, resolver, resolver)

	_, err := renderer.RenderBoard(t.Context(), "board-1", "", []string{source.String()})
	require.NoError(t, err)

	require.Len(t, resolver.issueRequests, 2)
	assert.Len(t, resolver.issueRequests[0].ids, 1000)
	assert.Len(t, resolver.issueRequests[1].ids, 1)
}

func TestRenderer_RenderBoardBoundsLogReferenceBatches(t *testing.T) {
	resolver := &testBoardReferenceResolver{}
	var source strings.Builder
	for index := range 1001 {
		id, err := issue.NewLogID(fmt.Sprintf("log_%032x", index))
		require.NoError(t, err)
		if index > 0 {
			source.WriteByte(' ')
		}
		source.WriteByte('%')
		source.WriteString(id.String())
		resolver.logReferences = append(resolver.logReferences, issue.LogReference{
			LogID: id, IssueID: "owner-1",
		})
	}
	renderer := markdown.NewWithReferences(resolver, resolver, resolver)

	_, err := renderer.RenderBoard(t.Context(), "board-1", "", []string{source.String()})
	require.NoError(t, err)

	require.Len(t, resolver.logRequests, 2)
	assert.Len(t, resolver.logRequests[0].ids, 1000)
	assert.Len(t, resolver.logRequests[1].ids, 1)
}

func TestRenderer_RenderBoardBoundsAttachmentReferenceBatches(t *testing.T) {
	resolver := &testBoardReferenceResolver{}
	var source strings.Builder
	for index := range 1001 {
		id := indexedAttachmentID(uint64(index))
		if index > 0 {
			source.WriteByte(' ')
		}
		source.WriteByte('%')
		source.WriteString(id.String())
	}
	renderer := markdown.NewWithReferences(resolver, resolver, resolver)

	_, err := renderer.RenderBoard(t.Context(), "board-1", "", []string{source.String()})
	require.NoError(t, err)

	require.Len(t, resolver.attachmentRequests, 2)
	assert.Len(t, resolver.attachmentRequests[0].AttachmentIDs, 1000)
	assert.Len(t, resolver.attachmentRequests[1].AttachmentIDs, 1)
}

type testLogReferenceRequest struct {
	boardID board.ID
	ids     []issue.LogID
}

type testIssueReferenceRequest struct {
	boardID board.ID
	ids     []issue.ID
}

type testBoardReferenceResolver struct {
	issueRequests         []testIssueReferenceRequest
	issueReferences       []issue.ID
	logRequests           []testLogReferenceRequest
	logReferences         []issue.LogReference
	attachmentRequests    []attachment.ResolveRequest
	attachmentResolutions []attachment.Resolution
}

func (r *testBoardReferenceResolver) ResolveIssueReferences(
	_ context.Context,
	boardID board.ID,
	ids []issue.ID,
) ([]issue.ID, error) {
	r.issueRequests = append(r.issueRequests, testIssueReferenceRequest{
		boardID: boardID,
		ids:     append([]issue.ID(nil), ids...),
	})
	references := make(map[issue.ID]struct{}, len(r.issueReferences))
	for _, reference := range r.issueReferences {
		references[reference] = struct{}{}
	}
	result := make([]issue.ID, 0, len(ids))
	for _, id := range ids {
		if _, ok := references[id]; ok {
			result = append(result, id)
		}
	}
	return result, nil
}

func (r *testBoardReferenceResolver) ResolveLogReferences(
	_ context.Context,
	boardID board.ID,
	ids []issue.LogID,
) ([]issue.LogReference, error) {
	r.logRequests = append(r.logRequests, testLogReferenceRequest{
		boardID: boardID,
		ids:     append([]issue.LogID(nil), ids...),
	})
	references := make(map[issue.LogID]issue.LogReference, len(r.logReferences))
	for _, reference := range r.logReferences {
		references[reference.LogID] = reference
	}
	result := make([]issue.LogReference, 0, len(ids))
	for _, id := range ids {
		if reference, ok := references[id]; ok {
			result = append(result, reference)
		}
	}
	return result, nil
}

func (r *testBoardReferenceResolver) ResolveAttachments(
	_ context.Context,
	request attachment.ResolveRequest,
) ([]attachment.Resolution, error) {
	r.attachmentRequests = append(r.attachmentRequests, request)
	resolutions := make(
		map[attachment.ID]attachment.Resolution,
		len(r.attachmentResolutions),
	)
	for _, resolution := range r.attachmentResolutions {
		resolutions[resolution.AttachmentID] = resolution
	}
	result := make([]attachment.Resolution, 0, len(request.AttachmentIDs))
	for _, id := range request.AttachmentIDs {
		if resolution, ok := resolutions[id]; ok {
			result = append(result, resolution)
			continue
		}
		result = append(result, attachment.Resolution{
			AttachmentID: id,
			State:        attachment.ResolutionUnknown,
		})
	}
	return result, nil
}

func indexedAttachmentID(index uint64) attachment.ID {
	var body [16]byte
	binary.BigEndian.PutUint64(body[8:], index)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString(body[:])
	id, err := attachment.NewID("att_" + strings.ToLower(encoded))
	if err != nil {
		panic(err)
	}
	return id
}
