package planning

import (
	"context"
	"slices"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

func (p *EditPolicy) applyDependencyEdit(command EditIssue) ([]issue.ID, bool, error) {
	dependencies := slices.Clone(p.snapshot.Dependencies)
	existing := makeSet(p.snapshot.ExistingIDs)
	changed := false
	for _, dependency := range uniqueIssueIDs(command.RemoveDependencies) {
		if !slices.Contains(dependencies, dependency) {
			return nil, false, errkind.Errorf(
				errkind.NotFound,
				"issue does not depend on %q",
				dependency,
			)
		}
		dependencies = deleteValue(dependencies, dependency)
		changed = true
	}
	for _, dependency := range uniqueIssueIDs(command.AddDependencies) {
		if dependency == command.IssueID {
			return nil, false, errkind.Errorf(errkind.InvalidInput, "an issue cannot be its own dependency")
		}
		if _, ok := existing[dependency]; !ok {
			return nil, false, errkind.Errorf(errkind.NotFound, "dependency %q: issue not found", dependency)
		}
		if slices.Contains(p.snapshot.DependencyAncestors[dependency], command.IssueID) {
			return nil, false, errkind.Errorf(errkind.InvalidInput, "dependency graph must remain acyclic")
		}
		if !slices.Contains(dependencies, dependency) {
			dependencies = append(dependencies, dependency)
			changed = true
		}
	}
	return dependencies, changed, nil
}

func (p *EditPolicy) applyParentEdit(command EditIssue) (*issue.ID, bool, error) {
	parent := cloneIssueID(p.snapshot.Parent)
	if !command.ParentSet {
		return parent, false, nil
	}
	if command.Parent == "" {
		if parent == nil {
			return nil, false, nil
		}
		return nil, true, nil
	}
	if command.Parent == command.IssueID || slices.Contains(p.snapshot.ContainmentAncestors, command.IssueID) {
		return nil, false, errkind.Errorf(errkind.InvalidInput, "containment would create a cycle")
	}
	if _, ok := makeSet(p.snapshot.ExistingIDs)[command.Parent]; !ok {
		return nil, false, errkind.Errorf(errkind.NotFound, "parent: issue not found")
	}
	if parent != nil && *parent == command.Parent {
		return parent, false, nil
	}
	newParent := command.Parent
	return &newParent, true, nil
}

// EditIssue is one atomic scalar, label, dependency, and containment edit.
type EditIssue struct {
	// IssueID identifies the issue to edit.
	IssueID issue.ID
	// Title, Kind, and Priority are replaced when non-nil.
	Title    *string
	Kind     *issue.Kind
	Priority *issue.Priority
	// Summary is used only when SummarySet is true.
	Summary string
	// SummarySet distinguishes omission from clearing the summary.
	SummarySet bool
	// Details is used only when DetailsSet is true.
	Details string
	// DetailsSet distinguishes omission from clearing details.
	DetailsSet bool
	// Parent is used only when ParentSet is true; an empty value removes containment.
	Parent issue.ID
	// ParentSet distinguishes omission from removing containment.
	ParentSet bool
	// AddDependencies and RemoveDependencies edit readiness edges atomically.
	AddDependencies    []issue.ID
	RemoveDependencies []issue.ID
	// AddLabels and RemoveLabels edit labels atomically.
	AddLabels    []issue.Label
	RemoveLabels []issue.Label
	// ReplaceLabels, when non-nil, replaces the complete label set. It cannot
	// be combined with AddLabels or RemoveLabels.
	ReplaceLabels *[]issue.Label
}

// EditSnapshot contains current issue, graph, and label state needed for one
// atomic edit. DependencyAncestors is keyed by each proposed prerequisite;
// ContainmentAncestors describes the proposed containment parent.
type EditSnapshot struct {
	// BoardID and Revision identify the board snapshot used for planning.
	BoardID  board.ID
	Revision board.Revision
	// Issue is the complete current kernel state.
	Issue issue.State
	// DirectChildren supplies containment state used by kind transitions.
	DirectChildren []issue.State
	// Labels and Dependencies are complete current relationship sets.
	Labels       []issue.Label
	Dependencies []issue.ID
	// Parent is the current containment parent; nil means no parent.
	Parent *issue.ID
	// ExistingIDs contains every durable issue identity in the selected board.
	ExistingIDs []issue.ID
	// DependencyAncestors is keyed by each proposed prerequisite.
	DependencyAncestors map[issue.ID][]issue.ID
	// ContainmentAncestors is the ancestor chain of the proposed parent.
	ContainmentAncestors []issue.ID
	// OccurredAt timestamps a changed issue.
	OccurredAt time.Time
}

// EditPolicy evaluates one atomic issue edit against validated current snapshot.
type EditPolicy struct{ snapshot EditSnapshot }

// IssueEdited is the semantic outcome of issue edited.
type IssueEdited struct {
	// Issue is the complete post-change kernel state.
	Issue issue.State
	// Labels and DependsOn are complete post-change relationship sets.
	Labels    []issue.Label
	DependsOn []issue.ID
	// Parent is the post-change containment parent; nil means no parent.
	Parent *issue.ID
	// Changed reports whether persistence must replace the returned projections.
	Changed bool
	CommittedRevision
}

// EditIssueResult reports the committed result of edit issue.
type EditIssueResult struct {
	// Issue is the canonical post-commit issue detail.
	Issue issue.Detail
}

// EditIssueRequest supplies caller input for edit issue.
type EditIssueRequest struct {
	// ID identifies the issue to edit.
	ID string
	// Title, Type, and Priority are replaced when non-nil.
	Title    *string
	Type     *string
	Priority *int
	// Summary is used only when SummarySet is true.
	Summary *string
	// SummarySet distinguishes omission from clearing the summary.
	SummarySet bool
	// Details is used only when DetailsSet is true.
	Details *string
	// DetailsSet distinguishes omission from clearing details.
	DetailsSet bool
	// Parent is used only when ParentSet is true; nil removes containment.
	Parent *string
	// ParentSet distinguishes omission from removing containment.
	ParentSet bool
	// AddDependencies and RemoveDependencies edit readiness edges atomically.
	AddDependencies    []string
	RemoveDependencies []string
	// AddLabels and RemoveLabels edit labels atomically.
	AddLabels    []string
	RemoveLabels []string

	// Labels, when non-nil, replaces the complete issue label set. It cannot
	// be combined with AddLabels or RemoveLabels.
	Labels *[]string
}

// LoadEdit validates snapshot and loads policy state for edit.
func LoadEdit(snapshot EditSnapshot) (*EditPolicy, error) {
	if err := validateIssueSnapshot(snapshot.BoardID, snapshot.Revision, snapshot.Issue); err != nil ||
		snapshot.OccurredAt.IsZero() {
		return nil, ErrIncompleteSnapshot
	}
	if _, err := uniqueLabels(snapshot.Labels); err != nil {
		return nil, ErrIncompleteSnapshot
	}
	return &EditPolicy{snapshot: snapshot}, nil
}

// EditIssue applies edit issue policy to the loaded board state.
func (p *EditPolicy) EditIssue(command EditIssue) (IssueEdited, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return IssueEdited{}, ErrIncompleteSnapshot
	}
	if err := validateEditOverlaps(command); err != nil {
		return IssueEdited{}, err
	}

	state := p.snapshot.Issue
	snapshot := state.Snapshot()
	changed := false
	if command.Title != nil {
		title := strings.TrimSpace(*command.Title)
		if title == "" {
			return IssueEdited{}, errkind.Errorf(errkind.InvalidInput, "invalid input: title required")
		}
		if title != state.Title() {
			snapshot.Title = title
			changed = true
		}
	}
	if command.Kind != nil && *command.Kind != state.Kind() {
		if _, err := issue.NewKind(command.Kind.String()); err != nil {
			return IssueEdited{}, err
		}
		if err := validateKindTransition(state.Kind(), *command.Kind, p.snapshot.DirectChildren); err != nil {
			return IssueEdited{}, err
		}
		snapshot.Kind = *command.Kind
		changed = true
	}
	if command.Priority != nil && *command.Priority != state.Priority() {
		if _, err := issue.NewPriority(command.Priority.Int()); err != nil {
			return IssueEdited{}, err
		}
		snapshot.Priority = *command.Priority
		changed = true
	}
	if command.SummarySet && command.Summary != state.Summary() {
		snapshot.Summary = command.Summary
		changed = true
	}
	if command.DetailsSet && command.Details != state.Details() {
		snapshot.Details = command.Details
		changed = true
	}
	if !snapshot.Kind.Executable() && state.ActiveClaim() != nil {
		return IssueEdited{}, errkind.Errorf(errkind.InvalidInput, "invalid input: %s issues cannot be assigned", snapshot.Kind)
	}

	labels, labelChanged, err := applyLabelEdit(
		p.snapshot.Labels,
		command.AddLabels,
		command.RemoveLabels,
		command.ReplaceLabels,
	)
	if err != nil {
		return IssueEdited{}, err
	}
	dependencies, dependencyChanged, err := p.applyDependencyEdit(command)
	if err != nil {
		return IssueEdited{}, err
	}
	parent, parentChanged, err := p.applyParentEdit(command)
	if err != nil {
		return IssueEdited{}, err
	}
	changed = changed || labelChanged || dependencyChanged || parentChanged
	if !changed {
		return IssueEdited{
			Issue: state, Labels: labels, DependsOn: dependencies, Parent: parent,
		}, nil
	}
	snapshot.Updated = p.snapshot.OccurredAt
	state, err = issue.Load(snapshot)
	if err != nil {
		return IssueEdited{}, err
	}
	return IssueEdited{
		Issue: state, Labels: labels, DependsOn: dependencies, Parent: parent,
		Changed: true,
	}, nil
}

func validateEditOverlaps(command EditIssue) error {
	if command.ReplaceLabels != nil && (len(command.AddLabels) != 0 || len(command.RemoveLabels) != 0) {
		return errkind.Errorf(errkind.InvalidInput, "invalid input: label replacement cannot be combined with additions or removals")
	}
	if valuesOverlap(command.AddDependencies, command.RemoveDependencies) {
		return errkind.Errorf(errkind.InvalidInput, "invalid input: a dependency cannot be both added and removed")
	}
	if valuesOverlap(command.AddLabels, command.RemoveLabels) {
		return errkind.Errorf(errkind.InvalidInput, "invalid input: a label cannot be both added and removed")
	}
	return nil
}

func validateKindTransition(current, target issue.Kind, _ []issue.State) error {
	switch {
	case current == target:
		return nil
	case (current == issue.KindTask || current == issue.KindWorkstream || current == issue.KindRoutine) &&
		(target == issue.KindTask || target == issue.KindWorkstream || target == issue.KindRoutine):
		return nil
	default:
		return errkind.Errorf(errkind.InvalidInput, "invalid input: issue kind cannot transition from %s to %s", current, target)
	}
}

func applyLabelEdit(current, additions, removals []issue.Label, replacement *[]issue.Label) ([]issue.Label, bool, error) {
	if replacement != nil {
		labels, err := uniqueLabels(*replacement)
		if err != nil {
			return nil, false, err
		}
		return labels, !equalLabelSets(current, labels), nil
	}
	added, err := uniqueLabels(additions)
	if err != nil {
		return nil, false, err
	}
	removed, err := uniqueLabels(removals)
	if err != nil {
		return nil, false, err
	}
	labels := slices.Clone(current)
	for _, label := range removed {
		if slices.Contains(labels, label) {
			labels = deleteValue(labels, label)
		}
	}
	for _, label := range added {
		if !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	return labels, !slices.Equal(current, labels), nil
}

func equalLabelSets(left, right []issue.Label) bool {
	if len(left) != len(right) {
		return false
	}
	for _, label := range left {
		if !slices.Contains(right, label) {
			return false
		}
	}
	return true
}

func valuesOverlap[T comparable](left, right []T) bool {
	set := make(map[T]struct{}, len(left))
	for _, value := range left {
		set[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}

func deleteValue[T comparable](values []T, target T) []T {
	index := slices.Index(values, target)
	if index < 0 {
		return values
	}
	return append(values[:index], values[index+1:]...)
}

func cloneIssueID(value *issue.ID) *issue.ID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// EditIssue validates caller input and executes edit issue.
func (p *Planner) EditIssue(ctx context.Context, _ issue.Invocation, req EditIssueRequest) (EditIssueResult, error) {
	configuration, err := p.resolveConfiguration(ctx)
	if err != nil {
		return EditIssueResult{}, err
	}
	command, err := editIssueCommand(req, configuration.Issue.Summary.MaxBytes)
	if err != nil {
		return EditIssueResult{}, err
	}
	outcome, err := p.changes.EditIssue(ctx, command)
	if err != nil {
		return EditIssueResult{}, err
	}
	view, err := p.readIssue(ctx, outcome.Issue.ID(), nil)
	if err != nil {
		return EditIssueResult{}, err
	}
	return EditIssueResult{Issue: view.Detail}, nil
}

func editIssueCommand(
	req EditIssueRequest,
	summaryMaxBytes configuration.ByteLimit,
) (EditIssue, error) {
	if req.SummarySet && req.Summary != nil {
		if err := validateSummary(*req.Summary, summaryMaxBytes); err != nil {
			return EditIssue{}, err
		}
	}
	command, err := baseEditIssueCommand(req.ID, req.Title, req.Type, req.Priority)
	if err != nil {
		return EditIssue{}, err
	}
	command.SummarySet = req.SummarySet
	if req.Summary != nil {
		command.Summary = *req.Summary
	}
	command.DetailsSet = req.DetailsSet
	if req.Details != nil {
		command.Details = *req.Details
	}
	command.ParentSet = req.ParentSet
	if req.Parent != nil {
		command.Parent, err = issue.NewID(*req.Parent)
		if err != nil {
			return EditIssue{}, err
		}
	}
	command.AddDependencies, err = issueIDs(req.AddDependencies)
	if err != nil {
		return EditIssue{}, err
	}
	command.RemoveDependencies, err = issueIDs(req.RemoveDependencies)
	if err != nil {
		return EditIssue{}, err
	}
	command.AddLabels, err = labels(req.AddLabels)
	if err != nil {
		return EditIssue{}, err
	}
	command.RemoveLabels, err = labels(req.RemoveLabels)
	if err != nil {
		return EditIssue{}, err
	}
	if req.Labels != nil {
		values, err := labels(*req.Labels)
		if err != nil {
			return EditIssue{}, err
		}
		command.ReplaceLabels = &values
	}
	return command, nil
}

func baseEditIssueCommand(idValue string, title, kindValue *string, priorityValue *int) (EditIssue, error) {
	id, err := issue.NewID(idValue)
	if err != nil {
		return EditIssue{}, err
	}
	command := EditIssue{IssueID: id, Title: title}
	if kindValue != nil {
		kind, err := issue.NewKind(*kindValue)
		if err != nil {
			return EditIssue{}, err
		}
		command.Kind = &kind
	}
	if priorityValue != nil {
		priority, err := issue.NewPriority(*priorityValue)
		if err != nil {
			return EditIssue{}, err
		}
		command.Priority = &priority
	}
	return command, nil
}
