package dump

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

const (
	testAttachmentA = attachment.ID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	testAttachmentB = attachment.ID("att_bbbbbbbbbbbbbbbbbbbbbbbbba")
)

func TestServiceRenderIncludesAssociatedAndReferencedAttachmentsWithoutExpandingIssues(t *testing.T) {
	referencedBody := []byte("referenced content")
	associatedBody := []byte("associated content")
	referenced := testAttachment(t, testAttachmentA, "referenced.txt", referencedBody, "an-other")
	associated := testAttachment(t, testAttachmentB, "associated.txt", associatedBody, "an-a")
	description := fmt.Sprintf(
		"[referenced file](attachment:%s)\n\n`[not a reference](attachment:%s)`",
		referenced.ID,
		testAttachmentB,
	)
	source := &attachmentSource{
		attachments: []attachment.Attachment{referenced, associated},
		content: map[attachment.ID][]byte{
			referenced.ID: referencedBody,
			associated.ID: associatedBody,
		},
	}
	service := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-a", Title: "Selected", Summary: &description},
			{ID: "an-other", Title: "Not selected"},
		},
	}, source)

	rendered, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-a"),
	})
	require.NoError(t, err)

	assert.Equal(t, 1, rendered.IssueCount)
	assert.Equal(t, []string{
		"attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/referenced.txt",
		"attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/metadata.yaml",
		"attachments/att_bbbbbbbbbbbbbbbbbbbbbbbbba/files/associated.txt",
		"attachments/att_bbbbbbbbbbbbbbbbbbbbbbbbba/metadata.yaml",
		"issues/an-a.md",
	}, renderedPaths(rendered.Files))
	issuePage := renderedBody(t, rendered.Files, "issues/an-a.md")
	assert.Contains(t, issuePage,
		"[referenced file](../attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/referenced.txt)")
	assert.Contains(t, issuePage,
		"`[not a reference](attachment:att_bbbbbbbbbbbbbbbbbbbbbbbbba)`")
	assert.Equal(t, fmt.Sprintf(
		"[referenced file](attachment:%s)\n\n`[not a reference](attachment:%s)`",
		referenced.ID,
		testAttachmentB,
	), description, "dump rewriting must not mutate stored Markdown")
	assert.Equal(t, []string{"an-a"}, source.listedIssueIDs())
	assert.Equal(t, [][]attachment.ID{{referenced.ID}}, source.resolveBatches)
	assert.Empty(t, source.opened, "rendering must preserve lazy attachment streaming")

	assert.Equal(t, `format_version: 1
attachment_id: att_aaaaaaaaaaaaaaaaaaaaaaaaaa
digest: `+referenced.Blob.Digest.String()+`
size_bytes: 18
media_type: text/plain
filename: referenced.txt
`, string(renderedFileContent(t, rendered.Files,
		"attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/metadata.yaml")))
}

func TestServiceRenderWholeBoardIncludesEveryActiveAttachment(t *testing.T) {
	boardBody := []byte("board attachment")
	issueBody := []byte("issue attachment")
	boardAttachment := testAttachment(t, testAttachmentA, "board.txt", boardBody, "")
	issueAttachment := testAttachment(t, testAttachmentB, "issue.txt", issueBody, "an-a")
	source := &attachmentSource{
		attachments: []attachment.Attachment{issueAttachment, boardAttachment},
		content: map[attachment.ID][]byte{
			boardAttachment.ID: boardBody,
			issueAttachment.ID: issueBody,
		},
	}
	service := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues:  []Issue{{ID: "an-a", Title: "Selected"}},
	}, source)

	rendered, err := service.Render(t.Context(), RenderRequest{Selection: WholeBoard()})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"README.md",
		"attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/board.txt",
		"attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/metadata.yaml",
		"attachments/att_bbbbbbbbbbbbbbbbbbbbbbbbba/files/issue.txt",
		"attachments/att_bbbbbbbbbbbbbbbbbbbbbbbbba/metadata.yaml",
		"issues/an-a.md",
	}, renderedPaths(rendered.Files))
	require.Len(t, source.listRequests, 1)
	assert.Nil(t, source.listRequests[0].OriginIssueID)
}

func TestServiceRenderRewritesBoardAndIssueAttachmentDestinations(t *testing.T) {
	body := []byte("image content")
	value := testAttachment(t, testAttachmentA, "diagram final #1.png", body, "")
	boardDescription := fmt.Sprintf("![board diagram](attachment:%s)", value.ID)
	issueDescription := fmt.Sprintf("[issue diagram](attachment:%s)", value.ID)
	source := &attachmentSource{
		attachments: []attachment.Attachment{value},
		content:     map[attachment.ID][]byte{value.ID: body},
	}
	service := newAttachmentTestService(t, BoardSnapshot{
		BoardID:     "board-1",
		Description: &boardDescription,
		Issues: []Issue{{
			ID: "an-a", Title: "Selected", Summary: &issueDescription,
		}},
	}, source)

	rendered, err := service.Render(t.Context(), RenderRequest{Selection: WholeBoard()})
	require.NoError(t, err)

	assert.Contains(t, renderedBody(t, rendered.Files, "README.md"),
		"![board diagram](attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/diagram%20final%20%231.png)")
	assert.Contains(t, renderedBody(t, rendered.Files, "issues/an-a.md"),
		"[issue diagram](../attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/diagram%20final%20%231.png)")
	assert.Equal(t, [][]attachment.ID{{value.ID}}, source.resolveBatches)
}

func TestServiceRenderAttachmentShorthandCopiesAndLinksReferencedAttachment(t *testing.T) {
	body := []byte("artifact")
	value := testAttachment(t, testAttachmentA, "artifact final.txt", body, "an-other")
	summary := "Portable attachment: %" + value.ID.String() + "."
	source := &attachmentSource{
		attachments: []attachment.Attachment{value},
		content:     map[attachment.ID][]byte{value.ID: body},
	}
	service := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-selected", Title: "Selected", Summary: &summary},
			{ID: "an-other", Title: "Not selected"},
		},
	}, source)

	rendered, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-selected"),
	})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/artifact final.txt",
		"attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/metadata.yaml",
		"issues/an-selected.md",
	}, renderedPaths(rendered.Files))
	assert.Contains(t, renderedBody(t, rendered.Files, "issues/an-selected.md"),
		"Portable attachment: [artifact final.txt]"+
			"(../attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/artifact%20final.txt).")
	assert.Equal(t, [][]attachment.ID{{value.ID}}, source.resolveBatches)
	assert.Equal(t, []string{"an-selected"}, source.listedIssueIDs())
	assert.Equal(t, "Portable attachment: %"+value.ID.String()+".", summary,
		"dump rewriting must not mutate stored Markdown")
}

func TestServiceRenderUnavailableAttachmentShorthandRemainsReadable(t *testing.T) {
	removed := testAttachment(t, testAttachmentA, "retired.log", []byte("old"), "an-other")
	removed.Lifecycle = attachment.LifecycleRemoved
	removed.Removed = &attachment.Attribution{
		Actor: "captain", At: time.Unix(2, 0), Revision: 2,
	}
	summary := strings.Join([]string{
		"Removed: %" + removed.ID.String() + ".",
		"Unknown: %" + testAttachmentB.String() + ".",
		"Escaped: \\" + "%" + removed.ID.String() + ".",
		"Code: `%" + removed.ID.String() + "`.",
		"Label: [see %" + removed.ID.String() + "](https://example.com).",
		"Alt: ![see %" + removed.ID.String() + "](https://example.com/image.png).",
		"Malformed: %att_not-an-attachment.",
	}, "\n")
	source := &attachmentSource{attachments: []attachment.Attachment{removed}}
	service := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-selected", Title: "Selected", Summary: &summary},
			{ID: "an-other", Title: "Not selected"},
		},
	}, source)

	rendered, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-selected"),
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"issues/an-selected.md"}, renderedPaths(rendered.Files))
	assert.Contains(t, renderedBody(t, rendered.Files, "issues/an-selected.md"),
		"Removed: Attachment unavailable: `retired.log` "+
			"(`att_aaaaaaaaaaaaaaaaaaaaaaaaaa`).\n"+
			"Unknown: %att_bbbbbbbbbbbbbbbbbbbbbbbbba.")
	for _, literal := range []string{
		"\\%att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		"`%att_aaaaaaaaaaaaaaaaaaaaaaaaaa`",
		"[see %att_aaaaaaaaaaaaaaaaaaaaaaaaaa](https://example.com)",
		"![see %att_aaaaaaaaaaaaaaaaaaaaaaaaaa](https://example.com/image.png)",
		"%att_not-an-attachment",
	} {
		assert.Contains(t, renderedBody(t, rendered.Files, "issues/an-selected.md"), literal)
	}
	assert.Equal(t, [][]attachment.ID{{
		testAttachmentA,
		testAttachmentB,
	}}, source.resolveBatches)
}

func TestServiceRenderResolvesEveryIncludedIssueMarkdownRecord(t *testing.T) {
	body := []byte("artifact")
	value := testAttachment(t, testAttachmentA, "artifact.txt", body, "an-a")
	destination := fmt.Sprintf("attachment:%s", value.ID)
	description := "[description](" + destination + ")"
	state := "[state](" + destination + ")"
	source := &attachmentSource{
		attachments: []attachment.Attachment{value},
		content:     map[attachment.ID][]byte{value.ID: body},
	}
	service := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{{
			ID: "an-a", Title: "Selected", Summary: &description, State: &state,
		}},
		Results: []Result{{IssueID: "an-a", Body: "[result](" + destination + ")"}},
		LogEntries: []LogEntry{{
			ID:      "cmt_00000000000000000000000000000001",
			IssueID: "an-a", Author: new("captain"),
			Body: "[logEntry](" + destination + ")",
		}},
	}, source)

	rendered, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-a"),
	})
	require.NoError(t, err)

	page := renderedBody(t, rendered.Files, "issues/an-a.md")
	for _, label := range []string{"description", "state", "result", "logEntry"} {
		assert.Contains(t, page, fmt.Sprintf(
			"[%s](../attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/artifact.txt)", label,
		))
	}
	assert.Equal(t, [][]attachment.ID{{value.ID}}, source.resolveBatches)
}

func TestServiceRenderTurnsRemovedAttachmentReferencesIntoStableText(t *testing.T) {
	value := testAttachment(t, testAttachmentA, "retired.log", []byte("old"), "an-a")
	value.Lifecycle = attachment.LifecycleRemoved
	value.Removed = &attachment.Attribution{
		Actor: "captain", At: time.Unix(2, 0), Revision: 2,
	}
	description := fmt.Sprintf("Before [old log](attachment:%s) after.", value.ID)
	source := &attachmentSource{attachments: []attachment.Attachment{value}}
	service := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{{
			ID: "an-a", Title: "Selected", Summary: &description,
		}},
	}, source)

	rendered, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-a"),
	})
	require.NoError(t, err)

	page := renderedBody(t, rendered.Files, "issues/an-a.md")
	assert.Contains(t, page,
		"Before Attachment unavailable: `retired.log` (`att_aaaaaaaaaaaaaaaaaaaaaaaaaa`) after.")
	assert.NotContains(t, page, "attachment:att_")
	assert.Equal(t, []string{"issues/an-a.md"}, renderedPaths(rendered.Files))
}

func TestServiceRenderTurnsRemovedReferenceLinksIntoStableText(t *testing.T) {
	value := testAttachment(t, testAttachmentA, "retired.log", []byte("old"), "an-a")
	value.Lifecycle = attachment.LifecycleRemoved
	value.Removed = &attachment.Attribution{
		Actor: "captain", At: time.Unix(2, 0), Revision: 2,
	}
	description := fmt.Sprintf(
		"Before [old log][artifact] after.\n\n[artifact]: attachment:%s\n",
		value.ID,
	)
	service := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{{
			ID: "an-a", Title: "Selected", Summary: &description,
		}},
	}, &attachmentSource{attachments: []attachment.Attachment{value}})

	rendered, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-a"),
	})
	require.NoError(t, err)

	page := renderedBody(t, rendered.Files, "issues/an-a.md")
	assert.Contains(t, page,
		"Before Attachment unavailable: `retired.log` (`att_aaaaaaaaaaaaaaaaaaaaaaaaaa`) after.")
	assert.NotContains(t, page, "attachment:att_")
}

func TestPublicationServiceRejectsUnknownReferenceBeforePublication(t *testing.T) {
	description := fmt.Sprintf("[missing](attachment:%s)", testAttachmentA)
	source := &attachmentSource{}
	publisher := &recordingPublisher{}
	renderer := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{{
			ID: "an-a", Title: "Selected", Summary: &description,
		}},
	}, source)
	service, err := NewPublicationService(renderer, publisher)
	require.NoError(t, err)

	_, err = service.Execute(t.Context(), Request{Selection: NamedIssuesOnly("an-a")})
	assert.EqualError(t, err,
		`resolve attachment "att_aaaaaaaaaaaaaaaaaaaaaaaaaa" referenced by issue "an-a" summary: attachment is unknown in board "board-1"`)
	assert.Empty(t, publisher.publications)
}

func TestPublicationServiceAttachmentCorruptionLeavesExistingDumpUntouched(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "Missing", err: attachment.ErrAttachmentContentMissing},
		{name: "SizeInvalid", err: attachment.ErrAttachmentContentSizeMismatch},
		{name: "DigestInvalid", err: attachment.ErrAttachmentContentDigestMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "dump")
			prior := testRenderedDump(t, "board-1", NamedIssuesOnly("an-a"), map[string]renderedFile{
				"issues/an-a.md": {identity: "issue:an-a", body: "prior issue\n"},
			})
			_, err := publishDump(t, Publication{Destination: destination, Rendered: prior})
			require.NoError(t, err)
			before, err := os.ReadFile(filepath.Join(destination, "issues/an-a.md"))
			require.NoError(t, err)

			body := []byte("corruptible")
			value := testAttachment(t, testAttachmentA, "artifact.bin", body, "an-a")
			description := fmt.Sprintf("[artifact](attachment:%s)", value.ID)
			source := &attachmentSource{
				attachments: []attachment.Attachment{value},
				openErrors:  map[attachment.ID]error{value.ID: tt.err},
			}
			renderer := newAttachmentTestService(t, BoardSnapshot{
				BoardID: "board-1",
				Issues: []Issue{{
					ID: "an-a", Title: "Selected", Summary: &description,
				}},
			}, source)
			service, err := NewPublicationService(renderer, &FilePublisher{})
			require.NoError(t, err)

			_, err = service.Execute(t.Context(), Request{
				Destination: destination,
				Selection:   NamedIssuesOnly("an-a"),
			})
			assert.ErrorIs(t, err, tt.err)
			after, readErr := os.ReadFile(filepath.Join(destination, "issues/an-a.md"))
			require.NoError(t, readErr)
			assert.Equal(t, before, after)
			assert.NoDirExists(t, filepath.Join(destination, "attachments"))
		})
	}
}

func TestPublicationServiceDeterministicallyRerunsAndClosesAttachmentContent(t *testing.T) {
	body := bytes.Repeat([]byte("streamed attachment\n"), 1024)
	value := testAttachment(t, testAttachmentA, "artifact.log", body, "an-a")
	description := fmt.Sprintf("[artifact](attachment:%s)", value.ID)
	source := &attachmentSource{
		attachments: []attachment.Attachment{value},
		content:     map[attachment.ID][]byte{value.ID: body},
	}
	renderer := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{{
			ID: "an-a", Title: "Selected", Summary: &description,
		}},
	}, source)
	service, err := NewPublicationService(renderer, &FilePublisher{})
	require.NoError(t, err)
	destination := filepath.Join(t.TempDir(), "dump")
	request := Request{
		Destination: destination,
		Selection:   NamedIssuesOnly("an-a"),
	}

	first, err := service.Execute(t.Context(), request)
	require.NoError(t, err)
	assert.Equal(t, 3, first.Written)
	second, err := service.Execute(t.Context(), request)
	require.NoError(t, err)
	assert.Equal(t, 3, second.Unchanged)
	assert.Equal(t, []attachment.ID{value.ID, value.ID, value.ID, value.ID}, source.opened)
	assert.Equal(t, []attachment.ID{value.ID, value.ID, value.ID, value.ID}, source.closed)
	assert.Equal(t, 1, source.maxOpenHandles)
	assert.Zero(t, source.openHandles)
	assert.Equal(t, body, renderedFileContentFromDisk(t, destination,
		"attachments/att_aaaaaaaaaaaaaaaaaaaaaaaaaa/files/artifact.log"))
}

func TestFilePublisherLimitsConcurrentAttachmentHandles(t *testing.T) {
	firstBody := []byte("first")
	secondBody := []byte("second")
	first := testAttachment(t, testAttachmentA, "first.txt", firstBody, "an-a")
	second := testAttachment(t, testAttachmentB, "second.txt", secondBody, "an-a")
	source := &attachmentSource{
		attachments: []attachment.Attachment{first, second},
		content: map[attachment.ID][]byte{
			first.ID:  firstBody,
			second.ID: secondBody,
		},
	}
	renderer := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1", Issues: []Issue{{ID: "an-a", Title: "Selected"}},
	}, source)
	rendered, err := renderer.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-a"),
	})
	require.NoError(t, err)

	_, err = (&FilePublisher{}).Publish(t.Context(), Publication{
		Destination: filepath.Join(t.TempDir(), "dump"), Rendered: rendered,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, source.maxOpenHandles)
	assert.Zero(t, source.openHandles)
	assert.Equal(t, []attachment.ID{first.ID, second.ID, first.ID, second.ID}, source.opened)
	assert.Equal(t, source.opened, source.closed)
}

func TestFilePublisherVerifiesEveryAttachmentBeforeInspectingDestination(t *testing.T) {
	firstBody := []byte("first")
	secondBody := []byte("second")
	first := testAttachment(t, testAttachmentA, "first.txt", firstBody, "an-a")
	second := testAttachment(t, testAttachmentB, "second.txt", secondBody, "an-a")
	source := &attachmentSource{
		attachments: []attachment.Attachment{first, second},
		content:     map[attachment.ID][]byte{first.ID: firstBody},
		openErrors:  map[attachment.ID]error{second.ID: attachment.ErrAttachmentContentDigestMismatch},
	}
	renderer := newAttachmentTestService(t, BoardSnapshot{
		BoardID: "board-1", Issues: []Issue{{ID: "an-a", Title: "Selected"}},
	}, source)
	rendered, err := renderer.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-a"),
	})
	require.NoError(t, err)
	files := &destinationInspectionRecorder{fileSystem: osFileSystem{}}

	_, err = (&FilePublisher{files: files}).Publish(t.Context(), Publication{
		Destination: filepath.Join(t.TempDir(), "dump"), Rendered: rendered,
	})
	assert.ErrorIs(t, err, attachment.ErrAttachmentContentDigestMismatch)
	assert.Zero(t, files.inspections)
	assert.Equal(t, []attachment.ID{first.ID}, source.closed)
}

type attachmentSource struct {
	attachments    []attachment.Attachment
	content        map[attachment.ID][]byte
	openErrors     map[attachment.ID]error
	listRequests   []attachment.ListRequest
	resolveBatches [][]attachment.ID
	opened         []attachment.ID
	closed         []attachment.ID
	openHandles    int
	maxOpenHandles int
}

func (s *attachmentSource) ListAttachments(
	_ context.Context,
	request attachment.ListRequest,
) (attachment.Page, error) {
	s.listRequests = append(s.listRequests, request)
	var values []attachment.Attachment
	for _, value := range s.attachments {
		if value.Lifecycle != attachment.LifecycleActive {
			continue
		}
		origin, associated := value.Association.OriginIssueID()
		if request.OriginIssueID != nil && (!associated || origin != *request.OriginIssueID) {
			continue
		}
		values = append(values, value)
	}
	return attachment.Page{Attachments: values}, nil
}

func (s *attachmentSource) ResolveAttachments(
	_ context.Context,
	request attachment.ResolveRequest,
) ([]attachment.Resolution, error) {
	s.resolveBatches = append(s.resolveBatches, append([]attachment.ID(nil), request.AttachmentIDs...))
	values := make(map[attachment.ID]attachment.Attachment, len(s.attachments))
	for _, value := range s.attachments {
		values[value.ID] = value
	}
	resolutions := make([]attachment.Resolution, 0, len(request.AttachmentIDs))
	for _, id := range request.AttachmentIDs {
		resolution := attachment.Resolution{
			AttachmentID: id,
			State:        attachment.ResolutionUnknown,
		}
		if value, ok := values[id]; ok {
			resolution.Attachment = &value
			resolution.State = attachment.ResolutionActive
			if value.Lifecycle == attachment.LifecycleRemoved {
				resolution.State = attachment.ResolutionRemoved
			}
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

func (s *attachmentSource) OpenAttachmentContent(
	_ context.Context,
	request attachment.OpenContentRequest,
) (attachment.OpenedContent, error) {
	s.opened = append(s.opened, request.AttachmentID)
	if err := s.openErrors[request.AttachmentID]; err != nil {
		return attachment.OpenedContent{}, err
	}
	for _, value := range s.attachments {
		if value.ID == request.AttachmentID {
			s.openHandles++
			s.maxOpenHandles = max(s.maxOpenHandles, s.openHandles)
			return attachment.OpenedContent{
				Attachment: value,
				Handle: &attachmentContentReader{
					Reader: bytes.NewReader(s.content[value.ID]),
					close: func() {
						s.openHandles--
						s.closed = append(s.closed, value.ID)
					},
				},
			}, nil
		}
	}
	return attachment.OpenedContent{}, attachment.ErrAttachmentNotFound
}

func (s *attachmentSource) listedIssueIDs() []string {
	var ids []string
	for _, request := range s.listRequests {
		if request.OriginIssueID != nil {
			ids = append(ids, request.OriginIssueID.String())
		}
	}
	return ids
}

type attachmentContentReader struct {
	*bytes.Reader
	close func()
}

type destinationInspectionRecorder struct {
	fileSystem
	inspections int
}

func (f *destinationInspectionRecorder) Lstat(path string) (os.FileInfo, error) {
	f.inspections++
	return f.fileSystem.Lstat(path)
}

func (r *attachmentContentReader) Close() error {
	r.close()
	return nil
}

func newAttachmentTestService(
	t *testing.T,
	snapshot BoardSnapshot,
	attachments AttachmentReader,
) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{
		Reader:      staticSnapshotReader{snapshot: snapshot},
		Attachments: attachments,
		Provenance:  testProvenance(snapshot.BoardID),
	})
	require.NoError(t, err)
	return service
}

func testAttachment(
	t *testing.T,
	id attachment.ID,
	filename string,
	body []byte,
	originIssueID string,
) attachment.Attachment {
	t.Helper()
	boardID, err := board.NewID("board-1")
	require.NoError(t, err)
	association, err := attachment.NewBoardAssociation(boardID)
	require.NoError(t, err)
	if originIssueID != "" {
		origin, issueErr := issue.NewID(originIssueID)
		require.NoError(t, issueErr)
		association, err = attachment.NewIssueAssociation(boardID, origin)
		require.NoError(t, err)
	}
	digestValue := sha256.Sum256(body)
	digest, err := attachment.NewDigest(fmt.Sprintf("sha256:%x", digestValue))
	require.NoError(t, err)
	portable, err := attachment.NewFilename(filename)
	require.NoError(t, err)
	mediaType, err := attachment.NewMediaType("text/plain")
	require.NoError(t, err)
	value := attachment.Attachment{
		ID:          id,
		Association: association,
		Blob: attachment.BlobDescriptor{
			Digest: digest, SizeBytes: uint64(len(body)),
		},
		Filename:     portable,
		MediaType:    mediaType,
		Lifecycle:    attachment.LifecycleActive,
		Availability: attachment.BlobAvailabilityPresentUnverified,
		Created: attachment.Attribution{
			Actor: "captain", At: time.Unix(1, 0), Revision: 1,
		},
	}
	require.NoError(t, value.Validate())
	return value
}

func renderedFileContent(t *testing.T, files []*GeneratedFile, path string) []byte {
	t.Helper()
	for _, file := range files {
		if file.Path() == path {
			reader, err := file.Open()
			require.NoError(t, err)
			body, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			require.NoError(t, errors.Join(readErr, closeErr))
			return body
		}
	}
	require.FailNow(t, "rendered file not found", path)
	return nil
}

func renderedFileContentFromDisk(t *testing.T, destination, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(path)))
	require.NoError(t, err)
	return body
}
