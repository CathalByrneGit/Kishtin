// Package haccp holds HACCP record types, critical limits and the pass/fail
// rules. It does no I/O: limits are always passed in, never read from a
// global or a database.
package haccp

import "math"

// CheckType names a check that has a critical limit.
type CheckType string

const (
	CheckFridge          CheckType = "fridge"
	CheckFreezer         CheckType = "freezer"
	CheckCook            CheckType = "cook"
	CheckReheat          CheckType = "reheat"
	CheckHotHold         CheckType = "hot_hold"
	CheckCooling         CheckType = "cooling"
	CheckChilledDelivery CheckType = "delivery_chilled"
	CheckFrozenDelivery  CheckType = "delivery_frozen"
)

// CheckTypes lists every check type with a limit, in display order.
var CheckTypes = []CheckType{
	CheckFridge, CheckFreezer, CheckCook, CheckReheat,
	CheckHotHold, CheckCooling, CheckChilledDelivery, CheckFrozenDelivery,
}

// Valid reports whether c is a known check type.
func (c CheckType) Valid() bool {
	for _, k := range CheckTypes {
		if c == k {
			return true
		}
	}
	return false
}

// Outcome is the result of checking a record against its limit.
type Outcome string

const (
	// OutcomePass means the reading met the limit.
	OutcomePass Outcome = "pass"
	// OutcomeBorderline means the reading is outside the target but inside
	// the borderline range. Rules.BorderlineNeedsAction decides whether it
	// needs a corrective action.
	OutcomeBorderline Outcome = "borderline"
	// OutcomeFail means the reading broke the limit.
	OutcomeFail Outcome = "fail"
	// OutcomeRecorded is for records with no limit, such as batch labels.
	OutcomeRecorded Outcome = "recorded"
)

// tenths converts a temperature to whole tenths of a degree. Readings and
// limits are compared in tenths so 5.0 and 4.99999 behave the same as a
// person reading the probe would expect.
func tenths(c float64) int64 { return int64(math.Round(c * 10)) }

// RoundTemp rounds a temperature to 0.1°C.
func RoundTemp(c float64) float64 { return float64(tenths(c)) / 10 }
