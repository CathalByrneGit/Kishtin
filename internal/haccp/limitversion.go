package haccp

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ScopeKind is the level a limit applies at.
type ScopeKind string

const (
	ScopeDish     ScopeKind = "dish"
	ScopeCategory ScopeKind = "category"
	ScopeUnit     ScopeKind = "unit"
	ScopeDefault  ScopeKind = "default"
)

// specificity ranks scope kinds, most specific first.
var specificity = []ScopeKind{ScopeDish, ScopeCategory, ScopeUnit, ScopeDefault}

// Valid reports whether k is a known scope kind.
func (k ScopeKind) Valid() bool { return slices.Contains(specificity, k) }

// Scope says what a limit applies to: a dish, a food category, a fridge or
// freezer unit, or the check type as a whole (Kind default, empty Ref).
type Scope struct {
	Kind ScopeKind `json:"kind"`
	Ref  string    `json:"ref,omitempty"`
}

// String returns a short label such as "unit:Walk-in fridge".
func (s Scope) String() string {
	if s.Kind == ScopeDefault {
		return string(ScopeDefault)
	}
	return string(s.Kind) + ":" + s.Ref
}

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

// LimitVersion is one version of a critical limit. Versions are never
// edited: a change creates a new version that supersedes the old one, and
// every HACCP record stores the ID of the version it was checked against.
type LimitVersion struct {
	ID    string    `json:"id"`
	Check CheckType `json:"check"`
	Scope Scope     `json:"scope"`

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
	// NonFood marks a unit that holds no food, such as a drinks fridge.
	NonFood bool `json:"non_food,omitempty"`

	Supersedes string    `json:"supersedes,omitempty"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	Reason     string    `json:"reason,omitempty"`
	PlanRef    string    `json:"plan_ref,omitempty"`
}

// SystemUser is the CreatedBy value for built-in FSAI default versions.
const SystemUser = "system"

func ptr(f float64) *float64 { return &f }

// FSAIDefaultID is the ID of the built-in default version for a check type.
// These versions are stored in the database on first start and never
// change there. If a default constant ever changes, give the new default a
// new ID (v2) that supersedes v1, so records checked against v1 still name
// the rule that applied to them.
func FSAIDefaultID(c CheckType) string { return "fsai-" + string(c) + "-v1" }

// FSAIDefault returns the built-in FSAI default limit for a check type.
func FSAIDefault(c CheckType) LimitVersion {
	lv := LimitVersion{
		ID:        FSAIDefaultID(c),
		Check:     c,
		Scope:     Scope{Kind: ScopeDefault},
		CreatedBy: SystemUser,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Reason:    "FSAI default",
	}
	switch c {
	case CheckFridge:
		lv.MaxC = ptr(FSAIFridgeMaxC)
	case CheckFreezer:
		lv.MaxC = ptr(FSAIFreezerMaxC)
	case CheckCook:
		lv.Combos = []TimeTemp{{MinC: FSAICookCoreMinC}}
	case CheckReheat:
		lv.Combos = []TimeTemp{{MinC: FSAIReheatCoreMinC}}
	case CheckHotHold:
		lv.MinC = ptr(FSAIHotHoldMinC)
	case CheckCooling:
		lv.Cooling = &CoolingLimit{
			IntoFridgeWithinSeconds: int64(FSAICoolingIntoFridgeWithin / time.Second),
			CutoffAfterSeconds:      int64(FSAICoolingCutoffAfter / time.Second),
			CutoffMaxC:              FSAICoolingCutoffMaxC,
		}
	case CheckChilledDelivery:
		lv.MaxC = ptr(FSAIChilledDeliveryMaxC)
		lv.BorderlineMaxC = ptr(FSAIChilledDeliveryBorderlineMaxC)
	case CheckFrozenDelivery:
		lv.MaxC = ptr(FSAIFrozenDeliveryMaxC)
		lv.BorderlineMaxC = ptr(FSAIFrozenDeliveryBorderlineMaxC)
	}
	return lv
}

// FSAIDefaults returns the built-in default version for every check type.
func FSAIDefaults() []LimitVersion {
	out := make([]LimitVersion, 0, len(CheckTypes))
	for _, c := range CheckTypes {
		out = append(out, FSAIDefault(c))
	}
	return out
}

// ValidateShape checks that a limit version has the fields its check type
// needs and no others.
func (lv LimitVersion) ValidateShape() error {
	if !lv.Check.Valid() {
		return fmt.Errorf("unknown check type %q", lv.Check)
	}
	if !lv.Scope.Kind.Valid() {
		return fmt.Errorf("unknown scope %q", lv.Scope.Kind)
	}
	if lv.Scope.Kind == ScopeDefault && lv.Scope.Ref != "" {
		return errors.New("a default limit has no scope ref")
	}
	if lv.Scope.Kind != ScopeDefault && strings.TrimSpace(lv.Scope.Ref) == "" {
		return fmt.Errorf("a %s limit needs a scope ref", lv.Scope.Kind)
	}
	isUnitCheck := lv.Check == CheckFridge || lv.Check == CheckFreezer
	if lv.Scope.Kind == ScopeUnit && !isUnitCheck {
		return errors.New("unit limits apply only to fridge and freezer checks")
	}
	if lv.NonFood && lv.Scope.Kind != ScopeUnit {
		return errors.New("only a unit limit can be marked non-food")
	}

	has := map[string]bool{
		"max_c":            lv.MaxC != nil,
		"borderline_max_c": lv.BorderlineMaxC != nil,
		"min_c":            lv.MinC != nil,
		"combos":           len(lv.Combos) > 0,
		"cooling":          lv.Cooling != nil,
	}
	var want []string
	switch lv.Check {
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
	for field, present := range has {
		needed := slices.Contains(want, field)
		if needed && !present {
			return fmt.Errorf("a %s limit needs %s", lv.Check, field)
		}
		if !needed && present {
			return fmt.Errorf("a %s limit does not use %s", lv.Check, field)
		}
	}

	if lv.MaxC != nil && lv.BorderlineMaxC != nil && tenths(*lv.BorderlineMaxC) < tenths(*lv.MaxC) {
		return errors.New("borderline_max_c must be at or above max_c")
	}
	for _, tt := range lv.Combos {
		if tt.HoldSeconds < 0 {
			return errors.New("hold_seconds cannot be negative")
		}
	}
	if c := lv.Cooling; c != nil {
		if c.IntoFridgeWithinSeconds <= 0 || c.CutoffAfterSeconds <= 0 {
			return errors.New("cooling times must be positive")
		}
		if c.CutoffAfterSeconds < c.IntoFridgeWithinSeconds {
			return errors.New("the cooling cutoff must be at or after the into-fridge time")
		}
	}
	return nil
}

// LooserThanFSAI lists every way lv is less strict than the FSAI default
// for its check type. An empty result means it is as strict or stricter.
func (lv LimitVersion) LooserThanFSAI() []string {
	def := FSAIDefault(lv.Check)
	var out []string
	if lv.MaxC != nil && def.MaxC != nil && tenths(*lv.MaxC) > tenths(*def.MaxC) {
		out = append(out, fmt.Sprintf("maximum %.1f°C is above the FSAI %.1f°C", *lv.MaxC, *def.MaxC))
	}
	if lv.BorderlineMaxC != nil && def.BorderlineMaxC != nil && tenths(*lv.BorderlineMaxC) > tenths(*def.BorderlineMaxC) {
		out = append(out, fmt.Sprintf("borderline maximum %.1f°C is above the FSAI %.1f°C", *lv.BorderlineMaxC, *def.BorderlineMaxC))
	}
	if lv.MinC != nil && def.MinC != nil && tenths(*lv.MinC) < tenths(*def.MinC) {
		out = append(out, fmt.Sprintf("minimum %.1f°C is below the FSAI %.1f°C", *lv.MinC, *def.MinC))
	}
	if len(def.Combos) > 0 {
		// A combination below the FSAI core temperature is a time–temperature
		// exception (for example sous-vide) and needs its validation cited.
		defMin := def.Combos[0].MinC
		for _, tt := range lv.Combos {
			if tenths(tt.MinC) < tenths(defMin) {
				out = append(out, fmt.Sprintf("core %.1f°C for %ds is below the FSAI %.1f°C", tt.MinC, tt.HoldSeconds, defMin))
			}
		}
	}
	if c, d := lv.Cooling, def.Cooling; c != nil && d != nil {
		if c.IntoFridgeWithinSeconds > d.IntoFridgeWithinSeconds {
			out = append(out, "allows longer than the FSAI time to get into the fridge")
		}
		if c.CutoffAfterSeconds > d.CutoffAfterSeconds {
			out = append(out, "the serve-or-discard cutoff is later than the FSAI cutoff")
		}
		if tenths(c.CutoffMaxC) > tenths(d.CutoffMaxC) {
			out = append(out, fmt.Sprintf("cutoff temperature %.1f°C is above the FSAI %.1f°C", c.CutoffMaxC, d.CutoffMaxC))
		}
	}
	return out
}

// ValidateNew checks a proposed new limit version: its shape, and that a
// limit looser than FSAI guidance carries a reason and a HACCP plan
// reference. Non-food units only need the reason.
func (lv LimitVersion) ValidateNew() error {
	if err := lv.ValidateShape(); err != nil {
		return err
	}
	if strings.TrimSpace(lv.Reason) == "" {
		return errors.New("a limit change needs a reason")
	}
	if looser := lv.LooserThanFSAI(); len(looser) > 0 && !lv.NonFood && strings.TrimSpace(lv.PlanRef) == "" {
		return fmt.Errorf("this limit is below FSAI guidance (%s): add the HACCP plan section or validation that supports it", strings.Join(looser, "; "))
	}
	return nil
}

// Current returns the versions that no other version supersedes: the
// limits in force for each check and scope.
func Current(history []LimitVersion) []LimitVersion {
	superseded := map[string]bool{}
	for _, lv := range history {
		if lv.Supersedes != "" {
			superseded[lv.Supersedes] = true
		}
	}
	var out []LimitVersion
	for _, lv := range history {
		if !superseded[lv.ID] {
			out = append(out, lv)
		}
	}
	return out
}

// Resolve picks the limit for a check from the version history, using the
// most specific of the given scopes that has a limit. It falls back to the
// check type's default version and then to the FSAI constant.
func Resolve(history []LimitVersion, check CheckType, scopes []Scope) LimitVersion {
	current := Current(history)
	best := -1
	var found LimitVersion
	candidates := append(slices.Clone(scopes), Scope{Kind: ScopeDefault})
	for _, lv := range current {
		if lv.Check != check {
			continue
		}
		for _, s := range candidates {
			if lv.Scope != s {
				continue
			}
			rank := len(specificity) - slices.Index(specificity, s.Kind)
			// Prefer the more specific scope; on a tie (two heads for one
			// scope, which the service prevents) prefer the newest.
			if rank > best || (rank == best && lv.CreatedAt.After(found.CreatedAt)) {
				best, found = rank, lv
			}
		}
	}
	if best < 0 {
		return FSAIDefault(check)
	}
	return found
}
