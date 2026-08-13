package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
)

const (
	maxPrefixLength = 16

	// DefaultSummaryMaxBytes is the built-in issue summary admission limit.
	DefaultSummaryMaxBytes uint64 = 2048

	// DefaultAttachmentMaxBytes is the built-in attachment admission limit.
	DefaultAttachmentMaxBytes uint64 = 100 << 20

	// DefaultPinMaxCount is the built-in board pin admission limit.
	DefaultPinMaxCount uint64 = 8
)

// Prefix is a validated issue ID prefix.
type Prefix string

// NewPrefix parses a lowercase issue prefix ending in a dash.
func NewPrefix(value string) (Prefix, error) {
	_, idErr := issue.NewID(value)
	if idErr != nil ||
		len(value) > maxPrefixLength ||
		value != strings.ToLower(value) ||
		!strings.HasSuffix(value, "-") {
		return "", fmt.Errorf(
			"issue ID prefix %q: must contain lowercase letters, digits, or dashes, start with a letter or digit, end in a dash, and be at most 16 characters",
			value,
		)
	}
	return Prefix(value), nil
}

// InferredPrefix derives a valid issue prefix from a project name.
func InferredPrefix(projectName string) Prefix {
	body := make([]byte, 0, len(projectName))
	separate := false
	for _, character := range projectName {
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') {
			if separate && len(body) > 0 {
				body = append(body, '-')
			}
			body = append(body, byte(character))
			separate = false
			continue
		}
		separate = len(body) > 0
	}
	if len(body) >= maxPrefixLength {
		body = body[:maxPrefixLength-1]
	}
	body = bytes.TrimRight(body, "-")
	if len(body) == 0 {
		return Defaults().Issue.ID.Prefix
	}
	body = append(body, '-')
	prefix, err := NewPrefix(string(body))
	must.NotErrorf(err, "inferred issue prefix %q must be valid", body)
	return prefix
}

// InitializationPrefix contains the project-layer writes selected for one
// initialization request.
type InitializationPrefix struct {
	// FreshProject is persisted when initialization creates the project.
	FreshProject *Prefix

	// RetainedProject is persisted when initialization retains the project.
	RetainedProject *Prefix
}

// SelectProjectCreationPrefix returns the project-level prefix override for a
// new project. Nil preserves an active store-level prefix.
func SelectProjectCreationPrefix(
	projectName string,
	requested *string,
	store Overrides,
) (*Prefix, error) {
	if requested != nil {
		prefix, err := NewPrefix(*requested)
		if err != nil {
			return nil, err
		}
		return &prefix, nil
	}
	if store.Issue.ID.Prefix != nil {
		return nil, nil
	}
	prefix := InferredPrefix(projectName)
	return &prefix, nil
}

// SelectInitializationPrefix applies explicit, store, and inferred prefix
// precedence for one initialization request.
func SelectInitializationPrefix(
	projectName string,
	requested *string,
	store Overrides,
) (InitializationPrefix, error) {
	freshProject, err := SelectProjectCreationPrefix(
		projectName,
		requested,
		store,
	)
	if err != nil {
		return InitializationPrefix{}, err
	}
	selected := InitializationPrefix{FreshProject: freshProject}
	if requested != nil {
		selected.RetainedProject = freshProject
	}
	return selected, nil
}

// String returns the configured issue ID prefix.
func (p Prefix) String() string { return string(p) }

// IDStrategy selects the issue ID allocation strategy.
type IDStrategy uint8

const (
	_ IDStrategy = iota

	// IDStrategyRandom allocates adaptive random issue suffixes.
	IDStrategyRandom

	// IDStrategySequential allocates increasing decimal issue suffixes.
	IDStrategySequential
)

// NewIDStrategy parses an issue ID allocation strategy.
func NewIDStrategy(value string) (IDStrategy, error) {
	switch value {
	case "random":
		return IDStrategyRandom, nil
	case "sequential":
		return IDStrategySequential, nil
	default:
		return 0, fmt.Errorf("invalid issue ID strategy %q", value)
	}
}

// String returns the stable issue ID strategy name.
func (s IDStrategy) String() string {
	switch s {
	case IDStrategyRandom:
		return "random"
	case IDStrategySequential:
		return "sequential"
	default:
		return ""
	}
}

// MarshalJSON preserves the textual strategy in structured output.
func (s IDStrategy) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// ByteLimit is a positive persisted byte limit representable by SQLite.
type ByteLimit uint64

// NewByteLimit parses a positive byte limit.
func NewByteLimit(value uint64) (ByteLimit, error) {
	if value == 0 || value > math.MaxInt64 {
		return 0, fmt.Errorf("byte limit %d: must be between 1 and %d", value, uint64(math.MaxInt64))
	}
	return ByteLimit(value), nil
}

// Uint64 returns the limit in bytes.
func (l ByteLimit) Uint64() uint64 { return uint64(l) }

// Configuration is one fully resolved nested configuration.
type Configuration struct {
	// Issue contains issue authoring and identity policy.
	Issue IssueConfiguration

	// Attachment contains attachment admission policy.
	Attachment AttachmentConfiguration

	// Board contains board-scoped coordination policy.
	Board BoardConfiguration
}

// IssueConfiguration contains fully resolved issue policy.
type IssueConfiguration struct {
	// ID controls issue identity allocation.
	ID IssueIDConfiguration

	// Summary controls summary write admission.
	Summary SummaryConfiguration
}

// IssueIDConfiguration contains fully resolved issue identity policy.
type IssueIDConfiguration struct {
	// Prefix is prepended to newly allocated issue IDs.
	Prefix Prefix

	// Strategy selects random or sequential suffix allocation.
	Strategy IDStrategy
}

// SummaryConfiguration contains fully resolved summary policy.
type SummaryConfiguration struct {
	// MaxBytes is the largest accepted summary in bytes.
	MaxBytes ByteLimit
}

// AttachmentConfiguration contains fully resolved attachment policy.
type AttachmentConfiguration struct {
	// MaxBytes is the largest attachment admitted for a new upload.
	MaxBytes ByteLimit
}

// BoardConfiguration contains fully resolved board policy.
type BoardConfiguration struct {
	// Pins controls the board's ordered pinned-issue collection.
	Pins PinConfiguration
}

// PinConfiguration contains fully resolved board pin policy.
type PinConfiguration struct {
	// MaxCount is the largest admitted pinned-issue collection.
	MaxCount board.PinLimit
}

// Defaults returns Cardamom's built-in fully resolved configuration.
func Defaults() Configuration {
	prefix, err := NewPrefix("cm-")
	if err != nil {
		panic(err)
	}
	summary, err := NewByteLimit(DefaultSummaryMaxBytes)
	if err != nil {
		panic(err)
	}
	attachment, err := NewByteLimit(DefaultAttachmentMaxBytes)
	if err != nil {
		panic(err)
	}
	return Configuration{
		Issue: IssueConfiguration{
			ID:      IssueIDConfiguration{Prefix: prefix, Strategy: IDStrategyRandom},
			Summary: SummaryConfiguration{MaxBytes: summary},
		},
		Attachment: AttachmentConfiguration{MaxBytes: attachment},
		Board: BoardConfiguration{
			Pins: PinConfiguration{MaxCount: board.PinLimit(DefaultPinMaxCount)},
		},
	}
}

// Overrides is one nested layer of optional per-field values.
type Overrides struct {
	// Issue contains optional issue policy values.
	Issue IssueOverrides

	// Attachment contains optional attachment policy values.
	Attachment AttachmentOverrides

	// Board contains optional board policy values.
	Board BoardOverrides
}

// IssueOverrides contains optional issue policy values.
type IssueOverrides struct {
	// ID contains optional issue identity values.
	ID IssueIDOverrides

	// Summary contains optional summary policy values.
	Summary SummaryOverrides
}

// IssueIDOverrides contains optional issue identity values.
type IssueIDOverrides struct {
	// Prefix overrides the inherited issue ID prefix when non-nil.
	Prefix *Prefix

	// Strategy overrides the inherited issue ID strategy when non-nil.
	Strategy *IDStrategy
}

// SummaryOverrides contains optional summary policy values.
type SummaryOverrides struct {
	// MaxBytes overrides the inherited summary limit when non-nil.
	MaxBytes *ByteLimit
}

// AttachmentOverrides contains optional attachment policy values.
type AttachmentOverrides struct {
	// MaxBytes overrides the inherited attachment limit when non-nil.
	MaxBytes *ByteLimit
}

// BoardOverrides contains optional board policy values.
type BoardOverrides struct {
	// Pins contains optional pinned-issue policy values.
	Pins PinOverrides
}

// PinOverrides contains optional pinned-issue policy values.
type PinOverrides struct {
	// MaxCount overrides the inherited pin limit when non-nil.
	MaxCount *board.PinLimit
}

// Empty reports whether the layer inherits every field.
func (o Overrides) Empty() bool {
	return o.Issue.ID.Prefix == nil &&
		o.Issue.ID.Strategy == nil &&
		o.Issue.Summary.MaxBytes == nil &&
		o.Attachment.MaxBytes == nil &&
		o.Board.Pins.MaxCount == nil
}

// Equal reports whether two layers contain the same inherited and overridden
// values.
func (o Overrides) Equal(other Overrides) bool {
	return equalOptional(o.Issue.ID.Prefix, other.Issue.ID.Prefix) &&
		equalOptional(o.Issue.ID.Strategy, other.Issue.ID.Strategy) &&
		equalOptional(o.Issue.Summary.MaxBytes, other.Issue.Summary.MaxBytes) &&
		equalOptional(o.Attachment.MaxBytes, other.Attachment.MaxBytes) &&
		equalOptional(o.Board.Pins.MaxCount, other.Board.Pins.MaxCount)
}

func equalOptional[T comparable](left, right *T) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// Validate verifies every present override.
func (o Overrides) Validate() error {
	if o.Issue.ID.Prefix != nil {
		prefix, err := NewPrefix(o.Issue.ID.Prefix.String())
		if err != nil || prefix != *o.Issue.ID.Prefix {
			return fmt.Errorf("issue ID prefix: %w", err)
		}
	}
	if o.Issue.ID.Strategy != nil && o.Issue.ID.Strategy.String() == "" {
		return fmt.Errorf("invalid issue ID strategy %d", *o.Issue.ID.Strategy)
	}
	if o.Issue.Summary.MaxBytes != nil {
		if _, err := NewByteLimit(o.Issue.Summary.MaxBytes.Uint64()); err != nil {
			return fmt.Errorf("summary maximum: %w", err)
		}
	}
	if o.Attachment.MaxBytes != nil {
		if _, err := NewByteLimit(o.Attachment.MaxBytes.Uint64()); err != nil {
			return fmt.Errorf("attachment maximum: %w", err)
		}
	}
	if o.Board.Pins.MaxCount != nil {
		if _, err := board.NewPinLimit(o.Board.Pins.MaxCount.Uint64()); err != nil {
			return fmt.Errorf("board pin maximum: %w", err)
		}
	}
	return nil
}

// Field is the closed set of mutable configuration fields.
type Field uint8

const (
	_ Field = iota

	// FieldIssueIDPrefix selects issue.id.prefix.
	FieldIssueIDPrefix

	// FieldIssueIDStrategy selects issue.id.strategy.
	FieldIssueIDStrategy

	// FieldIssueSummaryMaxBytes selects issue.summary.max_bytes.
	FieldIssueSummaryMaxBytes

	// FieldAttachmentMaxBytes selects attachment.max_bytes.
	FieldAttachmentMaxBytes

	// FieldBoardPinsMaxCount selects board.pins.max_count.
	FieldBoardPinsMaxCount
)

// String returns the stable dotted field name.
func (f Field) String() string {
	switch f {
	case FieldIssueIDPrefix:
		return "issue.id.prefix"
	case FieldIssueIDStrategy:
		return "issue.id.strategy"
	case FieldIssueSummaryMaxBytes:
		return "issue.summary.max_bytes"
	case FieldAttachmentMaxBytes:
		return "attachment.max_bytes"
	case FieldBoardPinsMaxCount:
		return "board.pins.max_count"
	default:
		return ""
	}
}

// Patch selects fields to replace or clear from one override layer.
// A selected field with a nil override clears that field.
type Patch struct {
	// Fields is the finite field mask applied atomically.
	Fields []Field

	// Overrides supplies replacement values for selected fields.
	Overrides Overrides
}

// Validate verifies the field mask and replacement values.
func (p Patch) Validate() error {
	if len(p.Fields) == 0 {
		return errors.New("configuration patch requires at least one field")
	}
	if err := p.Overrides.Validate(); err != nil {
		return err
	}
	selected := make(map[Field]struct{}, len(p.Fields))
	for _, field := range p.Fields {
		if field.String() == "" {
			return fmt.Errorf("invalid configuration field %d", field)
		}
		if _, ok := selected[field]; ok {
			return fmt.Errorf("duplicate configuration field %q", field.String())
		}
		selected[field] = struct{}{}
	}
	for field, present := range map[Field]bool{
		FieldIssueIDPrefix:        p.Overrides.Issue.ID.Prefix != nil,
		FieldIssueIDStrategy:      p.Overrides.Issue.ID.Strategy != nil,
		FieldIssueSummaryMaxBytes: p.Overrides.Issue.Summary.MaxBytes != nil,
		FieldAttachmentMaxBytes:   p.Overrides.Attachment.MaxBytes != nil,
		FieldBoardPinsMaxCount:    p.Overrides.Board.Pins.MaxCount != nil,
	} {
		if present {
			if _, ok := selected[field]; !ok {
				return fmt.Errorf("configuration value %q is outside the field mask", field.String())
			}
		}
	}
	return nil
}

// Apply returns one override layer with the selected replacements or clears.
func (p Patch) Apply(current Overrides) Overrides {
	for _, field := range p.Fields {
		switch field {
		case FieldIssueIDPrefix:
			current.Issue.ID.Prefix = p.Overrides.Issue.ID.Prefix
		case FieldIssueIDStrategy:
			current.Issue.ID.Strategy = p.Overrides.Issue.ID.Strategy
		case FieldIssueSummaryMaxBytes:
			current.Issue.Summary.MaxBytes = p.Overrides.Issue.Summary.MaxBytes
		case FieldAttachmentMaxBytes:
			current.Attachment.MaxBytes = p.Overrides.Attachment.MaxBytes
		case FieldBoardPinsMaxCount:
			current.Board.Pins.MaxCount = p.Overrides.Board.Pins.MaxCount
		}
	}
	return current
}
