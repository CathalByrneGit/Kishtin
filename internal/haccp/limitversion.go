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

// LimitVersion is one version of a critical limit. Versions are never
// edited: a change creates a new version that supersedes the old one, and
// every HACCP record stores the ID of the version it was checked against.
type LimitVersion struct {
	ID    string    `json:"id"`
	Check CheckType `json:"check"`
	Scope Scope     `json:"scope"`

	Thresholds
	// NonFood marks a unit that holds no food, such as a drinks fridge.
	NonFood bool `json:"non_food,omitempty"`

	// GuidanceVersionID is the reference guidance version the limit was
	// compared with when it was saved.
	GuidanceVersionID string `json:"guidance_version_id,omitempty"`

	Supersedes string    `json:"supersedes,omitempty"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	Reason     string    `json:"reason,omitempty"`
	PlanRef    string    `json:"plan_ref,omitempty"`
}

// SystemUser is the CreatedBy value for built-in FSAI default versions.
const SystemUser = "system"

// FSAIDefaultID is the ID of the built-in default version for a check type.
// These versions are stored in the database on first start and never
// change there. If a default constant ever changes, give the new default a
// new ID (v2) that supersedes v1, so records checked against v1 still name
// the rule that applied to them.
func FSAIDefaultID(c CheckType) string { return "fsai-" + string(c) + "-v1" }

// FSAIDefault returns the built-in FSAI default limit for a check type.
func FSAIDefault(c CheckType) LimitVersion {
	return LimitVersion{
		ID:                FSAIDefaultID(c),
		Check:             c,
		Scope:             Scope{Kind: ScopeDefault},
		Thresholds:        FSAIThresholds(c),
		GuidanceVersionID: BuiltinGuidanceID,
		CreatedBy:         SystemUser,
		CreatedAt:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Reason:            "FSAI default",
	}
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

	return lv.Validate(lv.Check)
}

// LooserThanFSAI lists every way lv is less strict than the FSAI default
// for its check type. An empty result means it is as strict or stricter.
func (lv LimitVersion) LooserThanFSAI() []string {
	return lv.LooserThan(FSAIThresholds(lv.Check), "FSAI")
}

// LooserThanGuidance lists every way lv is less strict than the
// restaurant's reference guidance for its check type.
func (lv LimitVersion) LooserThanGuidance(g Guidance) []string {
	return lv.LooserThan(g.For(lv.Check), g.Name)
}

// ValidateNew checks a proposed new limit version against the reference
// guidance in force: its shape, a reason, and a HACCP plan reference when
// the limit is looser than the guidance. Non-food units only need the
// reason.
func (lv LimitVersion) ValidateNew(g Guidance) error {
	if err := lv.ValidateShape(); err != nil {
		return err
	}
	if strings.TrimSpace(lv.Reason) == "" {
		return errors.New("a limit change needs a reason")
	}
	if looser := lv.LooserThanGuidance(g); len(looser) > 0 && !lv.NonFood && strings.TrimSpace(lv.PlanRef) == "" {
		return fmt.Errorf("this limit is below your reference guidance, %s (%s): add the HACCP plan section or validation that supports it", g.Name, strings.Join(looser, "; "))
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
