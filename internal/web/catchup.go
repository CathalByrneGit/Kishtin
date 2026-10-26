package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/service"
)

type catchUpIn struct {
	From string `query:"from" doc:"First day, YYYY-MM-DD (Dublin). Default: 13 days before the last day."`
	To   string `query:"to" doc:"Last day, YYYY-MM-DD (Dublin). Default: today."`
}

func (s *Server) catchUpRoutes() {
	tags := []string{"HACCP"}
	huma.Register(s.api, huma.Operation{
		OperationID: "catch-up-gaps", Method: http.MethodGet, Path: "/haccp/catch-up", Tags: tags,
		Summary:     "List the daily checks with no record in a period",
		Description: "Fridge and freezer checks and cleaning tasks that were due and have no record, from the first day records were kept, at most 62 days. Fill them in with POST /haccp/catch-up.",
	}, wrap(s, func(ctx context.Context, in *catchUpIn) (*out[service.CatchUp], error) {
		return reply(s.svc.CatchUpGaps(ctx, principal(ctx), in.From, in.To))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "preview-catch-up", Method: http.MethodPost, Path: "/haccp/catch-up/preview", Tags: tags,
		Summary:     "Check rows of checks done earlier, without saving",
		Description: "Each row needs reading.taken_at: when the check was done.",
	}, wrap(s, func(ctx context.Context, in *out[service.CatchUpInput]) (*out[paperChecksBody], error) {
		r, err := s.svc.PreviewCatchUp(ctx, principal(ctx), in.Body)
		return reply(paperChecksBody{Rows: r}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "save-catch-up", Method: http.MethodPost, Path: "/haccp/catch-up", Tags: tags,
		Summary:       "Save rows of checks done earlier, all or none",
		Description:   "Each record keeps when the check was done (reading.taken_at) and, if given, who did it (reading.signed_by). Results are checked against the limits in force now.",
		DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *out[service.CatchUpInput]) (*out[paperSavedBody], error) {
		r, err := s.svc.SaveCatchUp(ctx, principal(ctx), in.Body)
		return reply(paperSavedBody{Records: r}, err)
	}))
}
