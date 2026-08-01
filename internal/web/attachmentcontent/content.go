// Package attachmentcontent serves verified attachment bytes over raw HTTP.
package attachmentcontent

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"time"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/must"
)

// PathPattern is the canonical raw attachment route mounted before browser
// application fallbacks.
const PathPattern = "/board/{boardID}/attachment/{attachmentID}"

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
}

// Handler serves the one non-Connect browser content resource.
type Handler struct {
	attachments Attachments
	authorizer  Authorizer
}

var _ http.Handler = (*Handler)(nil)

// New constructs the raw attachment content handler.
func New(cfg Config) *Handler {
	must.NotBeNilf(cfg.Attachments, "attachmentcontent: attachments are required")
	must.NotBeNilf(cfg.Authorizer, "attachmentcontent: authorizer is required")
	return &Handler{attachments: cfg.Attachments, authorizer: cfg.Authorizer}
}

// ServeHTTP validates the raw route identity, authorizes it, and delegates HTTP
// content semantics to net/http over the verified seekable handle.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	boardID, err := board.NewID(r.PathValue("boardID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	attachmentID, err := attachment.NewID(r.PathValue("attachmentID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
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
