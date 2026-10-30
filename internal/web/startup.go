package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/autostart"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

// StartupStatus is whether Kishtin starts by itself with this computer.
type StartupStatus autostart.Status

type startupOut struct {
	Body StartupStatus
}

type startupIn struct {
	Body struct {
		On bool `json:"on"`
	}
}

func (s *Server) startupRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "get-setup-progress", Method: http.MethodGet, Path: "/getting-started", Tags: []string{"Setup"},
		Summary:     "How far the restaurant has got with setting Kishtin up",
		Description: "Each step is worked out from what is set up and recorded, so it ticks itself off. Steps marked optional are not counted in done and total.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[service.SetupProgress], error) {
		return reply(s.svc.SetupProgress(ctx, principal(ctx)))
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "get-startup", Method: http.MethodGet, Path: "/startup", Tags: []string{"Setup"},
		Summary:     "Whether Kishtin starts by itself with this computer",
		Description: "How Kishtin starts on the computer it runs on (Task Scheduler, a LaunchAgent or a systemd user service), and what that does not do.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*startupOut, error) {
		st, err := s.svc.StartupStatus(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		return &startupOut{Body: StartupStatus(st)}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "set-startup", Method: http.MethodPut, Path: "/startup", Tags: []string{"Setup"},
		Summary: "Turn Kishtin starting with this computer on or off",
		Description: "Changes the computer Kishtin runs on, as the user it runs as. Turning it off leaves this Kishtin running until it is stopped or the computer restarts. " +
			"Noted in the activity log.",
	}, wrap(s, func(ctx context.Context, in *startupIn) (*startupOut, error) {
		st, err := s.svc.SetStartup(ctx, principal(ctx), in.Body.On)
		if err != nil {
			return nil, err
		}
		return &startupOut{Body: StartupStatus(st)}, nil
	}))
}
