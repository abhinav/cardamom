package boardhistory

import (
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go.abhg.dev/cardamom/internal/issue"
)

// AttachmentAdd records immutable presentation and CAS metadata.
type AttachmentAdd struct {
	// ID is att_ followed by the canonical lowercase, unpadded base32
	// encoding of 16 bytes.
	ID string // required

	// Digest is sha256: followed by 64 lowercase hexadecimal digits
	// identifying the blob.
	Digest string // required

	// SizeBytes is the nonnegative length of the blob.
	SizeBytes int64 // required

	// Filename is a nonempty UTF-8 basename of at most 255 bytes.
	// It cannot contain control characters or <>:"/\|?*, end in a dot or space,
	// or use a Windows device name (CON, PRN, AUX, NUL, COM1–COM9, or LPT1–LPT9).
	Filename string // required

	// MediaType is a MIME type with parameters in the canonical form
	// returned by [mime.FormatMediaType].
	MediaType string // required

	// Actor is the nonblank identity that added the attachment.
	Actor string // required

	// At is the nonzero time the attachment was added.
	At time.Time // required

	// OriginIssueUID identifies the issue that introduced the attachment.
	// It is nil for board-scoped attachments.
	OriginIssueUID *issue.UID
}

// EventAttachmentAdd records attachment metadata and its blob reference.
const EventAttachmentAdd EventKind = "attachment.add"

func (*AttachmentAdd) kind() EventKind { return EventAttachmentAdd }

func (a *AttachmentAdd) validateV1() error {
	if err := validateAttachmentIDV1(a.ID); err != nil {
		return err
	}
	if err := validateAttachmentDigestV1(a.Digest); err != nil {
		return err
	}
	if err := validateAttachmentSizeV1(a.SizeBytes); err != nil {
		return err
	}
	if err := validateAttachmentFilenameV1(a.Filename); err != nil {
		return err
	}
	if err := validateAttachmentMediaTypeV1(a.MediaType); err != nil {
		return errors.New("attachment media type must be canonical")
	}
	if strings.TrimSpace(a.Actor) == "" || a.At.IsZero() {
		return errors.New("attachment add requires actor and time")
	}
	if a.OriginIssueUID != nil {
		return requireIssue(*a.OriginIssueUID)
	}
	return nil
}

func (a *AttachmentAdd) valueV1() any {
	var origin *[]byte
	if a.OriginIssueUID != nil {
		origin = new(a.OriginIssueUID.Bytes())
	}
	return attachmentAddV1{
		ID: a.ID, OriginIssueUID: origin, Digest: a.Digest,
		SizeBytes: a.SizeBytes, Filename: a.Filename, MediaType: a.MediaType,
		Actor: a.Actor, At: a.At.UTC(),
	}
}

type attachmentAddV1 struct {
	ID             string    `json:"id"`
	OriginIssueUID *[]byte   `json:"origin_issue_uid"`
	Digest         string    `json:"digest"`
	SizeBytes      int64     `json:"size_bytes"`
	Filename       string    `json:"filename"`
	MediaType      string    `json:"media_type"`
	Actor          string    `json:"actor"`
	At             time.Time `json:"at"`
}

func decodeAttachmentAddV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[attachmentAddV1](encoded)
	if err != nil {
		return nil, err
	}
	var origin *issue.UID
	if value.OriginIssueUID != nil {
		uid, err := issue.ParseUID(*value.OriginIssueUID)
		if err != nil {
			return nil, err
		}
		origin = &uid
	}
	return &AttachmentAdd{
		ID: value.ID, OriginIssueUID: origin, Digest: value.Digest,
		SizeBytes: value.SizeBytes, Filename: value.Filename,
		MediaType: value.MediaType, Actor: value.Actor, At: value.At,
	}, nil
}

// AttachmentRemove records one monotonic attachment tombstone.
type AttachmentRemove struct {
	// ID follows the retained spelling rules of AttachmentAdd.ID.
	ID string // required

	// Actor is the nonblank identity that removed the attachment.
	Actor string // required

	// At is the nonzero time the attachment was removed.
	At time.Time // required
}

// EventAttachmentRemove records an attachment tombstone.
const EventAttachmentRemove EventKind = "attachment.remove"

func (*AttachmentRemove) kind() EventKind { return EventAttachmentRemove }

func (r *AttachmentRemove) validateV1() error {
	if err := validateAttachmentIDV1(r.ID); err != nil {
		return err
	}
	if strings.TrimSpace(r.Actor) == "" || r.At.IsZero() {
		return errors.New("attachment removal requires identity, actor, and time")
	}
	return nil
}

func (r *AttachmentRemove) valueV1() any {
	return attachmentRemoveV1{ID: r.ID, Actor: r.Actor, At: r.At.UTC()}
}

type attachmentRemoveV1 struct {
	ID    string    `json:"id"`
	Actor string    `json:"actor"`
	At    time.Time `json:"at"`
}

func decodeAttachmentRemoveV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[attachmentRemoveV1](encoded)
	if err != nil {
		return nil, err
	}
	return &AttachmentRemove{ID: value.ID, Actor: value.Actor, At: value.At}, nil
}

func validateAttachmentIDV1(value string) error {
	encoded, ok := strings.CutPrefix(value, "att_")
	if !ok || len(encoded) != 26 || encoded != strings.ToLower(encoded) {
		return fmt.Errorf("invalid V1 attachment ID %q", value)
	}
	encoding := base32.StdEncoding.WithPadding(base32.NoPadding)
	decoded, err := encoding.DecodeString(strings.ToUpper(encoded))
	if err != nil || len(decoded) != 16 ||
		strings.ToLower(encoding.EncodeToString(decoded)) != encoded {
		return fmt.Errorf("invalid V1 attachment ID %q", value)
	}
	return nil
}

func validateAttachmentDigestV1(value string) error {
	encoded, ok := strings.CutPrefix(value, "sha256:")
	if !ok || len(encoded) != 64 || encoded != strings.ToLower(encoded) {
		return fmt.Errorf("invalid V1 attachment digest %q", value)
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return fmt.Errorf("invalid V1 attachment digest %q", value)
	}
	return nil
}

func validateAttachmentSizeV1(value int64) error {
	if value < 0 {
		return fmt.Errorf("invalid V1 attachment size %d", value)
	}
	return nil
}

func validateAttachmentFilenameV1(value string) error {
	if value == "" || !utf8.ValidString(value) || len(value) > 255 ||
		value == "." || value == ".." || strings.HasSuffix(value, ".") ||
		strings.HasSuffix(value, " ") {
		return fmt.Errorf("invalid V1 attachment filename %q", value)
	}
	for _, character := range value {
		if unicode.IsControl(character) || strings.ContainsRune(`<>:"/\|?*`, character) {
			return fmt.Errorf("invalid V1 attachment filename %q", value)
		}
	}
	base, _, _ := strings.Cut(value, ".")
	if isWindowsDeviceNameV1(strings.ToUpper(base)) {
		return fmt.Errorf("invalid V1 attachment filename %q", value)
	}
	return nil
}

func validateAttachmentMediaTypeV1(value string) error {
	base, parameters, err := mime.ParseMediaType(value)
	if err != nil || base == "" || mime.FormatMediaType(base, parameters) != value {
		return fmt.Errorf("invalid V1 attachment media type %q", value)
	}
	return nil
}

func isWindowsDeviceNameV1(value string) bool {
	switch value {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(value) == 4 &&
		(value[:3] == "COM" || value[:3] == "LPT") &&
		value[3] >= '1' && value[3] <= '9'
}
