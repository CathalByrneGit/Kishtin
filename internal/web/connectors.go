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

type connectIn struct {
	Till string `path:"till" enum:"square,sumup"`
	Body service.ConnectInput
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
		OperationID: "connect-till", Method: http.MethodPut, Path: "/connectors/{till}", Tags: tags,
		Summary: "Connect a till with its token: a Square access token or a SumUp API key (manager password)",
		Description: "Kishtin checks the token by asking the till for the account's locations, and saves it only if the till accepts it. " +
			"It is kept in a file only the server can read, outside the database, so it is never in a backup or export. " +
			"Not possible when the token is set in the environment or kishtin.toml. SumUp has only the production environment.",
	}, wrap(s, func(ctx context.Context, in *connectIn) (*out[service.ConnectResult], error) {
		return reply(s.svc.ConnectTill(ctx, principal(ctx), in.Till, in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "disconnect-till", Method: http.MethodDelete, Path: "/connectors/{till}", Tags: tags,
		Summary:       "Disconnect a till and delete its saved token (manager password); turn order import off first",
		DefaultStatus: http.StatusNoContent,
	}, wrap(s, func(ctx context.Context, in *tillIn) (*struct{}, error) {
		return nil, s.svc.DisconnectTill(ctx, principal(ctx), in.Till)
	}))
}
