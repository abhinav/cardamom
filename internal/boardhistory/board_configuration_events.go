package boardhistory

import (
	"fmt"
	"strings"
)

// BoardIssueIDPrefixSet assigns or clears the board issue ID prefix override.
type BoardIssueIDPrefixSet struct {
	// Prefix is nil to remove the board override. A value is 1–16 bytes,
	// starts with a lowercase ASCII letter or digit, contains only lowercase
	// ASCII letters, digits, and hyphens, and ends with a hyphen.
	Prefix *string
}

// EventBoardIssueIDPrefixSet assigns or clears the board prefix override.
const EventBoardIssueIDPrefixSet EventKind = "board.issue_id_prefix.set"

func (*BoardIssueIDPrefixSet) kind() EventKind { return EventBoardIssueIDPrefixSet }

func (s *BoardIssueIDPrefixSet) validateV1() error { return validateOptionalPrefix(s.Prefix) }

func (s *BoardIssueIDPrefixSet) valueV1() any {
	return boardIssueIDPrefixSetV1{Prefix: optionalTextToV1(s.Prefix)}
}

type boardIssueIDPrefixSetV1 struct {
	Prefix optionalTextV1 `json:"prefix"`
}

func decodeBoardIssueIDPrefixSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardIssueIDPrefixSetV1](encoded)
	if err != nil {
		return nil, err
	}
	prefix, err := value.Prefix.toText()
	if err != nil {
		return nil, err
	}
	return &BoardIssueIDPrefixSet{Prefix: prefix}, nil
}

// BoardIssueIDStrategySet assigns or clears the board issue ID strategy override.
type BoardIssueIDStrategySet struct {
	// Strategy is nil to remove the board override, or random or sequential.
	Strategy *string
}

// EventBoardIssueIDStrategySet assigns or clears the board ID strategy override.
const EventBoardIssueIDStrategySet EventKind = "board.issue_id_strategy.set"

func (*BoardIssueIDStrategySet) kind() EventKind { return EventBoardIssueIDStrategySet }

func (s *BoardIssueIDStrategySet) validateV1() error {
	return validateOptionalStrategy(s.Strategy)
}

func (s *BoardIssueIDStrategySet) valueV1() any {
	return boardIssueIDStrategySetV1{Strategy: optionalTextToV1(s.Strategy)}
}

type boardIssueIDStrategySetV1 struct {
	Strategy optionalTextV1 `json:"strategy"`
}

func decodeBoardIssueIDStrategySetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardIssueIDStrategySetV1](encoded)
	if err != nil {
		return nil, err
	}
	strategy, err := value.Strategy.toText()
	if err != nil {
		return nil, err
	}
	return &BoardIssueIDStrategySet{Strategy: strategy}, nil
}

// BoardSummaryLimitSet assigns or clears the board summary byte limit.
type BoardSummaryLimitSet struct {
	// Limit is nil to remove the board override, or a positive byte count.
	Limit *int64
}

// EventBoardSummaryLimitSet assigns or clears the board summary limit.
const EventBoardSummaryLimitSet EventKind = "board.summary_limit.set"

func (*BoardSummaryLimitSet) kind() EventKind { return EventBoardSummaryLimitSet }

func (s *BoardSummaryLimitSet) validateV1() error { return validateOptionalByteLimit(s.Limit) }

func (s *BoardSummaryLimitSet) valueV1() any {
	return boardLimitSetV1{Limit: optionalInt64ToV1(s.Limit)}
}

func decodeBoardSummaryLimitSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardLimitSetV1](encoded)
	if err != nil {
		return nil, err
	}
	limit, err := value.Limit.toInt64()
	if err != nil {
		return nil, err
	}
	return &BoardSummaryLimitSet{Limit: limit}, nil
}

// BoardAttachmentLimitSet assigns or clears the board attachment byte limit.
type BoardAttachmentLimitSet struct {
	// Limit is nil to remove the board override, or a positive byte count.
	Limit *int64
}

// EventBoardAttachmentLimitSet assigns or clears the board attachment limit.
const EventBoardAttachmentLimitSet EventKind = "board.attachment_limit.set"

func (*BoardAttachmentLimitSet) kind() EventKind { return EventBoardAttachmentLimitSet }

func (s *BoardAttachmentLimitSet) validateV1() error {
	return validateOptionalByteLimit(s.Limit)
}

func (s *BoardAttachmentLimitSet) valueV1() any {
	return boardLimitSetV1{Limit: optionalInt64ToV1(s.Limit)}
}

func decodeBoardAttachmentLimitSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardLimitSetV1](encoded)
	if err != nil {
		return nil, err
	}
	limit, err := value.Limit.toInt64()
	if err != nil {
		return nil, err
	}
	return &BoardAttachmentLimitSet{Limit: limit}, nil
}

// BoardPinLimitSet assigns or clears the board pin-count limit.
type BoardPinLimitSet struct {
	// Limit is nil to remove the board override, or a nonnegative pin count.
	Limit *int64
}

// EventBoardPinLimitSet assigns or clears the board pin limit.
const EventBoardPinLimitSet EventKind = "board.pin_limit.set"

func (*BoardPinLimitSet) kind() EventKind { return EventBoardPinLimitSet }

func (s *BoardPinLimitSet) validateV1() error { return validateOptionalPinLimit(s.Limit) }

func (s *BoardPinLimitSet) valueV1() any {
	return boardLimitSetV1{Limit: optionalInt64ToV1(s.Limit)}
}

func decodeBoardPinLimitSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardLimitSetV1](encoded)
	if err != nil {
		return nil, err
	}
	limit, err := value.Limit.toInt64()
	if err != nil {
		return nil, err
	}
	return &BoardPinLimitSet{Limit: limit}, nil
}

type boardLimitSetV1 struct {
	Limit optionalInt64V1 `json:"limit"`
}

func validateOptionalPrefix(value *string) error {
	if value == nil {
		return nil
	}
	return validatePrefixV1(*value)
}

func validateOptionalStrategy(value *string) error {
	if value == nil {
		return nil
	}
	return validateIDStrategyV1(*value)
}

func validateOptionalByteLimit(value *int64) error {
	if value == nil {
		return nil
	}
	return validateByteLimitV1(*value)
}

func validateOptionalPinLimit(value *int64) error {
	if value == nil {
		return nil
	}
	return validatePinLimitV1(*value)
}

func validatePrefixV1(value string) error {
	if len(value) == 0 || len(value) > 16 || !strings.HasSuffix(value, "-") ||
		value != strings.ToLower(value) {
		return fmt.Errorf("invalid V1 issue ID prefix %q", value)
	}
	for index, character := range []byte(value) {
		if isASCIILower(character) || isASCIIDigit(character) || index > 0 && character == '-' {
			continue
		}
		return fmt.Errorf("invalid V1 issue ID prefix %q", value)
	}
	return nil
}

func validateIDStrategyV1(value string) error {
	if value != "random" && value != "sequential" {
		return fmt.Errorf("invalid V1 issue ID strategy %q", value)
	}
	return nil
}

func validateByteLimitV1(value int64) error {
	if value <= 0 {
		return fmt.Errorf("invalid V1 byte limit %d", value)
	}
	return nil
}

func validatePinLimitV1(value int64) error {
	if value < 0 {
		return fmt.Errorf("invalid V1 pin limit %d", value)
	}
	return nil
}
