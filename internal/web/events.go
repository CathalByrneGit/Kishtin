package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// pingEvery keeps idle connections open through proxies and lets a closed
// session end the stream.
var pingEvery = 20 * time.Second

// serveEvents streams change notices as Server-Sent Events. Browsers read
// it with fetch (EventSource cannot send the API key header). Each event
// is {"kind": "record"|"limit"|"settings"|"staff", "id": "…"}; screens
// refetch what they show.
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	key, session := bearer(r.Header.Get("Authorization")), r.Header.Get(SessionHeader)
	c, err := s.svc.Authenticate(r.Context(), key, session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	hub := s.svc.Events()
	if hub == nil {
		http.Error(w, "Live updates are not available on this server.", http.StatusServiceUnavailable)
		return
	}
	rc := http.NewResponseController(w)
	// The server's write timeout would cut the stream; this connection
	// stays open until the client leaves or the server stops.
	_ = rc.SetWriteDeadline(time.Time{})

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")

	ch, stop := hub.Subscribe()
	defer stop()
	send := func(format string, args ...any) bool {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send("event: ready\ndata: {}\n\n") {
		return
	}
	// Screens connecting and dropping off are kept in the activity log.
	defer s.svc.ScreenConnected(r.Context(), c)()
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := s.svc.Authenticate(r.Context(), key, session); err != nil {
				send("event: ended\ndata: %q\n\n", err.Error())
				return
			}
			if !send(": ping\n\n") {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			if !send("event: change\ndata: %s\n\n", b) {
				return
			}
		}
	}
}
