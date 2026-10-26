package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
)

type catalogBody struct {
	Entries []service.CatalogRow `json:"entries"`
}

type mappingsIn struct {
	Mappings []service.MappingInput `json:"mappings" minItems:"1"`
}

type mappingsBody struct {
	Mappings []integrations.Mapping `json:"mappings"`
}

func (s *Server) squareRoutes() {
	tags := []string{"Square"}
	registerGroup(s, settings.KindSquare, "Square settings",
		"Whether orders are imported from Square, for which location, and how often Kishtin asks. The preview says exactly what is sent to Square. Turning it on needs the manager password.",
		func() *service.Group[integrations.POSSettings] { return s.svc.Square() })
	huma.Register(s.api, huma.Operation{
		OperationID: "square-status", Method: http.MethodGet, Path: "/square", Tags: tags,
		Summary: "Show whether order import from Square is on and working, recent till orders, and mappings",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[service.SquareStatus], error) {
		return reply(s.svc.SquareStatus(ctx, principal(ctx)))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "square-catalog", Method: http.MethodGet, Path: "/square/catalog", Tags: tags,
		Summary:     "Read the catalog from Square, with how each entry maps to the kitchen (manager password)",
		Description: "Sends Square only the access token. Suggestions match dishes and modifiers by name; nothing is saved until mappings are saved.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[catalogBody], error) {
		rows, err := s.svc.SquareCatalog(ctx, principal(ctx))
		return reply(catalogBody{Entries: rows}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "save-square-mappings", Method: http.MethodPut, Path: "/square/mappings", Tags: tags,
		Summary:     "Map till items to dishes and till modifiers to dish modifiers (manager password)",
		Description: "Mark an entry ignore for anything not for the kitchen, such as drinks. Open till orders with items that are now mapped get them added.",
	}, wrap(s, func(ctx context.Context, in *out[mappingsIn]) (*out[mappingsBody], error) {
		list, err := s.svc.SaveSquareMappings(ctx, principal(ctx), in.Body.Mappings)
		return reply(mappingsBody{Mappings: list}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "square-sync", Method: http.MethodPost, Path: "/square/sync", Tags: tags,
		Summary:     "Ask Square for new and changed orders now",
		Description: "Kishtin also asks on its own while import is on. Orders are deduped on Square's order ID and version.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[service.SyncResult], error) {
		return reply(s.svc.SyncSquareNow(ctx, principal(ctx)))
	}))
}
