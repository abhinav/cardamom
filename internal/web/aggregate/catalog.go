package aggregate

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"google.golang.org/protobuf/proto"
)

// catalog serializes source updates and publishes immutable snapshots so a
// reader never combines routes and source identity from different revisions.
type catalog struct {
	mu      sync.RWMutex
	sources []source
	states  []sourceState
	current *catalogSnapshot
}

// sourceState keeps one source's bootstrap and derived status entry from the
// same probe. A failed live probe changes only health so the last valid
// bootstrap can continue supplying stable routes.
type sourceState struct {
	bootstrap *v1.GetBootstrapResponse
	entry     *v1.SourceCatalogEntry
}

// catalogSnapshot is one coherent read generation of source identity,
// presentation catalogs, and board routing.
type catalogSnapshot struct {
	sources   []catalogSource
	projects  []*v1.Project
	boards    []*v1.BoardSummary
	boardByID map[string][]boardRoute
}

// catalogSource joins one immutable status entry to its process-lifetime source.
type catalogSource struct {
	source *source
	entry  *v1.SourceCatalogEntry
}

func newCatalog(sources []source, states []sourceState) (*catalog, error) {
	result := &catalog{
		sources: sources,
		states:  cloneSourceStates(states),
	}
	current, err := composeCatalog(result.sources, result.states)
	if err != nil {
		return nil, err
	}
	result.current = current
	return result, nil
}

func (c *catalog) snapshot() *catalogSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

// source returns an immutable process-lifetime source at its configured index.
// Source ordering is fixed before catalog construction.
func (c *catalog) source(index int) *source {
	return &c.sources[index]
}

// replaceSource validates and publishes a complete candidate snapshot.
func (c *catalog) replaceSource(index int, state sourceState) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.publishSource(index, state)
}

// setSourceHealth publishes health without discarding the source's last valid
// bootstrap and routes.
func (c *catalog) setSourceHealth(
	index int,
	health v1.SourceHealth,
	diagnostic string,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.states) {
		return false, fmt.Errorf("source index %d is out of range", index)
	}
	state := cloneSourceState(c.states[index])
	if state.entry == nil {
		state.entry = unavailableEntry(c.sources[index].config.Alias, diagnostic)
	}
	state.entry.Health = health
	state.entry.Diagnostic = diagnostic
	return c.publishSource(index, state)
}

// publishSource replaces one source state and its complete derived snapshot.
// Its caller holds the catalog write lock throughout candidate construction.
func (c *catalog) publishSource(index int, state sourceState) (bool, error) {
	if index < 0 || index >= len(c.states) {
		return false, fmt.Errorf("source index %d is out of range", index)
	}
	state = cloneSourceState(state)
	if sourceStatesEqual(c.states[index], state) {
		return false, nil
	}
	states := slices.Clone(c.states)
	states[index] = state
	current, err := composeCatalog(c.sources, states)
	if err != nil {
		return false, err
	}
	c.states = states
	c.current = current
	return true, nil
}

func composeCatalog(sources []source, states []sourceState) (*catalogSnapshot, error) {
	result := &catalogSnapshot{
		sources:   make([]catalogSource, 0, len(sources)),
		boardByID: make(map[string][]boardRoute),
	}
	lineages := make(map[string]string)
	for index := range sources {
		state := states[index]
		entry := proto.Clone(state.entry).(*v1.SourceCatalogEntry)
		result.sources = append(result.sources, catalogSource{
			source: &sources[index], entry: entry,
		})
		if state.bootstrap == nil {
			continue
		}
		ref := entry.GetSource()
		if lineage := ref.GetStoreLineageId(); lineage != "" {
			if prior, ok := lineages[lineage]; ok && prior != ref.GetSourceId() {
				return nil, fmt.Errorf(
					"store lineage %q is configured under sources %q and %q",
					lineage,
					prior,
					ref.GetSourceId(),
				)
			}
			lineages[lineage] = ref.GetSourceId()
		}
		for _, project := range state.bootstrap.GetProjects() {
			projectCopy := proto.Clone(project).(*v1.Project)
			projectCopy.Source = proto.Clone(ref).(*v1.SourceRef)
			result.projects = append(result.projects, projectCopy)
		}
		for _, board := range state.bootstrap.GetBoards() {
			if board.GetId() == "" {
				return nil, fmt.Errorf(
					"source %q returned a board without an ID",
					ref.GetSourceId(),
				)
			}
			boardCopy := proto.Clone(board).(*v1.BoardSummary)
			boardCopy.Source = proto.Clone(ref).(*v1.SourceRef)
			result.boards = append(result.boards, boardCopy)
			result.boardByID[board.GetId()] = append(
				result.boardByID[board.GetId()],
				boardRoute{
					source:  &sources[index],
					ref:     proto.Clone(ref).(*v1.SourceRef),
					boardID: board.GetId(),
				},
			)
		}
	}
	slices.SortFunc(result.projects, func(left, right *v1.Project) int {
		if result := strings.Compare(left.GetSource().GetSourceId(), right.GetSource().GetSourceId()); result != 0 {
			return result
		}
		return strings.Compare(left.GetId(), right.GetId())
	})
	slices.SortFunc(result.boards, func(left, right *v1.BoardSummary) int {
		if result := strings.Compare(left.GetSource().GetSourceId(), right.GetSource().GetSourceId()); result != 0 {
			return result
		}
		return strings.Compare(left.GetId(), right.GetId())
	})
	return result, nil
}

func cloneSourceStates(values []sourceState) []sourceState {
	result := make([]sourceState, len(values))
	for index, value := range values {
		result[index] = cloneSourceState(value)
	}
	return result
}

func cloneSourceState(value sourceState) sourceState {
	result := sourceState{}
	if value.bootstrap != nil {
		result.bootstrap = proto.Clone(value.bootstrap).(*v1.GetBootstrapResponse)
	}
	if value.entry != nil {
		result.entry = proto.Clone(value.entry).(*v1.SourceCatalogEntry)
	}
	return result
}

func sourceStatesEqual(left, right sourceState) bool {
	return proto.Equal(left.bootstrap, right.bootstrap) &&
		proto.Equal(left.entry, right.entry)
}

func healthyEntry(alias string, bootstrap *v1.GetBootstrapResponse) *v1.SourceCatalogEntry {
	ref := sourceRef(alias, bootstrap)
	return &v1.SourceCatalogEntry{
		Source:        ref,
		Health:        v1.SourceHealth_SOURCE_HEALTH_HEALTHY,
		Version:       bootstrap.GetVersion(),
		SchemaVersion: bootstrap.GetSchemaVersion(),
		ReadOnly:      bootstrap.GetAccessMode() == v1.AccessMode_ACCESS_MODE_READ_ONLY,
	}
}

func unavailableEntry(alias, diagnostic string) *v1.SourceCatalogEntry {
	return &v1.SourceCatalogEntry{
		Source:     &v1.SourceRef{SourceId: alias},
		Health:     v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE,
		Diagnostic: diagnostic,
		ReadOnly:   false,
	}
}

func sourceRef(alias string, bootstrap *v1.GetBootstrapResponse) *v1.SourceRef {
	if sources := bootstrap.GetSources(); len(sources) > 0 && sources[0].GetSource() != nil {
		ref := proto.Clone(sources[0].GetSource()).(*v1.SourceRef)
		ref.SourceId = alias
		return ref
	}
	return &v1.SourceRef{SourceId: alias}
}
