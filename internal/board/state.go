// Package board owns board identity, state, settings, revisions, attribution,
// and finite board mutations.
package board

import (
	"errors"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/errkind"
)

// ErrArchived reports that a mutation cannot change an archived board.
// Callers may recognize it as a conflict; the rejected operation leaves board
// state unchanged.
var ErrArchived = errkind.Wrap(errkind.Conflict, errors.New("board is archived"))

// Archive is the board's current logical archive state. Unarchiving clears this
// metadata; it is not a lifecycle history.
type Archive struct {
	// Actor identifies the invocation that archived the board.
	Actor string

	// At records when the archive transition committed.
	At time.Time

	// Reason is the optional normalized explanation supplied by the actor.
	Reason *string
}

// ID is the stable identity of one coordination context.
type ID string

// NewID parses a non-empty board identity without whitespace.
func NewID(value string) (ID, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return "", errkind.Errorf(errkind.InvalidInput, "invalid project namespace: board identity required")
	}
	return ID(value), nil
}

// String returns the persisted identity.
func (id ID) String() string { return string(id) }

// Snapshot is the semantic board representation accepted by Load.
// Repository row scans and board-creation operations construct snapshots at
// the persistence boundary. Snapshot carries no SQL row or transaction state.
type Snapshot struct {
	// ID is the board's stable persisted identity.
	ID ID

	// ProjectID identifies the project containing the board.
	ProjectID string

	// Name is the user-visible board name.
	Name string

	// Description is the optional Markdown source shared by the board.
	Description *string

	// Created records when the board was established.
	Created time.Time

	// Archived records the board's logical archive state, when present.
	Archived *Archive
}

// State is an explicitly established coordination context within a project.
// State has identity and must be passed and returned by pointer.
type State struct {
	// id is the board identity established for the lifetime of this state.
	id ID

	// projectID is the persisted identity of the containing project.
	projectID string

	// name is the normalized user-visible board name.
	name string

	// description owns a copy of the optional Markdown source.
	description *string

	// created records when the board identity was established.
	created time.Time

	// archived owns a copy of the optional logical archive metadata.
	archived *Archive
}

// Load validates and restores one persisted board state.
func Load(snapshot Snapshot) (*State, error) {
	if _, err := NewID(snapshot.ID.String()); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(snapshot.ProjectID)
	if projectID == "" || strings.ContainsAny(projectID, " \t\r\n") {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: project identity required")
	}
	name := strings.TrimSpace(snapshot.Name)
	if name == "" {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: board name required")
	}
	if snapshot.Created.IsZero() {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: board creation time required")
	}
	if snapshot.Description != nil && strings.TrimSpace(*snapshot.Description) == "" {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: board description required")
	}
	archived, err := normalizeArchive(snapshot.Archived)
	if err != nil {
		return nil, err
	}
	return &State{
		id:          snapshot.ID,
		projectID:   projectID,
		name:        name,
		description: cloneString(snapshot.Description),
		created:     snapshot.Created,
		archived:    archived,
	}, nil
}

// ID returns the board's stable identity.
func (b *State) ID() ID { return b.id }

// ProjectID returns the project containing the board.
func (b *State) ProjectID() string { return b.projectID }

// Name returns the user-visible board name.
func (b *State) Name() string { return b.name }

// Description returns a copy of the board's Markdown source, or nil when unset.
func (b *State) Description() *string { return cloneString(b.description) }

// Created returns when the board was established.
func (b *State) Created() time.Time { return b.created }

// Archived returns a copy of the archive metadata, or nil when active.
func (b *State) Archived() *Archive { return cloneArchive(b.archived) }

// RequireMutable returns ErrArchived while the board is archived. Callers must
// invoke it before changing state.
func (b *State) RequireMutable() error {
	if b.archived != nil {
		return ErrArchived
	}
	return nil
}

// ArchiveBoard records the first archive transition. Repeated calls preserve
// the existing metadata and report false without validating replacement input.
func (b *State) ArchiveBoard(actor string, at time.Time, reason *string) (bool, error) {
	if b.archived != nil {
		return false, nil
	}
	archive, err := normalizeArchive(&Archive{Actor: actor, At: at, Reason: reason})
	if err != nil {
		return false, err
	}
	b.archived = archive
	return true, nil
}

// Unarchive clears the current archive metadata. It reports false without
// changing an already active board.
func (b *State) Unarchive() bool {
	if b.archived == nil {
		return false
	}
	b.archived = nil
	return true
}

// SettingsEdit selects settings changed by one atomic board operation.
type SettingsEdit struct {
	// Name replaces the board name when non-nil.
	Name *string

	// Description selects whether to retain, replace, or clear the description.
	// Nil retains the current description.
	Description *DescriptionEdit
}

// DescriptionEdit is an explicit board-description replacement.
// Construct it with ReplaceDescription.
type DescriptionEdit struct {
	// replacement is nil when the operation clears the description.
	replacement *string
}

// ReplaceDescription returns an edit that replaces the description with value.
// A nil value clears the current description.
func ReplaceDescription(value *string) *DescriptionEdit {
	return &DescriptionEdit{replacement: cloneString(value)}
}

// EditSettings validates and applies one atomic board settings operation.
func (b *State) EditSettings(edit SettingsEdit) (*State, error) {
	if err := b.RequireMutable(); err != nil {
		return nil, err
	}
	if edit.Name == nil && edit.Description == nil {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: board setting required")
	}
	name := b.name
	if edit.Name != nil {
		name = strings.TrimSpace(*edit.Name)
		if name == "" {
			return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: board name required")
		}
	}
	description := cloneString(b.description)
	if edit.Description != nil {
		description = cloneString(edit.Description.replacement)
		if description != nil && strings.TrimSpace(*description) == "" {
			return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: board description required")
		}
	}
	return &State{
		id:          b.id,
		projectID:   b.projectID,
		name:        name,
		description: description,
		created:     b.created,
		archived:    cloneArchive(b.archived),
	}, nil
}

// normalizeArchive validates the all-or-nothing archive representation and
// returns a copy owned by board state.
func normalizeArchive(value *Archive) (*Archive, error) {
	if value == nil {
		return nil, nil
	}
	actor := strings.TrimSpace(value.Actor)
	if actor == "" || value.At.IsZero() {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid board archive metadata")
	}
	reason := cloneString(value.Reason)
	if reason != nil {
		trimmed := strings.TrimSpace(*reason)
		if trimmed == "" {
			return nil, errkind.Errorf(errkind.InvalidInput, "invalid board archive reason")
		}
		reason = &trimmed
	}
	return &Archive{Actor: actor, At: value.At, Reason: reason}, nil
}

func cloneArchive(value *Archive) *Archive {
	if value == nil {
		return nil
	}
	return &Archive{Actor: value.Actor, At: value.At, Reason: cloneString(value.Reason)}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
