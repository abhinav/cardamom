package aggregate

import (
	"context"
	"time"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"google.golang.org/protobuf/proto"
)

const (
	sourceReconnectDelay         = time.Second
	sourceCatalogRefreshInterval = time.Minute
)

// monitorSource owns catalog refreshes and one upstream change connection for
// an aggregate source. Browser subscriptions consume its shared events rather
// than opening their own upstream streams.
func (s *Server) monitorSource(ctx context.Context, index int) {
	go s.monitorSourceCatalog(ctx, index)
	for ctx.Err() == nil {
		available := s.catalog.snapshot().sources[index].entry.GetHealth() ==
			v1.SourceHealth_SOURCE_HEALTH_HEALTHY
		if available || s.refreshSource(ctx, index) {
			stream, err := s.catalog.source(index).changes.WatchChanges(
				ctx,
				connect.NewRequest(&v1.WatchChangesRequest{
					Scope: &v1.BoardScope{Selection: &v1.BoardScope_AllBoards{
						AllBoards: &v1.AllBoards{},
					}},
				}),
			)
			if err == nil {
				s.consumeSourceChanges(index, stream)
			}
			if ctx.Err() != nil {
				return
			}
			s.publishSourceProblem(index, "change stream unavailable")
		}
		if !waitForSourceReconnect(ctx) {
			return
		}
	}
}

// consumeSourceChanges forwards issue and board notifications immediately.
// Catalog notifications belong to the slower catalog monitor because upstream
// changes conservatively include that resource even for ordinary issue edits.
func (s *Server) consumeSourceChanges(
	index int,
	stream *connect.ServerStreamForClient[v1.WatchChangesResponse],
) {
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
		value := proto.Clone(stream.Msg()).(*v1.WatchChangesResponse)
		value.Resources = removeWatchResource(
			value.GetResources(),
			v1.WatchResource_WATCH_RESOURCE_BOARD_CATALOG,
		)
		if len(value.GetResources()) == 0 && value.GetHealth() == nil {
			continue
		}
		snapshot := s.catalog.snapshot()
		ref := snapshot.sourceRef(index)
		value.Source = ref
		if value.GetHealth() != nil {
			value.Health.Source = proto.Clone(ref).(*v1.SourceRef)
		}
		audience := matchingChangeScopes
		if value.GetHealth() != nil {
			audience = allChangeScopes
		}
		s.changes.publish(snapshot, value, audience)
	}
}

// monitorSourceCatalog discovers infrequent project and board changes without
// fetching the complete source bootstrap for every ordinary issue update.
func (s *Server) monitorSourceCatalog(ctx context.Context, index int) {
	ticker := time.NewTicker(sourceCatalogRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshSource(ctx, index)
		}
	}
}

// refreshSource replaces one source's catalog as a unit. Invalid refreshes
// retain the last valid source snapshot and publish degraded health instead.
func (s *Server) refreshSource(ctx context.Context, index int) bool {
	state := probeSource(ctx, s.catalog.source(index))
	if state.bootstrap == nil {
		s.publishSourceProblem(index, state.entry.GetDiagnostic())
		return false
	}
	changed, err := s.catalog.replaceSource(index, state)
	if err != nil {
		s.publishSourceProblem(index, "source catalog is invalid")
		return false
	}
	if changed {
		s.publishCatalogChange(index)
	}
	return true
}

// probeSource obtains a complete catalog and its corresponding health from a
// single upstream bootstrap response.
func probeSource(ctx context.Context, value *source) sourceState {
	response, err := value.project.GetBootstrap(
		ctx, connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	if err != nil {
		return sourceState{entry: unavailableEntry(value.config.Alias, "source unavailable")}
	}
	if response == nil || response.Msg == nil {
		return sourceState{entry: unavailableEntry(value.config.Alias, "source returned no bootstrap")}
	}
	return sourceState{
		bootstrap: response.Msg,
		entry:     healthyEntry(value.config.Alias, response.Msg),
	}
}

// publishSourceProblem retains the last valid routes while changing the
// source's visible health and waking every browser scope.
func (s *Server) publishSourceProblem(index int, diagnostic string) {
	health := v1.SourceHealth_SOURCE_HEALTH_DEGRADED
	if s.catalog.snapshot().sources[index].entry.GetSource().GetStoreLineageId() == "" {
		health = v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE
	}
	changed, err := s.catalog.setSourceHealth(index, health, diagnostic)
	if err != nil || !changed {
		return
	}
	snapshot := s.catalog.snapshot()
	ref := snapshot.sourceRef(index)
	value := &v1.WatchChangesResponse{
		Source:    ref,
		Resources: []v1.WatchResource{v1.WatchResource_WATCH_RESOURCE_BOARD_CATALOG},
		Health: &v1.SourceHealthEvent{
			Source: proto.Clone(ref).(*v1.SourceRef), Health: health,
			Diagnostic: diagnostic,
		},
	}
	s.changes.publish(snapshot, value, allChangeScopes)
}

// publishCatalogChange notifies every browser scope only after the replacement
// snapshot is available to subsequent catalog reads.
func (s *Server) publishCatalogChange(index int) {
	snapshot := s.catalog.snapshot()
	ref := snapshot.sourceRef(index)
	s.changes.publish(snapshot, &v1.WatchChangesResponse{
		Source:    ref,
		Resources: []v1.WatchResource{v1.WatchResource_WATCH_RESOURCE_BOARD_CATALOG},
	}, allChangeScopes)
}

func removeWatchResource(
	resources []v1.WatchResource,
	remove v1.WatchResource,
) []v1.WatchResource {
	result := make([]v1.WatchResource, 0, len(resources))
	for _, resource := range resources {
		if resource != remove {
			result = append(result, resource)
		}
	}
	return result
}

func waitForSourceReconnect(ctx context.Context) bool {
	timer := time.NewTimer(sourceReconnectDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
