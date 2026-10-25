package books

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func bundles(t *testing.T) []Bundle {
	t.Helper()
	b, err := Builtin()
	require.NoError(t, err)
	return b
}

func TestSettingsNormalize(t *testing.T) {
	no := false
	s := RecordBookSettings{Bundle: " general ", Books: []BookSetting{
		{Template: "general-fridges"},
		{Template: " general-cooking ", Title: "  Cooking log ", Notes: []string{" ", "Plan section 4"}},
		{Template: "general-hot-hold", ManagerCheck: &no},
		{Template: "general-fridges", CopyFile: " ABC "},
	}}.Normalize()
	assert.Equal(t, "general", s.Bundle)
	require.Len(t, s.Books, 3, "an entry that changes nothing is dropped")
	assert.Equal(t, BookSetting{Template: "general-cooking", Title: "Cooking log", Notes: []string{"Plan section 4"}}, s.Books[0])
	assert.Equal(t, "abc", s.Books[2].CopyFile)
	assert.NotNil(t, s.Custom)
}

func custom(code string) Template {
	return Template{
		ID: CustomPrefix + strings.ToLower(code), Code: code, Title: "Relevés de température", Builder: BuilderUnitGrid, UnitsPerPage: 4,
		Columns: []Column{{Label: "Jour", Field: "date", Width: 1}, {Label: "Relevés", Field: "unit_checks", Width: 8}, {Label: "Visa", Field: "signed", Width: 2}},
	}
}

func TestSettingsValidate(t *testing.T) {
	all := bundles(t)
	assert.NoError(t, DefaultSettings().Validate(all))
	tests := []struct {
		name string
		s    RecordBookSettings
		want string
	}{
		{"unknown bundle", RecordBookSettings{Bundle: "fr-ddpp"}, `no record book bundle "fr-ddpp"`},
		{"booklet from another bundle", RecordBookSettings{Bundle: "general", Books: []BookSetting{{Template: "fsai-sc2", Off: true}}}, `no booklet "fsai-sc2"`},
		{"two entries", RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{{Template: "fsai-sc2", Off: true}, {Template: "fsai-sc2", Title: "X"}}}, "two sets of changes"},
		{"unsupported booklet", RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{{Template: "fsai-sc7", Title: "Fitness"}}}, "cannot be filled in yet"},
		{"bad field", RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{{Template: "fsai-sc4", Columns: []Column{{Label: "Probe", Field: "probe", Width: 1}}}}}, `field "probe"`},
		{"grid layout", RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{{Template: "fsai-sc2", UnitsPerPage: 20}}}, "1 to 12"},
		{"bad copy", RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{{Template: "fsai-sc2", CopyFile: "abc"}}}, "SHA-256"},
		{"custom id", RecordBookSettings{Bundle: DefaultBundle, Custom: []Template{func() Template { c := custom("R1"); c.ID = "r1"; return c }()}}, `start with "custom-"`},
		{"custom code taken", RecordBookSettings{Bundle: DefaultBundle, Custom: []Template{custom("SC2")}}, "already used"},
		{"custom without kind", RecordBookSettings{Bundle: DefaultBundle, Custom: []Template{func() Template { c := custom("R1"); c.Builder = ""; return c }()}}, "what kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorContains(t, tt.s.Normalize().Validate(all), tt.want)
		})
	}
	ok := RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{{Template: "fsai-sc7", Off: true}, {Template: custom("R1").ID, Off: true}}, Custom: []Template{custom("R1")}}
	assert.NoError(t, ok.Normalize().Validate(all), "unsupported booklets can be turned off; added booklets can be changed")
}

func TestResolve(t *testing.T) {
	all := bundles(t)
	no := false
	sc2 := "b987e38308398fda3b688a987e69de6ce9a9eaeb3511a7fc75d53b1e4ff528e9"
	other := strings.Repeat("a", 64)
	s := RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{
		{Template: "fsai-sc2", Title: "Fridge log", UnitsPerPage: 4, Notes: []string{"Probe the dummy food."}, CopyFile: sc2},
		{Template: "fsai-sc3", Columns: []Column{{Label: "Date", Field: "date", Width: 1}, {Label: "Food", Field: "food", Width: 2}, {Label: "Core temp", Field: "core_temp", Width: 1}}, CopyFile: other},
		{Template: "fsai-sc4", ManagerCheck: &no},
		{Template: "fsai-sc9", Off: true},
	}, Custom: []Template{custom("R1")}}.Normalize()

	bundle, booklets, err := Resolve(all, s)
	require.NoError(t, err)
	assert.Equal(t, "Ireland", bundle.Country)
	require.Len(t, booklets, 10, "nine FSAI booklets and one added")

	get := func(code string) Booklet {
		b, ok := Find(booklets, code)
		require.True(t, ok, code)
		return b
	}
	b2 := get("sc2")
	assert.Equal(t, "Fridge log", b2.Template.Title)
	assert.Equal(t, 4, b2.Template.UnitsPerPage)
	assert.Equal(t, "Probe the dummy food.", b2.Template.Notes[len(b2.Template.Notes)-1], "added under the booklet's own notes")
	assert.Len(t, b2.Template.Notes, 3)
	assert.Equal(t, []string{"title", "notes", "sheet layout"}, b2.Changed)
	require.NotNil(t, b2.Base)
	assert.Equal(t, "Fridge/Freezer/Chill Display Temperature Records", b2.Base.Title)
	assert.Equal(t, &CopyCheck{File: sc2, Known: true, Matches: true}, b2.Copy)

	b3 := get("SC3")
	require.Len(t, b3.Template.Columns, 3)
	assert.InDelta(t, PageWidth/4, b3.Template.Columns[0].Width, 0.001, "widths are shares of the page")
	assert.InDelta(t, PageWidth/2, b3.Template.Columns[1].Width, 0.001)
	assert.Equal(t, &CopyCheck{File: other, Known: true, Matches: false}, b3.Copy)

	assert.False(t, get("SC4").Template.ManagerCheck)
	assert.True(t, get("SC9").Off)
	assert.Nil(t, get("SC1").Base, "unchanged")
	assert.Empty(t, get("SC1").Changed)

	r1 := get("R1")
	assert.True(t, r1.Custom)
	assert.Nil(t, r1.Base)
	assert.Equal(t, booklets[len(booklets)-1].Template.ID, r1.Template.ID, "added booklets come last")

	// The changed booklets fill in with their own columns.
	var r recs
	r.add(haccp.RecordCook, on(5, 12, 0), haccp.Reading{Food: "Chicken", TempC: c(80)}, haccp.OutcomePass)
	book, err := Build(Input{Template: b3.Template, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0), Records: r.list})
	require.NoError(t, err)
	assert.Equal(t, []string{"05/10/26", "Chicken", "80.0°C"}, texts(book.Pages[0].Rows[0]))

	fr, err := Build(Input{Template: r1.Template, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0),
		Units: []haccp.Unit{{Name: "Frigo 1", Kind: haccp.RecordFridge}}})
	require.NoError(t, err)
	assert.Equal(t, "Jour", fr.Pages[0].Columns[0].Label)
	assert.Equal(t, "Frigo 1", fr.Pages[0].Columns[1].Group)

	// Switching to the general bundle.
	gen, gb, err := Resolve(all, RecordBookSettings{Bundle: "general"})
	require.NoError(t, err)
	assert.Equal(t, "Any country", gen.Country)
	assert.Len(t, gb, 3)
	assert.Nil(t, gb[0].Copy)
}

func TestSummarize(t *testing.T) {
	all := bundles(t)
	yes := true
	cur := RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{
		{Template: "fsai-sc2", Title: "Fridge log", CopyFile: strings.Repeat("a", 64)},
		{Template: "fsai-sc9", Off: true},
	}, Custom: []Template{custom("R1"), custom("R2")}}
	next := RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{
		{Template: "fsai-sc9"},
		{Template: "fsai-sc2", Title: "Fridge log", CopyFile: strings.Repeat("b", 64), ManagerCheck: &yes},
		{Template: "fsai-sc3", Notes: []string{"Probe thick parts."}},
	}, Custom: []Template{func() Template { c := custom("R1"); c.Title = "Températures"; return c }(), custom("R3")}}
	assert.Equal(t, []string{
		"R1 changed",
		"R3 Relevés de température added",
		"R2 Relevés de température removed",
		"SC2: manager check changed, copy replaced",
		"SC9: turned back on",
		"SC3: notes changed",
	}, Summarize(all, cur, next))
	assert.Empty(t, Summarize(all, cur, cur))
	reordered := cur
	reordered.Books = []BookSetting{cur.Books[1], cur.Books[0]}
	assert.Empty(t, Summarize(all, cur, reordered), "order does not matter")
}
