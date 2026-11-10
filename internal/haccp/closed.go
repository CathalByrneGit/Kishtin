package haccp

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ClosedDays are the days the restaurant is closed. Checks are not due on
// a closed day, so they are not missed; a check logged on one still counts.
type ClosedDays struct {
	// Weekdays are the days closed every week: "mon", "tue", "wed", "thu",
	// "fri", "sat", "sun".
	Weekdays []string `json:"weekdays"`
	// Dates are other times closed, such as holidays.
	Dates []ClosedDate `json:"dates"`
}

// ClosedDate is one day, or a run of days, the restaurant is closed.
type ClosedDate struct {
	// From and To are the first and last day closed (2006-01-02). To is
	// From for a single day.
	From string `json:"from"`
	To   string `json:"to"`
	// Note says why, such as "Christmas".
	Note string `json:"note,omitempty"`
}

// Weekdays lists the weekday keys, Monday first.
var Weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// MaxClosedDates caps how many closed dates are kept, and
// MaxClosedRunDays how long one can be.
const (
	MaxClosedDates   = 200
	MaxClosedRunDays = 366
)

func weekdayKey(d time.Weekday) string { return strings.ToLower(d.String()[:3]) }

// On reports whether the restaurant is closed on a day (2006-01-02).
func (c ClosedDays) On(day string) bool {
	for _, d := range c.Dates {
		if day >= d.From && day <= d.To {
			return true
		}
	}
	if len(c.Weekdays) == 0 {
		return false
	}
	t, err := time.Parse(DateLayout, day)
	return err == nil && slices.Contains(c.Weekdays, weekdayKey(t.Weekday()))
}

// Normalize trims and orders closed days, and fills in a missing To.
func (c ClosedDays) Normalize() ClosedDays {
	out := ClosedDays{Weekdays: []string{}, Dates: []ClosedDate{}}
	for _, k := range Weekdays {
		for _, w := range c.Weekdays {
			if strings.EqualFold(strings.TrimSpace(w), k) && !slices.Contains(out.Weekdays, k) {
				out.Weekdays = append(out.Weekdays, k)
			}
		}
	}
	// Anything that is not a weekday is kept, so Validate can refuse it.
	for _, w := range c.Weekdays {
		if k := strings.ToLower(strings.TrimSpace(w)); !slices.Contains(Weekdays, k) {
			out.Weekdays = append(out.Weekdays, k)
		}
	}
	for _, d := range c.Dates {
		nd := ClosedDate{From: strings.TrimSpace(d.From), To: strings.TrimSpace(d.To), Note: strings.TrimSpace(d.Note)}
		if nd.To == "" {
			nd.To = nd.From
		}
		out.Dates = append(out.Dates, nd)
	}
	slices.SortStableFunc(out.Dates, func(a, b ClosedDate) int { return strings.Compare(a.From, b.From) })
	return out
}

// Validate checks the closed days.
func (c ClosedDays) Validate() error {
	var errs []error
	for _, w := range c.Weekdays {
		if !slices.Contains(Weekdays, w) {
			errs = append(errs, fmt.Errorf("closed weekday %q must be one of %s", w, strings.Join(Weekdays, ", ")))
		}
	}
	if len(c.Weekdays) >= len(Weekdays) {
		errs = append(errs, errors.New("the restaurant cannot be closed every day of the week"))
	}
	if len(c.Dates) > MaxClosedDates {
		errs = append(errs, fmt.Errorf("at most %d closed dates: remove ones that have passed", MaxClosedDates))
	}
	for _, d := range c.Dates {
		from, err1 := time.Parse(DateLayout, d.From)
		to, err2 := time.Parse(DateLayout, d.To)
		switch {
		case err1 != nil || err2 != nil:
			errs = append(errs, fmt.Errorf("closed date %q to %q: give dates as 2026-12-25", d.From, d.To))
		case to.Before(from):
			errs = append(errs, fmt.Errorf("closed date %s ends before it starts", d.From))
		case to.Sub(from) > MaxClosedRunDays*24*time.Hour:
			errs = append(errs, fmt.Errorf("closed from %s to %s is more than %d days", d.From, d.To, MaxClosedRunDays))
		}
		if len([]rune(d.Note)) > 80 {
			errs = append(errs, fmt.Errorf("the note for closed date %s is at most 80 characters", d.From))
		}
	}
	return errors.Join(errs...)
}

// MarshalJSON writes empty lists, never null, so lists saved before closed
// days existed compare cleanly with ones saved after.
func (c ClosedDays) MarshalJSON() ([]byte, error) {
	type plain ClosedDays
	p := plain(c)
	if p.Weekdays == nil {
		p.Weekdays = []string{}
	}
	if p.Dates == nil {
		p.Dates = []ClosedDate{}
	}
	return json.Marshal(p)
}
