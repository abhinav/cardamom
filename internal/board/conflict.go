package board

import "fmt"

// Field identifies a board setting whose current value can be unresolved.
type Field uint8

const (
	_ Field = iota
	// FieldName identifies the display name.
	FieldName
	// FieldDescription identifies shared Markdown context.
	FieldDescription
	// FieldArchive identifies the complete logical archive state.
	FieldArchive
	// FieldIssueIDPrefix identifies the board's issue prefix override.
	FieldIssueIDPrefix
	// FieldIssueIDStrategy identifies the board's issue allocation override.
	FieldIssueIDStrategy
	// FieldIssueSummaryMaxBytes identifies the summary limit override.
	FieldIssueSummaryMaxBytes
	// FieldAttachmentMaxBytes identifies the attachment limit override.
	FieldAttachmentMaxBytes
	// FieldPinsMaxCount identifies the pin limit override.
	FieldPinsMaxCount
)

// String returns the field name used by board conflict inspection.
func (f Field) String() string {
	switch f {
	case FieldName:
		return "name"
	case FieldDescription:
		return "description"
	case FieldArchive:
		return "archive"
	case FieldIssueIDPrefix:
		return "issue.id.prefix"
	case FieldIssueIDStrategy:
		return "issue.id.strategy"
	case FieldIssueSummaryMaxBytes:
		return "issue.summary.max_bytes"
	case FieldAttachmentMaxBytes:
		return "attachment.max_bytes"
	case FieldPinsMaxCount:
		return "board.pins.max_count"
	default:
		return ""
	}
}

// ConflictReference identifies the dispute that prevents one board setting
// from having an authoritative value. Inspection loads its alternatives
// separately; board catalogs retain only these compact references.
type ConflictReference struct {
	// ID identifies the retained contender set.
	ID string `json:"id"` // required
	// Field identifies the affected setting.
	Field Field `json:"field"` // required
}

// MarshalText encodes the stable field name for structured conflict references.
func (f Field) MarshalText() ([]byte, error) {
	name := f.String()
	if name == "" {
		return nil, fmt.Errorf("unknown conflict field %d", f)
	}
	return []byte(name), nil
}
