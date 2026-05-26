package http

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"sync"
	"time"
)

// broker fans out "something changed" events from a single background
// poll loop to every connected SSE subscriber. SQLite has no cross-
// process pub/sub, so the only way to detect a `clu close` run in
// another terminal is to poll — but we poll once on the server and
// fan out to N clients rather than letting each client poll
// independently.
//
// Events are intentionally coarse-grained. A single "issues-changed"
// signal triggers TanStack Query refetches on the client; the
// refetch is cheap (the user's already on a view that needs the
// data) and avoids the bookkeeping of fine-grained event types.
type broker struct {
	mu       sync.Mutex
	subs     map[chan event]struct{}
	lastSeen int64

	// pollInterval is exposed for tests; production uses 1s.
	pollInterval time.Duration
}

type event struct {
	Type string
	Data string
}

func newBroker() *broker {
	return &broker{
		subs:         make(map[chan event]struct{}),
		pollInterval: time.Second,
	}
}

// start launches the poll-and-fanout goroutine. Returns when ctx is
// cancelled. Safe to call once per Server lifetime; subsequent calls
// would race the subscriber map.
func (b *broker) start(ctx context.Context, lookup func(context.Context) (int64, error)) {
	// Seed lastSeen so the first poll doesn't fire a phantom event
	// against a fresh broker.
	if v, err := lookup(ctx); err == nil {
		b.mu.Lock()
		b.lastSeen = v
		b.mu.Unlock()
	}

	go func() {
		ticker := time.NewTicker(b.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.tick(ctx, lookup)
			}
		}
	}()
}

// tick is one poll cycle: ask the store for the watermark, fire an
// event if it advanced. Errors are swallowed — a transient DB hiccup
// shouldn't kill the broker. Next tick will retry.
func (b *broker) tick(ctx context.Context, lookup func(context.Context) (int64, error)) {
	v, err := lookup(ctx)
	if err != nil {
		return
	}
	b.mu.Lock()
	advanced := v > b.lastSeen
	if advanced {
		b.lastSeen = v
	}
	b.mu.Unlock()
	if advanced {
		b.publish(event{
			Type: "issues-changed",
			Data: fmt.Sprintf(`{"max_updated":%d}`, v),
		})
	}
}

// subscribe returns a buffered channel of events plus an unsubscribe
// func. Buffer size 4 is enough for any realistic burst; if a client
// is slow enough to overflow, publish drops the event (the next one
// still triggers a refetch, so worst case is one missed wake-up).
func (b *broker) subscribe() (chan event, func()) {
	ch := make(chan event, 4)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
		close(ch)
	}
}

// publish sends e to every subscriber non-blockingly. A full channel
// is the subscriber's problem — we drop rather than block the poll
// loop or other subscribers.
func (b *broker) publish(e event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
			// drop; see comment above
		}
	}
}

// handleEvents serves text/event-stream. The browser's EventSource
// handles reconnect automatically — we don't track resume tokens
// because the client always refetches on connect anyway.
func (s *Server) handleEvents(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	flusher, ok := w.(stdhttp.Flusher)
	if !ok {
		writeError(w, stdhttp.StatusInternalServerError, "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Some reverse proxies (nginx) buffer SSE by default; this
	// header tells them not to. Harmless when no proxy is present.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(stdhttp.StatusOK)

	ch, unsub := s.broker.subscribe()
	defer unsub()

	// Synthetic "ready" event so the client knows the stream is up
	// without waiting for the first real change. Lets the UI flip
	// from "polling" to "live" status indicators if desired.
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()

	// SSE comment heartbeat — keeps idle connections alive through
	// middleboxes (default nginx/cloudflare cuts at ~60s). Comments
	// (lines starting with `:`) don't trigger client event handlers.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case e, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, e.Data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
