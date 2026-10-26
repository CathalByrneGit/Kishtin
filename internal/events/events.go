// Package events broadcasts changes to every open screen, so devices see
// new records without reloading.
package events

import "sync"

// Event kinds.
const (
	KindRecord   = "record"
	KindLimit    = "limit"
	KindSettings = "settings"
	KindStaff    = "staff"
	KindMenu     = "menu"
	KindOrder    = "order"
)

// Event says something changed. Screens refetch what they show; the event
// carries no record content.
type Event struct {
	Kind string `json:"kind"`
	// ID is the changed record or limit version, or the settings group.
	ID string `json:"id,omitempty"`
}

// Hub fans events out to subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

// Subscribe returns a channel of events and a function that ends the
// subscription.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
}

// Publish sends e to every subscriber. A subscriber whose buffer is full
// misses the event; it already has events waiting that make it refetch.
func (h *Hub) Publish(e Event) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
