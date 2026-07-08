package planning

import (
	"context"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// CreateIssue is the normalized domain command for creating one issue.
type CreateIssue struct {
	// Title is normalized before the issue state is created.
	Title string
	// Kind selects the issue behavior class.
	Kind issue.Kind
	// Priority controls readiness ordering.
	Priority issue.Priority
	// Labels is the complete initial label set.
	Labels []issue.Label
	// DependsOn is the complete initial prerequisite set.
	DependsOn []issue.ID

	// Parent is the optional containment parent for the new issue.
	// Nil creates an issue without containment.
	Parent *issue.ID

	// Summary is the optional concise stable contract.
	Summary string
	// Details is optional expanded stable material.
	Details string
}

// CreateSnapshot contains the state required to create one issue.
type CreateSnapshot struct {
	// BoardID identifies the selected board that owns the new issue.
	BoardID board.ID
	// Revision is the board snapshot revision used for planning.
	Revision board.Revision
	// AllocatedID is the identity reserved for the new issue.
	AllocatedID issue.ID
	// ExistingIDs contains every durable issue identity in the selected board.
	ExistingIDs []issue.ID
	// OccurredAt timestamps the new issue.
	OccurredAt time.Time
}

// CreatePolicy evaluates creation against one validated board snapshot.
type CreatePolicy struct{ snapshot CreateSnapshot }

// IssueCreated is the semantic outcome of creating one issue.
type IssueCreated struct {
	// Issue is the complete new kernel state.
	Issue issue.State
	// Labels and DependsOn are the complete committed relationship sets.
	Labels    []issue.Label
	DependsOn []issue.ID

	// Parent is the containment parent committed with Issue.
	// Nil means the issue has no containment parent.
	Parent *issue.ID

	CommittedRevision
}

// CreateIssueResult reports the committed result of create issue.
type CreateIssueResult struct {
	// Issue is the canonical post-commit issue detail.
	Issue issue.Detail
}

// CreateIssueRequest supplies caller input for create issue.
type CreateIssueRequest struct {
	// Title is required after trimming.
	Title string
	// Type defaults to task when empty.
	Type string
	// Priority is validated against the issue priority range.
	Priority int
	// Labels and DependsOn supply initial relationships.
	Labels    []string
	DependsOn []string

	// Parent is an optional containment parent ID.
	// An empty value creates an issue without containment.
	Parent string

	// Summary is the optional concise stable contract.
	Summary string
	// Details is optional expanded stable material.
	Details string
}

// LoadCreate validates snapshot and loads policy state for create.
func LoadCreate(snapshot CreateSnapshot) (*CreatePolicy, error) {
	if err := validateSnapshot(snapshot.BoardID, snapshot.Revision); err != nil ||
		snapshot.AllocatedID == "" || snapshot.OccurredAt.IsZero() {
		return nil, ErrIncompleteSnapshot
	}
	return &CreatePolicy{snapshot: snapshot}, nil
}

// CreateIssue applies create issue policy to the loaded board state.
func (p *CreatePolicy) CreateIssue(command CreateIssue) (IssueCreated, error) {
	title := strings.TrimSpace(command.Title)
	if title == "" {
		return IssueCreated{}, errkind.Errorf(errkind.InvalidInput, "invalid input: title required")
	}
	if _, err := issue.NewKind(command.Kind.String()); err != nil {
		return IssueCreated{}, err
	}
	if _, err := issue.NewPriority(command.Priority.Int()); err != nil {
		return IssueCreated{}, err
	}
	existing := makeSet(p.snapshot.ExistingIDs)
	dependencies := uniqueIssueIDs(command.DependsOn)
	for _, dependency := range dependencies {
		if _, ok := existing[dependency]; !ok {
			return IssueCreated{}, errkind.Errorf(errkind.NotFound, "issue not found: %s", dependency)
		}
	}
	parent := cloneIssueID(command.Parent)
	if parent != nil {
		if *parent == p.snapshot.AllocatedID {
			return IssueCreated{}, errkind.Errorf(errkind.InvalidInput, "containment would create a cycle")
		}
		if _, ok := existing[*parent]; !ok {
			return IssueCreated{}, errkind.Errorf(errkind.NotFound, "parent: issue not found")
		}
	}
	labels, err := uniqueLabels(command.Labels)
	if err != nil {
		return IssueCreated{}, err
	}
	state, err := issue.Load(issue.Snapshot{
		ID:        p.snapshot.AllocatedID,
		Title:     title,
		Kind:      command.Kind,
		Lifecycle: issue.LifecycleOpen,
		Priority:  command.Priority,
		Created:   p.snapshot.OccurredAt,
		Updated:   p.snapshot.OccurredAt,
		Summary:   command.Summary,
		Details:   command.Details,
	})
	if err != nil {
		return IssueCreated{}, err
	}
	return IssueCreated{
		Issue:     state,
		Labels:    labels,
		DependsOn: dependencies,
		Parent:    parent,
	}, nil
}

// CreateIssue validates caller input and executes create issue.
func (p *Planner) CreateIssue(ctx context.Context, _ issue.Invocation, req CreateIssueRequest) (CreateIssueResult, error) {
	configuration, err := p.resolveConfiguration(ctx)
	if err != nil {
		return CreateIssueResult{}, err
	}
	command, err := createIssueCommand(req, configuration.Issue.Summary.MaxBytes)
	if err != nil {
		return CreateIssueResult{}, err
	}
	outcome, err := p.changes.CreateIssue(
		ctx,
		configuration.Issue.ID,
		command,
	)
	if err != nil {
		return CreateIssueResult{}, err
	}
	view, err := p.readIssue(ctx, outcome.Issue.ID(), nil)
	if err != nil {
		return CreateIssueResult{}, err
	}
	return CreateIssueResult{Issue: view.Detail}, nil
}

func createIssueCommand(
	req CreateIssueRequest,
	summaryMaxBytes configuration.ByteLimit,
) (CreateIssue, error) {
	if err := validateSummary(req.Summary, summaryMaxBytes); err != nil {
		return CreateIssue{}, err
	}
	kind, err := kindOrDefault(req.Type)
	if err != nil {
		return CreateIssue{}, err
	}
	priority, err := issue.NewPriority(req.Priority)
	if err != nil {
		return CreateIssue{}, err
	}
	issueLabels, err := labels(req.Labels)
	if err != nil {
		return CreateIssue{}, err
	}
	dependencies, err := issueIDs(req.DependsOn)
	if err != nil {
		return CreateIssue{}, err
	}
	var parent *issue.ID
	if req.Parent != "" {
		value, err := issue.NewID(req.Parent)
		if err != nil {
			return CreateIssue{}, err
		}
		parent = &value
	}
	return CreateIssue{
		Title:     req.Title,
		Kind:      kind,
		Priority:  priority,
		Labels:    issueLabels,
		DependsOn: dependencies,
		Parent:    parent,
		Summary:   req.Summary,
		Details:   req.Details,
	}, nil
}

func kindOrDefault(value string) (issue.Kind, error) {
	if value == "" {
		return issue.KindTask, nil
	}
	return issue.NewKind(value)
}

func labels(values []string) ([]issue.Label, error) {
	result := make([]issue.Label, len(values))
	for index, value := range values {
		label, err := issue.NewLabel(value)
		if err != nil {
			return nil, err
		}
		result[index] = label
	}
	return result, nil
}

func issueIDs(values []string) ([]issue.ID, error) {
	result := make([]issue.ID, len(values))
	for index, value := range values {
		id, err := issue.NewID(value)
		if err != nil {
			return nil, err
		}
		result[index] = id
	}
	return result, nil
}

func uniqueLabels(values []issue.Label) ([]issue.Label, error) {
	result := make([]issue.Label, 0, len(values))
	seen := make(map[issue.Label]struct{}, len(values))
	for _, value := range values {
		validated, err := issue.NewLabel(value.String())
		if err != nil {
			return nil, err
		}
		if validated != value {
			return nil, errkind.Errorf(errkind.InvalidInput, "invalid input: label must be normalized")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func uniqueIssueIDs(values []issue.ID) []issue.ID {
	result := make([]issue.ID, 0, len(values))
	seen := make(map[issue.ID]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func makeSet(values []issue.ID) map[issue.ID]struct{} {
	result := make(map[issue.ID]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
