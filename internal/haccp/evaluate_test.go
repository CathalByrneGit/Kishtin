package haccp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func f(v float64) *float64 { return &v }
func i(v int64) *int64     { return &v }
func b(v bool) *bool       { return &v }

func evalDefault(t *testing.T, rt RecordType, r Reading) Result {
	t.Helper()
	r = r.Normalize()
	require.NoError(t, r.Validate(rt))
	var lim *LimitVersion
	if c, ok := CheckFor(rt, r); ok {
		l := FSAIDefault(c)
		lim = &l
	}
	res, err := Evaluate(rt, r, lim)
	require.NoError(t, err)
	return res
}

func TestTemperatureLimitBoundaries(t *testing.T) {
	tests := []struct {
		name string
		rt   RecordType
		r    Reading
		want Outcome
	}{
		{"fridge below", RecordFridge, Reading{Unit: "Walk-in", TempC: f(4.9)}, OutcomePass},
		{"fridge on limit", RecordFridge, Reading{Unit: "Walk-in", TempC: f(5.0)}, OutcomePass},
		{"fridge above", RecordFridge, Reading{Unit: "Walk-in", TempC: f(5.1)}, OutcomeFail},
		{"fridge float noise on limit", RecordFridge, Reading{Unit: "Walk-in", TempC: f(5.0000001)}, OutcomePass},

		{"freezer below", RecordFreezer, Reading{Unit: "Chest", TempC: f(-18.1)}, OutcomePass},
		{"freezer on limit", RecordFreezer, Reading{Unit: "Chest", TempC: f(-18.0)}, OutcomePass},
		{"freezer above", RecordFreezer, Reading{Unit: "Chest", TempC: f(-17.9)}, OutcomeFail},

		{"cook below", RecordCook, Reading{Food: "Chicken", TempC: f(74.9)}, OutcomeFail},
		{"cook on limit", RecordCook, Reading{Food: "Chicken", TempC: f(75.0)}, OutcomePass},
		{"cook above", RecordCook, Reading{Food: "Chicken", TempC: f(75.1)}, OutcomePass},

		{"reheat below", RecordReheat, Reading{Food: "Soup", TempC: f(69.9)}, OutcomeFail},
		{"reheat on limit", RecordReheat, Reading{Food: "Soup", TempC: f(70.0)}, OutcomePass},
		{"reheat above", RecordReheat, Reading{Food: "Soup", TempC: f(70.1)}, OutcomePass},

		{"hot hold below", RecordHotHold, Reading{Food: "Curry", TempC: f(62.9)}, OutcomeFail},
		{"hot hold on limit", RecordHotHold, Reading{Food: "Curry", TempC: f(63.0)}, OutcomePass},
		{"hot hold above", RecordHotHold, Reading{Food: "Curry", TempC: f(63.1)}, OutcomePass},

		{"chilled delivery below", RecordDelivery, chilled(4.9), OutcomePass},
		{"chilled delivery on limit", RecordDelivery, chilled(5.0), OutcomePass},
		{"chilled delivery above limit", RecordDelivery, chilled(5.1), OutcomeBorderline},
		{"chilled delivery on borderline", RecordDelivery, chilled(8.0), OutcomeBorderline},
		{"chilled delivery above borderline", RecordDelivery, chilled(8.1), OutcomeFail},

		{"frozen delivery below", RecordDelivery, frozen(-18.1), OutcomePass},
		{"frozen delivery on limit", RecordDelivery, frozen(-18.0), OutcomePass},
		{"frozen delivery above limit", RecordDelivery, frozen(-17.9), OutcomeBorderline},
		{"frozen delivery on borderline", RecordDelivery, frozen(-15.0), OutcomeBorderline},
		{"frozen delivery above borderline", RecordDelivery, frozen(-14.9), OutcomeFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := evalDefault(t, tc.rt, tc.r)
			assert.Equal(t, tc.want, res.Outcome, res.Reason)
			assert.NotEmpty(t, res.Reason)
		})
	}
}

func chilled(c float64) Reading {
	return Reading{Supplier: "Synthetic Dairy Ltd", Product: "Cream", DeliveryKind: DeliveryChilled, TempC: &c, PackagingOK: b(true), InDate: b(true)}
}

func frozen(c float64) Reading {
	return Reading{Supplier: "Synthetic Frozen Ltd", Product: "Peas", DeliveryKind: DeliveryFrozen, TempC: &c, PackagingOK: b(true), InDate: b(true)}
}

func TestDeliveryChecks(t *testing.T) {
	ambient := Reading{Supplier: "Synthetic Dry Goods", Product: "Flour", DeliveryKind: DeliveryAmbient, PackagingOK: b(true), InDate: b(true)}
	assert.Equal(t, OutcomePass, evalDefault(t, RecordDelivery, ambient).Outcome)

	torn := ambient
	torn.PackagingOK = b(false)
	res := evalDefault(t, RecordDelivery, torn)
	assert.Equal(t, OutcomeFail, res.Outcome)
	assert.Equal(t, "Packaging is not intact. Reject the delivery.", res.Reason)

	old := chilled(3)
	old.InDate = b(false)
	res = evalDefault(t, RecordDelivery, old)
	assert.Equal(t, OutcomeFail, res.Outcome, "out of date fails even at a good temperature")
	assert.Equal(t, "Not in date. Reject the delivery.", res.Reason)

	warmAndTorn := chilled(9)
	warmAndTorn.PackagingOK = b(false)
	res = evalDefault(t, RecordDelivery, warmAndTorn)
	assert.Contains(t, res.Reason, "Packaging is not intact.")
	assert.Contains(t, res.Reason, "9.0°C is above the 8.0°C limit.")

	assert.Contains(t, evalDefault(t, RecordDelivery, chilled(6)).Reason, "Accept only if used or stored straight away.")
	assert.Contains(t, evalDefault(t, RecordDelivery, chilled(8.1)).Reason, "Reject the delivery.")

	unticked := chilled(3)
	unticked.InDate = nil
	assert.Error(t, unticked.Normalize().Validate(RecordDelivery), "both ticks must be answered")
}

func TestBatchShelfLife(t *testing.T) {
	five := 5
	r := Reading{Product: "Grape soda syrup", MadeOn: "2026-05-01", ShelfLifeDays: &five}.Normalize()
	assert.Equal(t, "2026-05-06", r.UseBy)
	require.NoError(t, r.Validate(RecordBatchLabel))

	mismatch := Reading{Product: "Syrup", MadeOn: "2026-05-01", UseBy: "2026-05-09", ShelfLifeDays: &five}.Normalize()
	assert.Error(t, mismatch.Validate(RecordBatchLabel))

	tooLong := 400
	assert.Error(t, Reading{Product: "Syrup", MadeOn: "2026-05-01", ShelfLifeDays: &tooLong}.Normalize().Validate(RecordBatchLabel))
}

func TestCoolingBoundaries(t *testing.T) {
	cooked := time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := cooked.Add(d); return &v }
	tests := []struct {
		name string
		r    Reading
		want Outcome
	}{
		{"into fridge before 2h", Reading{IntoFridgeAt: at(119 * time.Minute)}, OutcomePass},
		{"into fridge at 2h", Reading{IntoFridgeAt: at(2 * time.Hour)}, OutcomePass},
		{"into fridge after 2h, no cutoff temp", Reading{IntoFridgeAt: at(2*time.Hour + time.Second)}, OutcomeFail},
		{"late, below 10 at cutoff", Reading{IntoFridgeAt: at(3 * time.Hour), TempAtCutoffC: f(9.9)}, OutcomeBorderline},
		{"late, on 10 at cutoff", Reading{TempAtCutoffC: f(10.0)}, OutcomeBorderline},
		{"late, above 10 at cutoff", Reading{TempAtCutoffC: f(10.1)}, OutcomeFail},
		{"not in fridge, no temp", Reading{}, OutcomeFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.r
			r.Food, r.CookedAt = "Beef stew", &cooked
			res := evalDefault(t, RecordCooling, r)
			assert.Equal(t, tc.want, res.Outcome, res.Reason)
		})
	}
}

func TestTimeTemperatureCombos(t *testing.T) {
	lim := FSAIDefault(CheckCook)
	lim.Combos = []TimeTemp{{MinC: 75}, {MinC: 70, HoldSeconds: 120}}
	tests := []struct {
		name string
		temp float64
		hold *int64
		want Outcome
	}{
		{"75 instantly", 75, nil, OutcomePass},
		{"70 held 2m", 70, i(120), OutcomePass},
		{"70 held 1m59s", 70, i(119), OutcomeFail},
		{"69.9 held 5m", 69.9, i(300), OutcomeFail},
		{"72 held 3m", 72, i(180), OutcomePass},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Reading{Food: "Pork belly", TempC: f(tc.temp), HoldSeconds: tc.hold}.Normalize()
			res, err := Evaluate(RecordCook, r, &lim)
			require.NoError(t, err)
			assert.Equal(t, tc.want, res.Outcome, res.Reason)
		})
	}
}

func TestNoLimitRecords(t *testing.T) {
	assert.Equal(t, OutcomePass, evalDefault(t, RecordCleaning, Reading{Task: "Deep clean fryer", Done: b(true)}).Outcome)
	assert.Equal(t, OutcomeFail, evalDefault(t, RecordCleaning, Reading{Task: "Deep clean fryer", Done: b(false)}).Outcome)
	assert.Equal(t, OutcomeRecorded, evalDefault(t, RecordBatchLabel, Reading{Product: "Grape soda syrup", MadeOn: "2026-05-01", UseBy: "2026-05-07"}).Outcome)
}

func TestEvaluateRejectsWrongLimit(t *testing.T) {
	r := Reading{Unit: "Walk-in", TempC: f(3)}
	_, err := Evaluate(RecordFridge, r, nil)
	assert.Error(t, err)
	lim := FSAIDefault(CheckFreezer)
	_, err = Evaluate(RecordFridge, r, &lim)
	assert.Error(t, err)
}

func TestReadingValidate(t *testing.T) {
	cooked := time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)
	before := cooked.Add(-time.Minute)
	tests := []struct {
		name string
		rt   RecordType
		r    Reading
	}{
		{"fridge needs unit", RecordFridge, Reading{TempC: f(3)}},
		{"fridge needs temp", RecordFridge, Reading{Unit: "Walk-in"}},
		{"cook negative hold", RecordCook, Reading{Food: "Chicken", TempC: f(80), HoldSeconds: i(-1)}},
		{"cooling needs cooked_at", RecordCooling, Reading{Food: "Rice"}},
		{"cooling fridge before cooked", RecordCooling, Reading{Food: "Rice", CookedAt: &cooked, IntoFridgeAt: &before}},
		{"delivery bad kind", RecordDelivery, Reading{Supplier: "S", Product: "P", DeliveryKind: "warm", PackagingOK: b(true), InDate: b(true)}},
		{"chilled delivery needs temp", RecordDelivery, Reading{Supplier: "S", Product: "P", DeliveryKind: DeliveryChilled, PackagingOK: b(true), InDate: b(true)}},
		{"label bad date", RecordBatchLabel, Reading{Product: "Syrup", MadeOn: "1 May", UseBy: "2026-05-07"}},
		{"label use-by before made", RecordBatchLabel, Reading{Product: "Syrup", MadeOn: "2026-05-07", UseBy: "2026-05-01"}},
		{"cleaning needs done", RecordCleaning, Reading{Task: "Mop"}},
		{"unknown type", RecordType("nope"), Reading{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, tc.r.Normalize().Validate(tc.rt))
		})
	}
}

func TestNormalize(t *testing.T) {
	loc := time.FixedZone("IST", 3600)
	cooked := time.Date(2026, 5, 1, 15, 0, 0, 123456789, loc)
	r := Reading{Unit: "  Walk-in ", TempC: f(3.4499), CookedAt: &cooked, DeliveryKind: "Chilled", DocketFile: "ABC"}.Normalize()
	assert.Equal(t, "Walk-in", r.Unit)
	assert.Equal(t, 3.4, *r.TempC)
	assert.Equal(t, time.UTC, r.CookedAt.Location())
	assert.Equal(t, 123456000, r.CookedAt.Nanosecond())
	assert.Equal(t, "chilled", r.DeliveryKind)
	assert.Equal(t, "abc", r.DocketFile)
}
