package web

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/service"
)

// publicKey marks the few operations that work without an API key: setting
// up a new install, before any key exists.
const publicKey = "public"

// SetSetupCode sets the one-time code another device must give to set up a
// new install, and what to do once setup is done. Without a code, setup only
// works on the server machine itself.
func (s *Server) SetSetupCode(code string, done func()) {
	s.setupCode, s.setupDone = code, done
}

// fromThisMachine reports whether a request came from the server machine
// itself. A request passed on by a proxy counts as from elsewhere, even when
// the proxy runs on this machine.
func fromThisMachine(ctx huma.Context) bool {
	for _, h := range []string{"Forwarded", "X-Forwarded-For", "X-Real-Ip"} {
		if ctx.Header(h) != "" {
			return false
		}
	}
	host, _, err := net.SplitHostPort(ctx.RemoteAddr())
	if err != nil {
		host = ctx.RemoteAddr()
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type remoteKey struct{}

// noteOrigin keeps whether the request came from this machine, for the
// setup routes.
func noteOrigin(ctx huma.Context) huma.Context {
	return huma.WithValue(ctx, remoteKey{}, !fromThisMachine(ctx))
}

func remote(ctx context.Context) bool {
	r, _ := ctx.Value(remoteKey{}).(bool)
	return r
}

type setupStatusOut struct {
	Body struct {
		// Needed is true while Kishtin has no staff.
		Needed bool `json:"needed"`
		// CodeNeeded is true when this device must give the setup code
		// shown on the server machine.
		CodeNeeded bool `json:"code_needed"`
		// Possible is false when this device cannot set up Kishtin at all:
		// it is not the server machine, and no setup code was made.
		Possible bool `json:"possible"`
	}
}

type setupIn struct {
	Body struct {
		service.FirstRun
		// Code is the setup code shown on the server machine, needed from
		// any other device.
		Code string `json:"code,omitempty"`
	}
}

type setupOut struct {
	Body service.FirstRunResult
}

func (s *Server) setupRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "setup-status", Method: http.MethodGet, Path: "/setup", Tags: []string{"Setup"},
		Summary:     "Whether this install still needs setting up",
		Description: "Works without an API key. Needed is true while there are no staff.",
		Security:    []map[string][]string{},
		Metadata:    map[string]any{publicKey: true},
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*setupStatusOut, error) {
		needed, err := s.svc.SetupNeeded(ctx)
		if err != nil {
			return nil, err
		}
		out := &setupStatusOut{}
		out.Body.Needed = needed
		out.Body.CodeNeeded = needed && remote(ctx)
		out.Body.Possible = needed && (!remote(ctx) || s.setupCode != "")
		return out, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "setup", Method: http.MethodPost, Path: "/setup", Tags: []string{"Setup"},
		Summary: "Set up a new install from the screens",
		Description: "Creates the first manager, a device key for the screen doing the setup, and the restaurant's name. " +
			"Works without an API key, only while there are no staff, and from another device only with the setup code shown on the server machine. " +
			"The device key and API key are shown once.",
		Security: []map[string][]string{},
		Metadata: map[string]any{publicKey: true},
	}, wrap(s, func(ctx context.Context, in *setupIn) (*setupOut, error) {
		if remote(ctx) {
			code := normalCode(in.Body.Code)
			if s.setupCode == "" {
				return nil, huma.Error403Forbidden("set Kishtin up on the computer it runs on: open it there in a browser")
			}
			if subtle.ConstantTimeCompare([]byte(code), []byte(normalCode(s.setupCode))) != 1 {
				return nil, huma.Error403Forbidden("that setup code is wrong: it is shown where Kishtin was started, and in setup-code.txt in its data folder")
			}
		}
		res, err := s.svc.SetupFromScreen(ctx, in.Body.FirstRun)
		if err != nil {
			return nil, err
		}
		if s.setupDone != nil {
			s.setupDone()
		}
		return &setupOut{Body: res}, nil
	}))
}

// normalCode lets a setup code be typed in any case, with or without the
// dashes and spaces.
func normalCode(c string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '-' || r == ' ':
			return -1
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		}
		return r
	}, c)
}
