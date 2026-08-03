package markdown_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/markdown"
)

func TestRenderer_RenderBoardResolvesAttachmentDestinationsInOneBatch(t *testing.T) {
	imageID := testAttachmentID('a')
	documentID := testAttachmentID('b')
	resolver := &testAttachmentResolver{resolutions: []attachment.Resolution{
		testResolution(imageID, attachment.ResolutionActive, "screenshot.png", "image/png", attachment.BlobAvailabilityVerified),
		testResolution(documentID, attachment.ResolutionActive, "report.pdf", "application/pdf", attachment.BlobAvailabilityPresentUnverified),
	}}
	renderer := markdown.NewWithAttachments(resolver)
	sources := []string{
		"![failure](attachment:" + imageID.String() + ")",
		"![report](attachment:" + documentID.String() + ") and [again](attachment:" + imageID.String() + ")",
		"`[not a reference](attachment:" + documentID.String() + ")`\n\n```markdown\n![also not](attachment:" + imageID.String() + ")\n```",
	}

	rendered, err := renderer.RenderBoard(t.Context(), "board-1", "", sources)
	require.NoError(t, err)

	require.Len(t, resolver.requests, 1)
	assert.Equal(t, board.ID("board-1"), resolver.requests[0].BoardID)
	assert.Equal(t, []attachment.ID{imageID, documentID}, resolver.requests[0].AttachmentIDs)
	require.Len(t, rendered, 3)
	assert.Contains(t, rendered[0], `<img src="/board/board-1/attachment/`+imageID.String()+`" alt="failure">`)
	assert.NotContains(t, rendered[1], "<img")
	assert.Contains(t, rendered[1], `<a href="/board/board-1/attachment/`+documentID.String()+`">report</a>`)
	assert.Contains(t, rendered[1], `<a href="/board/board-1/attachment/`+imageID.String()+`">again</a>`)
	assert.Contains(t, rendered[2], "attachment:"+documentID.String())
	assert.Contains(t, rendered[2], "attachment:"+imageID.String())
	assert.Equal(t, sources, []string{
		"![failure](attachment:" + imageID.String() + ")",
		"![report](attachment:" + documentID.String() + ") and [again](attachment:" + imageID.String() + ")",
		"`[not a reference](attachment:" + documentID.String() + ")`\n\n```markdown\n![also not](attachment:" + imageID.String() + ")\n```",
	})
}

func TestRenderer_RenderBoardMarksUnavailableAttachments(t *testing.T) {
	unknownID := testAttachmentID('a')
	removedID := testAttachmentID('b')
	missingID := testAttachmentID('c')
	sizeID := testAttachmentID('d')
	digestID := testAttachmentID('e')
	resolver := &testAttachmentResolver{resolutions: []attachment.Resolution{
		{AttachmentID: unknownID, State: attachment.ResolutionUnknown},
		testResolution(removedID, attachment.ResolutionRemoved, "removed.png", "image/png", attachment.BlobAvailabilityVerified),
		testResolution(missingID, attachment.ResolutionActive, "missing.png", "image/png", attachment.BlobAvailabilityMissing),
		testResolution(sizeID, attachment.ResolutionActive, "wrong-size.png", "image/png", attachment.BlobAvailabilitySizeMismatch),
		testResolution(digestID, attachment.ResolutionActive, "corrupt.png", "image/png", attachment.BlobAvailabilityDigestMismatch),
	}}
	renderer := markdown.NewWithAttachments(resolver)
	source := strings.Join([]string{
		"[unknown](attachment:" + unknownID.String() + ")",
		"![removed](attachment:" + removedID.String() + ")",
		"![missing](attachment:" + missingID.String() + ")",
		"![wrong size](attachment:" + sizeID.String() + ")",
		"![corrupt](attachment:" + digestID.String() + ")",
		"[malformed](attachment:not-an-id)",
	}, "\n\n")

	rendered, err := renderer.RenderBoard(t.Context(), "board-1", "", []string{source})
	require.NoError(t, err)

	require.Len(t, resolver.requests, 1)
	assert.Equal(t, []attachment.ID{unknownID, removedID, missingID, sizeID, digestID}, resolver.requests[0].AttachmentIDs)
	require.Len(t, rendered, 1)
	for _, label := range []string{"unknown", "removed", "missing", "wrong size", "corrupt", "malformed"} {
		assert.Contains(t, rendered[0], label+" (attachment unavailable)")
	}
	assert.NotContains(t, rendered[0], "<img")
	assert.NotContains(t, rendered[0], "<a href")
}

type testAttachmentResolver struct {
	requests    []attachment.ResolveRequest
	resolutions []attachment.Resolution
}

func (r *testAttachmentResolver) ResolveAttachments(
	_ context.Context,
	request attachment.ResolveRequest,
) ([]attachment.Resolution, error) {
	r.requests = append(r.requests, request)
	return r.resolutions, nil
}

func testAttachmentID(character byte) attachment.ID {
	return attachment.ID("att_" + strings.Repeat(string(character), 25) + "a")
}

func testResolution(
	id attachment.ID,
	state attachment.ResolutionState,
	filename string,
	mediaType string,
	availability attachment.BlobAvailability,
) attachment.Resolution {
	association, err := attachment.NewBoardAssociation("board-1")
	if err != nil {
		panic(err)
	}
	created := attachment.Attribution{
		Actor: "test", At: time.Unix(1, 0).UTC(), Revision: 1,
	}
	lifecycle := attachment.LifecycleActive
	var removed *attachment.Attribution
	if state == attachment.ResolutionRemoved {
		lifecycle = attachment.LifecycleRemoved
		removed = &attachment.Attribution{
			Actor: "test", At: time.Unix(2, 0).UTC(), Revision: 2,
		}
	}
	return attachment.Resolution{
		AttachmentID: id,
		State:        state,
		Attachment: &attachment.Attachment{
			ID: id, Association: association,
			Blob: attachment.BlobDescriptor{
				Digest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SizeBytes: 1,
			},
			Filename: attachment.Filename(filename), MediaType: attachment.MediaType(mediaType),
			Lifecycle: lifecycle, Availability: availability,
			Created: created, Removed: removed,
		},
	}
}
