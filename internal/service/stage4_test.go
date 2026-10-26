package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func TestInspectionTrainingFitnessTransport(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	chef, err := f.svc.CreateUser(ctx, f.manager, NewUser{Name: "Aoife Chef", Role: "head_chef", PIN: "2222"})
	require.NoError(t, err)
	chefP := &auth.Principal{User: chef, Method: auth.MethodPIN}
	yes, no := true, false

	insp := NewRecord{Type: haccp.RecordInspection, Reading: haccp.Reading{Checklist: "fsai-sc5", Answers: []haccp.Answer{
		{Section: "Pest Control", Item: "Are insectocutors (if provided) properly maintained?", OK: &no, Action: "Replace UV tubes."},
		{Section: "Waste Control", Item: "Is waste in food rooms stored correctly?", OK: &yes},
	}}}
	_, err = f.svc.LogRecord(ctx, f.cook, insp)
	assert.Equal(t, KindForbidden, KindOf(err), "inspections are for the head chef or manager")
	rec, err := f.svc.LogRecord(ctx, chefP, insp)
	require.NoError(t, err)
	assert.Equal(t, haccp.OutcomeFail, rec.Outcome)
	assert.Equal(t, "Are insectocutors (if provided) properly maintained? Replace UV tubes.", rec.CorrectiveAction, "the item actions are the corrective action")

	train := NewRecord{Type: haccp.RecordTraining, Reading: haccp.Reading{Staff: "Liam Cook", TrainingKind: haccp.TrainingInHouse, Topics: []string{"Cooking"}, Trainer: "Aoife Chef"}}
	_, err = f.svc.LogRecord(ctx, f.cook, train)
	assert.Equal(t, KindForbidden, KindOf(err))
	_, err = f.svc.LogRecord(ctx, chefP, train)
	require.NoError(t, err)

	fit := NewRecord{Type: haccp.RecordFitness, Reading: haccp.Reading{Staff: "Liam Cook", FitnessReason: haccp.FitnessReturn, Fit: &no}}
	_, err = f.svc.LogRecord(ctx, chefP, fit)
	assert.Equal(t, KindForbidden, KindOf(err), "fitness to work is for a manager with a password")
	_, err = f.svc.LogRecord(ctx, f.manager, fit)
	assert.ErrorContains(t, err, "say what action was taken")
	fit.CorrectiveAction = "Sent home and referred to GP"
	fitRec, err := f.svc.LogRecord(ctx, f.manager, fit)
	require.NoError(t, err)
	assert.Equal(t, haccp.OutcomeRecorded, fitRec.Outcome)

	tr := NewRecord{Type: haccp.RecordTransport, Reading: haccp.Reading{Product: "Chicken curry", Customer: "Synthetic Takeaway",
		Separated: &yes, DeliveryKind: haccp.DeliveryHot, TempC: fp(70)}}
	trRec, err := f.svc.LogRecord(ctx, f.cook, tr)
	require.NoError(t, err)
	assert.Equal(t, haccp.OutcomePass, trRec.Outcome)
	require.NotNil(t, trRec.LimitScope)

	// Only managers see fitness records.
	all, err := f.svc.ListRecords(ctx, f.manager, RecordQuery{})
	require.NoError(t, err)
	assert.Len(t, all, 4)
	forChef, err := f.svc.ListRecords(ctx, chefP, RecordQuery{})
	require.NoError(t, err)
	assert.Len(t, forChef, 3)
	for _, r := range forChef {
		assert.NotEqual(t, haccp.RecordFitness, r.Type)
	}
	_, err = f.svc.GetRecord(ctx, f.cook, fitRec.ID)
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = f.svc.RecordBook(ctx, chefP, "SC7", "")
	assert.Equal(t, KindForbidden, KindOf(err))
	sc7, err := f.svc.RecordBook(ctx, f.manager, "SC7", "")
	require.NoError(t, err)
	assert.Equal(t, "Not fit", sc7.Pages[0].Rows[0].Cells[3].Text)

	sc6, err := f.svc.RecordBook(ctx, chefP, "SC6", "")
	require.NoError(t, err)
	assert.Equal(t, "Name: Liam Cook · Position: line cook", sc6.Pages[0].SheetLabel)
	sc5, err := f.svc.RecordBook(ctx, chefP, "SC5", "")
	require.NoError(t, err)
	require.Len(t, sc5.Pages, 1)
	sc9, err := f.svc.RecordBook(ctx, chefP, "SC9", "")
	require.NoError(t, err)
	assert.Equal(t, "70.0°C", sc9.Pages[0].Rows[0].Cells[5].Text)
}
