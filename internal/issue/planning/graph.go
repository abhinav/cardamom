package planning

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// IssueAlias identifies one issue within an apply document.
type IssueAlias string

// NewIssueAlias parses one non-empty alias token.
func NewIssueAlias(value string) (IssueAlias, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return "", errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: issue alias must be one non-empty token",
		)
	}
	return IssueAlias(value), nil
}

// String returns the textual representation of an issue alias.
func (a IssueAlias) String() string { return string(a) }

// ExternalKey is an exact producer-owned identity bound within one board.
type ExternalKey string

// NewExternalKey parses one non-empty producer identity without normalizing it.
func NewExternalKey(value string) (ExternalKey, error) {
	if value == "" {
		return "", errkind.Errorf(errkind.InvalidInput, "invalid input: external key required")
	}
	return ExternalKey(value), nil
}

// String returns the exact textual representation of an external key.
func (k ExternalKey) String() string { return string(k) }

// ApplyExistingPolicy selects how an apply document treats existing targets.
type ApplyExistingPolicy uint8

const (
	// ApplyExistingError rejects an entry that resolves to an existing issue.
	ApplyExistingError ApplyExistingPolicy = iota

	// ApplyExistingSkip preserves an entry that resolves to an existing issue.
	ApplyExistingSkip

	// ApplyExistingUpdate applies present editable fields to an existing issue.
	ApplyExistingUpdate
)

// NewApplyExistingPolicy parses existing-target behavior.
// An empty value selects the default error policy.
func NewApplyExistingPolicy(value string) (ApplyExistingPolicy, error) {
	switch value {
	case "", "error":
		return ApplyExistingError, nil
	case "skip":
		return ApplyExistingSkip, nil
	case "update":
		return ApplyExistingUpdate, nil
	default:
		return 0, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: invalid existing issue policy %q (valid: error, skip, update)",
			value,
		)
	}
}

// String returns the textual representation of an existing-target policy.
func (p ApplyExistingPolicy) String() string {
	switch p {
	case ApplyExistingError:
		return "error"
	case ApplyExistingSkip:
		return "skip"
	case ApplyExistingUpdate:
		return "update"
	default:
		return ""
	}
}

// ApplyMode selects validation-only or atomic application behavior.
type ApplyMode uint8

const (
	// ApplyModeUnknown is not a valid invocation mode.
	ApplyModeUnknown ApplyMode = iota

	// ApplyModeCommit atomically persists accepted document changes.
	ApplyModeCommit

	// ApplyModeDryRun plans the document without allocating durable state.
	ApplyModeDryRun
)

// ApplyAction reports how one document entry was handled.
type ApplyAction uint8

const (
	// ApplyActionUnknown is not a valid receipt action.
	ApplyActionUnknown ApplyAction = iota

	// ApplyActionCreate reports a new issue.
	ApplyActionCreate

	// ApplyActionUpdate reports a changed existing issue.
	ApplyActionUpdate

	// ApplyActionSkip reports an existing issue preserved by policy.
	ApplyActionSkip

	// ApplyActionNoChange reports an update already matching requested state.
	ApplyActionNoChange
)

// String returns the textual representation of an apply receipt action.
func (a ApplyAction) String() string {
	switch a {
	case ApplyActionCreate:
		return "create"
	case ApplyActionUpdate:
		return "update"
	case ApplyActionSkip:
		return "skip"
	case ApplyActionNoChange:
		return "no_change"
	default:
		return ""
	}
}

// ApplyReferenceKind identifies the namespace carried by an issue reference.
type ApplyReferenceKind uint8

const (
	// ApplyReferenceUnknown is not a valid reference kind.
	ApplyReferenceUnknown ApplyReferenceKind = iota

	// ApplyReferenceAlias selects a document-local alias.
	ApplyReferenceAlias

	// ApplyReferenceID selects a durable issue ID.
	ApplyReferenceID

	// ApplyReferenceKey selects a board-scoped producer key.
	ApplyReferenceKey
)

// ApplyIssueReference identifies one issue in an explicit namespace.
type ApplyIssueReference struct {
	// Kind selects which value carries the reference.
	Kind ApplyReferenceKind

	// Alias carries a document-local alias for ApplyReferenceAlias.
	Alias string

	// ID carries a durable issue identity for ApplyReferenceID.
	ID string

	// Key carries an exact producer identity for ApplyReferenceKey.
	Key string
}

// ParentChangeKind identifies an omitted, replacement, or clearing parent edit.
type ParentChangeKind uint8

const (
	// ParentUnchanged preserves the current parent.
	ParentUnchanged ParentChangeKind = iota

	// ParentReplace replaces the current parent with Reference.
	ParentReplace

	// ParentClear removes the current parent.
	ParentClear
)

// ApplyParentChange carries an explicit containment edit.
type ApplyParentChange struct {
	// Kind selects replacement or clearing behavior.
	Kind ParentChangeKind

	// Reference identifies the replacement parent for ParentReplace.
	Reference ApplyIssueReference
}

// ApplyIssue is caller input for one canonical document entry.
type ApplyIssue struct {
	// Alias identifies the entry only within this document when present.
	Alias *string

	// ID targets an existing issue when present.
	ID *string

	// Key targets or establishes a producer identity when present.
	Key *string

	// Title, Type, Priority, Summary, and Details replace values when present.
	Title    *string
	Type     *string
	Priority *int
	Summary  *string
	Details  *string

	// Labels replaces the complete label set when present.
	Labels *[]string

	// Parent carries explicit replacement or clearing behavior.
	Parent ApplyParentChange

	// DependsOn replaces the complete prerequisite set when present.
	DependsOn *[]ApplyIssueReference
}

// ApplyDocumentRequest supplies one versioned document and invocation mode.
type ApplyDocumentRequest struct {
	// Version must equal one.
	Version int

	// Issues contains entries in receipt order.
	Issues []ApplyIssue

	// OnExisting selects error, skip, or presence-aware update behavior.
	OnExisting ApplyExistingPolicy

	// Mode selects a dry run or atomic commit.
	Mode ApplyMode
}

// ApplyReceiptEntry reports one document entry in document order.
type ApplyReceiptEntry struct {
	// InputIndex is the zero-based document entry index.
	InputIndex int

	// Alias is populated when the entry supplied a document-local identity.
	Alias *string

	// ID is populated for an existing target or committed creation.
	ID *string

	// Key is populated when the entry supplied or resolved through a key.
	Key *string

	// Action reports the entry decision.
	Action ApplyAction
}

// ApplyCounts summarizes receipt actions.
type ApplyCounts struct {
	// Create is the number of created issues.
	Create int

	// Update is the number of changed existing issues.
	Update int

	// Skip is the number of existing issues preserved by policy.
	Skip int

	// NoChange is the number of updates already matching requested state.
	NoChange int
}

// ApplyReceipt is the deterministic caller-facing apply outcome.
type ApplyReceipt struct {
	// Entries contains one decision per document issue.
	Entries []ApplyReceiptEntry

	// Counts summarizes entry actions.
	Counts ApplyCounts

	// Revision identifies the canonical revision when durable state changed.
	Revision *int64

	// DryRun reports that no durable state was allocated or committed.
	DryRun bool
}

// ApplySnapshot contains the complete board state used by one apply decision.
type ApplySnapshot struct {
	// BoardID and Revision identify the retained repository snapshot.
	BoardID  board.ID
	Revision board.Revision

	// IssueIDs orders every selected-board issue deterministically.
	IssueIDs []issue.ID

	// Issues contains complete state and graph projections for every issue.
	Issues map[issue.ID]ApplyIssueSnapshot

	// ForeignIssueBoards identifies references owned by other boards.
	ForeignIssueBoards map[issue.ID]board.ID

	// ExternalKeys maps producer identities to selected-board issues.
	ExternalKeys map[ExternalKey]issue.ID

	// AllocatedIDs aligns commit-only identities with document entries.
	AllocatedIDs []issue.ID

	// OccurredAt timestamps committed issue projections.
	OccurredAt time.Time

	// Mode selects dry-run or commit materialization requirements.
	Mode ApplyMode
}

// ApplyIssueSnapshot is one issue's complete apply-owned projection.
type ApplyIssueSnapshot struct {
	// State contains immutable issue workflow and editable metadata state.
	State issue.State

	// Labels is the complete label set.
	Labels []issue.Label

	// Dependencies is the complete prerequisite set.
	Dependencies []issue.ID

	// Parent is the current containment parent, or nil when uncontained.
	Parent *issue.ID
}

// AppliedIssue is one finite projection write accepted by apply policy.
type AppliedIssue struct {
	// Issue is the complete post-change issue state.
	Issue issue.State

	// Existing distinguishes update from creation persistence.
	Existing bool

	// WriteIssue reports that the issue row must be inserted or updated.
	WriteIssue bool

	// Labels is the complete post-change label set.
	Labels []issue.Label

	// WriteLabels reports that Labels must replace the persisted set.
	WriteLabels bool

	// Dependencies is the complete post-change prerequisite set.
	Dependencies []issue.ID

	// WriteDependencies reports that Dependencies must replace the persisted set.
	WriteDependencies bool

	// Parent is the complete post-change containment parent.
	Parent *issue.ID

	// WriteParent reports that Parent must replace persisted containment.
	WriteParent bool

	// ExternalKey is a new producer-key binding to persist.
	ExternalKey *ExternalKey
}

// DocumentApplied is the semantic projection and receipt produced by policy.
type DocumentApplied struct {
	// Applied contains one persistence plan per document entry.
	Applied []AppliedIssue

	// Receipt is the deterministic caller-facing outcome.
	Receipt ApplyReceipt

	// CommittedRevision is populated after repository publication.
	CommittedRevision
}

// ApplyPolicy plans one document against a complete board snapshot.
type ApplyPolicy struct {
	snapshot ApplySnapshot
}

// LoadApply validates a complete repository snapshot and loads apply policy.
func LoadApply(snapshot ApplySnapshot) (*ApplyPolicy, error) {
	if err := validateSnapshot(snapshot.BoardID, snapshot.Revision); err != nil {
		return nil, ErrIncompleteSnapshot
	}
	if snapshot.Mode != ApplyModeCommit && snapshot.Mode != ApplyModeDryRun {
		return nil, ErrIncompleteSnapshot
	}
	if snapshot.Mode == ApplyModeCommit && snapshot.OccurredAt.IsZero() {
		return nil, ErrIncompleteSnapshot
	}
	if len(snapshot.IssueIDs) != len(snapshot.Issues) {
		return nil, ErrIncompleteSnapshot
	}
	for _, id := range snapshot.IssueIDs {
		value, ok := snapshot.Issues[id]
		if !ok || value.State.ID() != id {
			return nil, ErrIncompleteSnapshot
		}
	}
	return &ApplyPolicy{snapshot: snapshot}, nil
}

// ApplyDocument validates and plans one complete apply document.
func (p *ApplyPolicy) ApplyDocument(document ApplyDocument) (DocumentApplied, error) {
	if document.existing > ApplyExistingUpdate {
		return DocumentApplied{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: invalid existing policy %d",
			document.existing,
		)
	}
	if len(document.issues) == 0 {
		return DocumentApplied{}, errkind.Errorf(errkind.InvalidInput, "invalid input: apply document has no issues")
	}
	if p.snapshot.Mode == ApplyModeCommit && len(p.snapshot.AllocatedIDs) != len(document.issues) {
		return DocumentApplied{}, ErrIncompleteSnapshot
	}

	plan, err := p.plan(document)
	if err != nil {
		return DocumentApplied{}, err
	}
	if err := plan.validateGraph(); err != nil {
		return DocumentApplied{}, err
	}
	return p.materialize(document, plan)
}

// ApplyDocument is the normalized domain command owned by apply policy.
type ApplyDocument struct {
	issues   []applyDocumentIssue
	existing ApplyExistingPolicy
	// summaryMaxBytes is the effective summary limit for changed values.
	summaryMaxBytes configuration.ByteLimit
}

// ReferencedIssueIDs returns unique durable IDs named by document targets and
// graph references in their first document occurrence order.
func (d ApplyDocument) ReferencedIssueIDs() []issue.ID {
	seen := make(map[issue.ID]struct{})
	var ids []issue.ID
	appendID := func(id issue.ID) {
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	appendReference := func(reference applyReference) {
		if reference.kind == ApplyReferenceID {
			appendID(reference.id)
		}
	}
	for _, input := range d.issues {
		if input.id != nil {
			appendID(*input.id)
		}
		if input.parent.kind == ParentReplace {
			appendReference(input.parent.reference)
		}
		if input.dependsOn != nil {
			for _, dependency := range *input.dependsOn {
				appendReference(dependency)
			}
		}
	}
	return ids
}

type applyDocumentIssue struct {
	alias     *IssueAlias
	id        *issue.ID
	key       *ExternalKey
	title     *string
	kind      *issue.Kind
	priority  *issue.Priority
	summary   *string
	details   *string
	labels    *[]issue.Label
	parent    applyParentChange
	dependsOn *[]applyReference
}

type applyReference struct {
	kind  ApplyReferenceKind
	alias IssueAlias
	id    issue.ID
	key   ExternalKey
}

type applyParentChange struct {
	kind      ParentChangeKind
	reference applyReference
}

type applyPlan struct {
	nodes       []applyNode
	entries     []applyEntryPlan
	inputByNode map[int]int
}

type applyNode struct {
	id           issue.ID
	existing     bool
	state        issue.State
	title        string
	kind         issue.Kind
	lifecycle    issue.Lifecycle
	priority     issue.Priority
	summary      string
	details      string
	labels       []issue.Label
	dependencies []int
	parent       int
}

type applyEntryPlan struct {
	node              int
	action            ApplyAction
	bindKey           bool
	writeIssue        bool
	writeLabels       bool
	writeDependencies bool
	writeParent       bool
}

func (p *ApplyPolicy) plan(document ApplyDocument) (applyPlan, error) {
	plan := applyPlan{
		nodes:       make([]applyNode, 0, len(p.snapshot.IssueIDs)+len(document.issues)),
		entries:     make([]applyEntryPlan, len(document.issues)),
		inputByNode: make(map[int]int, len(document.issues)),
	}
	indexByID := make(map[issue.ID]int, len(p.snapshot.IssueIDs))
	for _, id := range p.snapshot.IssueIDs {
		current := p.snapshot.Issues[id]
		indexByID[id] = len(plan.nodes)
		plan.nodes = append(plan.nodes, applyNode{
			id: id, existing: true, state: current.State,
			title: current.State.Title(), kind: current.State.Kind(),
			lifecycle: current.State.Lifecycle(), priority: current.State.Priority(),
			summary: current.State.Summary(), details: current.State.Details(),
			labels: slices.Clone(current.Labels), parent: -1,
		})
	}
	for id, current := range p.snapshot.Issues {
		node, ok := indexByID[id]
		if !ok {
			return applyPlan{}, ErrIncompleteSnapshot
		}
		for _, dependencyID := range current.Dependencies {
			dependency, ok := indexByID[dependencyID]
			if !ok {
				return applyPlan{}, ErrIncompleteSnapshot
			}
			plan.nodes[node].dependencies = append(plan.nodes[node].dependencies, dependency)
		}
		if current.Parent != nil {
			parent, ok := indexByID[*current.Parent]
			if !ok {
				return applyPlan{}, ErrIncompleteSnapshot
			}
			plan.nodes[node].parent = parent
		}
	}

	aliases := make(map[IssueAlias]int, len(document.issues))
	documentKeys := make(map[ExternalKey]int, len(document.issues))
	targets := make(map[int]int, len(document.issues))
	for inputIndex, input := range document.issues {
		if input.alias != nil {
			if previous, ok := aliases[*input.alias]; ok {
				return applyPlan{}, errkind.Errorf(
					errkind.InvalidInput,
					"duplicate value: alias %q used by issues %d and %d",
					*input.alias,
					previous+1,
					inputIndex+1,
				)
			}
			aliases[*input.alias] = inputIndex
		}
		if input.key != nil {
			if previous, ok := documentKeys[*input.key]; ok {
				return applyPlan{}, errkind.Errorf(
					errkind.InvalidInput,
					"duplicate value: external key %q used by issues %d and %d",
					*input.key,
					previous+1,
					inputIndex+1,
				)
			}
			documentKeys[*input.key] = inputIndex
		}
		node, existing, bindKey, err := p.resolveTarget(input, indexByID)
		if err != nil {
			return applyPlan{}, fmt.Errorf("issue %d: %w", inputIndex+1, err)
		}
		if !existing {
			node = len(plan.nodes)
			allocatedID := issue.ID("")
			if p.snapshot.Mode == ApplyModeCommit {
				allocatedID = p.snapshot.AllocatedIDs[inputIndex]
				if allocatedID == "" {
					return applyPlan{}, ErrIncompleteSnapshot
				}
				if _, duplicate := indexByID[allocatedID]; duplicate {
					return applyPlan{}, errkind.Errorf(
						errkind.InvalidInput,
						"duplicate value: allocated issue ID %q exists",
						allocatedID,
					)
				}
				indexByID[allocatedID] = node
			}
			plan.nodes = append(plan.nodes, applyNode{
				id: allocatedID, kind: issue.KindTask, lifecycle: issue.LifecycleOpen,
				priority: issue.PriorityNormal, parent: -1,
			})
		}
		if previous, duplicate := targets[node]; duplicate {
			return applyPlan{}, errkind.Errorf(
				errkind.InvalidInput,
				"duplicate value: issues %d and %d target the same issue",
				previous+1,
				inputIndex+1,
			)
		}
		targets[node] = inputIndex
		plan.inputByNode[node] = inputIndex
		plan.entries[inputIndex] = applyEntryPlan{node: node, bindKey: bindKey}
	}

	aliasNodes := make(map[IssueAlias]int, len(aliases))
	for alias, inputIndex := range aliases {
		aliasNodes[alias] = plan.entries[inputIndex].node
	}
	keyNodes := make(map[ExternalKey]int, len(p.snapshot.ExternalKeys)+len(documentKeys))
	for key, id := range p.snapshot.ExternalKeys {
		node, ok := indexByID[id]
		if !ok {
			return applyPlan{}, ErrIncompleteSnapshot
		}
		keyNodes[key] = node
	}
	for key, inputIndex := range documentKeys {
		keyNodes[key] = plan.entries[inputIndex].node
	}

	for inputIndex, input := range document.issues {
		entry := &plan.entries[inputIndex]
		node := &plan.nodes[entry.node]
		if node.existing {
			switch document.existing {
			case ApplyExistingError:
				return applyPlan{}, errkind.Errorf(
					errkind.InvalidInput,
					"issue %d: existing issue %q rejected by on_existing error",
					inputIndex+1,
					node.id,
				)
			case ApplyExistingSkip:
				if entry.bindKey {
					return applyPlan{}, errkind.Errorf(
						errkind.InvalidInput,
						"issue %d: skip requires key %q to be bound to issue %q",
						inputIndex+1,
						*input.key,
						node.id,
					)
				}
				entry.action = ApplyActionSkip
				continue
			case ApplyExistingUpdate:
				if node.lifecycle != issue.LifecycleOpen || node.state.ActiveClaim() != nil {
					return applyPlan{}, errkind.Errorf(
						errkind.InvalidInput,
						"issue %d: existing issue %q must be open and unclaimed",
						inputIndex+1,
						node.id,
					)
				}
			}
		} else if input.title == nil || input.kind == nil {
			return applyPlan{}, errkind.Errorf(
				errkind.InvalidInput,
				"issue %d: new issue requires title and type",
				inputIndex+1,
			)
		}

		if err := p.applyMetadata(node, input, entry, document.summaryMaxBytes); err != nil {
			return applyPlan{}, fmt.Errorf("issue %d: %w", inputIndex+1, err)
		}
		if input.labels != nil {
			labels, err := uniqueLabels(*input.labels)
			if err != nil {
				return applyPlan{}, fmt.Errorf("issue %d: %w", inputIndex+1, err)
			}
			entry.writeLabels = !equalLabelSet(node.labels, labels)
			node.labels = labels
		} else if !node.existing {
			entry.writeLabels = true
		}
		if input.dependsOn != nil {
			dependencies, err := resolveReferences(
				*input.dependsOn,
				aliasNodes,
				indexByID,
				keyNodes,
				p.snapshot.ForeignIssueBoards,
			)
			if err != nil {
				return applyPlan{}, fmt.Errorf("issue %d dependencies: %w", inputIndex+1, err)
			}
			if slices.Contains(dependencies, entry.node) {
				return applyPlan{}, errkind.Errorf(errkind.InvalidInput, "an issue cannot be its own dependency")
			}
			for _, dependency := range dependencies {
				if plan.nodes[dependency].lifecycle == issue.LifecycleCancelled &&
					!slices.Contains(node.dependencies, dependency) {
					return applyPlan{}, errkind.Errorf(
						errkind.InvalidInput,
						"issue %d: new dependency cannot target cancelled issue %q",
						inputIndex+1,
						plan.nodes[dependency].id,
					)
				}
			}
			entry.writeDependencies = !equalIntSet(node.dependencies, dependencies)
			node.dependencies = dependencies
		} else if !node.existing {
			entry.writeDependencies = true
		}
		switch input.parent.kind {
		case ParentUnchanged:
			if !node.existing {
				entry.writeParent = true
			}
		case ParentClear:
			entry.writeParent = node.parent >= 0
			node.parent = -1
		case ParentReplace:
			parent, err := resolveReference(
				input.parent.reference,
				aliasNodes,
				indexByID,
				keyNodes,
				p.snapshot.ForeignIssueBoards,
			)
			if err != nil {
				return applyPlan{}, fmt.Errorf("issue %d parent: %w", inputIndex+1, err)
			}
			if parent == entry.node {
				return applyPlan{}, errkind.Errorf(errkind.InvalidInput, "containment would create a cycle")
			}
			entry.writeParent = node.parent != parent
			node.parent = parent
		default:
			return applyPlan{}, errkind.Errorf(
				errkind.InvalidInput,
				"issue %d: invalid parent change %d",
				inputIndex+1,
				input.parent.kind,
			)
		}

		if !node.existing {
			entry.action = ApplyActionCreate
			entry.writeIssue = true
			continue
		}
		if entry.bindKey || entry.writeIssue || entry.writeLabels ||
			entry.writeDependencies || entry.writeParent {
			entry.action = ApplyActionUpdate
		} else {
			entry.action = ApplyActionNoChange
		}
	}
	return plan, nil
}

func (p *ApplyPolicy) resolveTarget(
	input applyDocumentIssue,
	indexByID map[issue.ID]int,
) (node int, existing bool, bindKey bool, err error) {
	node = -1
	if input.id != nil {
		var ok bool
		node, ok = indexByID[*input.id]
		if !ok {
			if owner, foreign := p.snapshot.ForeignIssueBoards[*input.id]; foreign {
				return -1, false, false, errkind.Errorf(
					errkind.InvalidInput,
					"invalid input: issue ID %q belongs to board %q, not selected board %q",
					*input.id,
					owner,
					p.snapshot.BoardID,
				)
			}
			return -1, false, false, errkind.Errorf(
				errkind.NotFound,
				"issue not found: issue ID %q",
				*input.id,
			)
		}
		existing = true
	}
	if input.key == nil {
		return node, existing, false, nil
	}
	if id, ok := p.snapshot.ExternalKeys[*input.key]; ok {
		keyNode, complete := indexByID[id]
		if !complete {
			return -1, false, false, ErrIncompleteSnapshot
		}
		if existing && node != keyNode {
			return -1, false, false, errkind.Errorf(
				errkind.InvalidInput,
				"duplicate value: external key %q belongs to %q, not requested issue %q",
				*input.key,
				id,
				*input.id,
			)
		}
		return keyNode, true, false, nil
	}
	if existing {
		return node, true, true, nil
	}
	return -1, false, true, nil
}

func (p *ApplyPolicy) applyMetadata(
	node *applyNode,
	input applyDocumentIssue,
	entry *applyEntryPlan,
	summaryMaxBytes configuration.ByteLimit,
) error {
	if input.title != nil {
		title := strings.TrimSpace(*input.title)
		if title == "" {
			return errkind.Errorf(errkind.InvalidInput, "invalid input: title required")
		}
		entry.writeIssue = entry.writeIssue || node.title != title
		node.title = title
	}
	if input.kind != nil {
		if node.existing {
			if err := validateKindTransition(node.kind, *input.kind, nil); err != nil {
				return err
			}
		}
		entry.writeIssue = entry.writeIssue || node.kind != *input.kind
		node.kind = *input.kind
	}
	if input.priority != nil {
		entry.writeIssue = entry.writeIssue || node.priority != *input.priority
		node.priority = *input.priority
	}
	if input.summary != nil {
		if err := validateSummary(*input.summary, summaryMaxBytes); err != nil {
			return err
		}
		entry.writeIssue = entry.writeIssue || node.summary != *input.summary
		node.summary = *input.summary
	}
	if input.details != nil {
		entry.writeIssue = entry.writeIssue || node.details != *input.details
		node.details = *input.details
	}
	return nil
}

func (p applyPlan) validateGraph() error {
	if dependencyCycle(p.nodes) {
		return errkind.Errorf(errkind.InvalidInput, "dependency graph must remain acyclic")
	}
	if containmentCycle(p.nodes) {
		return errkind.Errorf(errkind.InvalidInput, "containment would create a cycle")
	}
	children := make([]int, len(p.nodes))
	for _, node := range p.nodes {
		if node.parent < 0 {
			continue
		}
		parent := p.nodes[node.parent]
		children[node.parent]++
		if node.lifecycle == issue.LifecycleOpen &&
			(parent.lifecycle != issue.LifecycleOpen ||
				parent.kind != issue.KindWorkstream && parent.kind != issue.KindRoutine) {
			return errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: open issue requires an open workstream or routine parent",
			)
		}
	}
	for node, childCount := range children {
		if childCount > 0 && p.nodes[node].kind == issue.KindTask {
			return errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: task issue %q cannot contain children",
				p.nodes[node].id,
			)
		}
	}
	return nil
}

func (p *ApplyPolicy) materialize(
	document ApplyDocument,
	plan applyPlan,
) (DocumentApplied, error) {
	out := DocumentApplied{
		Applied: make([]AppliedIssue, len(document.issues)),
		Receipt: ApplyReceipt{
			Entries: make([]ApplyReceiptEntry, len(document.issues)),
			DryRun:  p.snapshot.Mode == ApplyModeDryRun,
		},
	}
	for inputIndex, input := range document.issues {
		entry := plan.entries[inputIndex]
		node := plan.nodes[entry.node]
		receipt := ApplyReceiptEntry{InputIndex: inputIndex, Action: entry.action}
		if input.alias != nil {
			value := input.alias.String()
			receipt.Alias = &value
		}
		if node.id != "" {
			value := node.id.String()
			receipt.ID = &value
		}
		if input.key != nil {
			value := input.key.String()
			receipt.Key = &value
		}
		out.Receipt.Entries[inputIndex] = receipt
		switch entry.action {
		case ApplyActionCreate:
			out.Receipt.Counts.Create++
		case ApplyActionUpdate:
			out.Receipt.Counts.Update++
		case ApplyActionSkip:
			out.Receipt.Counts.Skip++
		case ApplyActionNoChange:
			out.Receipt.Counts.NoChange++
		default:
			return DocumentApplied{}, ErrIncompleteSnapshot
		}
		if p.snapshot.Mode == ApplyModeDryRun || entry.action == ApplyActionSkip ||
			entry.action == ApplyActionNoChange {
			continue
		}

		state, err := p.materializeState(node, entry)
		if err != nil {
			return DocumentApplied{}, fmt.Errorf("issue %d: %w", inputIndex+1, err)
		}
		applied := AppliedIssue{
			Issue: state, Existing: node.existing, WriteIssue: entry.writeIssue,
			Labels: slices.Clone(node.labels), WriteLabels: entry.writeLabels,
			Dependencies:      nodeDependencyIDs(plan.nodes, node.dependencies),
			WriteDependencies: entry.writeDependencies,
			Parent:            nodeParentID(plan.nodes, node.parent), WriteParent: entry.writeParent,
		}
		if entry.bindKey {
			key := *input.key
			applied.ExternalKey = &key
		}
		out.Applied[inputIndex] = applied
	}
	return out, nil
}

func (p *ApplyPolicy) materializeState(node applyNode, entry applyEntryPlan) (issue.State, error) {
	if node.existing {
		if !entry.writeIssue {
			return node.state, nil
		}
		snapshot := node.state.Snapshot()
		snapshot.Title = node.title
		snapshot.Kind = node.kind
		snapshot.Priority = node.priority
		snapshot.Summary = node.summary
		snapshot.Details = node.details
		snapshot.Updated = p.snapshot.OccurredAt
		return issue.Load(snapshot)
	}
	return issue.Load(issue.Snapshot{
		ID: node.id, Title: node.title, Kind: node.kind,
		Lifecycle: issue.LifecycleOpen, Priority: node.priority,
		Created: p.snapshot.OccurredAt, Updated: p.snapshot.OccurredAt,
		Summary: node.summary, Details: node.details,
	})
}

func resolveReferences(
	references []applyReference,
	aliases map[IssueAlias]int,
	ids map[issue.ID]int,
	keys map[ExternalKey]int,
	foreign map[issue.ID]board.ID,
) ([]int, error) {
	result := make([]int, 0, len(references))
	for _, reference := range references {
		node, err := resolveReference(reference, aliases, ids, keys, foreign)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(result, node) {
			result = append(result, node)
		}
	}
	return result, nil
}

func resolveReference(
	reference applyReference,
	aliases map[IssueAlias]int,
	ids map[issue.ID]int,
	keys map[ExternalKey]int,
	foreign map[issue.ID]board.ID,
) (int, error) {
	switch reference.kind {
	case ApplyReferenceAlias:
		if node, ok := aliases[reference.alias]; ok {
			return node, nil
		}
		return -1, errkind.Errorf(
			errkind.NotFound,
			"issue not found: alias %q",
			reference.alias,
		)
	case ApplyReferenceID:
		if node, ok := ids[reference.id]; ok {
			return node, nil
		}
		if owner, ok := foreign[reference.id]; ok {
			return -1, errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: issue ID %q belongs to board %q",
				reference.id,
				owner,
			)
		}
		return -1, errkind.Errorf(
			errkind.NotFound,
			"issue not found: issue ID %q",
			reference.id,
		)
	case ApplyReferenceKey:
		if node, ok := keys[reference.key]; ok {
			return node, nil
		}
		return -1, errkind.Errorf(
			errkind.NotFound,
			"issue not found: external key %q",
			reference.key,
		)
	default:
		return -1, errkind.Errorf(errkind.InvalidInput, "invalid input: issue reference required")
	}
}

func dependencyCycle(nodes []applyNode) bool {
	state := make([]uint8, len(nodes))
	var visit func(int) bool
	visit = func(node int) bool {
		state[node] = 1
		for _, dependency := range nodes[node].dependencies {
			if state[dependency] == 1 || state[dependency] == 0 && visit(dependency) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range nodes {
		if state[node] == 0 && visit(node) {
			return true
		}
	}
	return false
}

func containmentCycle(nodes []applyNode) bool {
	for start := range nodes {
		seen := make(map[int]struct{})
		for node := start; node >= 0; node = nodes[node].parent {
			if _, ok := seen[node]; ok {
				return true
			}
			seen[node] = struct{}{}
		}
	}
	return false
}

func equalLabelSet(left, right []issue.Label) bool {
	left = slices.Clone(left)
	right = slices.Clone(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func equalIntSet(left, right []int) bool {
	left = slices.Clone(left)
	right = slices.Clone(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func nodeDependencyIDs(nodes []applyNode, dependencies []int) []issue.ID {
	result := make([]issue.ID, len(dependencies))
	for index, dependency := range dependencies {
		result[index] = nodes[dependency].id
	}
	slices.Sort(result)
	return result
}

func nodeParentID(nodes []applyNode, parent int) *issue.ID {
	if parent < 0 {
		return nil
	}
	value := nodes[parent].id
	return &value
}

// ApplyDocument validates caller input and applies it through one repository operation.
func (p *Planner) ApplyDocument(
	ctx context.Context,
	_ issue.Invocation,
	req ApplyDocumentRequest,
) (ApplyReceipt, error) {
	configuration, err := p.resolveConfiguration(ctx)
	if err != nil {
		return ApplyReceipt{}, err
	}
	command, err := applyDocumentCommand(req, configuration.Issue.Summary.MaxBytes)
	if err != nil {
		return ApplyReceipt{}, err
	}
	out, err := p.changes.ApplyDocument(
		ctx,
		configuration.Issue.ID,
		command,
		req.Mode,
	)
	if err != nil {
		return ApplyReceipt{}, err
	}
	receipt := out.Receipt
	if out.Revision != 0 {
		revision := int64(out.Revision)
		receipt.Revision = &revision
	}
	return receipt, nil
}

func applyDocumentCommand(
	req ApplyDocumentRequest,
	summaryMaxBytes configuration.ByteLimit,
) (ApplyDocument, error) {
	if req.Version != 1 {
		return ApplyDocument{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: apply document version must equal 1",
		)
	}
	if req.Mode != ApplyModeCommit && req.Mode != ApplyModeDryRun {
		return ApplyDocument{}, errkind.Errorf(errkind.InvalidInput, "invalid input: apply mode required")
	}
	issues := make([]applyDocumentIssue, len(req.Issues))
	for index, input := range req.Issues {
		converted, err := applyIssueCommand(input)
		if err != nil {
			return ApplyDocument{}, fmt.Errorf("issue %d: %w", index+1, err)
		}
		issues[index] = converted
	}
	return ApplyDocument{
		issues: issues, existing: req.OnExisting, summaryMaxBytes: summaryMaxBytes,
	}, nil
}

func applyIssueCommand(input ApplyIssue) (applyDocumentIssue, error) {
	var out applyDocumentIssue
	var err error
	if input.Alias != nil {
		value, err := NewIssueAlias(*input.Alias)
		if err != nil {
			return out, err
		}
		out.alias = &value
	}
	if input.ID != nil {
		value, err := issue.NewID(*input.ID)
		if err != nil {
			return out, err
		}
		out.id = &value
	}
	if input.Key != nil {
		value, err := NewExternalKey(*input.Key)
		if err != nil {
			return out, err
		}
		out.key = &value
	}
	out.title = input.Title
	if input.Type != nil {
		value, err := issue.NewKind(*input.Type)
		if err != nil {
			return out, err
		}
		out.kind = &value
	}
	if input.Priority != nil {
		value, err := issue.NewPriority(*input.Priority)
		if err != nil {
			return out, err
		}
		out.priority = &value
	}
	out.summary = input.Summary
	out.details = input.Details
	if input.Labels != nil {
		values, err := labels(*input.Labels)
		if err != nil {
			return out, err
		}
		out.labels = &values
	}
	out.parent.kind = input.Parent.Kind
	if input.Parent.Kind == ParentReplace {
		out.parent.reference, err = applyReferenceCommand(input.Parent.Reference)
		if err != nil {
			return out, err
		}
	}
	if input.DependsOn != nil {
		values := make([]applyReference, len(*input.DependsOn))
		for index, reference := range *input.DependsOn {
			values[index], err = applyReferenceCommand(reference)
			if err != nil {
				return out, fmt.Errorf("dependency %d: %w", index+1, err)
			}
		}
		out.dependsOn = &values
	}
	return out, nil
}

func applyReferenceCommand(input ApplyIssueReference) (applyReference, error) {
	switch input.Kind {
	case ApplyReferenceAlias:
		value, err := NewIssueAlias(input.Alias)
		return applyReference{kind: input.Kind, alias: value}, err
	case ApplyReferenceID:
		value, err := issue.NewID(input.ID)
		return applyReference{kind: input.Kind, id: value}, err
	case ApplyReferenceKey:
		value, err := NewExternalKey(input.Key)
		return applyReference{kind: input.Kind, key: value}, err
	default:
		return applyReference{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: issue reference must select alias, id, or key",
		)
	}
}
