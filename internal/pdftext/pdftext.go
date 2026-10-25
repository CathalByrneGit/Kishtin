// Package pdftext reads the text of a PDF, page by page, using PDFium
// compiled to WebAssembly, so the server stays free of CGO. The engine
// starts on first use and is shared after that.
package pdftext

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

// MaxPages is the most pages read from one file.
const MaxPages = 40

var (
	once    sync.Once
	pool    pdfium.Pool
	initErr error
)

func instance() (pdfium.Pdfium, error) {
	once.Do(func() {
		pool, initErr = webassembly.Init(webassembly.Config{MinIdle: 0, MaxIdle: 1, MaxTotal: 1})
	})
	if initErr != nil {
		return nil, fmt.Errorf("start the PDF reader: %w", initErr)
	}
	return pool.GetInstance(2 * time.Minute)
}

// Pages returns the text of each page of a PDF, up to MaxPages.
func Pages(pdf []byte) ([]string, error) {
	if len(pdf) == 0 {
		return nil, errors.New("the file is empty")
	}
	inst, err := instance()
	if err != nil {
		return nil, err
	}
	defer inst.Close()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &pdf})
	if err != nil {
		return nil, fmt.Errorf("open the PDF: %w", err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}) //nolint:errcheck // closing a document we only read
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, fmt.Errorf("count the pages: %w", err)
	}
	n := min(count.PageCount, MaxPages)
	out := make([]string, 0, n)
	for i := range n {
		res, err := inst.GetPageText(&requests.GetPageText{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}}})
		if err != nil {
			return nil, fmt.Errorf("read page %d: %w", i+1, err)
		}
		out = append(out, res.Text)
	}
	return out, nil
}
