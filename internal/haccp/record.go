package haccp

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// RecordType names a kind of HACCP record.
type RecordType string

const (
	RecordFridge     RecordType = "fridge"
	RecordFreezer    RecordType = "freezer"
	RecordDelivery   RecordType = "delivery"
	RecordCook       RecordType = "cook"
	RecordReheat     RecordType = "reheat"
	RecordCooling    RecordType = "cooling"
	RecordHotHold    RecordType = "hot_hold"
	RecordBatchLabel RecordType = "batch_label"
	RecordCleaning   RecordType = "cleaning"
	// RecordSignOff is a manager's sign-off of the records for a period:
	// the "Manager/Supervisor check" of a paper record book.
	RecordSignOff RecordType = "sign_off"
)

// RecordTypes lists every record type, in display order.
var RecordTypes = []RecordType{
	RecordFridge, RecordFreezer, RecordDelivery, RecordCook, RecordReheat,
	RecordCooling, RecordHotHold, RecordBatchLabel, RecordCleaning, RecordSignOff,
}

// Valid reports whether t is a known record type.
func (t RecordType) Valid() bool {
	for _, k := range RecordTypes {
		if t == k {
			return true
		}
	}
	return false
}

// Delivery kinds.
const (
	DeliveryChilled = "chilled"
	DeliveryFrozen  = "frozen"
	DeliveryAmbient = "ambient"
)

// DateLayout is the layout for calendar dates such as use-by dates.
const DateLayout = "2006-01-02"

// Reading holds what was observed. Which fields apply depends on the
// record type; Validate checks them.
type Reading struct {
	// Unit is the fridge or freezer checked.
	Unit string `json:"unit,omitempty"`
	// Food is what was cooked, reheated, cooled or held.
	Food string `json:"food,omitempty"`
	// Dish and FoodCategory select a dish or category limit, when set.
	Dish         string `json:"dish,omitempty"`
	FoodCategory string `json:"food_category,omitempty"`

	TempC       *float64 `json:"temp_c,omitempty"`
	HoldSeconds *int64   `json:"hold_seconds,omitempty"`

	// Cooling.
	CookedAt      *time.Time `json:"cooked_at,omitempty"`
	IntoFridgeAt  *time.Time `json:"into_fridge_at,omitempty"`
	TempAtCutoffC *float64   `json:"temp_at_cutoff_c,omitempty"`

	// Delivery.
	Supplier     string `json:"supplier,omitempty"`
	Product      string `json:"product,omitempty"`
	DeliveryKind string `json:"delivery_kind,omitempty"`
	// DocketFile is the SHA-256 of an attached docket or invoice photo.
	DocketFile string `json:"docket_file,omitempty"`
	// PackagingOK and InDate are the delivery checks staff tick. A delivery
	// with either one false fails.
	PackagingOK *bool `json:"packaging_ok,omitempty"`
	InDate      *bool `json:"in_date,omitempty"`
	// VehicleOK is the delivery vehicle check: clean and at the right
	// temperature. Nil means not checked. A vehicle that fails fails the
	// delivery.
	VehicleOK *bool `json:"vehicle_ok,omitempty"`
	// BatchCode is the supplier's batch or lot code, for tracing.
	BatchCode string `json:"batch_code,omitempty"`

	// Batch label. UseBy is also a delivery's use-by date.
	MadeOn string `json:"made_on,omitempty"`
	UseBy  string `json:"use_by,omitempty"`
	// ShelfLifeDays sets UseBy to MadeOn plus this many days when UseBy is
	// not given.
	ShelfLifeDays *int   `json:"shelf_life_days,omitempty"`
	Quantity      string `json:"quantity,omitempty"`

	// Cleaning.
	Task string `json:"task,omitempty"`
	Area string `json:"area,omitempty"`
	Done *bool  `json:"done,omitempty"`

	// Sign-off: the dates covered (inclusive) and what the manager saw.
	PeriodFrom string  `json:"period_from,omitempty"`
	PeriodTo   string  `json:"period_to,omitempty"`
	Review     *Review `json:"review,omitempty"`

	Notes string `json:"notes,omitempty"`
}

// Review is what a sign-off covered, counted when it was saved.
type Review struct {
	// Records is how many records the period has, after corrections.
	Records int `json:"records"`
	// OutOfLimit counts failed and borderline readings.
	OutOfLimit int `json:"out_of_limit"`
	// WithoutAction counts out-of-limit readings with no corrective action.
	WithoutAction int `json:"without_action"`
	// Missed counts fridge and freezer checks that were due and have no
	// reading.
	Missed int `json:"missed"`
}

// MaxSignOffDays is the longest period one sign-off can cover.
const MaxSignOffDays = 92

// Normalize rounds temperatures to 0.1°C, trims text and stores times in
// UTC at microsecond precision, so a reading hashes the same after a round
// trip through any storage backend.
func (r Reading) Normalize() Reading {
	roundPtr := func(p *float64) *float64 {
		if p == nil {
			return nil
		}
		v := RoundTemp(*p)
		return &v
	}
	timePtr := func(p *time.Time) *time.Time {
		if p == nil {
			return nil
		}
		v := NormalizeTime(*p)
		return &v
	}
	r.TempC = roundPtr(r.TempC)
	r.TempAtCutoffC = roundPtr(r.TempAtCutoffC)
	r.CookedAt = timePtr(r.CookedAt)
	r.IntoFridgeAt = timePtr(r.IntoFridgeAt)
	for _, s := range []*string{
		&r.Unit, &r.Food, &r.Dish, &r.FoodCategory, &r.Supplier, &r.Product,
		&r.DeliveryKind, &r.DocketFile, &r.MadeOn, &r.UseBy, &r.Quantity,
		&r.Task, &r.Area, &r.Notes, &r.PeriodFrom, &r.PeriodTo, &r.BatchCode,
	} {
		*s = strings.TrimSpace(*s)
	}
	r.DeliveryKind = strings.ToLower(r.DeliveryKind)
	r.DocketFile = strings.ToLower(r.DocketFile)
	if r.UseBy == "" && r.ShelfLifeDays != nil {
		if made, err := time.Parse(DateLayout, r.MadeOn); err == nil {
			r.UseBy = made.AddDate(0, 0, *r.ShelfLifeDays).Format(DateLayout)
		}
	}
	return r
}

// NormalizeTime returns t in UTC, truncated to microseconds.
func NormalizeTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// Validate checks that the reading has the fields a record type needs.
func (r Reading) Validate(t RecordType) error {
	need := func(name, v string) error {
		if v == "" {
			return fmt.Errorf("%s is required for a %s record", name, t)
		}
		return nil
	}
	needTemp := func() error {
		if r.TempC == nil {
			return fmt.Errorf("temp_c is required for a %s record", t)
		}
		return nil
	}
	switch t {
	case RecordFridge, RecordFreezer:
		return errors.Join(need("unit", r.Unit), needTemp())
	case RecordCook, RecordReheat:
		if r.HoldSeconds != nil && *r.HoldSeconds < 0 {
			return errors.New("hold_seconds cannot be negative")
		}
		return errors.Join(need("food", r.Food), needTemp())
	case RecordHotHold:
		return errors.Join(need("food", r.Food), needTemp())
	case RecordCooling:
		if err := need("food", r.Food); err != nil {
			return err
		}
		if r.CookedAt == nil {
			return errors.New("cooked_at is required for a cooling record")
		}
		if r.IntoFridgeAt != nil && r.IntoFridgeAt.Before(*r.CookedAt) {
			return errors.New("into_fridge_at cannot be before cooked_at")
		}
		return nil
	case RecordDelivery:
		if err := errors.Join(need("supplier", r.Supplier), need("product", r.Product)); err != nil {
			return err
		}
		if r.PackagingOK == nil || r.InDate == nil {
			return errors.New("say whether the packaging is intact (packaging_ok) and the delivery is in date (in_date)")
		}
		if r.UseBy != "" {
			if _, err := time.Parse(DateLayout, r.UseBy); err != nil {
				return errors.New("use_by must be a date like 2026-10-31")
			}
		}
		if len([]rune(r.BatchCode)) > 60 {
			return errors.New("batch_code is at most 60 characters")
		}
		switch r.DeliveryKind {
		case DeliveryChilled, DeliveryFrozen:
			return needTemp()
		case DeliveryAmbient:
			return nil
		default:
			return fmt.Errorf("delivery_kind must be %s, %s or %s", DeliveryChilled, DeliveryFrozen, DeliveryAmbient)
		}
	case RecordBatchLabel:
		if err := errors.Join(need("product", r.Product), need("made_on", r.MadeOn), need("use_by", r.UseBy)); err != nil {
			return err
		}
		made, err := time.Parse(DateLayout, r.MadeOn)
		if err != nil {
			return fmt.Errorf("made_on must be a date like 2026-05-01: %w", err)
		}
		useBy, err := time.Parse(DateLayout, r.UseBy)
		if err != nil {
			return fmt.Errorf("use_by must be a date like 2026-05-07: %w", err)
		}
		if useBy.Before(made) {
			return errors.New("use_by cannot be before made_on")
		}
		if d := r.ShelfLifeDays; d != nil {
			if *d < 0 || *d > 365 {
				return errors.New("shelf_life_days must be between 0 and 365")
			}
			if !made.AddDate(0, 0, *d).Equal(useBy) {
				return errors.New("use_by does not match made_on plus shelf_life_days: send one or the other")
			}
		}
		return nil
	case RecordCleaning:
		if r.Done == nil {
			return errors.New("done is required for a cleaning record")
		}
		return need("task", r.Task)
	case RecordSignOff:
		from, err := time.Parse(DateLayout, r.PeriodFrom)
		if err != nil {
			return errors.New("period_from must be a date like 2026-10-01")
		}
		to, err := time.Parse(DateLayout, r.PeriodTo)
		if err != nil {
			return errors.New("period_to must be a date like 2026-10-07")
		}
		if to.Before(from) {
			return errors.New("the sign-off period ends before it starts")
		}
		if to.Sub(from) >= MaxSignOffDays*24*time.Hour {
			return fmt.Errorf("a sign-off covers at most %d days", MaxSignOffDays)
		}
		if r.Review == nil {
			return errors.New("a sign-off needs the review of its period")
		}
		if (r.Review.WithoutAction > 0 || r.Review.Missed > 0) && r.Notes == "" {
			return errors.New("there are missed checks or readings without a corrective action in this period: add a note saying what was done about them")
		}
		return nil
	default:
		return fmt.Errorf("unknown record type %q", t)
	}
}

// CheckFor returns the check type whose limit applies to a record, or false
// if the record has no limit (batch labels, cleaning, ambient deliveries).
func CheckFor(t RecordType, r Reading) (CheckType, bool) {
	switch t {
	case RecordFridge:
		return CheckFridge, true
	case RecordFreezer:
		return CheckFreezer, true
	case RecordCook:
		return CheckCook, true
	case RecordReheat:
		return CheckReheat, true
	case RecordHotHold:
		return CheckHotHold, true
	case RecordCooling:
		return CheckCooling, true
	case RecordDelivery:
		switch r.DeliveryKind {
		case DeliveryChilled:
			return CheckChilledDelivery, true
		case DeliveryFrozen:
			return CheckFrozenDelivery, true
		}
	}
	return "", false
}

// Scopes returns the limit scopes a reading can match, most specific first.
func Scopes(t RecordType, r Reading) []Scope {
	var out []Scope
	if r.Dish != "" {
		out = append(out, Scope{Kind: ScopeDish, Ref: r.Dish})
	}
	if r.FoodCategory != "" {
		out = append(out, Scope{Kind: ScopeCategory, Ref: r.FoodCategory})
	}
	if (t == RecordFridge || t == RecordFreezer) && r.Unit != "" {
		out = append(out, Scope{Kind: ScopeUnit, Ref: r.Unit})
	}
	return out
}

// Record is one saved HACCP record. Records are append-only and chained:
// Hash covers the record's content and PrevHash, the hash of the record
// before it.
type Record struct {
	ID           string     `json:"id"`
	Seq          int64      `json:"seq"`
	Type         RecordType `json:"type"`
	LoggedBy     string     `json:"logged_by"`
	LoggedByName string     `json:"logged_by_name"`
	LoggedAt     time.Time  `json:"logged_at"`
	Reading      Reading    `json:"reading"`

	Outcome        Outcome `json:"outcome"`
	OutcomeReason  string  `json:"outcome_reason"`
	LimitVersionID string  `json:"limit_version_id,omitempty"`
	LimitScope     *Scope  `json:"limit_scope,omitempty"`
	// RulesVersionID is the food-safety rules version in force when the
	// record was saved.
	RulesVersionID string `json:"rules_version_id,omitempty"`

	CorrectiveAction string `json:"corrective_action,omitempty"`
	// Corrects is the ID of an earlier record this one corrects.
	Corrects         string `json:"corrects,omitempty"`
	CorrectionReason string `json:"correction_reason,omitempty"`

	PrevHash string `json:"prev_hash"`
	Hash     string `json:"hash"`
}
