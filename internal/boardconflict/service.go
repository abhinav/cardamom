package boardconflict

import (
	"context"
	"strings"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// Repository persists finite conflict operations for one selected board.
// Reads use a coherent snapshot. Resolution reloads the reviewed identity,
// applies domain policy, appends its assignment, and replaces projections in
// one writer transaction. A stale identity changes no state.
type Repository interface {
	// ListConflicts reads compact metadata without loading alternative bodies.
	ListConflicts(context.Context, ListRequest) (Page, error)
	// ReadConflict loads compact metadata for one current conflict.
	ReadConflict(context.Context, string) (Inspection, error)
	// ReadAlternative loads one retained typed body for an inspected value.
	ReadAlternative(context.Context, string, string) (Value, error)
	// ResolveConflict commits one reviewed field assignment.
	ResolveConflict(context.Context, ResolveRequest) (Resolution, error)
	// RepairRelations commits a reviewed compound relationship repair.
	RepairRelations(context.Context, RelationsRequest) (Resolution, error)
	// RepairExecution commits a coherent choice for coupled execution fields.
	RepairExecution(context.Context, ExecutionRequest) (Resolution, error)
}

// Entry is the closed set of inspected conflict forms.
// The concrete value is either a field conflict or a finite constraint conflict.
type Entry interface {
	ID() string
	BoardID() board.ID
	conflictEntry()
}

func (*Conflict) conflictEntry()           {}
func (*ConstraintConflict) conflictEntry() {}

// Inspection combines reviewed metadata with current public references.
type Inspection struct {
	// Conflict is the current field or constraint conflict.
	Conflict Entry // required
	// Issues maps retained identities to current issue references.
	Issues []IssueReference
}

// IssueReference identifies an issue independently of a mutable public ID.
type IssueReference struct {
	UID       issue.UID
	Reference issue.Reference
}

// Value is one retained assignment and the current references needed to display it.
type Value struct {
	Payload boardhistory.Payload
	Issues  []IssueReference
}

// RelationsRequest uses public issue references at the operation boundary.
// The repository resolves them to private identities in its writer snapshot.
type RelationsRequest struct {
	ConflictID string
	Remove     []AssignmentID
	Parents    []ParentSelection
}

// ParentSelection chooses a new parent or removes an implicated child's parent.
type ParentSelection struct {
	ChildID  issue.ID
	ParentID *issue.ID
}

// ExecutionRequest selects compatible values without inventing defaults.
// The repository supplies the operation timestamp to the domain repair.
type ExecutionRequest struct {
	ConflictID        string
	Waiting           WaitingChoice
	WaitingAssignment AssignmentID
	Kind              *issue.Kind
	Lifecycle         *issue.Lifecycle
}

// ListRequest selects a bounded page of unresolved board or issue conflicts.
// An empty IssueID includes both board settings and every issue in the board.
type ListRequest struct {
	// IssueID limits results to conflicts affecting one public issue reference.
	IssueID string
	// After is the last conflict ID from a preceding page.
	After string
	// Limit is the maximum result count; zero selects the default page size.
	Limit int
}

// Summary identifies one unresolved unit without loading alternative bodies.
type Summary struct {
	// ID binds the subject and its currently observed contender set.
	ID string
	// Unit identifies the field whose authoritative current value is absent.
	Unit Unit
	// ConstraintKind is zero for field conflicts and identifies structural conflicts otherwise.
	ConstraintKind ConstraintKind
	// Issues identifies issues affected by this conflict, including structural
	// conflicts and competing external-key ownership.
	Issues []issue.Reference
	// Alternatives is the number of distinct semantic values.
	Alternatives int
}

// Page is one bounded conflict collection and its continuation cursor.
type Page struct {
	// Conflicts contains compact summaries in stable conflict-ID order.
	Conflicts []Summary
	// Next is empty when this snapshot has no remaining page.
	Next string
}

// Resolution reports the committed resolution and the new board revision.
type Resolution struct {
	// ConflictID identifies the reviewed alternatives that were resolved.
	ConflictID string
	// Revision is the board change cursor after the resolution committed.
	Revision int64
}

// ConfigurationResolver supplies per-operation local admission policy.
type ConfigurationResolver interface {
	ResolveConfiguration(context.Context, board.ID) (configuration.Configuration, error)
}

// Service exposes board-scoped conflict inspection and deliberate resolution.
type Service struct {
	// Repository owns coherent snapshots and transactional publication.
	Repository Repository // required
	// Configuration admits newly composed Summary text; retained values and clears do not consume it.
	Configuration ConfigurationResolver // required
}

// List returns compact unresolved units and a continuation cursor.
func (s *Service) List(ctx context.Context, request ListRequest) (Page, error) {
	if request.Limit < 0 || request.Limit > 100 {
		return Page{}, errkind.Errorf(errkind.InvalidInput, "conflict page limit must be between 0 and 100")
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.IssueID != "" {
		if _, err := issue.NewID(request.IssueID); err != nil {
			return Page{}, err
		}
	}
	return s.Repository.ListConflicts(ctx, request)
}

// Read loads current alternative identities without reading their bodies.
func (s *Service) Read(ctx context.Context, id string) (Inspection, error) {
	if id == "" {
		return Inspection{}, errkind.Errorf(errkind.InvalidInput, "conflict identity required")
	}
	return s.Repository.ReadConflict(ctx, id)
}

// Resolve commits the choice against the alternative set the caller reviewed.
func (s *Service) Resolve(ctx context.Context, request ResolveRequest) (Resolution, error) {
	if request.ConflictID == "" || request.Choice == nil {
		return Resolution{}, errkind.Errorf(errkind.InvalidInput, "conflict identity and resolution choice required")
	}
	if text, ok := request.Choice.(ReplaceText); ok && strings.TrimSpace(text.Body) != "" {
		inspection, err := s.Repository.ReadConflict(ctx, request.ConflictID)
		if err != nil {
			return Resolution{}, err
		}
		if field, ok := inspection.Conflict.(*Conflict); ok && field.Unit().Kind == boardhistory.EventIssueSummarySet {
			policy, err := s.Configuration.ResolveConfiguration(ctx, field.BoardID())
			if err != nil {
				return Resolution{}, err
			}
			if err := policy.RequireResolved(configuration.FieldIssueSummaryMaxBytes); err != nil {
				return Resolution{}, err
			}
			length, maximum := uint64(len(strings.TrimSpace(text.Body))), policy.Issue.Summary.MaxBytes.Uint64()
			if length > maximum {
				return Resolution{}, errkind.Errorf(errkind.InvalidInput, "summary is %d bytes; maximum is %d bytes", length, maximum)
			}
		}
	}
	return s.Repository.ResolveConflict(ctx, request)
}

// ReadAlternative loads one value after the caller selects its identity.
func (s *Service) ReadAlternative(ctx context.Context, conflictID, alternativeID string) (Value, error) {
	if conflictID == "" || alternativeID == "" {
		return Value{}, errkind.Errorf(errkind.InvalidInput, "conflict and alternative identities required")
	}
	return s.Repository.ReadAlternative(ctx, conflictID, alternativeID)
}

// RepairRelations validates and commits changes to reviewed relationship assignments.
func (s *Service) RepairRelations(ctx context.Context, request RelationsRequest) (Resolution, error) {
	if request.ConflictID == "" {
		return Resolution{}, errkind.Errorf(errkind.InvalidInput, "conflict identity required")
	}
	return s.Repository.RepairRelations(ctx, request)
}

// RepairExecution resolves coupled execution facts against the reviewed identity.
func (s *Service) RepairExecution(ctx context.Context, request ExecutionRequest) (Resolution, error) {
	if request.ConflictID == "" {
		return Resolution{}, errkind.Errorf(errkind.InvalidInput, "conflict identity required")
	}
	return s.Repository.RepairExecution(ctx, request)
}
