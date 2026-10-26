package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

type fileBody struct {
	// File is the SHA-256 of an uploaded file.
	File string `json:"file" minLength:"64" maxLength:"64"`
}

type copyIn struct {
	Book string `path:"book" doc:"Record book ID or code, such as SC2"`
	Body fileBody
}

type editionTargetsBody struct {
	Targets []service.EditionTarget `json:"targets"`
}

type editionResultsBody struct {
	Results []service.EditionResult `json:"results"`
}

type paperChecksBody struct {
	Rows []service.PaperCheck `json:"rows"`
}

type paperSavedBody struct {
	Records []haccp.Record `json:"records"`
}

func (s *Server) copyRoutes() {
	tags := []string{"Record books"}
	huma.Register(s.api, huma.Operation{
		OperationID: "compare-copy", Method: http.MethodPost, Path: "/haccp/books/{book}/compare", Tags: tags,
		Summary:     "Compare a booklet with an uploaded copy of its official PDF",
		Description: "Reads the copy's text on this machine and says which of the booklet's columns and questions it has, and whether it is the edition the booklet was made from. Nothing is sent anywhere.",
	}, wrap(s, func(ctx context.Context, in *copyIn) (*out[service.CopyComparison], error) {
		return reply(s.svc.CompareCopy(ctx, principal(ctx), in.Book, in.Body.File))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "suggest-columns", Method: http.MethodPost, Path: "/haccp/books/{book}/suggest", Tags: tags,
		Summary:     "Ask the local model to read a copy's columns (manager password)",
		Description: "Needs [ai] url in kishtin.toml: a model server on this machine or the local network. The result is a draft to review and save in Setup › Food safety › HACCP records; nothing is saved.",
	}, wrap(s, func(ctx context.Context, in *copyIn) (*out[service.Suggestion], error) {
		return reply(s.svc.SuggestColumns(ctx, principal(ctx), in.Book, in.Body.File))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "edition-targets", Method: http.MethodGet, Path: "/haccp/editions", Tags: tags,
		Summary:     "List the official PDFs that checking for new editions would download",
		Description: "Nothing is fetched. Show this list before asking to check.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[editionTargetsBody], error) {
		t, err := s.svc.EditionTargets(ctx, principal(ctx))
		return reply(editionTargetsBody{Targets: t}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "check-editions", Method: http.MethodPost, Path: "/haccp/editions/check", Tags: tags,
		Summary:     "Download the official PDFs and compare their editions (manager password)",
		Description: "Downloads each PDF listed by GET /haccp/editions from its publisher. Only public files are downloaded and nothing about the restaurant is sent. A changed PDF is kept so it can be compared with its booklet.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[editionResultsBody], error) {
		r, err := s.svc.CheckEditions(ctx, principal(ctx))
		return reply(editionResultsBody{Results: r}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "preview-paper", Method: http.MethodPost, Path: "/haccp/paper/preview", Tags: tags,
		Summary: "Check rows transcribed from a photo of a paper page, without saving",
	}, wrap(s, func(ctx context.Context, in *out[service.PaperImport]) (*out[paperChecksBody], error) {
		r, err := s.svc.PreviewPaper(ctx, principal(ctx), in.Body)
		return reply(paperChecksBody{Rows: r}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "import-paper", Method: http.MethodPost, Path: "/haccp/paper", Tags: tags,
		Summary:       "Save rows transcribed from a photo of a paper page, all or none",
		Description:   "Each record keeps when the check was done (reading.taken_at), the name on the paper (reading.signed_by) and the photo. Results are checked against the limits in force now.",
		DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *out[service.PaperImport]) (*out[paperSavedBody], error) {
		r, err := s.svc.ImportPaper(ctx, principal(ctx), in.Body)
		return reply(paperSavedBody{Records: r}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "read-paper", Method: http.MethodPost, Path: "/haccp/paper/read", Tags: tags,
		Summary:     "Ask the local model to read a photo of a paper page into draft rows",
		Description: "Needs [ai] url in kishtin.toml. Nothing is saved: check each row against the photo, then save with POST /haccp/paper.",
	}, wrap(s, func(ctx context.Context, in *out[fileBody]) (*out[service.PaperDraft], error) {
		return reply(s.svc.ReadPaper(ctx, principal(ctx), in.Body.File))
	}))
}
