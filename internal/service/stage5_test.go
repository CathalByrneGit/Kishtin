package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/ai"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func pdfWith(t *testing.T, lines ...string) []byte {
	t.Helper()
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.AddPage()
	for _, l := range lines {
		pdf.CellFormat(0, 6, l, "", 1, "L", false, 0, "")
	}
	var buf bytes.Buffer
	require.NoError(t, pdf.Output(&buf))
	return buf.Bytes()
}

func pngPhoto(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8))))
	return buf.Bytes()
}

func fakeModel(t *testing.T, content string) *ai.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + content + `}}]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := ai.New(srv.URL, "test-model", 5*time.Second)
	require.NoError(t, err)
	return c
}

func TestCompareAndSuggest(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	copyPDF := pdfWith(t, "SC3 - Cooking/Cooling/Reheating Records", "Date Food Time Started Cooking Time Finished Cooking Core Temp Signed",
		"Time Chilled Signed Core Temp Signed Comments/Action")
	m, err := f.svc.UploadFile(ctx, f.manager, bytes.NewReader(copyPDF), "sc3-2027.pdf")
	require.NoError(t, err)

	cmp, err := f.svc.CompareCopy(ctx, f.manager, "SC3", m.SHA256)
	require.NoError(t, err)
	require.NotNil(t, cmp.Edition)
	assert.False(t, cmp.Edition.Matches, "not the 2020 edition")
	assert.False(t, cmp.AI)
	var missing []string
	for _, c := range cmp.Report.Columns {
		if !c.Found {
			missing = append(missing, c.Label)
		}
	}
	assert.Equal(t, []string{"Time into fridge/blast chiller"}, missing)

	_, err = f.svc.SuggestColumns(ctx, f.manager, "SC3", m.SHA256)
	assert.ErrorContains(t, err, "no local model is set up")

	photo, err := f.svc.UploadFile(ctx, f.manager, bytes.NewReader(pngPhoto(t)), "photo.png")
	require.NoError(t, err)
	_, err = f.svc.CompareCopy(ctx, f.manager, "SC3", photo.SHA256)
	assert.ErrorContains(t, err, "must be a PDF")

	f.svc.ai = fakeModel(t, `"{\"columns\":[{\"label\":\"Date\",\"field\":\"date\"},{\"label\":\"Food\",\"field\":\"food\"},{\"label\":\"Core Temp\",\"group\":\"Cooking\",\"field\":\"core_temp\"},{\"label\":\"Time Chilled\",\"group\":\"Cooling\",\"field\":\"into_fridge\"},{\"label\":\"Probe used\",\"field\":\"probe\"}]}"`)
	sug, err := f.svc.SuggestColumns(ctx, f.manager, "SC3", m.SHA256)
	require.NoError(t, err)
	require.Len(t, sug.Columns, 5)
	assert.Equal(t, "into_fridge", sug.Columns[3].Field)
	assert.Equal(t, "", sug.Columns[4].Field, "an unknown field becomes a blank column")
	assert.Len(t, sug.Notes, 2)
	_, err = f.svc.SuggestColumns(ctx, f.cook, "SC3", m.SHA256)
	assert.Equal(t, KindForbidden, KindOf(err))
}

func TestCheckEditions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	targets, err := f.svc.EditionTargets(ctx, f.manager)
	require.NoError(t, err)
	assert.Len(t, targets, 9)
	assert.Contains(t, targets[1].URL, "fsai.ie")

	newSC2 := pdfWith(t, "SC2 - Fridge/Freezer/Chill Display Temperature Records (2027)")
	var asked []string
	f.svc.fetch = func(_ context.Context, url string) ([]byte, error) {
		asked = append(asked, url)
		if url == targets[1].URL {
			return newSC2, nil
		}
		return nil, errors.New("connection refused")
	}
	_, err = f.svc.CheckEditions(ctx, f.cook)
	assert.Equal(t, KindForbidden, KindOf(err))
	assert.Empty(t, asked, "nothing is fetched without a manager asking")

	res, err := f.svc.CheckEditions(ctx, f.manager)
	require.NoError(t, err)
	require.Len(t, res, 9)
	assert.Equal(t, EditionChanged, res[1].Status)
	assert.Len(t, res[1].File, 64, "the new edition is kept")
	assert.Equal(t, EditionUnreachable, res[0].Status)
	assert.Equal(t, "connection refused", res[0].Error)
	cmp, err := f.svc.CompareCopy(ctx, f.manager, "SC2", res[1].File)
	require.NoError(t, err)
	assert.Equal(t, 1, cmp.Report.Pages)
}

func TestPaperImport(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	photo, err := f.svc.UploadFile(ctx, f.manager, bytes.NewReader(pngPhoto(t)), "sc2-april.png")
	require.NoError(t, err)
	at := func(day, hh int) *time.Time { v := time.Date(2026, 4, day, hh, 0, 0, 0, time.UTC); return &v }
	rows := []NewRecord{
		{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(3.5), TakenAt: at(28, 8), SignedBy: "A Jones"}},
		{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(7), TakenAt: at(28, 14), SignedBy: "A Jones"}},
		{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(3), TakenAt: at(29, 8)}},
		{Type: haccp.RecordCleaning, Reading: haccp.Reading{Task: "Floors swept & mopped", Done: bt(true), TakenAt: at(29, 8)}},
		{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(3), TakenAt: func() *time.Time { v := start.Add(48 * time.Hour); return &v }()}},
	}
	_, err = f.svc.PreviewPaper(ctx, f.cook, PaperImport{File: photo.SHA256, Rows: rows})
	assert.Equal(t, KindForbidden, KindOf(err))
	checks, err := f.svc.PreviewPaper(ctx, f.manager, PaperImport{File: photo.SHA256, Rows: rows})
	require.NoError(t, err)
	assert.Empty(t, checks[0].Problem)
	assert.Equal(t, haccp.OutcomePass, checks[0].Check.Result.Outcome)
	assert.Contains(t, checks[1].Problem, "action taken from the paper")
	assert.Contains(t, checks[3].Problem, "cannot be transcribed from paper")
	assert.Contains(t, checks[4].Problem, "future")

	_, err = f.svc.ImportPaper(ctx, f.manager, PaperImport{File: photo.SHA256, Rows: rows})
	assert.ErrorContains(t, err, "row 2", "all or none")
	all, err := f.svc.ListRecords(ctx, f.manager, RecordQuery{})
	require.NoError(t, err)
	assert.Empty(t, all, "nothing saved")

	rows[1].CorrectiveAction = "Moved food, engineer called"
	saved, err := f.svc.ImportPaper(ctx, f.manager, PaperImport{File: photo.SHA256, Rows: rows[:3]})
	require.NoError(t, err)
	require.Len(t, saved, 3)
	assert.Equal(t, photo.SHA256, saved[0].Reading.PaperFile)
	assert.Equal(t, "A Jones", saved[0].Reading.SignedBy)
	assert.Equal(t, "Máire Manager", saved[0].LoggedByName, "logged by the person transcribing")

	_, err = f.svc.LogRecord(ctx, f.manager, NewRecord{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(3), TakenAt: at(1, 9), PaperFile: photo.SHA256}})
	assert.ErrorContains(t, err, "paper import", "a backdated record needs paper import")

	// The April booklet places them by when they were taken and shows the paper's signature.
	b, err := f.svc.RecordBook(ctx, f.manager, "SC2", "2026-04")
	require.NoError(t, err)
	day28 := b.Pages[0].Rows[27]
	assert.Equal(t, "3.5°C", day28.Cells[1].Text)
	assert.Equal(t, "7.0°C", day28.Cells[2].Text)
	assert.Contains(t, day28.Cells[len(day28.Cells)-1].Text, "A")
	assert.Equal(t, 3, b.FromPaper)

	// A local model reads a photo into draft rows.
	f.svc.ai = fakeModel(t, `"{\"rows\":[{\"type\":\"fridge\",\"date\":\"2026-04-30\",\"time\":\"09:00\",\"unit\":\"Walk-in fridge\",\"temp_c\":4.1,\"signed_by\":\"AJ\"},{\"type\":\"oven\",\"date\":\"2026-04-30\",\"time\":\"10:00\"},{\"type\":\"hot_hold\",\"date\":\"\",\"food\":\"Soup\",\"temp_c\":70}]}"`)
	draft, err := f.svc.ReadPaper(ctx, f.manager, photo.SHA256)
	require.NoError(t, err)
	require.Len(t, draft.Rows, 3)
	assert.Equal(t, "AJ", draft.Rows[0].Reading.SignedBy)
	require.NotNil(t, draft.Rows[0].Reading.TakenAt)
	assert.Len(t, draft.Problems, 2)
	assert.Contains(t, draft.Problems[0], "row 2")
	assert.Contains(t, draft.Problems[1], "date or time")
}
