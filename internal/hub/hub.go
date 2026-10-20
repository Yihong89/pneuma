// Package hub fans out events to connected clients.
package hub

import "sync"

// Event is something that happened that every client should know about.
type Event struct {
	Type string // "message" or "delete"
	Data []byte // JSON payload
}

// Hub is a simple broadcaster. Slow subscribers drop events instead of
// blocking the publisher; clients resync when they reconnect.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func New() *Hub { return &Hub{subs: make(map[chan Event]struct{})} }

// Subscribe returns a channel of events and a function to unsubscribe.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Publish sends e to every subscriber without blocking.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
			// Subscriber is too slow: disconnect it so it resyncs.
			delete(h.subs, ch)
			close(ch)
		}
	}
}
