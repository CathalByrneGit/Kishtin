// Package analytics works out reports from what Kishtin records: how well
// HACCP checks were kept up, and how fast courses reached the pass. It is
// pure: callers read the records and pass them in.
package analytics

import (
	"math"
	"slices"
	"sort"
	"time"
)

// Spread summarises a set of durations, in seconds.
type Spread struct {
	Count  int `json:"count"`
	Median int `json:"median_seconds"`
	// P90 is the time nine in ten were faster than.
	P90 int `json:"p90_seconds"`
}

func spread(ds []time.Duration) Spread {
	if len(ds) == 0 {
		return Spread{}
	}
	s := slices.Clone(ds)
	slices.Sort(s)
	return Spread{Count: len(s), Median: secs(quantile(s, 0.5)), P90: secs(quantile(s, 0.9))}
}

// quantile reads a quantile from sorted durations, by the nearest-rank
// method: the smallest value with at least q of the values at or below it.
func quantile(sorted []time.Duration, q float64) time.Duration {
	i := int(math.Ceil(q*float64(len(sorted)))) - 1
	return sorted[max(0, min(i, len(sorted)-1))]
}

func secs(d time.Duration) int { return int(d.Round(time.Second) / time.Second) }

func percent(n, of int) int {
	if of == 0 {
		return 0
	}
	return int(math.Round(float64(n) * 100 / float64(of)))
}

// day is the start of t's day in loc.
func day(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// Weekdays are the days of the week in report order, Monday first.
var Weekdays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

// Count is a number for a named group, such as missed checks on Mondays.
type Count struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
	// Of is what Count is out of, where that means something (checks due).
	Of int `json:"of,omitempty"`
}

func sortCounts(cs []Count) {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Count > cs[j].Count })
}
