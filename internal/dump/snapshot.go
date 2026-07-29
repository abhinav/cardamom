// Package dump owns selection, deterministic rendering, filesystem publication,
// and generated-file ownership for Cardamom dump output from one coherent board
// snapshot.
package dump

import (
	"errors"

	"go.abhg.dev/cardamom/internal/issue"
)

// Provenance identifies the project and board represented by a dump.
type Provenance struct {
	// ProjectID is the stable identity of the source project.
	ProjectID string

	// ProjectName is the user-visible name of the source project.
	ProjectName string

	// BoardID is the stable identity of the source board.
	BoardID string

	// BoardName is the user-visible name of the source board.
	BoardName string
}

func (p Provenance) validate() error {
	switch {
	case p.ProjectID == "":
		return errors.New("dump project ID is required")
	case p.ProjectName == "":
		return errors.New("dump project name is required")
	case p.BoardID == "":
		return errors.New("dump board ID is required")
	case p.BoardName == "":
		return errors.New("dump board name is required")
	default:
		return nil
	}
}

// BoardSnapshot is the complete dump-specific board state returned by a
// repository at one canonical revision.
type BoardSnapshot struct {
	// BoardID is the stable identity of the source board.
	BoardID string

	// Revision is the canonical board revision shared by every included read.
	Revision int64

	// Description is nil when the board has no shared description.
	Description *string

	// Issues contains every current issue projection in the board.
	Issues []Issue

	// Dependencies contains every readiness edge in the board.
	Dependencies []Dependency

	// Containment contains every context edge in the board.
	Containment []Containment

	// Results contains each issue's current durable outcome when set.
	Results []Result

	// LogEntries contains every attributed issue record in repository
	// chronological order.
	LogEntries []LogEntry
}

// Snapshot is the normalized issue selection from one BoardSnapshot.
type Snapshot struct {
	BoardSnapshot

	// Provenance identifies the selected project and board represented by the
	// snapshot.
	Provenance Provenance

	// Selection is the normalized issue selection represented by the snapshot.
	Selection Selection

	// Full-board reference targets retain ownership needed to rewrite
	// references without expanding Selection.
	referenceTargets referenceTargets
}

// Issue is one current issue projection in a dump snapshot.
type Issue struct {
	// ID is the stable issue identity.
	ID string

	// Title is the issue's current user-visible title.
	Title string

	// Type is workstream, task, checkpoint, or routine.
	Type string

	// Status is ready, blocked, in_progress, waiting, closed, or cancelled.
	Status string

	// Priority ranges from 0 (highest) through 4 (lowest).
	Priority int

	// Assignee is the active claim owner when the issue is in progress.
	Assignee *string

	// Created is the issue creation time as a Unix timestamp in seconds.
	Created int64

	// Updated is the issue's last projection update as a Unix timestamp in
	// seconds.
	Updated int64

	// StartedAt is the first claim time as a Unix timestamp in seconds.
	StartedAt *int64

	// Closed is the terminal transition time as a Unix timestamp in seconds.
	Closed *int64

	// Summary is the concise stable contract when set.
	Summary *string

	// Details is expanded stable material when set.
	Details *string

	// State is the current recovery state when set.
	State *string

	// NextAction is the optional current planned transition.
	NextAction *string

	// Revision is the canonical revision represented by this issue projection.
	Revision int64

	// Labels contains the issue's sorted routing and grouping metadata.
	Labels []string
}

// Dependency is one readiness edge from ChildID to prerequisite ParentID.
type Dependency struct {
	// ChildID identifies the issue blocked by the prerequisite.
	ChildID string

	// ParentID identifies the prerequisite issue.
	ParentID string
}

// Containment is one context edge from ChildID to ParentID.
type Containment struct {
	// ChildID identifies the contained issue.
	ChildID string

	// ParentID identifies the containing workstream.
	ParentID string
}

// Result is one issue's current durable outcome.
type Result struct {
	// IssueID identifies the issue that owns the outcome.
	IssueID string

	// Body is the current durable outcome text.
	Body string
}

// LogEntry is one immutable typed issue record.
type LogEntry struct {
	// ID is the stable log-entry identity.
	ID issue.LogID

	// IssueID identifies the issue that owns the record.
	IssueID string

	// Kind selects the finite Log payload contract.
	Kind string

	// Author is absent when the event has no body attribution.
	Author *string

	// Committer is absent when the preserving actor is unknown.
	Committer *string

	// Body is the authored record text.
	Body string

	// NextAction is the planned transition preserved with a State snapshot.
	NextAction *string

	// Created is absent when the event has no recorded time.
	Created *int64
}
