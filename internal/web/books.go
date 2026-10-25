package web

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
)

type bookIn struct {
	Book  string `path:"book" doc:"Record book ID or code, such as SC2"`
	Month string `query:"month" pattern:"^[0-9]{4}-[0-9]{2}$" doc:"Month as YYYY-MM, Dublin time. Leave out for this month."`
}

func (s *Server) bookRoutes() {
	tags := []string{"Record books"}
	huma.Register(s.api, huma.Operation{
		OperationID: "list-record-books", Method: http.MethodGet, Path: "/haccp/books", Tags: tags,
		Summary:     "List the record books in use, and the bundles and booklet kinds available",
		Description: "booklets are the bundle's booklets with the restaurant's changes, then the booklets it added. Booklets Kishtin cannot fill yet say why in missing.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[service.RecordBooks], error) {
		return reply(s.svc.RecordBooks(ctx, principal(ctx)))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "get-record-book", Method: http.MethodGet, Path: "/haccp/books/{book}", Tags: tags,
		Summary:     "Fill in a record book for a month, to preview",
		Description: "Returns the booklet's pages as rows of cells, the checks that were due and have no reading (gaps), and the columns Kishtin does not record. Nothing is saved.",
	}, wrap(s, func(ctx context.Context, in *bookIn) (*out[books.Book], error) {
		return reply(s.svc.RecordBook(ctx, principal(ctx), in.Book, in.Month))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "export-record-book", Method: http.MethodGet, Path: "/haccp/books/{book}/pdf", Tags: tags,
		Summary:     "Download a filled-in record book as PDF",
		Description: "The same pages as the preview, laid out like the booklet, with record numbers and the hash chain head in the footer.",
		Responses: map[string]*huma.Response{"200": {
			Description: "The filled-in record book.",
			Content:     map[string]*huma.MediaType{"application/pdf": {Schema: &huma.Schema{Type: "string", Format: "binary"}}},
		}},
	}, wrap(s, func(ctx context.Context, in *bookIn) (*pdfOut, error) {
		var buf bytes.Buffer
		b, sum, err := s.svc.RecordBookPDF(ctx, principal(ctx), in.Book, in.Month, &buf)
		if err != nil {
			return nil, err
		}
		name := safeFilename(fmt.Sprintf("%s-%s.pdf", b.Template.Code, b.Month))
		return &pdfOut{
			ContentType:        "application/pdf",
			ContentDisposition: fmt.Sprintf("attachment; filename=%q", name),
			Records:            sum.Records,
			ChainHead:          sum.Chain.HeadHash,
			ChainOK:            sum.Chain.OK,
			Body:               buf.Bytes(),
		}, nil
	}))
	registerGroup(s, settings.KindRecordBooks, "record book settings",
		"The record book bundle in use, the restaurant's changes to its booklets (turned off, title, columns, notes, sheet layout, manager check, uploaded copy) and booklets it added. Records never change; only how they are printed.",
		func() *service.Group[books.RecordBookSettings] { return s.svc.RecordBookSettings() })
}
