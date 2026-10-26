package web

import (
	"context"
	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
)

type versionedOut[T any] struct {
	Body service.Versioned[T]
}

type historyOut[T any] struct {
	Body struct {
		Versions []service.Versioned[T] `json:"versions"`
	}
}

type changeIn[T any] struct {
	Body service.SettingsChange[T]
}

type previewOut[T any] struct {
	Body service.SettingsPreview[T]
}

func (s *Server) settingsRoutes() {
	registerGroup(s, settings.KindGuidance, "reference guidance",
		"The reference limits are measured against. It starts as the FSAI defaults. A limit looser than the guidance needs a HACCP plan reference; guidance looser than FSAI needs one too.",
		func() *service.Group[haccp.Guidance] { return s.svc.Guidance() })
	registerGroup(s, settings.KindRules, "food-safety rules",
		"Whether borderline readings need a corrective action, and whether names must come from the lists. A failed reading always needs a corrective action.",
		func() *service.Group[haccp.Rules] { return s.svc.Rules() })
	registerGroup(s, settings.KindLists, "lists",
		"The fridges, freezers, food categories and cleaning tasks staff choose from.",
		func() *service.Group[haccp.Lists] { return s.svc.Lists() })
	registerGroup(s, settings.KindSystem, "system settings",
		"Backup schedule and retention, upload size and limit preview length. Until changed here they come from kishtin.toml.",
		func() *service.Group[settings.System] { return s.svc.System() })
	registerGroup(s, settings.KindPermissions, "permissions",
		"Who may do what, where it differs from Kishtin's defaults: per action, the roles and named staff allowed, and whether a PIN is enough. Managers may always do everything. Staff, API keys, permissions and fitness to work are always managers with a password. See GET /api/auth/actions.",
		func() *service.Group[auth.Permissions] { return s.svc.Permissions() })
}

func registerGroup[T any](s *Server, kind settings.Kind, label, desc string, group func() *service.Group[T]) {
	k := string(kind)
	tags := []string{"Settings"}
	huma.Register(s.api, huma.Operation{
		OperationID: "get-" + k, Method: http.MethodGet, Path: "/settings/" + k, Tags: tags,
		Summary: "Show the " + label + " in force", Description: desc,
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*versionedOut[T], error) {
		v, err := group().Current(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		return &versionedOut[T]{Body: v}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: k + "-history", Method: http.MethodGet, Path: "/settings/" + k + "/history", Tags: tags,
		Summary: "List every saved version of the " + label + ", oldest first",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*historyOut[T], error) {
		vs, err := group().History(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		out := &historyOut[T]{}
		out.Body.Versions = vs
		return out, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "preview-" + k, Method: http.MethodPost, Path: "/settings/" + k + "/preview", Tags: tags,
		Summary:     "Preview a change to the " + label,
		Description: "Shows each field that would change, anything that makes food safety less strict, effects on limits and recent records, and anything that stops the change being saved.",
	}, wrap(s, func(ctx context.Context, in *changeIn[T]) (*previewOut[T], error) {
		pv, err := group().Preview(ctx, principal(ctx), in.Body)
		if err != nil {
			return nil, err
		}
		return &previewOut[T]{Body: pv}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "save-" + k, Method: http.MethodPut, Path: "/settings/" + k, Tags: tags,
		Summary:     "Save a new version of the " + label,
		Description: "Needs a manager password login or a manager API key, and a reason. A change that makes food safety less strict also needs plan_ref. Saved records are never re-graded.",
	}, wrap(s, func(ctx context.Context, in *changeIn[T]) (*versionedOut[T], error) {
		v, err := group().Save(ctx, principal(ctx), in.Body)
		if err != nil {
			return nil, err
		}
		return &versionedOut[T]{Body: v}, nil
	}))
}
