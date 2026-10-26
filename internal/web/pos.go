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
	Till string `path:"till" enum:"square,sumup"`
	Body struct {
		Mappings []service.MappingInput `json:"mappings" minItems:"1"`
	}
}

type mappingsBody struct {
	Mappings []integrations.Mapping `json:"mappings"`
}

// tillIn names the till a route acts on.
type tillIn struct {
	Till string `path:"till" enum:"square,sumup" doc:"The till: square or sumup."`
}

func (s *Server) tillRoutes() {
	tags := []string{"Tills"}
	registerGroup(s, settings.KindSquare, "Square settings",
		"Whether orders are imported from Square, for which location, and how often Kishtin asks. The preview says exactly what is sent to Square. Turning it on needs the manager password.",
		func() *service.Group[integrations.POSSettings] { return s.svc.Square() })
	registerGroup(s, settings.KindSumUp, "SumUp settings",
		"Whether paid sales are imported from SumUp, for which merchant account (its merchant code is the location ID), and how often Kishtin asks. The preview says exactly what is sent to SumUp. Turning it on needs the manager password.",
		func() *service.Group[integrations.POSSettings] { return s.svc.SumUp() })
	huma.Register(s.api, huma.Operation{
		OperationID: "till-status", Method: http.MethodGet, Path: "/tills/{till}", Tags: tags,
		Summary: "Show whether order import from a till is on and working, recent till orders, and mappings",
	}, wrap(s, func(ctx context.Context, in *tillIn) (*out[service.TillStatus], error) {
		return reply(s.svc.TillStatus(ctx, principal(ctx), in.Till))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "till-catalog", Method: http.MethodGet, Path: "/tills/{till}/catalog", Tags: tags,
		Summary: "Read the catalog from a till, with how each entry maps to the kitchen (manager password)",
		Description: "Sends the till only its token and location. Suggestions match dishes and modifiers by name; nothing is saved until mappings are saved. " +
			"SumUp has no product list in its API, so its catalog is the items sold in recent sales.",
	}, wrap(s, func(ctx context.Context, in *tillIn) (*out[catalogBody], error) {
		rows, err := s.svc.TillCatalog(ctx, principal(ctx), in.Till)
		return reply(catalogBody{Entries: rows}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "save-till-mappings", Method: http.MethodPut, Path: "/tills/{till}/mappings", Tags: tags,
		Summary:     "Map till items to dishes and till modifiers to dish modifiers (manager password)",
		Description: "Mark an entry ignore for anything not for the kitchen, such as drinks. Open till orders with items that are now mapped get them added.",
	}, wrap(s, func(ctx context.Context, in *mappingsIn) (*out[mappingsBody], error) {
		list, err := s.svc.SaveTillMappings(ctx, principal(ctx), in.Till, in.Body.Mappings)
		return reply(mappingsBody{Mappings: list}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "till-sync", Method: http.MethodPost, Path: "/tills/{till}/sync", Tags: tags,
		Summary:     "Ask a till for new and changed orders now",
		Description: "Kishtin also asks on its own while import is on. Orders are deduped on the till's order ID and version.",
	}, wrap(s, func(ctx context.Context, in *tillIn) (*out[service.SyncResult], error) {
		return reply(s.svc.SyncTillNow(ctx, principal(ctx), in.Till))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "till-locations", Method: http.MethodGet, Path: "/tills/{till}/locations", Tags: tags,
		Summary: "List a till account's locations; for SumUp, its merchant accounts (sends the till its token only)",
	}, wrap(s, func(ctx context.Context, in *tillIn) (*out[locationsBody], error) {
		list, err := s.svc.TillLocations(ctx, principal(ctx), in.Till)
		return reply(locationsBody{Locations: list}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "till-order-versions", Method: http.MethodGet, Path: "/tills/{till}/orders/{id}/versions", Tags: tags,
		Summary: "List every version of a till order Kishtin saw",
	}, wrap(s, func(ctx context.Context, in *struct {
		Till string `path:"till" enum:"square,sumup"`
		ID   string `path:"id"`
	}) (*out[versionsBody], error) {
		r, err := s.svc.TillOrderVersions(ctx, principal(ctx), in.Till, in.ID)
		return reply(versionsBody{Versions: r}, err)
	}))
}
