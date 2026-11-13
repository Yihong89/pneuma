// Package hub fans out events to connected clients and tracks which devices
// currently have pneuma open.
package hub

import (
	"slices"
	"sync"
)

// Event is something that happened that every client should know about.
type Event struct {
	Type string // "message", "delete", "chat", "chat_delete", "presence", ...
	Data []byte // JSON payload
	// Audience limits delivery to these stable device IDs; empty means
	// everyone.
	Audience []string
}

// Hub is a simple broadcaster. Slow subscribers drop events instead of
// blocking the publisher; clients resync when they reconnect.
type Hub struct {
	mu     sync.Mutex
	subs   map[chan Event]string // connection -> account ID
	active map[string]int        // account ID -> open connections
}

func New() *Hub {
	return &Hub{subs: make(map[chan Event]string), active: make(map[string]int)}
}

// Subscribe registers a connection from an account (its stable member ID).
// It returns a channel of events and a function to unsubscribe. changed
// reports whether the person just came online, so the caller can announce
// it.
func (h *Hub) Subscribe(id string) (events <-chan Event, unsubscribe func() (changed bool), changed bool) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = id
	h.active[id]++
	changed = h.active[id] == 1
	h.mu.Unlock()
	return ch, func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.drop(ch)
	}, changed
}

// drop removes a subscriber; h.mu must be held. It reports whether its
// account went offline as a result.
func (h *Hub) drop(ch chan Event) bool {
	id, ok := h.subs[ch]
	if !ok {
		return false
	}
	delete(h.subs, ch)
	close(ch)
	h.active[id]--
	if h.active[id] <= 0 {
		delete(h.active, id)
		return true
	}
	return false
}

// Active returns the account IDs with pneuma open, sorted.
func (h *Hub) Active() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.active))
	for d := range h.active {
		out = append(out, d)
	}
	slices.Sort(out)
	return out
}

// Publish sends e to every subscriber without blocking.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, id := range h.subs {
		if len(e.Audience) > 0 && !slices.Contains(e.Audience, id) {
			continue
		}
		select {
		case ch <- e:
		default:
			// Too slow: disconnect it so it resyncs on reconnect. Its person
			// may briefly look offline; the reconnect fixes that.
			h.drop(ch)
		}
	}
}
