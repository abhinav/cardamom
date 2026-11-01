package issue

import "fmt"

// Field identifies a semantic issue value whose current state can be unresolved.
// Collection fields may have several conflicts over different members.
type Field uint8

const (
	_ Field = iota
	// FieldTitle identifies the one-line issue identity.
	FieldTitle
	// FieldKind identifies the workflow role.
	FieldKind
	// FieldLifecycle includes lifecycle and its closure time.
	FieldLifecycle
	// FieldPriority identifies scheduling priority.
	FieldPriority
	// FieldWaiting includes the waiting reason and transition time.
	FieldWaiting
	// FieldSummary identifies the inherited stable contract.
	FieldSummary
	// FieldDetails identifies expanded stable context.
	FieldDetails
	// FieldState includes mutable recovery content and attribution.
	FieldState
	// FieldResult identifies the durable outcome.
	FieldResult
	// FieldLabels identifies inert label membership.
	FieldLabels
	// FieldDependencies identifies readiness relationships.
	FieldDependencies
	// FieldParent identifies the containment parent.
	FieldParent
	// FieldExternalKeys identifies producer-key ownership.
	FieldExternalKeys
	// FieldPins identifies board-pin membership or position.
	FieldPins
	// FieldCheckpoint identifies an approval or denial decision.
	FieldCheckpoint
	// FieldBoardDescription identifies unresolved inherited board context.
	FieldBoardDescription
)

// String returns the field spelling used by conflict inspection.
func (f Field) String() string {
	names := [...]string{"", "title", "kind", "lifecycle", "priority", "waiting", "summary", "details", "state", "result", "labels", "dependencies", "parent", "external_keys", "pins", "checkpoint", "board_description"}
	if int(f) >= len(names) {
		return ""
	}
	return names[f]
}

// ParseField restores one known conflict field from persisted metadata.
func ParseField(value string) (Field, error) {
	for field := FieldTitle; field <= FieldBoardDescription; field++ {
		if field.String() == value {
			return field, nil
		}
	}
	return 0, fmt.Errorf("unknown issue conflict field %q", value)
}

// ConflictReference identifies why an issue field has no authoritative value.
// Alternatives and their bodies are loaded through conflict inspection;
// collection reads retain only this compact reference.
type ConflictReference struct {
	// ID binds the conflict's displayed causal contender set.
	ID string `json:"id"` // required

	// Field identifies the affected issue value.
	Field Field `json:"field"` // required

	// Key identifies a disputed label or external-key membership. Empty names the whole field.
	Key string `json:"member_key,omitempty"`
}

// Unresolved reports whether references includes an unresolved value for field.
func Unresolved(references []ConflictReference, field Field) bool {
	for _, reference := range references {
		if reference.Field == field {
			return true
		}
	}
	return false
}

// MarshalText encodes the stable field name for structured conflict references.
func (f Field) MarshalText() ([]byte, error) {
	name := f.String()
	if name == "" {
		return nil, fmt.Errorf("unknown conflict field %d", f)
	}
	return []byte(name), nil
}
