package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

type signOffPreviewIn struct {
	From string `query:"from" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$" doc:"First day, Dublin time. Leave out to start the day after the last sign-off."`
	To   string `query:"to" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$" doc:"Last day, inclusive. Leave out for yesterday."`
}

func (s *Server) signOffRoutes() {
	tags := []string{"Record books"}
	huma.Register(s.api, huma.Operation{
		OperationID: "preview-sign-off", Method: http.MethodGet, Path: "/haccp/sign-offs/preview", Tags: tags,
		Summary:     "Show what signing off a period would cover",
		Description: "Counts the period's records, out-of-limit readings, readings without a corrective action and missed checks, and lists them. Nothing is saved.",
	}, wrap(s, func(ctx context.Context, in *signOffPreviewIn) (*out[service.SignOffPreview], error) {
		return reply(s.svc.PreviewSignOff(ctx, principal(ctx), service.SignOffInput{From: in.From, To: in.To}))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "sign-off", Method: http.MethodPost, Path: "/haccp/sign-offs", Tags: tags,
		Summary:       "Sign off the HACCP records for a period (manager password)",
		Description:   "Saves a sign-off record in the hash chain with what the period held when it was signed. It fills the manager or supervisor check line of every record book. A period with missed checks or readings without a corrective action needs a note.",
		DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *out[service.SignOffInput]) (*out[haccp.Record], error) {
		return reply(s.svc.SignOff(ctx, principal(ctx), in.Body))
	}))
}
