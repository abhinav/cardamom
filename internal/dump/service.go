package dump

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.abhg.dev/cardamom/internal/errkind"
)

//go:generate go tool mockgen -destination mocks_test.go -package dump -typed -write_package_comment=false . SnapshotReader,Publisher

// SnapshotReader returns the complete board state from one coherent read.
type SnapshotReader interface {
	// ReadDumpSnapshot returns one canonical board revision and all dump data
	// associated with that revision.
	ReadDumpSnapshot(context.Context) (BoardSnapshot, error)
}

// Service selects and renders deterministic dump artifacts from one coherent
// board snapshot.
type Service struct {
	reader      SnapshotReader
	attachments AttachmentReader
	provenance  Provenance
}

// ServiceConfig supplies the selected source and immutable artifact provenance.
type ServiceConfig struct {
	// Reader returns one coherent snapshot of the selected board.
	Reader SnapshotReader // required

	// Attachments supplies board-scoped metadata and verified content.
	Attachments AttachmentReader // required

	// Provenance identifies the selected project and board.
	Provenance Provenance // required
}

// NewService constructs deterministic rendering for one selected project and
// board.
func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.Reader == nil {
		return nil, errors.New("dump snapshot reader is required")
	}
	if cfg.Attachments == nil {
		return nil, errors.New("dump attachment reader is required")
	}
	if err := cfg.Provenance.validate(); err != nil {
		return nil, err
	}
	return &Service{
		reader: cfg.Reader, attachments: cfg.Attachments, provenance: cfg.Provenance,
	}, nil
}

// RenderRequest identifies the issues represented by one rendered artifact.
type RenderRequest struct {
	// Selection identifies the issues represented by the artifact.
	Selection Selection
}

// Render reads, selects, and renders one coherent deterministic artifact.
func (s *Service) Render(ctx context.Context, request RenderRequest) (RenderedDump, error) {
	snapshot, err := s.snapshot(ctx, request.Selection)
	if err != nil {
		return RenderedDump{}, err
	}
	snapshot = cloneMarkdownSnapshot(snapshot)
	records := collectMarkdownRecords(snapshot)
	snapshot, attachmentFiles, attachments, err := s.renderAttachments(ctx, snapshot, records)
	if err != nil {
		return RenderedDump{}, err
	}
	snapshot = rewriteReferenceRecords(snapshot, records, attachments)
	rendered, err := render(snapshot)
	if err != nil {
		return RenderedDump{}, fmt.Errorf("render dump: %w", err)
	}
	rendered.Files = append(rendered.Files, attachmentFiles...)
	slices.SortFunc(rendered.Files, func(a, b *GeneratedFile) int {
		return cmp.Compare(a.Path(), b.Path())
	})
	return rendered, nil
}

// snapshot reads one complete board revision and applies issue selection.
func (s *Service) snapshot(ctx context.Context, selection Selection) (Snapshot, error) {
	board, err := s.reader.ReadDumpSnapshot(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read dump snapshot: %w", err)
	}
	board = orderedBoardSnapshot(board)
	if board.BoardID != s.provenance.BoardID {
		return Snapshot{}, fmt.Errorf(
			"dump snapshot board %q does not match selected board %q",
			board.BoardID, s.provenance.BoardID,
		)
	}

	normalized, selected, err := normalizeSelection(selection, board)
	if err != nil {
		return Snapshot{}, err
	}
	return selectSnapshot(board, s.provenance, normalized, selected), nil
}

// SelectionMode identifies how a dump chooses issues.
type SelectionMode int

const (
	_ SelectionMode = iota

	// SelectionWholeBoard includes every issue in the selected board.
	SelectionWholeBoard

	// SelectionIssues includes named issues with the configured descendant
	// policy.
	SelectionIssues
)

// String returns the stable selection name used by command output.
func (m SelectionMode) String() string {
	switch m {
	case SelectionWholeBoard:
		return "whole-board"
	case SelectionIssues:
		return "issues"
	default:
		return fmt.Sprintf("selection-mode(%d)", m)
	}
}

// DescendantSelection controls containment expansion for selected issue roots.
type DescendantSelection int

const (
	// ExcludeDescendants selects only the named issue roots.
	ExcludeDescendants DescendantSelection = iota
	// IncludeDescendants selects each named root and its transitive containment
	// descendants.
	IncludeDescendants
)

// Selection describes one issue selection. Use WholeBoard, SelectedIssues, or
// NamedIssuesOnly to construct it.
type Selection struct {
	// Mode identifies whether the selection covers the whole board or named
	// issue roots.
	Mode SelectionMode

	// IssueIDs contains normalized roots only for SelectionIssues.
	IssueIDs []string

	// Descendants records whether SelectionIssues expands containment descendants.
	Descendants DescendantSelection
}

// WholeBoard selects every issue.
func WholeBoard() Selection { return Selection{Mode: SelectionWholeBoard} }

// SelectedIssues selects each named issue and its transitive containment descendants.
func SelectedIssues(issueIDs ...string) Selection {
	return Selection{
		Mode: SelectionIssues, IssueIDs: slices.Clone(issueIDs),
		Descendants: IncludeDescendants,
	}
}

// NamedIssuesOnly selects only the named issues without containment expansion.
func NamedIssuesOnly(issueIDs ...string) Selection {
	return Selection{Mode: SelectionIssues, IssueIDs: slices.Clone(issueIDs)}
}

// UnknownIssuesError reports every normalized selector ID absent from the
// source board.
type UnknownIssuesError struct {
	// IssueIDs is sorted and contains no duplicates.
	IssueIDs []string
}

// Error reports the unknown issue IDs in deterministic order.
func (e *UnknownIssuesError) Error() string {
	quoted := make([]string, len(e.IssueIDs))
	for index, issueID := range e.IssueIDs {
		quoted[index] = strconv.Quote(issueID)
	}
	if len(quoted) == 1 {
		return "unknown issue ID " + quoted[0]
	}
	return "unknown issue IDs: " + strings.Join(quoted, ", ")
}

func normalizeSelection(selection Selection, board BoardSnapshot) (Selection, map[string]struct{}, error) {
	known := make(map[string]struct{}, len(board.Issues))
	for _, issue := range board.Issues {
		known[issue.ID] = struct{}{}
	}

	switch selection.Mode {
	case SelectionWholeBoard:
		return WholeBoard(), known, nil
	case SelectionIssues:
		ids := slices.Clone(selection.IssueIDs)
		slices.Sort(ids)
		ids = slices.Compact(ids)
		var unknown []string
		for _, issueID := range ids {
			if _, ok := known[issueID]; !ok {
				unknown = append(unknown, issueID)
			}
		}
		if len(unknown) > 0 {
			return Selection{}, nil, errkind.Wrap(
				errkind.NotFound,
				&UnknownIssuesError{IssueIDs: unknown},
			)
		}
		if selection.Descendants == ExcludeDescendants {
			selected := make(map[string]struct{}, len(ids))
			for _, issueID := range ids {
				selected[issueID] = struct{}{}
			}
			return NamedIssuesOnly(ids...), selected, nil
		}
		if selection.Descendants != IncludeDescendants {
			return Selection{}, nil, errkind.Errorf(
				errkind.InvalidInput,
				"unsupported descendant selection %d",
				selection.Descendants,
			)
		}
		normalized, selected := normalizeSelectedDescendants(ids, board.Containment)
		return normalized, selected, nil
	default:
		return Selection{}, nil, errkind.Errorf(
			errkind.InvalidInput,
			"unsupported dump selection mode %d",
			selection.Mode,
		)
	}
}

func normalizeSelectedDescendants(roots []string, containment []Containment) (Selection, map[string]struct{}) {
	selected := make(map[string]struct{})
	coveredRoots := make(map[string]struct{})
	for _, root := range roots {
		descendants := containmentClosure(root, containment)
		for issueID := range descendants {
			selected[issueID] = struct{}{}
		}
		for _, candidate := range roots {
			if candidate != root && contains(descendants, candidate) {
				coveredRoots[candidate] = struct{}{}
			}
		}
	}

	normalizedRoots := make([]string, 0, len(roots)-len(coveredRoots))
	for _, root := range roots {
		if !contains(coveredRoots, root) {
			normalizedRoots = append(normalizedRoots, root)
		}
	}
	return SelectedIssues(normalizedRoots...), selected
}

// containmentClosure returns one root and every issue reachable through
// containment child edges.
func containmentClosure(root string, containment []Containment) map[string]struct{} {
	selected := map[string]struct{}{root: {}}
	for added := true; added; {
		added = false
		for _, edge := range containment {
			if !contains(selected, edge.ParentID) || contains(selected, edge.ChildID) {
				continue
			}
			selected[edge.ChildID] = struct{}{}
			added = true
		}
	}
	return selected
}

func selectSnapshot(
	board BoardSnapshot,
	provenance Provenance,
	selection Selection,
	selected map[string]struct{},
) Snapshot {
	result := Snapshot{
		BoardID: board.BoardID, Revision: board.Revision,
		Description:      board.Description,
		Provenance:       provenance,
		Selection:        selection,
		referenceTargets: newReferenceTargets(board, selected),
	}
	for _, issue := range board.Issues {
		if contains(selected, issue.ID) {
			result.Issues = append(result.Issues, issue)
		}
	}
	for _, edge := range board.Dependencies {
		if contains(selected, edge.ChildID) || contains(selected, edge.ParentID) {
			result.Dependencies = append(result.Dependencies, edge)
		}
	}
	for _, edge := range board.Containment {
		if contains(selected, edge.ChildID) || contains(selected, edge.ParentID) {
			result.Containment = append(result.Containment, edge)
		}
	}
	for _, item := range board.Results {
		if contains(selected, item.IssueID) {
			result.Results = append(result.Results, item)
		}
	}
	for _, item := range board.LogEntries {
		if contains(selected, item.IssueID) {
			result.LogEntries = append(result.LogEntries, item)
		}
	}
	result.Issues = nonNil(result.Issues)
	result.Dependencies = nonNil(result.Dependencies)
	result.Containment = nonNil(result.Containment)
	result.Results = nonNil(result.Results)
	result.LogEntries = nonNil(result.LogEntries)
	return result
}

func orderedBoardSnapshot(board BoardSnapshot) BoardSnapshot {
	result := board
	result.Issues = slices.Clone(board.Issues)
	issuesByID := make(map[string]Issue, len(result.Issues))
	for index := range result.Issues {
		result.Issues[index].Labels = slices.Clone(result.Issues[index].Labels)
		slices.Sort(result.Issues[index].Labels)
		issuesByID[result.Issues[index].ID] = result.Issues[index]
	}
	result.Dependencies = slices.Clone(board.Dependencies)
	result.Containment = slices.Clone(board.Containment)
	result.Results = slices.Clone(board.Results)
	result.LogEntries = slices.Clone(board.LogEntries)
	slices.SortFunc(result.Issues, compareIssueCreation)
	compareID := func(left, right string) int {
		return compareIssueCreation(issuesByID[left], issuesByID[right])
	}
	slices.SortFunc(result.Dependencies, func(a, b Dependency) int {
		if order := compareID(a.ChildID, b.ChildID); order != 0 {
			return order
		}
		return compareID(a.ParentID, b.ParentID)
	})
	slices.SortFunc(result.Containment, func(a, b Containment) int {
		if order := compareID(a.ChildID, b.ChildID); order != 0 {
			return order
		}
		return compareID(a.ParentID, b.ParentID)
	})
	slices.SortFunc(result.Results, func(a, b Result) int {
		return compareID(a.IssueID, b.IssueID)
	})
	return result
}

func compareIssueCreation(left, right Issue) int {
	if order := cmp.Compare(left.Created, right.Created); order != 0 {
		return order
	}
	return cmp.Compare(left.ID, right.ID)
}

func contains(values map[string]struct{}, value string) bool {
	_, ok := values[value]
	return ok
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
