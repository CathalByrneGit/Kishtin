package web

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// uiCSP allows only the app's own files. Inline styles are allowed because
// components set style attributes; inline scripts are not. Media from blob:
// URLs plays spoken replies fetched from the server's text-to-speech.
const uiCSP = "default-src 'self'; img-src 'self' blob: data:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; font-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// serveUI serves the built kitchen screens. Paths that are not files get
// index.html, so the app can handle them. Hashed assets are cached for a
// year; index.html is never cached, so an update shows straight away.
func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if info, err := fs.Stat(s.ui, name); err != nil || info.IsDir() {
		name = "index.html"
	}
	h := w.Header()
	h.Set("Content-Security-Policy", uiCSP)
	if strings.HasPrefix(name, "assets/") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-store")
	}
	http.ServeFileFS(w, r, s.ui, name)
}
