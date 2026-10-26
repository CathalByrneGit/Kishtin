package haccp

import (
	"errors"
	"fmt"
	"strings"
)

// Guidance is the reference the restaurant's limits are measured against.
// It starts as the FSAI defaults. A restaurant can replace it with its own
// HACCP plan, for example to recognise 70°C for 2 minutes as equal to 75°C.
// A limit looser than the guidance needs a HACCP plan reference; guidance
// looser than FSAI needs one too, on the guidance change itself.
type Guidance struct {
	// Name labels the guidance in warnings, such as "FSAI Safe Catering".
	Name string `json:"name"`
	// Source cites where the numbers come from.
	Source string          `json:"source,omitempty"`
	Checks []GuidanceCheck `json:"checks"`
}

// GuidanceCheck is the guidance for one check type.
type GuidanceCheck struct {
	Check CheckType `json:"check"`
	Thresholds
}

// BuiltinGuidanceID is the version ID of the built-in FSAI guidance.
const BuiltinGuidanceID = "builtin-guidance-v1"

// FSAIGuidance returns the FSAI defaults as reference guidance.
func FSAIGuidance() Guidance {
	g := Guidance{
		Name:   "FSAI",
		Source: "Food Safety Authority of Ireland, Safe Catering pack",
	}
	for _, c := range CheckTypes {
		g.Checks = append(g.Checks, GuidanceCheck{Check: c, Thresholds: FSAIThresholds(c)})
	}
	return g
}

// For returns the guidance for check c, falling back to FSAI.
func (g Guidance) For(c CheckType) Thresholds {
	for _, gc := range g.Checks {
		if gc.Check == c {
			return gc.Thresholds
		}
	}
	return FSAIThresholds(c)
}

// Normalize trims text and rounds temperatures.
func (g Guidance) Normalize() Guidance {
	g.Name = strings.TrimSpace(g.Name)
	g.Source = strings.TrimSpace(g.Source)
	checks := make([]GuidanceCheck, len(g.Checks))
	for k, gc := range g.Checks {
		checks[k] = GuidanceCheck{Check: gc.Check, Thresholds: gc.Rounded()}
	}
	g.Checks = checks
	return g
}

// Validate checks that the guidance has a name and covers every check type
// exactly once with the right fields.
func (g Guidance) Validate() error {
	if g.Name == "" {
		return errors.New("give the guidance a name, such as \"Our HACCP plan\"")
	}
	seen := map[CheckType]bool{}
	for _, gc := range g.Checks {
		if seen[gc.Check] {
			return fmt.Errorf("the guidance lists %s twice", gc.Check)
		}
		seen[gc.Check] = true
		if err := gc.Validate(gc.Check); err != nil {
			return err
		}
	}
	for _, c := range CheckTypes {
		if !seen[c] {
			return fmt.Errorf("the guidance is missing %s", c)
		}
	}
	return nil
}

// LooserThanFSAI lists every way the guidance is less strict than FSAI.
func (g Guidance) LooserThanFSAI() []string {
	var out []string
	for _, gc := range g.Checks {
		for _, why := range gc.LooserThan(FSAIThresholds(gc.Check), "FSAI") {
			out = append(out, string(gc.Check)+": "+why)
		}
	}
	return out
}
