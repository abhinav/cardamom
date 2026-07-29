// Package project owns project identity, state, selection, and fresh setup
// contracts.
package project

import (
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/errkind"
)

// ID is the stable identity of one repository or product namespace.
type ID string

// NewID parses a non-empty project identity without whitespace.
func NewID(value string) (ID, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return "", errkind.Errorf(errkind.InvalidInput, "invalid project namespace: project identity required")
	}
	return ID(value), nil
}

// String returns the persisted identity.
func (id ID) String() string { return string(id) }

// Selector identifies a project by stable identity or exact name.
type Selector struct {
	// value is the normalized identity or exact name supplied by the caller.
	value string
}

// NewSelector parses a non-empty project selector.
func NewSelector(value string) (Selector, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Selector{}, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: project selector required")
	}
	return Selector{value: value}, nil
}

// String returns the normalized external selector.
func (s Selector) String() string { return s.value }

// Matches reports whether candidate has the selected identity or exact name.
func (s Selector) Matches(candidate *State) bool {
	return candidate != nil && (s.value == candidate.ID().String() || s.value == candidate.Name())
}

// Snapshot contains the persisted values required to load a State.
type Snapshot struct {
	// ID is the project's stable persisted identity.
	ID ID

	// Name is the user-visible project name.
	Name string

	// Created records when the project was established.
	Created time.Time
}

// State identifies a repository or product whose boards share a store.
// State has identity and must be passed and returned by pointer.
type State struct {
	// id is the project identity established for the lifetime of this state.
	id ID

	// name is the normalized user-visible project name.
	name string

	// created records when the project identity was established.
	created time.Time
}

// Load validates and restores one persisted project state.
func Load(snapshot Snapshot) (*State, error) {
	if _, err := NewID(snapshot.ID.String()); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(snapshot.Name)
	if name == "" {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: project name required")
	}
	if snapshot.Created.IsZero() {
		return nil, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: project creation time required")
	}
	return &State{id: snapshot.ID, name: name, created: snapshot.Created}, nil
}

// ID returns the project's stable identity.
func (p *State) ID() ID { return p.id }

// Name returns the user-visible project name.
func (p *State) Name() string { return p.name }

// Created returns when the project was established.
func (p *State) Created() time.Time { return p.created }
