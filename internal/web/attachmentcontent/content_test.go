package attachmentcontent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.uber.org/mock/gomock"
)

const (
	testContentID = attachment.ID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	testDigest    = attachment.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
)

func TestHandlerServesRangesConditionalsAndHead(t *testing.T) {
	opener := &testContentOpener{content: []byte("0123456789")}
	authorizer := NewMockAuthorizer(gomock.NewController(t))
	authorizer.EXPECT().AuthorizeAttachmentContent(
		gomock.Any(), board.ID("board-1"), testContentID,
	).Return(nil).Times(5)
	handler := testContentHandler(opener, authorizer)

	response := serveContentRequest(t, handler, http.MethodGet, contentURL(""), map[string]string{
		"Range": "bytes=2-5",
	})
	assert.Equal(t, http.StatusPartialContent, response.Code)
	assert.Equal(t, "2345", response.Body.String())
	assert.Equal(t, "bytes 2-5/10", response.Header().Get("Content-Range"))
	assert.Equal(t, `"`+testDigest.String()+`"`, response.Header().Get("ETag"))
	assert.Equal(t, "private, no-cache", response.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))

	response = serveContentRequest(t, handler, http.MethodGet, contentURL(""), map[string]string{
		"If-None-Match": `"` + testDigest.String() + `"`,
	})
	assert.Equal(t, http.StatusNotModified, response.Code)
	assert.Empty(t, response.Body.String())

	response = serveContentRequest(t, handler, http.MethodGet, contentURL(""), map[string]string{
		"Range":    "bytes=2-5",
		"If-Range": `"` + testDigest.String() + `"`,
	})
	assert.Equal(t, http.StatusPartialContent, response.Code)
	assert.Equal(t, "2345", response.Body.String())

	response = serveContentRequest(t, handler, http.MethodGet, contentURL(""), map[string]string{
		"Range":    "bytes=2-5",
		"If-Range": `"different"`,
	})
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "0123456789", response.Body.String())

	response = serveContentRequest(t, handler, http.MethodHead, contentURL(""), nil)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, response.Body.String())
	assert.Equal(t, "10", response.Header().Get("Content-Length"))
	assert.Equal(t, 5, opener.opens)
	assert.Equal(t, 5, opener.closes)
}

func TestHandlerUsesSafeContentDisposition(t *testing.T) {
	tests := []struct {
		name            string
		filename        attachment.Filename
		mediaType       attachment.MediaType
		wantDisposition string
	}{
		{
			name: "RasterImage", filename: "screenshot.png", mediaType: "image/png",
			wantDisposition: "inline",
		},
		{
			name: "Download", filename: "r\u00e9sum\u00e9.pdf", mediaType: "application/pdf",
			wantDisposition: "attachment",
		},
		{
			name: "HostileMetadata", filename: "report\r\nX-Evil: injected.pdf", mediaType: "application/pdf",
			wantDisposition: "attachment",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opener := &testContentOpener{
				content: []byte("content"), filename: tt.filename, mediaType: tt.mediaType,
			}
			authorizer := NewMockAuthorizer(gomock.NewController(t))
			authorizer.EXPECT().AuthorizeAttachmentContent(
				gomock.Any(), board.ID("board-2"), testContentID,
			).Return(nil)
			response := serveContentRequest(
				t,
				testContentHandler(opener, authorizer),
				http.MethodGet,
				contentURL("board-2"),
				nil,
			)

			assert.Equal(t, http.StatusOK, response.Code)
			disposition, parameters, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantDisposition, disposition)
			assert.NotContains(t, response.Header().Get("Content-Disposition"), "\r")
			assert.NotContains(t, response.Header().Get("Content-Disposition"), "\n")
			if tt.name == "Download" {
				assert.Equal(t, "r\u00e9sum\u00e9.pdf", parameters["filename"])
			}
			if tt.name == "HostileMetadata" {
				assert.Equal(t, "attachment", parameters["filename"])
			}
			assert.Equal(t, board.ID("board-2"), opener.lastRequest.BoardID)
		})
	}
}

func TestHandlerMapsUnavailableContent(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "Unknown", err: attachment.ErrAttachmentNotFound, wantStatus: http.StatusNotFound},
		{name: "Removed", err: attachment.ErrAttachmentRemoved, wantStatus: http.StatusGone},
		{name: "Missing", err: attachment.ErrAttachmentContentMissing, wantStatus: http.StatusConflict},
		{name: "SizeMismatch", err: attachment.ErrAttachmentContentSizeMismatch, wantStatus: http.StatusConflict},
		{name: "DigestMismatch", err: attachment.ErrAttachmentContentDigestMismatch, wantStatus: http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewMockAuthorizer(gomock.NewController(t))
			authorizer.EXPECT().AuthorizeAttachmentContent(
				gomock.Any(), board.ID("board-1"), testContentID,
			).Return(nil)
			response := serveContentRequest(
				t,
				testContentHandler(&testContentOpener{err: tt.err}, authorizer),
				http.MethodGet,
				contentURL(""),
				nil,
			)
			assert.Equal(t, tt.wantStatus, response.Code)
		})
	}
}

func TestHandlerAuthorizesIdentityBeforeOpeningContent(t *testing.T) {
	authorizationErr := errors.New("access denied")
	authorizer := NewMockAuthorizer(gomock.NewController(t))
	authorizer.EXPECT().AuthorizeAttachmentContent(
		gomock.Any(), board.ID("board-2"), testContentID,
	).Return(authorizationErr)
	opener := &testContentOpener{content: []byte("secret")}

	response := serveContentRequest(
		t,
		testContentHandler(opener, authorizer),
		http.MethodGet,
		contentURL("board-2"),
		nil,
	)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Zero(t, opener.opens)
}

func TestHandlerUsesCanonicalRouteBoardScope(t *testing.T) {
	opener := &testContentOpener{content: []byte("content")}
	authorizer := NewMockAuthorizer(gomock.NewController(t))
	authorizer.EXPECT().AuthorizeAttachmentContent(
		gomock.Any(), board.ID("board-2"), testContentID,
	).Return(nil)
	authorizer.EXPECT().AuthorizeAttachmentContent(
		gomock.Any(), board.ID("board/three"), testContentID,
	).Return(nil)
	handler := testContentHandler(opener, authorizer)

	response := serveContentRequest(
		t,
		handler,
		http.MethodGet,
		"/board/board-2/attachment/"+testContentID.String(),
		nil,
	)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, board.ID("board-2"), opener.lastRequest.BoardID)

	response = serveContentRequest(
		t,
		handler,
		http.MethodGet,
		"/board/board%2Fthree/attachment/"+testContentID.String(),
		nil,
	)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, board.ID("board/three"), opener.lastRequest.BoardID)
}

func TestHandlerRejectsPathsAndMethodsOutsideRawRoute(t *testing.T) {
	opener := &testContentOpener{content: []byte("secret")}
	handler := testContentHandler(opener, NewMockAuthorizer(gomock.NewController(t)))
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "FilenameSuffix", method: http.MethodGet, path: contentURL("") + "/report.txt", wantStatus: http.StatusNotFound},
		{name: "InvalidBoard", method: http.MethodGet, path: "/board/%20/attachment/" + testContentID.String(), wantStatus: http.StatusNotFound},
		{name: "InvalidID", method: http.MethodGet, path: "/board/board-1/attachment/not-an-id", wantStatus: http.StatusNotFound},
		{name: "Mutation", method: http.MethodPost, path: contentURL(""), wantStatus: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := serveContentRequest(t, handler, tt.method, tt.path, nil)
			assert.Equal(t, tt.wantStatus, response.Code)
		})
	}
	assert.Zero(t, opener.opens)
}

func TestHandlerIgnoresLegacyQueryBoardScope(t *testing.T) {
	opener := &testContentOpener{content: []byte("content")}
	authorizer := NewMockAuthorizer(gomock.NewController(t))
	authorizer.EXPECT().AuthorizeAttachmentContent(
		gomock.Any(), board.ID("board-1"), testContentID,
	).Return(nil)
	handler := testContentHandler(opener, authorizer)

	response := serveContentRequest(
		t,
		handler,
		http.MethodGet,
		contentURL("board-1")+"?board_id=board-2",
		nil,
	)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, board.ID("board-1"), opener.lastRequest.BoardID)
	assert.Equal(t, 1, opener.opens)
}

func testContentHandler(opener Attachments, authorizer Authorizer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(PathPattern, New(Config{
		Attachments: opener,
		Authorizer:  authorizer,
	}))
	return mux
}

func contentURL(boardID string) string {
	if boardID == "" {
		boardID = "board-1"
	}
	return "/board/" + boardID + "/attachment/" + testContentID.String()
}

func serveContentRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type testContentOpener struct {
	content     []byte
	filename    attachment.Filename
	mediaType   attachment.MediaType
	err         error
	opens       int
	closes      int
	lastRequest attachment.OpenContentRequest
}

func (o *testContentOpener) OpenAttachmentContent(
	_ context.Context,
	request attachment.OpenContentRequest,
) (attachment.OpenedContent, error) {
	o.opens++
	o.lastRequest = request
	if o.err != nil {
		return attachment.OpenedContent{}, o.err
	}
	filename := o.filename
	if filename == "" {
		filename = "screenshot.png"
	}
	mediaType := o.mediaType
	if mediaType == "" {
		mediaType = "image/png"
	}
	association, err := attachment.NewBoardAssociation(request.BoardID)
	if err != nil {
		return attachment.OpenedContent{}, err
	}
	return attachment.OpenedContent{
		Attachment: attachment.Attachment{
			ID: testContentID, Association: association,
			Blob: attachment.BlobDescriptor{
				Digest: testDigest, SizeBytes: uint64(len(o.content)),
			},
			Filename: filename, MediaType: mediaType,
			Lifecycle:    attachment.LifecycleActive,
			Availability: attachment.BlobAvailabilityVerified,
			Created: attachment.Attribution{
				Actor: "test", At: time.Unix(1, 0).UTC(), Revision: 1,
			},
		},
		Handle: &testContentHandle{
			Reader: bytes.NewReader(o.content),
			close:  func() { o.closes++ },
		},
	}, nil
}

type testContentHandle struct {
	*bytes.Reader
	close func()
}

func (h *testContentHandle) Close() error {
	h.close()
	return nil
}

var _ io.ReadSeeker = (*testContentHandle)(nil)
