package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHub(t *testing.T) {
	h := NewHub()
	a, stopA := h.Subscribe()
	b, stopB := h.Subscribe()
	h.Publish(Event{Kind: KindRecord, ID: "r1"})
	assert.Equal(t, Event{Kind: KindRecord, ID: "r1"}, <-a)
	assert.Equal(t, Event{Kind: KindRecord, ID: "r1"}, <-b)

	stopB()
	stopB() // safe twice
	h.Publish(Event{Kind: KindLimit})
	assert.Equal(t, KindLimit, (<-a).Kind)
	_, open := <-b
	assert.False(t, open, "a stopped subscription is closed")

	for range 100 {
		h.Publish(Event{Kind: KindRecord}) // never blocks on a full buffer
	}
	stopA()

	var nilHub *Hub
	nilHub.Publish(Event{Kind: KindRecord}) // a nil hub is a no-op
}
