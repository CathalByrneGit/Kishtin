package books

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func TestChecklistBook(t *testing.T) {
	tpl, bundle := book(t, "SC5")
	require.NotEmpty(t, tpl.Checklist)
	assert.Equal(t, "Pest Control", tpl.Checklist[4].Title)
	yes, no := true, false
	var r recs
	r.add(haccp.RecordInspection, on(8, 10, 0), haccp.Reading{Checklist: tpl.ID, Answers: []haccp.Answer{
		{Section: "Food Storage", Item: "Is food in fridges/freezers covered?", OK: &yes},
		{Section: "Pest Control", Item: "Are insectocutors (if provided) properly maintained?", OK: &no, Action: "Replace UV tubes."},
		{Section: "Pest Control", Item: "Is food properly protected from risk of contamination by pests?"},
	}}, haccp.OutcomeFail, func(x *haccp.Record) { x.LoggedByName = "Máire Manager" })
	b, err := Build(Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0), Records: r.list})
	require.NoError(t, err)
	require.Len(t, b.Pages, 1)
	p := b.Pages[0]
	assert.Equal(t, "Date: 08/10/26 · checked by Máire Manager · Kishtin ref #1", p.SheetLabel)
	require.Len(t, p.Rows, 5)
	assert.True(t, p.Rows[0].Heading)
	assert.Equal(t, "Food Storage", p.Rows[0].Cells[0].Text)
	assert.Equal(t, []string{"Is food in fridges/freezers covered?", "Yes", "", ""}, texts(p.Rows[1]))
	assert.Equal(t, []string{"Are insectocutors (if provided) properly maintained?", "", "No", "Replace UV tubes."}, texts(p.Rows[3]))
	assert.Equal(t, FlagFail, p.Rows[3].Cells[2].Flag)
	assert.Equal(t, []string{"Is food properly protected from risk of contamination by pests?", "", "", ""}, texts(p.Rows[4]), "not checked")
	assert.Equal(t, 1, b.Records)
}

func TestTrainingBook(t *testing.T) {
	tpl, bundle := book(t, "SC6")
	var r recs
	r.add(haccp.RecordTraining, on(1, 10, 0).AddDate(0, -2, 0), haccp.Reading{Staff: "Liam Cook", TrainingKind: haccp.TrainingInHouse,
		Topics: []string{"Cooking", "Storage"}, Trainer: "Aoife Chef", OnDate: "2026-08-01"}, haccp.OutcomeRecorded)
	r.add(haccp.RecordTraining, on(12, 10, 0), haccp.Reading{Staff: "liam cook", TrainingKind: haccp.TrainingInHouse,
		Topics: []string{"Cooking", "Sous-vide"}, Trainer: "Máire Manager"}, haccp.OutcomeRecorded)
	r.add(haccp.RecordTraining, on(14, 10, 0), haccp.Reading{Staff: "Liam Cook", TrainingKind: haccp.TrainingFurther,
		Topics: []string{"Food safety level 2"}, Provider: "Synthetic Training Ltd", OnDate: "2026-10-13"}, haccp.OutcomeRecorded)
	r.add(haccp.RecordTraining, on(1, 10, 0).AddDate(0, 1, 0), haccp.Reading{Staff: "Sean Floor", TrainingKind: haccp.TrainingInHouse,
		Topics: []string{"Cleaning"}, Trainer: "Aoife Chef"}, haccp.OutcomeRecorded)

	b, err := Build(Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0), Records: r.list,
		Roles: map[string]string{"Liam Cook": "line cook"}})
	require.NoError(t, err)
	require.Len(t, b.Pages, 2, "Liam only: Sean's training is after October")
	in := b.Pages[0]
	assert.Equal(t, "Name: Liam Cook · Position: line cook", in.SheetLabel)
	assert.Equal(t, in.Sheet, b.Pages[1].Sheet, "both tables on Liam's sheet")
	find := func(topic string) Row {
		for _, row := range in.Rows {
			if !row.Heading && row.Cells[0].Text == topic {
				return row
			}
		}
		t.Fatalf("no row for %s", topic)
		return Row{}
	}
	assert.Equal(t, []string{"Cooking", "01/08/26, 12/10/26", "Aoife Chef, Máire Manager", ""}, texts(find("Cooking")))
	assert.Equal(t, []string{"Cleaning", "", "", ""}, texts(find("Cleaning")), "every topic is listed, trained or not")
	assert.Equal(t, []string{"Sous-vide", "12/10/26", "Máire Manager", ""}, texts(find("Sous-vide")), "a topic not on the list goes under Other")
	assert.True(t, in.Rows[len(in.Rows)-2].Heading)
	assert.Equal(t, "Other", in.Rows[len(in.Rows)-2].Cells[0].Text)
	assert.Equal(t, []string{"Food safety level 2", "Synthetic Training Ltd", "13/10/26", ""}, texts(b.Pages[1].Rows[0]))
	assert.Equal(t, 3, b.Records)
}

func TestFitnessAndTransportBooks(t *testing.T) {
	yes, no := true, false
	var r recs
	r.add(haccp.RecordFitness, on(5, 9, 0), haccp.Reading{Staff: "Liam Cook", FitnessReason: haccp.FitnessReturn, Fit: &no}, haccp.OutcomeRecorded,
		func(x *haccp.Record) {
			x.CorrectiveAction, x.LoggedByName = "Sent home and referred to GP", "Máire Manager"
		})
	r.add(haccp.RecordFitness, on(9, 9, 0), haccp.Reading{Staff: "Liam Cook", FitnessReason: haccp.FitnessReturn, Fit: &yes, OnDate: "2026-10-08"}, haccp.OutcomeRecorded,
		func(x *haccp.Record) { x.LoggedByName = "Máire Manager" })
	r.add(haccp.RecordTransport, on(6, 12, 0), haccp.Reading{Product: "Chicken curry", BatchCode: "CC-6", UseBy: "2026-10-08", Customer: "Synthetic Takeaway",
		Separated: &yes, DeliveryKind: haccp.DeliveryChilled, TempC: c(4)}, haccp.OutcomePass)
	r.add(haccp.RecordTransport, on(7, 12, 0), haccp.Reading{Product: "Beef stew", Customer: "Synthetic Cafe",
		Separated: &no, DeliveryKind: haccp.DeliveryHot, TempC: c(60)}, haccp.OutcomeFail, func(x *haccp.Record) {
		x.OutcomeReason = "Raw and ready-to-eat food were not kept apart. 60.0°C is below the 63.0°C limit."
		x.CorrectiveAction = "Discarded, customer refunded"
	})

	sc7, bundle := book(t, "SC7")
	b, err := Build(Input{Template: sc7, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0), Records: r.list})
	require.NoError(t, err)
	rows := b.Pages[0].Rows
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"05/10/26", "Liam Cook", "Return to work after illness", "Not fit", "Sent home and referred to GP", "Máire Manager", "#1"}, texts(rows[0]))
	assert.Equal(t, FlagFail, rows[0].Cells[3].Flag)
	assert.Equal(t, "08/10/26", rows[1].Cells[0].Text, "the assessment date, not the day it was logged")

	sc9, _ := book(t, "SC9")
	b, err = Build(Input{Template: sc9, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(25, 12, 0), Records: r.list})
	require.NoError(t, err)
	rows = b.Pages[0].Rows
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"06/10/26", "Chicken curry", "CC-6 / 08/10/26", "Synthetic Takeaway", "Yes", "4.0°C", "", "Aoife", "#3"}, texts(rows[0]))
	assert.Equal(t, "No", rows[1].Cells[4].Text)
	assert.Equal(t, FlagFail, rows[1].Cells[4].Flag)
	assert.Equal(t, FlagFail, rows[1].Cells[5].Flag)
}

func TestChecklistEdits(t *testing.T) {
	all := bundles(t)
	s := RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{{Template: "fsai-sc5", Checklist: []ChecklistSection{
		{Title: " Bar ", Items: []string{"Is the ice machine clean?", " "}},
	}}}}.Normalize()
	assert.Equal(t, []ChecklistSection{{Title: "Bar", Items: []string{"Is the ice machine clean?"}}}, s.Books[0].Checklist)
	_, booklets, err := Resolve(all, s)
	require.NoError(t, err)
	b, _ := Find(booklets, "SC5")
	assert.Equal(t, []string{"questions"}, b.Changed)
	assert.Equal(t, "Bar", b.Template.Checklist[0].Title)
	assert.Equal(t, []string{"SC5: questions changed"}, Summarize(all, DefaultSettings(), s))
	bad := RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{{Template: "fsai-sc2", Checklist: []ChecklistSection{{Items: []string{"x"}}}}}}
	assert.ErrorContains(t, bad.Validate(all), "only an inspection checklist or training record")
}
