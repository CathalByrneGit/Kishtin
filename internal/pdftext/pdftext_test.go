package pdftext

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPages(t *testing.T) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.AddPage()
	pdf.CellFormat(0, 8, "SC2 - Fridge/Freezer/Chill Display Temperature Records", "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 8, "Comments/Action   Sign", "", 1, "L", false, 0, "")
	pdf.AddPage()
	pdf.CellFormat(0, 8, "Manager/Supervisor check on", "", 1, "L", false, 0, "")
	var buf bytes.Buffer
	require.NoError(t, pdf.Output(&buf))

	pages, err := Pages(buf.Bytes())
	require.NoError(t, err)
	require.Len(t, pages, 2)
	assert.True(t, strings.Contains(pages[0], "Chill Display Temperature Records"), pages[0])
	assert.Contains(t, pages[1], "Manager/Supervisor check on")

	_, err = Pages([]byte("not a pdf"))
	assert.Error(t, err)
	_, err = Pages(nil)
	assert.Error(t, err)
}
