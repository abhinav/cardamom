// Package attachmentcontent serves verified attachment bytes over raw HTTP.
package attachmentcontent

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/must"
)

// PathPrefix is the raw attachment content route prefix mounted before browser
// application fallbacks.
const PathPrefix = "/attachments/"

// Attachments opens verified immutable attachment content.
type Attachments interface {
	// OpenAttachmentContent returns active metadata and a caller-owned handle.
	OpenAttachmentContent(context.Context, attachment.OpenContentRequest) (attachment.OpenedContent, error)
}

// Authorizer is the insertion point for attachment content access policy.
type Authorizer interface {
	// AuthorizeAttachmentContent authorizes one parsed board and attachment
	// identity before content is opened.
	AuthorizeAttachmentContent(context.Context, board.ID, attachment.ID) error
}

// Config supplies the raw attachment content boundary.
type Config struct {
	// Attachments opens fully verified content without exposing storage paths.
	Attachments Attachments // required

	// Authorizer approves access before Attachments is called.
	Authorizer Authorizer // required

	// DefaultBoardID scopes path-only requests to the board selected when the
	// server started. It is nil when path-only requests have no board scope.
	// A board_id query selects another board in the same store.
	DefaultBoardID *board.ID
}

// Handler serves the one non-Connect browser content resource.
type Handler struct {
	attachments    Attachments
	authorizer     Authorizer
	defaultBoardID *board.ID
}

var _ http.Handler = (*Handler)(nil)

// New constructs the raw attachment content handler.
func New(cfg Config) *Handler {
	must.NotBeNilf(cfg.Attachments, "attachmentcontent: attachments are required")
	must.NotBeNilf(cfg.Authorizer, "attachmentcontent: authorizer is required")
	var defaultBoardID *board.ID
	if cfg.DefaultBoardID != nil {
		value := *cfg.DefaultBoardID
		defaultBoardID = &value
	}
	return &Handler{
		attachments: cfg.Attachments, authorizer: cfg.Authorizer,
		defaultBoardID: defaultBoardID,
	}
}

// ServeHTTP validates the raw route identity, authorizes it, and delegates HTTP
// content semantics to net/http over the verified seekable handle.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	attachmentID, ok := routeAttachmentID(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var boardID board.ID
	if value := r.URL.Query().Get("board_id"); value != "" {
		parsed, err := board.NewID(value)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		boardID = parsed
	} else if h.defaultBoardID == nil {
		http.NotFound(w, r)
		return
	} else {
		boardID = *h.defaultBoardID
	}
	if _, err := board.NewID(boardID.String()); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.authorizer.AuthorizeAttachmentContent(
		r.Context(),
		boardID,
		attachmentID,
	); err != nil {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	opened, err := h.attachments.OpenAttachmentContent(r.Context(), attachment.OpenContentRequest{
		BoardID: boardID, AttachmentID: attachmentID,
	})
	if err != nil {
		status := attachmentContentErrorStatus(err)
		http.Error(w, http.StatusText(status), status)
		return
	}
	defer func() { _ = opened.Handle.Close() }()

	value := opened.Attachment
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", `"`+value.Blob.Digest.String()+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", value.MediaType.String())
	w.Header().Set(
		"Content-Disposition",
		contentDisposition(value.Filename, attachment.IsInlineMediaType(value.MediaType)),
	)
	http.ServeContent(w, r, value.Filename.String(), time.Time{}, opened.Handle)
}

func routeAttachmentID(requestPath string) (attachment.ID, bool) {
	if !strings.HasPrefix(requestPath, PathPrefix) ||
		!strings.HasSuffix(requestPath, "/content") {
		return "", false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(requestPath, PathPrefix), "/content")
	if value == "" || strings.ContainsRune(value, '/') {
		return "", false
	}
	id, err := attachment.NewID(value)
	return id, err == nil
}

func attachmentContentErrorStatus(err error) int {
	switch {
	case errors.Is(err, attachment.ErrAttachmentNotFound):
		return http.StatusNotFound
	case errors.Is(err, attachment.ErrAttachmentRemoved):
		return http.StatusGone
	case errors.Is(err, attachment.ErrAttachmentContentMissing),
		errors.Is(err, attachment.ErrAttachmentContentSizeMismatch),
		errors.Is(err, attachment.ErrAttachmentContentDigestMismatch):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func contentDisposition(filename attachment.Filename, inline bool) string {
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	value := filename.String()
	if _, err := attachment.NewFilename(value); err != nil {
		value = "attachment"
	}
	return mime.FormatMediaType(disposition, map[string]string{"filename": value})
}
