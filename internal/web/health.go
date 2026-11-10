package web

import (
	"encoding/json"
	"net/http"
)

// AppVersion is the Kishtin release serving, set by kishtind at start.
var AppVersion = "dev"

// serveHealth answers "is Kishtin up, and can it read its database?" for
// monitoring and scripts. It needs no key and says nothing about the
// kitchen: only that Kishtin is running, and which version.
func (s *Server) serveHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	out := map[string]string{"status": "ok", "version": AppVersion}
	if _, err := s.svc.SetupNeeded(r.Context()); err != nil {
		out["status"] = "database unavailable"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(out)
}
