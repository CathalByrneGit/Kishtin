package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

type connectorsBody struct {
	Connectors []service.ConnectorStatus `json:"connectors"`
}

type locationsBody struct {
	Locations []integrations.Location `json:"locations"`
}

func (s *Server) connectorRoutes() {
	tags := []string{"Connections"}
	huma.Register(s.api, huma.Operation{
		OperationID: "list-connectors", Method: http.MethodGet, Path: "/connectors", Tags: tags,
		Summary: "List what Kishtin can connect to, what each sends, and whether it is connected",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[connectorsBody], error) {
		list, err := s.svc.Connectors(ctx, principal(ctx))
		return reply(connectorsBody{Connectors: list}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "connect-square", Method: http.MethodPut, Path: "/connectors/square", Tags: tags,
		Summary:     "Connect Square with an access token (manager password)",
		Description: "Kishtin checks the token by asking Square for the account's locations, and saves it only if Square accepts it. It is kept in a file only the server can read, outside the database, so it is never in a backup or export. Not possible when the token is set in the environment or kishtin.toml.",
	}, wrap(s, func(ctx context.Context, in *out[service.ConnectInput]) (*out[service.ConnectResult], error) {
		return reply(s.svc.ConnectSquare(ctx, principal(ctx), in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "disconnect-square", Method: http.MethodDelete, Path: "/connectors/square", Tags: tags,
		Summary:       "Disconnect Square and delete the saved token (manager password); turn order import off first",
		DefaultStatus: http.StatusNoContent,
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		return nil, s.svc.DisconnectSquare(ctx, principal(ctx))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "square-locations", Method: http.MethodGet, Path: "/square/locations", Tags: []string{"Square"},
		Summary: "List the Square account's locations (sends Square the access token only)",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[locationsBody], error) {
		list, err := s.svc.SquareLocations(ctx, principal(ctx))
		return reply(locationsBody{Locations: list}, err)
	}))
}
