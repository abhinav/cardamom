package boardhistory

import (
	"errors"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/board"
)

// BoardOrigin records shared board fields at creation or baseline capture.
// The mutation Header supplies board identity. Configuration fields retain
// board overrides; inherited effective values are not part of this event.
type BoardOrigin struct {
	// CreatedAt is the nonzero board creation time.
	CreatedAt time.Time // required

	// Name is the nonblank stored board name.
	Name string // required

	// Description is nil when absent, or contains nonblank text.
	Description *string

	// Archive is nil when the board is active.
	// An archive requires a nonblank Actor and nonzero At.
	Archive *board.Archive

	// IssueIDPrefix follows the override rules of BoardIssueIDPrefixSet.Prefix.
	IssueIDPrefix *string

	// IssueIDStrategy follows the override rules of BoardIssueIDStrategySet.Strategy.
	IssueIDStrategy *string

	// SummaryMaxBytes follows the override rules of BoardSummaryLimitSet.Limit.
	SummaryMaxBytes *int64

	// AttachmentMaxBytes follows the override rules of BoardAttachmentLimitSet.Limit.
	AttachmentMaxBytes *int64

	// PinsMaxCount follows the override rules of BoardPinLimitSet.Limit.
	PinsMaxCount *int64
}

// EventBoardOrigin records the initial or baseline board projection.
const EventBoardOrigin EventKind = "board.origin"

func (*BoardOrigin) kind() EventKind { return EventBoardOrigin }

func (o *BoardOrigin) validateV1() error {
	if o.CreatedAt.IsZero() || strings.TrimSpace(o.Name) == "" {
		return errors.New("board origin requires creation time and name")
	}
	if err := validateArchive(o.Archive); err != nil {
		return err
	}
	if o.Description != nil && strings.TrimSpace(*o.Description) == "" {
		return errors.New("board origin description must not be blank")
	}
	if err := validateOptionalPrefix(o.IssueIDPrefix); err != nil {
		return err
	}
	if err := validateOptionalStrategy(o.IssueIDStrategy); err != nil {
		return err
	}
	if err := validateOptionalByteLimit(o.SummaryMaxBytes); err != nil {
		return err
	}
	if err := validateOptionalByteLimit(o.AttachmentMaxBytes); err != nil {
		return err
	}
	return validateOptionalPinLimit(o.PinsMaxCount)
}

func (o *BoardOrigin) valueV1() any {
	return boardOriginV1{
		CreatedAt:          o.CreatedAt.UTC(),
		Name:               o.Name,
		Description:        o.Description,
		Archive:            archiveToV1(o.Archive),
		IssueIDPrefix:      o.IssueIDPrefix,
		IssueIDStrategy:    o.IssueIDStrategy,
		SummaryMaxBytes:    o.SummaryMaxBytes,
		AttachmentMaxBytes: o.AttachmentMaxBytes,
		PinsMaxCount:       o.PinsMaxCount,
	}
}

type boardOriginV1 struct {
	CreatedAt          time.Time  `json:"created_at"`
	Name               string     `json:"name"`
	Description        *string    `json:"description"`
	Archive            *archiveV1 `json:"archive"`
	IssueIDPrefix      *string    `json:"issue_id_prefix"`
	IssueIDStrategy    *string    `json:"issue_id_strategy"`
	SummaryMaxBytes    *int64     `json:"summary_max_bytes"`
	AttachmentMaxBytes *int64     `json:"attachment_max_bytes"`
	PinsMaxCount       *int64     `json:"pins_max_count"`
}

func decodeBoardOriginV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardOriginV1](encoded)
	if err != nil {
		return nil, err
	}
	return &BoardOrigin{
		CreatedAt:          value.CreatedAt,
		Name:               value.Name,
		Description:        value.Description,
		Archive:            value.Archive.toArchive(),
		IssueIDPrefix:      value.IssueIDPrefix,
		IssueIDStrategy:    value.IssueIDStrategy,
		SummaryMaxBytes:    value.SummaryMaxBytes,
		AttachmentMaxBytes: value.AttachmentMaxBytes,
		PinsMaxCount:       value.PinsMaxCount,
	}, nil
}

// BoardNameSet assigns the board name without assigning other board fields.
type BoardNameSet struct {
	// Name is the nonblank replacement board name.
	Name string // required
}

// EventBoardNameSet assigns the board name.
const EventBoardNameSet EventKind = "board.name.set"

func (*BoardNameSet) kind() EventKind { return EventBoardNameSet }

func (s *BoardNameSet) validateV1() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("board name assignment requires a name")
	}
	return nil
}

func (s *BoardNameSet) valueV1() any { return boardNameSetV1{Name: s.Name} }

type boardNameSetV1 struct {
	Name string `json:"name"`
}

func decodeBoardNameSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardNameSetV1](encoded)
	if err != nil {
		return nil, err
	}
	return &BoardNameSet{Name: value.Name}, nil
}

// BoardDescriptionSet assigns or clears the board description.
type BoardDescriptionSet struct {
	// Description is nil to clear the board description, or contains nonblank text.
	Description *string
}

// EventBoardDescriptionSet assigns or clears the board description.
const EventBoardDescriptionSet EventKind = "board.description.set"

func (*BoardDescriptionSet) kind() EventKind { return EventBoardDescriptionSet }

func (s *BoardDescriptionSet) validateV1() error {
	if s.Description != nil && strings.TrimSpace(*s.Description) == "" {
		return errors.New("board description assignment must not be blank")
	}
	return nil
}

func (s *BoardDescriptionSet) valueV1() any {
	return boardDescriptionSetV1{Description: optionalTextToV1(s.Description)}
}

type boardDescriptionSetV1 struct {
	Description optionalTextV1 `json:"description"`
}

func decodeBoardDescriptionSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardDescriptionSetV1](encoded)
	if err != nil {
		return nil, err
	}
	description, err := value.Description.toText()
	if err != nil {
		return nil, err
	}
	return &BoardDescriptionSet{Description: description}, nil
}

// BoardArchiveSet assigns the board's complete archive state.
type BoardArchiveSet struct {
	// Archive is nil to restore an active board.
	// An archive requires a nonblank Actor and nonzero At.
	Archive *board.Archive
}

// EventBoardArchiveSet assigns the complete board archive state.
const EventBoardArchiveSet EventKind = "board.archive.set"

func (*BoardArchiveSet) kind() EventKind { return EventBoardArchiveSet }

func (s *BoardArchiveSet) validateV1() error { return validateArchive(s.Archive) }

func (s *BoardArchiveSet) valueV1() any {
	return boardArchiveSetV1{
		Archived: s.Archive != nil,
		Archive:  archiveToV1(s.Archive),
	}
}

type boardArchiveSetV1 struct {
	Archived bool       `json:"archived"`
	Archive  *archiveV1 `json:"archive"`
}

func decodeBoardArchiveSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardArchiveSetV1](encoded)
	if err != nil {
		return nil, err
	}
	if value.Archived != (value.Archive != nil) {
		return nil, errors.New("board archive assignment has inconsistent state")
	}
	return &BoardArchiveSet{Archive: value.Archive.toArchive()}, nil
}

type archiveV1 struct {
	Actor  string    `json:"actor"`
	At     time.Time `json:"at"`
	Reason *string   `json:"reason"`
}

func (v *archiveV1) toArchive() *board.Archive {
	if v == nil {
		return nil
	}
	return &board.Archive{Actor: v.Actor, At: v.At, Reason: v.Reason}
}

func archiveToV1(value *board.Archive) *archiveV1 {
	if value == nil {
		return nil
	}
	return &archiveV1{
		Actor:  value.Actor,
		At:     value.At.UTC(),
		Reason: value.Reason,
	}
}

func validateArchive(value *board.Archive) error {
	if value == nil {
		return nil
	}
	if value.At.IsZero() || strings.TrimSpace(value.Actor) == "" {
		return errors.New("board archive requires time and actor")
	}
	return nil
}
