package haccp

import (
	"fmt"
	"strings"
	"time"
)

// Result is the outcome of checking a reading, with a reason a person can
// read.
type Result struct {
	Outcome Outcome `json:"outcome"`
	Reason  string  `json:"reason"`
}

// Evaluate checks a normalized, validated reading against a limit. limit is
// nil for record types that have no limit. The limit must match the
// reading's check type; Evaluate never looks a limit up itself.
func Evaluate(t RecordType, r Reading, limit *LimitVersion) (Result, error) {
	if t == RecordTransport {
		return evaluateTransport(r, limit)
	}
	check, hasLimit := CheckFor(t, r)
	if !hasLimit {
		return evaluateNoLimit(t, r), nil
	}
	if limit == nil {
		return Result{}, fmt.Errorf("a %s record needs a %s limit", t, check)
	}
	if limit.Check != check {
		return Result{}, fmt.Errorf("limit %s is for %s, not %s", limit.ID, limit.Check, check)
	}
	switch check {
	case CheckChilledDelivery, CheckFrozenDelivery:
		return evaluateDelivery(r, evaluateMax(*r.TempC, limit)), nil
	case CheckFridge, CheckFreezer:
		return evaluateMax(*r.TempC, limit), nil
	case CheckHotHold:
		return evaluateMin(*r.TempC, limit), nil
	case CheckCook, CheckReheat:
		var hold int64
		if r.HoldSeconds != nil {
			hold = *r.HoldSeconds
		}
		return evaluateCombos(*r.TempC, hold, limit), nil
	case CheckCooling:
		return evaluateCooling(r, limit), nil
	}
	return Result{}, fmt.Errorf("no rule for check %s", check)
}

func evaluateNoLimit(t RecordType, r Reading) Result {
	switch t {
	case RecordInspection:
		var bad []string
		for _, a := range r.Answers {
			if a.OK != nil && !*a.OK {
				bad = append(bad, a.Item)
			}
		}
		if len(bad) == 0 {
			return Result{Outcome: OutcomePass, Reason: "Every item checked is satisfactory."}
		}
		item := "item is"
		if len(bad) > 1 {
			item = "items are"
		}
		return Result{Outcome: OutcomeFail, Reason: fmt.Sprintf("%d %s not satisfactory: %s.", len(bad), item, strings.Join(bad, "; "))}
	case RecordFitness:
		if r.Fit != nil && !*r.Fit {
			return Result{Outcome: OutcomeRecorded, Reason: "Not fit to work."}
		}
		return Result{Outcome: OutcomeRecorded, Reason: "Fit to work."}
	case RecordCleaning:
		if r.Done != nil && *r.Done {
			return Result{Outcome: OutcomePass, Reason: "Task done."}
		}
		return Result{Outcome: OutcomeFail, Reason: "Task not done."}
	case RecordDelivery:
		return evaluateDelivery(r, Result{Outcome: OutcomePass, Reason: "Ambient delivery: no temperature limit."})
	default:
		return Result{Outcome: OutcomeRecorded, Reason: "Recorded. No limit applies."}
	}
}

// evaluateTransport checks food delivered to a customer: its temperature
// against the chilled, frozen or hot holding limit, and that raw and
// ready-to-eat food were kept apart.
func evaluateTransport(r Reading, limit *LimitVersion) (Result, error) {
	res := Result{Outcome: OutcomePass, Reason: "Ambient food: no temperature limit."}
	if check, ok := CheckFor(RecordTransport, r); ok {
		if limit == nil || limit.Check != check {
			return Result{}, fmt.Errorf("a transport record needs a %s limit", check)
		}
		if check == CheckHotHold {
			res = evaluateMin(*r.TempC, limit)
		} else {
			res = evaluateMax(*r.TempC, limit)
		}
	}
	if r.Separated != nil && !*r.Separated {
		reason := "Raw and ready-to-eat food were not kept apart."
		if res.Outcome != OutcomePass {
			reason += " " + res.Reason
		}
		res = Result{Outcome: OutcomeFail, Reason: reason}
	}
	return res, nil
}

// evaluateDelivery adds the packaging, date and vehicle checks to a delivery's
// temperature result, and says what to do with the delivery.
func evaluateDelivery(r Reading, res Result) Result {
	var problems []string
	if r.PackagingOK != nil && !*r.PackagingOK {
		problems = append(problems, "Packaging is not intact.")
	}
	if r.InDate != nil && !*r.InDate {
		problems = append(problems, "Not in date.")
	}
	if r.VehicleOK != nil && !*r.VehicleOK {
		problems = append(problems, "The delivery vehicle failed its check.")
	}
	if len(problems) > 0 {
		reasons := problems
		if res.Outcome != OutcomePass {
			reasons = append(reasons, res.Reason)
		}
		res = Result{Outcome: OutcomeFail, Reason: strings.Join(reasons, " ")}
	}
	switch res.Outcome {
	case OutcomeBorderline:
		res.Reason += " Accept only if used or stored straight away."
	case OutcomeFail:
		res.Reason += " Reject the delivery."
	}
	return res
}

func evaluateMax(temp float64, l *LimitVersion) Result {
	max := *l.MaxC
	if tenths(temp) <= tenths(max) {
		return Result{OutcomePass, fmt.Sprintf("%.1f°C is at or below the %.1f°C limit.", temp, max)}
	}
	if l.BorderlineMaxC != nil && tenths(temp) <= tenths(*l.BorderlineMaxC) {
		return Result{OutcomeBorderline, fmt.Sprintf("%.1f°C is above the %.1f°C limit but within the %.1f°C borderline.", temp, max, *l.BorderlineMaxC)}
	}
	return Result{OutcomeFail, fmt.Sprintf("%.1f°C is above the %.1f°C limit.", temp, upper(l))}
}

func upper(l *LimitVersion) float64 {
	if l.BorderlineMaxC != nil {
		return *l.BorderlineMaxC
	}
	return *l.MaxC
}

func evaluateMin(temp float64, l *LimitVersion) Result {
	min := *l.MinC
	if tenths(temp) >= tenths(min) {
		return Result{OutcomePass, fmt.Sprintf("%.1f°C is at or above the %.1f°C limit.", temp, min)}
	}
	return Result{OutcomeFail, fmt.Sprintf("%.1f°C is below the %.1f°C limit.", temp, min)}
}

func evaluateCombos(temp float64, hold int64, l *LimitVersion) Result {
	reading := fmt.Sprintf("%.1f°C", temp)
	if hold > 0 {
		reading += " held " + formatHold(hold)
	}
	var options []string
	for _, tt := range l.Combos {
		if tenths(temp) >= tenths(tt.MinC) && hold >= tt.HoldSeconds {
			return Result{OutcomePass, fmt.Sprintf("%s meets %s.", reading, formatCombo(tt))}
		}
		options = append(options, formatCombo(tt))
	}
	return Result{OutcomeFail, fmt.Sprintf("%s does not meet %s.", reading, strings.Join(options, " or "))}
}

func formatCombo(tt TimeTemp) string {
	if tt.HoldSeconds == 0 {
		return fmt.Sprintf("%.1f°C", tt.MinC)
	}
	return fmt.Sprintf("%.1f°C for %s", tt.MinC, formatHold(tt.HoldSeconds))
}

func formatHold(sec int64) string {
	if sec == 0 {
		return "instantly"
	}
	return formatDuration(time.Duration(sec) * time.Second)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0 && s > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

func evaluateCooling(r Reading, l *LimitVersion) Result {
	c := l.Cooling
	within := time.Duration(c.IntoFridgeWithinSeconds) * time.Second
	cutoff := time.Duration(c.CutoffAfterSeconds) * time.Second
	if r.IntoFridgeAt != nil {
		took := r.IntoFridgeAt.Sub(*r.CookedAt)
		if took <= within {
			return Result{OutcomePass, fmt.Sprintf("Into fridge %s after cooking, within %s.", formatDuration(took), formatDuration(within))}
		}
	}
	if r.TempAtCutoffC == nil {
		return Result{OutcomeFail, fmt.Sprintf("Not into fridge within %s and no temperature taken at %s. Serve immediately or discard.", formatDuration(within), formatDuration(cutoff))}
	}
	t := *r.TempAtCutoffC
	if tenths(t) > tenths(c.CutoffMaxC) {
		return Result{OutcomeFail, fmt.Sprintf("Not into fridge within %s and %.1f°C at %s is above %.1f°C. Serve immediately or discard.", formatDuration(within), t, formatDuration(cutoff), c.CutoffMaxC)}
	}
	return Result{OutcomeBorderline, fmt.Sprintf("Not into fridge within %s, but %.1f°C at %s is at or below %.1f°C.", formatDuration(within), t, formatDuration(cutoff), c.CutoffMaxC)}
}
