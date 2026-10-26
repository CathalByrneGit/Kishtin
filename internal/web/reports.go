package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/analytics"
)

type reportIn struct {
	From string `query:"from" doc:"First day, YYYY-MM-DD (Dublin). Default: 27 days before the last day."`
	To   string `query:"to" doc:"Last day, YYYY-MM-DD (Dublin). Default: today."`
}

func (s *Server) reportRoutes() {
	tags := []string{"Reports"}
	huma.Register(s.api, huma.Operation{
		OperationID: "report-haccp", Method: http.MethodGet, Path: "/reports/haccp", Tags: tags,
		Summary:     "How well the daily HACCP checks were kept up in a period",
		Description: "Fridge and freezer checks due, done and missed (counted as the record books count them), by day of the week and by unit; cleaning tasks done; records out of limit; each unit's temperatures per day. At most 366 days.",
	}, wrap(s, func(ctx context.Context, in *reportIn) (*out[analytics.HACCPReport], error) {
		return reply(s.svc.ReportHACCP(ctx, principal(ctx), in.From, in.To))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "report-pass", Method: http.MethodGet, Path: "/reports/pass", Tags: tags,
		Summary:     "How fast courses reached the pass in a period",
		Description: "Courses fired in the period that reached the pass: share on time (within a minute of target), time from fire to pass and from pass to served, late courses by the scheduler's cause, by course and by hour, and each dish's actual time against its set time. Head chef or manager.",
	}, wrap(s, func(ctx context.Context, in *reportIn) (*out[analytics.PassReport], error) {
		return reply(s.svc.ReportPass(ctx, principal(ctx), in.From, in.To))
	}))
}
