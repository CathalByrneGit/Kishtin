// Package web serves Kishtin's HTTP API. Every route calls the same
// service function the CLI uses.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

// Version is the API version reported in the OpenAPI spec.
const Version = "0.1.0"

// SessionHeader carries a staff session token from a kitchen device.
const SessionHeader = "X-Kishtin-Session"

// Server is the HTTP API.
type Server struct {
	svc      *service.Service
	log      *slog.Logger
	api      huma.API
	mux      *http.ServeMux
	specJSON []byte
	ui       fs.FS
}

type callerKey struct{}

func init() {
	// Kishtin always sends empty lists as [], never null, so the spec says
	// so and clients need not handle null arrays.
	huma.DefaultArrayNullable = false
}

// Config returns the Huma configuration for the Kishtin API. Docs pages
// and schema links are off: they load scripts from the internet, and
// Kishtin works offline.
func Config() huma.Config {
	cfg := huma.DefaultConfig("Kishtin API", Version)
	cfg.DocsPath = ""
	cfg.OpenAPIPath = ""
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil
	cfg.Info.Description = "Kishtin kitchen operations API. Every request needs an API key: " +
		"`Authorization: Bearer kishtin_key_…`. Kitchen devices also send a staff session in " + SessionHeader + "."
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"apiKey":  {Type: "http", Scheme: "bearer", Description: "A Kishtin API key."},
		"session": {Type: "apiKey", In: "header", Name: SessionHeader, Description: "A staff session from PIN or password login on a device."},
	}
	cfg.Security = []map[string][]string{{"apiKey": {}}, {"apiKey": {}, "session": {}}}
	return cfg
}

// New builds the server. ui is the built kitchen screens; nil serves a
// short page saying the screens are not built.
func New(svc *service.Service, log *slog.Logger, ui fs.FS) (*Server, error) {
	mux := http.NewServeMux()
	s := &Server{svc: svc, log: log, mux: mux, ui: ui}
	s.api = humago.NewWithPrefix(mux, "/api", Config())
	s.api.UseMiddleware(s.authenticate)
	s.routes()

	spec, err := s.api.OpenAPI().MarshalJSON()
	if err != nil {
		return nil, err
	}
	s.specJSON = spec
	mux.HandleFunc("GET /api/openapi.json", s.serveSpec)
	mux.HandleFunc("GET /api/events", s.serveEvents)
	if ui != nil {
		mux.HandleFunc("GET /", s.serveUI)
	} else {
		mux.HandleFunc("GET /{$}", serveHome)
	}
	return s, nil
}

// OpenAPI returns the API description, for generating clients.
func (s *Server) OpenAPI() *huma.OpenAPI { return s.api.OpenAPI() }

// ServeHTTP serves the API.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	s.mux.ServeHTTP(w, r)
}

func bearer(h string) string {
	if v, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (s *Server) authenticate(ctx huma.Context, next func(huma.Context)) {
	c, err := s.svc.Authenticate(ctx.Context(), bearer(ctx.Header("Authorization")), ctx.Header(SessionHeader))
	if err != nil {
		s.writeErr(ctx, err)
		return
	}
	next(huma.WithValue(ctx, callerKey{}, c))
}

func (s *Server) serveSpec(w http.ResponseWriter, r *http.Request) {
	if _, err := s.svc.Authenticate(r.Context(), bearer(r.Header.Get("Authorization")), ""); err != nil {
		http.Error(w, "An API key is required.", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(s.specJSON)
}

func caller(ctx context.Context) service.Caller {
	c, _ := ctx.Value(callerKey{}).(service.Caller)
	return c
}

func principal(ctx context.Context) *auth.Principal { return caller(ctx).Principal }

// apiError maps a service error to an HTTP error.
func apiError(err error) error {
	var se *service.Error
	if !errors.As(err, &se) {
		return err
	}
	switch se.Kind {
	case service.KindInvalid:
		return huma.Error422UnprocessableEntity(se.Msg)
	case service.KindUnauthenticated:
		return huma.Error401Unauthorized(se.Msg)
	case service.KindForbidden:
		return huma.Error403Forbidden(se.Msg)
	case service.KindNotFound:
		return huma.Error404NotFound(se.Msg)
	case service.KindConflict:
		return huma.Error409Conflict(se.Msg)
	case service.KindLocked:
		return huma.Error429TooManyRequests(se.Msg)
	}
	return err
}

func (s *Server) writeErr(ctx huma.Context, err error) {
	var se huma.StatusError
	if errors.As(apiError(err), &se) {
		_ = huma.WriteErr(s.api, ctx, se.GetStatus(), se.Error())
		return
	}
	s.log.Error("request failed", "path", ctx.URL().Path, "err", err)
	_ = huma.WriteErr(s.api, ctx, http.StatusInternalServerError, "Something went wrong on the server. The error has been logged.")
}

// wrap logs unexpected errors and hides their detail from the client.
func wrap[I, O any](s *Server, fn func(context.Context, *I) (*O, error)) func(context.Context, *I) (*O, error) {
	return func(ctx context.Context, in *I) (*O, error) {
		out, err := fn(ctx, in)
		if err == nil {
			return out, nil
		}
		mapped := apiError(err)
		var se huma.StatusError
		if errors.As(mapped, &se) {
			return nil, mapped
		}
		s.log.Error("request failed", "err", err)
		return nil, huma.Error500InternalServerError("Something went wrong on the server. The error has been logged.")
	}
}

const homePage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kishtin</title>
<style>body{font:18px/1.5 system-ui,sans-serif;background:#fff;color:#111;max-width:40rem;margin:3rem auto;padding:0 1rem}code{background:#f2f2f2;padding:0 .25rem}</style>
</head><body>
<h1>Kishtin is running</h1>
<p>Kishtin, from <i>cistin</i>, Irish for kitchen.</p>
<p>The kitchen screens are not built yet. For now, use the <code>kishtin</code> command line or the API at <code>/api</code>.
The API description is at <code>/api/openapi.json</code> and needs an API key.</p>
</body></html>
`

func serveHome(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	_, _ = w.Write([]byte(homePage))
}

// Spec returns the API's OpenAPI description as indented JSON, without
// needing a running server.
func Spec() ([]byte, error) {
	s, err := New(nil, slog.New(slog.DiscardHandler), nil)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(s.specJSON, &v); err != nil {
		return nil, err
	}
	return json.MarshalIndent(v, "", "  ")
}
