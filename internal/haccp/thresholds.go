package haccp

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// TimeTemp is one time–temperature combination for a cooking limit: the
// core must reach MinC and stay there for HoldSeconds.
type TimeTemp struct {
	MinC        float64 `json:"min_c"`
	HoldSeconds int64   `json:"hold_seconds"`
}

// CoolingLimit is the cooling rule. Food passes if it goes into the fridge
// within IntoFridgeWithinSeconds of cooking. If it doesn't, and is still
// above CutoffMaxC at CutoffAfterSeconds, it must be served or discarded.
type CoolingLimit struct {
	IntoFridgeWithinSeconds int64   `json:"into_fridge_within_seconds"`
	CutoffAfterSeconds      int64   `json:"cutoff_after_seconds"`
	CutoffMaxC              float64 `json:"cutoff_max_c"`
}

// Thresholds are the numbers a check is measured against. Which fields are
// set depends on the check type; Validate checks them. Limit versions and
// reference guidance both use Thresholds.
type Thresholds struct {
	// MaxC is the highest passing temperature (fridges, freezers, deliveries).
	MaxC *float64 `json:"max_c,omitempty"`
	// BorderlineMaxC is the highest borderline temperature (deliveries).
	BorderlineMaxC *float64 `json:"borderline_max_c,omitempty"`
	// MinC is the lowest passing temperature (hot holding).
	MinC *float64 `json:"min_c,omitempty"`
	// Combos are the time–temperature combinations for cooking and
	// reheating. A reading passes if it meets any one of them.
	Combos []TimeTemp `json:"combos,omitempty"`
	// Cooling is the cooling rule.
	Cooling *CoolingLimit `json:"cooling,omitempty"`
}

func ptr(f float64) *float64 { return &f }

// FSAIThresholds returns the FSAI default thresholds for a check type.
func FSAIThresholds(c CheckType) Thresholds {
	var t Thresholds
	switch c {
	case CheckFridge:
		t.MaxC = ptr(FSAIFridgeMaxC)
	case CheckFreezer:
		t.MaxC = ptr(FSAIFreezerMaxC)
	case CheckCook:
		t.Combos = []TimeTemp{{MinC: FSAICookCoreMinC}}
	case CheckReheat:
		t.Combos = []TimeTemp{{MinC: FSAIReheatCoreMinC}}
	case CheckHotHold:
		t.MinC = ptr(FSAIHotHoldMinC)
	case CheckCooling:
		t.Cooling = &CoolingLimit{
			IntoFridgeWithinSeconds: int64(FSAICoolingIntoFridgeWithin / time.Second),
			CutoffAfterSeconds:      int64(FSAICoolingCutoffAfter / time.Second),
			CutoffMaxC:              FSAICoolingCutoffMaxC,
		}
	case CheckChilledDelivery:
		t.MaxC = ptr(FSAIChilledDeliveryMaxC)
		t.BorderlineMaxC = ptr(FSAIChilledDeliveryBorderlineMaxC)
	case CheckFrozenDelivery:
		t.MaxC = ptr(FSAIFrozenDeliveryMaxC)
		t.BorderlineMaxC = ptr(FSAIFrozenDeliveryBorderlineMaxC)
	}
	return t
}

// Validate checks that t has the fields check c needs and no others.
func (t Thresholds) Validate(c CheckType) error {
	if !c.Valid() {
		return fmt.Errorf("unknown check type %q", c)
	}
	has := map[string]bool{
		"max_c":            t.MaxC != nil,
		"borderline_max_c": t.BorderlineMaxC != nil,
		"min_c":            t.MinC != nil,
		"combos":           len(t.Combos) > 0,
		"cooling":          t.Cooling != nil,
	}
	var want []string
	switch c {
	case CheckFridge, CheckFreezer:
		want = []string{"max_c"}
	case CheckChilledDelivery, CheckFrozenDelivery:
		want = []string{"max_c", "borderline_max_c"}
	case CheckHotHold:
		want = []string{"min_c"}
	case CheckCook, CheckReheat:
		want = []string{"combos"}
	case CheckCooling:
		want = []string{"cooling"}
	}
	for _, field := range []string{"max_c", "borderline_max_c", "min_c", "combos", "cooling"} {
		needed := slices.Contains(want, field)
		if needed && !has[field] {
			return fmt.Errorf("a %s check needs %s", c, field)
		}
		if !needed && has[field] {
			return fmt.Errorf("a %s check does not use %s", c, field)
		}
	}
	if t.MaxC != nil && t.BorderlineMaxC != nil && tenths(*t.BorderlineMaxC) < tenths(*t.MaxC) {
		return errors.New("borderline_max_c must be at or above max_c")
	}
	for _, tt := range t.Combos {
		if tt.HoldSeconds < 0 {
			return errors.New("hold_seconds cannot be negative")
		}
	}
	if cl := t.Cooling; cl != nil {
		if cl.IntoFridgeWithinSeconds <= 0 || cl.CutoffAfterSeconds <= 0 {
			return errors.New("cooling times must be positive")
		}
		if cl.CutoffAfterSeconds < cl.IntoFridgeWithinSeconds {
			return errors.New("the cooling cutoff must be at or after the into-fridge time")
		}
	}
	return nil
}

// Rounded returns t with every temperature rounded to 0.1°C.
func (t Thresholds) Rounded() Thresholds {
	round := func(p *float64) *float64 {
		if p == nil {
			return nil
		}
		v := RoundTemp(*p)
		return &v
	}
	out := Thresholds{MaxC: round(t.MaxC), BorderlineMaxC: round(t.BorderlineMaxC), MinC: round(t.MinC)}
	if t.Combos != nil {
		out.Combos = slices.Clone(t.Combos)
		for k := range out.Combos {
			out.Combos[k].MinC = RoundTemp(out.Combos[k].MinC)
		}
	}
	if t.Cooling != nil {
		cl := *t.Cooling
		cl.CutoffMaxC = RoundTemp(cl.CutoffMaxC)
		out.Cooling = &cl
	}
	return out
}

// LooserThan lists every way t is less strict than base. name labels base
// in the messages, such as "FSAI" or the name of the restaurant's guidance.
// An empty result means t is as strict as base or stricter.
func (t Thresholds) LooserThan(base Thresholds, name string) []string {
	var out []string
	if t.MaxC != nil && base.MaxC != nil && tenths(*t.MaxC) > tenths(*base.MaxC) {
		out = append(out, fmt.Sprintf("maximum %.1f°C is above the %s %.1f°C", *t.MaxC, name, *base.MaxC))
	}
	if t.BorderlineMaxC != nil && base.BorderlineMaxC != nil && tenths(*t.BorderlineMaxC) > tenths(*base.BorderlineMaxC) {
		out = append(out, fmt.Sprintf("borderline maximum %.1f°C is above the %s %.1f°C", *t.BorderlineMaxC, name, *base.BorderlineMaxC))
	}
	if t.MinC != nil && base.MinC != nil && tenths(*t.MinC) < tenths(*base.MinC) {
		out = append(out, fmt.Sprintf("minimum %.1f°C is below the %s %.1f°C", *t.MinC, name, *base.MinC))
	}
	if len(base.Combos) > 0 {
		// A combination is covered when some combination in base is at
		// least as easy to meet: no hotter and held no longer. Anything not
		// covered (for example a sous-vide time) is looser than base.
		for _, tt := range t.Combos {
			covered := slices.ContainsFunc(base.Combos, func(b TimeTemp) bool {
				return tenths(tt.MinC) >= tenths(b.MinC) && tt.HoldSeconds >= b.HoldSeconds
			})
			if !covered {
				out = append(out, fmt.Sprintf("core %s is not one of the %s combinations", formatCombo(tt), name))
			}
		}
	}
	if c, d := t.Cooling, base.Cooling; c != nil && d != nil {
		if c.IntoFridgeWithinSeconds > d.IntoFridgeWithinSeconds {
			out = append(out, fmt.Sprintf("allows longer than the %s time to get into the fridge", name))
		}
		if c.CutoffAfterSeconds > d.CutoffAfterSeconds {
			out = append(out, fmt.Sprintf("the serve-or-discard cutoff is later than the %s cutoff", name))
		}
		if tenths(c.CutoffMaxC) > tenths(d.CutoffMaxC) {
			out = append(out, fmt.Sprintf("cutoff temperature %.1f°C is above the %s %.1f°C", c.CutoffMaxC, name, d.CutoffMaxC))
		}
	}
	return out
}
