// Package board owns board identity, state, settings, revisions, attribution,
// and finite board mutations.
package board

import (
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/errkind"
)

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
	return &State{
		id:          snapshot.ID,
		projectID:   projectID,
		name:        name,
		description: cloneString(snapshot.Description),
		created:     snapshot.Created,
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
	}, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
