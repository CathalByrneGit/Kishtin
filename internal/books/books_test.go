package books

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

var dublin = func() *time.Location {
	l, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		panic(err)
	}
	return l
}()

// on returns a time in October 2026, Dublin time.
func on(day, hh, mm int) time.Time { return time.Date(2026, 10, day, hh, mm, 0, 0, dublin) }

type recs struct {
	list []haccp.Record
}

func (r *recs) add(t haccp.RecordType, at time.Time, rd haccp.Reading, o haccp.Outcome, extra ...func(*haccp.Record)) haccp.Record {
	rec := haccp.Record{
		ID: fmt.Sprintf("r%d", len(r.list)+1), Seq: int64(len(r.list) + 1), Type: t,
		LoggedAt: at, LoggedByName: "Aoife Synthetic", Reading: rd, Outcome: o,
	}
	for _, f := range extra {
		f(&rec)
	}
	r.list = append(r.list, rec)
	return rec
}

func c(v float64) *float64 { return &v }

func book(t *testing.T, code string) (Template, Bundle) {
	t.Helper()
	bundles, err := Builtin()
	require.NoError(t, err)
	_, booklets, err := Resolve(bundles, DefaultSettings())
	require.NoError(t, err)
	bk, ok := Find(booklets, code)
	require.True(t, ok, code)
	b, _ := FindBundle(bundles, DefaultBundle)
	return bk.Template, b
}

func texts(r Row) []string {
	var out []string
	for _, c := range r.Cells {
		out = append(out, c.Text)
	}
	return out
}

func TestBuiltinBundles(t *testing.T) {
	bundles, err := Builtin()
	require.NoError(t, err)
	var ids []string
	for _, b := range bundles {
		ids = append(ids, b.ID)
	}
	assert.ElementsMatch(t, []string{"fsai-safe-catering", "general"}, ids)

	ie, ok := FindBundle(bundles, "fsai-safe-catering")
	require.True(t, ok)
	assert.Equal(t, "Ireland", ie.Country)
	assert.Len(t, ie.Templates, 9)
	var supported []string
	for _, tpl := range ie.Templates {
		require.NotNil(t, tpl.Source, tpl.Code)
		assert.Len(t, tpl.Source.SHA256, 64, tpl.Code)
		if tpl.Supported() {
			supported = append(supported, tpl.Code)
		} else {
			assert.NotEmpty(t, tpl.Missing, tpl.Code)
		}
	}
	assert.Equal(t, []string{"SC1", "SC2", "SC3", "SC4", "SC8"}, supported)

	gen, ok := FindBundle(bundles, "general")
	require.True(t, ok)
	for _, tpl := range gen.Templates {
		assert.True(t, tpl.Supported(), tpl.Code)
		assert.Nil(t, tpl.Source, "the general bundle copies no official booklet")
	}

	sc4, _ := book(t, "sc4")
	assert.Equal(t, []string{"Time into hot hold"}, sc4.Unfilled())
	_, booklets, err := Resolve(bundles, DefaultSettings())
	require.NoError(t, err)
	_, ok = Find(booklets, "SC10")
	assert.False(t, ok)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		tpl  Template
		want string
	}{
		{"unknown field", Template{ID: "x", Code: "X", Title: "X", Builder: BuilderHotHold, Columns: []Column{{Label: "Probe", Field: "probe", Width: 10}}}, `uses field "probe"`},
		{"unknown builder", Template{ID: "x", Code: "X", Title: "X", Builder: "stock", Columns: []Column{{Label: "A", Width: 1}}}, "unknown builder"},
		{"no width", Template{ID: "x", Code: "X", Title: "X", Builder: BuilderHotHold, Columns: []Column{{Label: "Food", Field: "food"}}}, "needs a width"},
		{"grid without units", Template{ID: "x", Code: "X", Title: "X", Builder: BuilderUnitGrid, UnitsPerPage: 6, Columns: []Column{{Label: "Date", Field: "date", Width: 10}}}, "unit_checks"},
		{"unsupported needs a reason", Template{ID: "x", Code: "X", Title: "X"}, "missing"},
		{"two grids", Template{ID: "x", Code: "X", Title: "X", Builder: BuilderUnitGrid, UnitsPerPage: 6, Columns: []Column{{Label: "A", Field: "unit_checks", Width: 1}, {Label: "B", Field: "unit_checks", Width: 1}}}, "exactly one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorContains(t, tt.tpl.Validate(), tt.want)
		})
	}
	_, err := ParseBundle([]byte(`{"id":"p","name":"P","country":"X","templates":[],"extra":1}`))
	assert.ErrorContains(t, err, "unknown field")
}

func TestUnitGrid(t *testing.T) {
	tpl, bundle := book(t, "SC2")
	units := []haccp.Unit{
		{Name: "Walk-in fridge", Kind: haccp.RecordFridge},
		{Name: "Chest freezer", Kind: haccp.RecordFreezer},
	}
	var r recs
	// Before October: not in the book.
	r.add(haccp.RecordFridge, on(1, 0, 0).Add(-time.Minute), haccp.Reading{Unit: "Walk-in fridge", TempC: c(9)}, haccp.OutcomeFail)
	r.add(haccp.RecordFridge, on(1, 0, 30), haccp.Reading{Unit: "Walk-in fridge", TempC: c(3.2)}, haccp.OutcomePass)
	r.add(haccp.RecordFridge, on(1, 15, 0), haccp.Reading{Unit: "walk-in fridge", TempC: c(7.2)}, haccp.OutcomeFail, func(x *haccp.Record) {
		x.OutcomeReason, x.CorrectiveAction = "7.2°C is above 5.0°C", "Moved food to the prep fridge"
	})
	r.add(haccp.RecordFreezer, on(1, 9, 0), haccp.Reading{Unit: "Chest freezer", TempC: c(-12)}, haccp.OutcomePass)
	// A correction of the freezer reading, logged later: shown in its place.
	r.add(haccp.RecordFreezer, on(1, 10, 0), haccp.Reading{Unit: "Chest freezer", TempC: c(-19)}, haccp.OutcomePass, func(x *haccp.Record) {
		x.Corrects, x.CorrectionReason, x.LoggedByName = "r4", "typed the wrong number", "Ciarán Example"
	})
	// A unit not on the list shows, but never as missed.
	r.add(haccp.RecordFridge, on(2, 9, 0), haccp.Reading{Unit: "Drinks fridge", TempC: c(6)}, haccp.OutcomePass)

	b, err := Build(Input{
		Template: tpl, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin,
		Now: on(3, 13, 0), Since: on(1, 0, 30), Records: r.list, Units: units,
	})
	require.NoError(t, err)
	assert.Equal(t, "2026-10", b.Month)
	assert.Equal(t, "FSAI Safe Catering record books", b.BundleName)
	require.Len(t, b.Pages, 1)
	page := b.Pages[0]

	var heads []string
	for _, col := range page.Columns {
		heads = append(heads, col.Group+" "+col.Label)
	}
	assert.Equal(t, []string{
		" Date", "Walk-in fridge AM", "Walk-in fridge PM", "Chest freezer AM", "Chest freezer PM",
		"Drinks fridge AM", "Drinks fridge PM", " Comments/Action", " Sign",
	}, heads)
	total := 0.0
	for _, col := range page.Columns {
		total += col.Width
	}
	assert.InDelta(t, 273, total, 0.01, "the page keeps its width")

	require.Len(t, page.Rows, 31)
	day1 := page.Rows[0]
	assert.Equal(t, []string{"1st", "3.2°C", "7.2°C", "-19.0°C", "missed", "", "",
		"Walk-in fridge PM 7.2°C: 7.2°C is above 5.0°C. Action: Moved food to the prep fridge\nChest freezer AM -19.0°C: Corrected: typed the wrong number",
		"Aoife, Ciarán"}, texts(day1))
	assert.Equal(t, FlagFail, day1.Cells[2].Flag)
	assert.Equal(t, FlagMissed, day1.Cells[4].Flag)

	// Day 2: everything listed missed; the drinks fridge is shown.
	assert.Equal(t, []string{"2nd", "missed", "missed", "missed", "missed", "6.0°C", ""}, texts(page.Rows[1])[:7])
	// Day 3 at 13:00: AM is missed, PM is still due.
	assert.Equal(t, []string{"3rd", "missed", "", "missed", ""}, texts(page.Rows[2])[:5])
	assert.Equal(t, []string{"31st", "", ""}, texts(page.Rows[30])[:3])

	assert.Len(t, b.Gaps, 7)
	assert.Equal(t, Gap{Date: "2026-10-01", Unit: "Chest freezer", Check: "PM"}, b.Gaps[0])
	assert.Equal(t, 4, b.Records, "the correction stands in for the record it corrects")
	assert.Equal(t, 1, b.Corrected)
	assert.Equal(t, int64(2), b.FirstSeq)
	assert.Equal(t, int64(6), b.LastSeq)
}

func TestUnitGridPages(t *testing.T) {
	tpl, bundle := book(t, "SC2")
	var units []haccp.Unit
	for i := range 8 {
		units = append(units, haccp.Unit{Name: fmt.Sprintf("Fridge %d", i+1), Kind: haccp.RecordFridge})
	}
	units[7].Checks = []haccp.CheckTime{{Label: "Open", From: "00:00"}, {Label: "Mid", From: "12:00"}, {Label: "Close", From: "18:00"}}
	b, err := Build(Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.February, Loc: dublin, Now: on(1, 0, 0), Units: units})
	require.NoError(t, err)
	require.Len(t, b.Pages, 2)
	assert.Len(t, b.Pages[0].Columns, 3+12)
	assert.Len(t, b.Pages[1].Columns, 3+5)
	assert.Len(t, b.Pages[1].Rows, 28, "February 2026")
	assert.Equal(t, "Close", b.Pages[1].Columns[5].Label)
	assert.Len(t, b.Gaps, 8*2*28+28, "every check in February was missed")
}

func TestCooking(t *testing.T) {
	tpl, bundle := book(t, "SC3")
	var r recs
	r.add(haccp.RecordCook, on(5, 12, 0), haccp.Reading{Food: "Chicken fillets", TempC: c(85)}, haccp.OutcomePass)
	r.add(haccp.RecordCook, on(5, 13, 0), haccp.Reading{Food: "Whole chicken", TempC: c(70), HoldSeconds: ptr(int64(120))}, haccp.OutcomePass)
	r.add(haccp.RecordCooling, on(5, 14, 0), haccp.Reading{Food: "chicken fillets", CookedAt: ptr(on(5, 12, 5)), IntoFridgeAt: ptr(on(5, 13, 45))}, haccp.OutcomePass,
		func(x *haccp.Record) { x.LoggedByName = "Ciarán Example" })
	// A cooling with no cooking record: its own row.
	r.add(haccp.RecordCooling, on(5, 18, 0), haccp.Reading{Food: "Rice", CookedAt: ptr(on(5, 15, 0)), TempAtCutoffC: c(14)}, haccp.OutcomeFail, func(x *haccp.Record) {
		x.OutcomeReason, x.CorrectiveAction = "not in the fridge within 2 h", "Discarded"
	})
	r.add(haccp.RecordReheat, on(6, 12, 30), haccp.Reading{Food: "Lasagne", TempC: c(68)}, haccp.OutcomeFail, func(x *haccp.Record) {
		x.OutcomeReason, x.CorrectiveAction = "68.0°C is below 70.0°C", "Reheated again to 76°C"
	})

	b, err := Build(Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0), Records: r.list})
	require.NoError(t, err)
	require.Len(t, b.Pages, 1)
	rows := b.Pages[0].Rows
	require.Len(t, rows, 4)
	assert.Equal(t, []string{"05/10/26", "Chicken fillets", "N/A", "12:00", "85.0°C", "Aoife", "13:45", "Ciarán", "", "", "", "#1, #3"}, texts(rows[0]))
	assert.Equal(t, []string{"05/10/26", "Whole chicken", "N/A", "13:00", "70.0°C for 2 min", "Aoife", "", "", "", "", "", "#2"}, texts(rows[1]))
	assert.Equal(t, []string{"05/10/26", "Rice", "", "15:00", "", "", "not in fridge, 14.0°C at 2.5 h", "Aoife", "", "",
		"Cooling: not in the fridge within 2 h. Action: Discarded", "#4"}, texts(rows[2]))
	assert.Equal(t, FlagFail, rows[2].Cells[6].Flag)
	assert.Equal(t, []string{"06/10/26", "Lasagne", "", "", "", "", "", "", "68.0°C", "Aoife", "68.0°C is below 70.0°C. Action: Reheated again to 76°C", "#5"}, texts(rows[3]))
	assert.Equal(t, FlagFail, rows[3].Cells[8].Flag)
	assert.Empty(t, b.Gaps)
	assert.Empty(t, b.Unfilled)
	assert.Equal(t, 5, b.Records)
}

func TestHotHold(t *testing.T) {
	tpl, bundle := book(t, "SC4")
	var r recs
	for i, v := range []float64{82, 70, 66, 64} {
		r.add(haccp.RecordHotHold, on(7, 12+i, 0), haccp.Reading{Food: "Chicken soup", TempC: c(v)}, haccp.OutcomePass)
	}
	r.add(haccp.RecordHotHold, on(7, 12, 10), haccp.Reading{Food: "Beef stew", TempC: c(58)}, haccp.OutcomeFail, func(x *haccp.Record) {
		x.OutcomeReason, x.CorrectiveAction = "58.0°C is below 63.0°C", "Thrown out"
	})
	b, err := Build(Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0), Records: r.list})
	require.NoError(t, err)
	rows := b.Pages[0].Rows
	require.Len(t, rows, 3)
	assert.Equal(t, []string{"07/10/26", "Chicken soup", "", "82.0°C at 12:00", "70.0°C at 13:00", "66.0°C at 14:00", "", "Aoife", "#1, #2, #3"}, texts(rows[0]))
	assert.Equal(t, []string{"07/10/26", "Beef stew", "", "58.0°C at 12:10", "", "", "1st check: 58.0°C is below 63.0°C. Action: Thrown out", "Aoife", "#5"}, texts(rows[1]))
	assert.Equal(t, FlagFail, rows[1].Cells[3].Flag)
	assert.Equal(t, "64.0°C at 15:00", rows[2].Cells[3].Text, "a fourth check starts a new row")
	assert.Equal(t, []string{"Time into hot hold"}, b.Unfilled)
}

func TestUnsupported(t *testing.T) {
	tpl, bundle := book(t, "SC7")
	_, err := Build(Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.October})
	assert.ErrorContains(t, err, "cannot fill SC7")
}

func TestOrdinal(t *testing.T) {
	for d, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 23: "23rd", 31: "31st"} {
		assert.Equal(t, want, ordinal(d))
	}
}

func ptr[T any](v T) *T { return &v }

func TestDeliveries(t *testing.T) {
	tpl, bundle := book(t, "SC1")
	yes, no := true, false
	var r recs
	r.add(haccp.RecordDelivery, on(1, 9, 0), haccp.Reading{Supplier: "Synthetic Meats", Product: "Roast beef", BatchCode: "123-11-20", UseBy: "2026-10-07",
		DeliveryKind: haccp.DeliveryChilled, TempC: c(4), PackagingOK: &yes, InDate: &yes, VehicleOK: &yes}, haccp.OutcomePass)
	r.add(haccp.RecordDelivery, on(2, 9, 0), haccp.Reading{Supplier: "Synthetic Poultry", Product: "Cooked turkey", BatchCode: "T32401",
		DeliveryKind: haccp.DeliveryChilled, TempC: c(15), PackagingOK: &yes, InDate: &yes, VehicleOK: &no}, haccp.OutcomeFail, func(x *haccp.Record) {
		x.OutcomeReason = "The delivery vehicle failed its check. 15.0°C is above the 8.0°C limit. Reject the delivery."
		x.CorrectiveAction = "Rejected, rang supplier"
	})
	r.add(haccp.RecordDelivery, on(3, 9, 0), haccp.Reading{Supplier: "Synthetic Dry Goods", Product: "Flour",
		DeliveryKind: haccp.DeliveryAmbient, PackagingOK: &no, InDate: &yes}, haccp.OutcomeFail, func(x *haccp.Record) {
		x.OutcomeReason, x.CorrectiveAction = "Packaging is not intact. Reject the delivery.", "Returned"
	})
	b, err := Build(Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0), Records: r.list})
	require.NoError(t, err)
	rows := b.Pages[0].Rows
	require.Len(t, rows, 3)
	assert.Equal(t, []string{"01/10/26", "Roast beef", "123-11-20", "Synthetic Meats", "07/10/26", "4.0°C", "OK", "", "Aoife", "#1"}, texts(rows[0]))
	assert.Equal(t, "2026-10-01", rows[0].Date)
	assert.Equal(t, []string{"02/10/26", "Cooked turkey", "T32401", "Synthetic Poultry", "", "15.0°C", "",
		"The delivery vehicle failed its check. 15.0°C is above the 8.0°C limit. Reject the delivery. Action: Rejected, rang supplier", "Aoife", "#2"}, texts(rows[1]))
	assert.Equal(t, FlagFail, rows[1].Cells[5].Flag, "the temperature was out")
	assert.Equal(t, FlagFail, rows[1].Cells[6].Flag)
	assert.Equal(t, "ambient", rows[2].Cells[5].Text)
	assert.Empty(t, rows[2].Cells[5].Flag, "failed on packaging, not temperature")
	assert.Empty(t, rows[2].Cells[6].Text, "vehicle not checked")
}

func TestDaily(t *testing.T) {
	bundles, err := Builtin()
	require.NoError(t, err)
	bundle, booklets, err := Resolve(bundles, DefaultSettings())
	require.NoError(t, err)
	sc8, ok := Find(booklets, "SC8")
	require.True(t, ok)
	var sections []Template
	for _, id := range sc8.Template.Sections {
		s, ok := Find(booklets, id)
		require.True(t, ok)
		sections = append(sections, s.Template)
	}
	yes := true
	var r recs
	r.add(haccp.RecordDelivery, on(2, 9, 0), haccp.Reading{Supplier: "Synthetic Meats", Product: "Roast beef", DeliveryKind: haccp.DeliveryChilled,
		TempC: c(4), PackagingOK: &yes, InDate: &yes}, haccp.OutcomePass)
	r.add(haccp.RecordFridge, on(2, 10, 0), haccp.Reading{Unit: "Walk-in fridge", TempC: c(3)}, haccp.OutcomePass)
	r.add(haccp.RecordHotHold, on(3, 12, 0), haccp.Reading{Food: "Soup", TempC: c(70)}, haccp.OutcomePass)
	units := []haccp.Unit{{Name: "Walk-in fridge", Kind: haccp.RecordFridge}}

	b, err := Build(Input{Template: sc8.Template, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin,
		Now: on(3, 13, 0), Since: on(2, 9, 0), Records: r.list, Units: units, Sections: sections})
	require.NoError(t, err)
	require.Len(t, b.Pages, 8, "two days (the 2nd and 3rd) of four sections")
	var heads []string
	for _, p := range b.Pages[:4] {
		heads = append(heads, p.Day+" "+p.Title)
	}
	assert.Equal(t, []string{
		"2026-10-02 Food Delivery Records",
		"2026-10-02 Fridge/Freezer/Chill Display Temperature Records",
		"2026-10-02 Cooking/Cooling/Reheating Records",
		"2026-10-02 Hot Hold/Display Records",
	}, heads)
	assert.Equal(t, "Food item", b.Pages[0].Columns[0].Label, "the date column goes: the sheet has the date")
	assert.Equal(t, "Roast beef", b.Pages[0].Rows[0].Cells[0].Text)
	assert.Equal(t, []string{"3.0°C", "missed"}, texts(b.Pages[1].Rows[0])[:2], "the walk-in's AM and PM on the 2nd")
	assert.Empty(t, b.Pages[3].Rows, "no hot holding on the 2nd")
	assert.Equal(t, "Soup", b.Pages[7].Rows[0].Cells[0].Text)
	assert.Equal(t, 3, b.Records)
	assert.Equal(t, []string{"Time into hot hold"}, b.Unfilled)
	require.Len(t, b.Gaps, 2)
	assert.Equal(t, Gap{Date: "2026-10-02", Unit: "Walk-in fridge", Check: "PM"}, b.Gaps[0])
	assert.Equal(t, Gap{Date: "2026-10-03", Unit: "Walk-in fridge", Check: "AM"}, b.Gaps[1])
}

func TestDailyValidate(t *testing.T) {
	_, err := ParseBundle([]byte(`{"id":"p","name":"P","country":"X","templates":[{"id":"d","code":"D","title":"Daily","builder":"daily","sections":["nope"]}]}`))
	assert.ErrorContains(t, err, `section "nope"`)
	assert.ErrorContains(t, Template{ID: "d", Code: "D", Title: "D", Builder: BuilderDaily}.Validate(), "needs its sections")
	assert.ErrorContains(t, Template{ID: "d", Code: "D", Title: "D", Builder: BuilderHotHold, Sections: []string{"x"}, Columns: []Column{{Label: "F", Field: "food", Width: 1}}}.Validate(), "only a daily record")
	s := RecordBookSettings{Bundle: DefaultBundle, Custom: []Template{{ID: "custom-d", Code: "D9", Title: "Mine", Builder: BuilderDaily, Sections: []string{"fsai-sc2"}}}}
	assert.ErrorContains(t, s.Normalize().Validate(bundles(t)), "cannot be a daily record")
}
