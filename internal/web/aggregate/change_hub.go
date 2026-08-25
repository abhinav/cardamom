package aggregate

import (
	"sync"

	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"google.golang.org/protobuf/proto"
)

// aggregateChangeBuffer absorbs one ordinary multi-board source revision.
// Larger bursts disconnect a slow browser so its reconnect refreshes current
// state rather than allowing an unbounded process queue.
const aggregateChangeBuffer = 256

type changeAudience uint8

const (
	matchingChangeScopes changeAudience = iota
	allChangeScopes
)

// changeHub fans process-owned source events out to browser subscriptions.
// A subscriber that cannot keep up is disconnected so its browser reconnects
// and refreshes active reads from the current catalog snapshot.
type changeHub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[uint64]*changeSubscription
}

type changeSubscription struct {
	id      uint64
	hub     *changeHub
	scope   *v1.BoardScope
	changes chan *v1.WatchChangesResponse
	once    sync.Once
}

func newChangeHub() *changeHub {
	return &changeHub{subscribers: make(map[uint64]*changeSubscription)}
}

func (h *changeHub) subscribe(scope *v1.BoardScope) *changeSubscription {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	subscription := &changeSubscription{
		id:      h.nextID,
		hub:     h,
		scope:   proto.Clone(scope).(*v1.BoardScope),
		changes: make(chan *v1.WatchChangesResponse, aggregateChangeBuffer),
	}
	h.subscribers[subscription.id] = subscription
	return subscription
}

func (h *changeHub) publish(
	snapshot *catalogSnapshot,
	value *v1.WatchChangesResponse,
	audience changeAudience,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, subscription := range h.subscribers {
		if audience == matchingChangeScopes &&
			!snapshot.matchesChange(subscription.scope, value) {
			continue
		}
		select {
		case subscription.changes <- proto.Clone(value).(*v1.WatchChangesResponse):
		default:
			delete(h.subscribers, id)
			close(subscription.changes)
		}
	}
}

func (s *changeSubscription) close() {
	s.once.Do(func() {
		s.hub.mu.Lock()
		defer s.hub.mu.Unlock()
		if _, exists := s.hub.subscribers[s.id]; !exists {
			return
		}
		delete(s.hub.subscribers, s.id)
		close(s.changes)
	})
}
