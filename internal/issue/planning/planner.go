// Package planning owns issue creation, metadata editing, relationships, and
// atomic graph application for one board.
package planning

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
)

// ErrIncompleteSnapshot reports a code-owned incomplete planning snapshot.
var ErrIncompleteSnapshot = errors.New("incomplete board planning snapshot")

// CommittedRevision identifies the canonical board revision published after a
// planning change commits.
type CommittedRevision struct {
	// Revision is the canonical revision shared by every projection in the result.
	Revision board.Revision
}

// Changes executes finite planning operations against one board. Mutations own
// their complete atomic projection boundaries.
type Changes interface {
	// CreateIssue persists one issue and its initial relationships.
	CreateIssue(
		context.Context,
		configuration.IssueIDConfiguration,
		CreateIssue,
	) (IssueCreated, error)

	// EditIssue persists one atomic metadata and relationship edit.
	EditIssue(context.Context, EditIssue) (IssueEdited, error)

	// ApplyDocument validates or atomically persists one apply document.
	ApplyDocument(
		context.Context,
		configuration.IssueIDConfiguration,
		ApplyDocument,
		ApplyMode,
	) (DocumentApplied, error)
}

// IssueReader supplies the post-commit issue projection required by planning
// results.
type IssueReader interface {
	// ReadIssue returns one issue detail from the Planner's board.
	ReadIssue(context.Context, issue.ReadRequest) (issue.View, error)
}

// Planner owns caller-facing issue planning use cases for one board.
type Planner struct {
	// changes owns atomic planning persistence.
	changes Changes

	// issues reads canonical post-commit issue projections.
	issues IssueReader

	// boardID identifies the configuration scope for every operation.
	boardID board.ID

	// configuration resolves current policy at each operation boundary.
	configuration ConfigurationResolver
}

// ConfigurationResolver resolves current policy for one board operation.
type ConfigurationResolver interface {
	// ResolveConfiguration returns one fully resolved per-operation snapshot.
	ResolveConfiguration(context.Context, board.ID) (configuration.Configuration, error)
}

// PlannerOptions supplies optional issue planning policy overrides.
type PlannerOptions struct {
	// BoardID identifies the configuration scope. It is required when
	// Configuration is non-nil.
	BoardID board.ID

	// Configuration resolves live policy. Nil uses built-in defaults.
	Configuration ConfigurationResolver
}

// NewPlanner constructs a Planner from its direct required collaborators.
// It panics when either collaborator is nil because process composition must
// provide both dependencies.
func NewPlanner(changes Changes, issues IssueReader, options *PlannerOptions) *Planner {
	must.NotBeNilf(changes, "issue planning Changes is required")
	must.NotBeNilf(issues, "issue planning IssueReader is required")
	planner := &Planner{changes: changes, issues: issues}
	if options != nil {
		planner.boardID = options.BoardID
		planner.configuration = options.Configuration
	}
	return planner
}

func (p *Planner) resolveConfiguration(
	ctx context.Context,
) (configuration.Configuration, error) {
	if p.configuration == nil {
		defaults := configuration.Defaults()
		// Repository tests may retain their existing constructor-level identity
		// policy while production supplies live configuration.
		defaults.Issue.ID = configuration.IssueIDConfiguration{}
		return defaults, nil
	}
	return p.configuration.ResolveConfiguration(ctx, p.boardID)
}

func validateSummary(summary string, maximum configuration.ByteLimit) error {
	if uint64(len(summary)) <= maximum.Uint64() {
		return nil
	}
	return errkind.Errorf(
		errkind.InvalidInput,
		"invalid input: summary is %d bytes; maximum is %d bytes",
		len(summary),
		maximum.Uint64(),
	)
}

func (p *Planner) readIssue(
	ctx context.Context,
	id issue.ID,
	contextDepth *int,
) (issue.View, error) {
	return p.issues.ReadIssue(ctx, issue.ReadRequest{
		IssueID:      id.String(),
		ContextDepth: contextDepth,
	})
}

func validateSnapshot(boardID board.ID, revision board.Revision) error {
	if boardID == "" {
		return ErrIncompleteSnapshot
	}
	if err := revision.Validate(); err != nil {
		return ErrIncompleteSnapshot
	}
	return nil
}

func validateIssueSnapshot(
	boardID board.ID,
	revision board.Revision,
	state issue.State,
) error {
	if err := validateSnapshot(boardID, revision); err != nil || state.ID() == "" {
		return ErrIncompleteSnapshot
	}
	return nil
}
