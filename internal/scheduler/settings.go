package scheduler

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Strategy names.
const (
	// StrategyBackward plans back from each course's target time so every
	// item reaches the pass together, within equipment capacity.
	StrategyBackward = "backward"
	// StrategyFIFO starts items in the order their courses were fired, as
	// soon as equipment allows. It suits a quiet service.
	StrategyFIFO = "fifo"
)

// Strategies lists the strategies Kishtin has.
var Strategies = []string{StrategyBackward, StrategyFIFO}

// StationBuffer adds time before a station's items are due, such as 30
// seconds for a slow plating station.
type StationBuffer struct {
	StationID string `json:"station_id"`
	Seconds   int    `json:"seconds"`
}

// Settings are the scheduler settings a manager can change. They are
// versioned like the HACCP settings, and every plan says which version
// produced it.
type Settings struct {
	Strategy string `json:"strategy"`
	// Batching lets the scheduler cook the same dish for several tables at
	// once, when the dish allows it.
	Batching bool `json:"batching"`
	// BatchWindowSeconds is how far apart two start times can be and still
	// be cooked together.
	BatchWindowSeconds int `json:"batch_window_seconds"`
	// MaxPassWaitSeconds is how long a ready course can wait at the pass
	// before it counts as late.
	MaxPassWaitSeconds int `json:"max_pass_wait_seconds"`
	// StationBuffers add time per station.
	StationBuffers []StationBuffer `json:"station_buffers"`
	// PriorityLeadSeconds is how much earlier a priority course is put in
	// the queue, per priority it has.
	PriorityLeadSeconds int `json:"priority_lead_seconds"`
	// PriorityTakeaway and PriorityAllergy make takeaways and allergy
	// orders priorities. An urgent course (flagged by the expo) always is.
	PriorityTakeaway bool `json:"priority_takeaway"`
	PriorityAllergy  bool `json:"priority_allergy"`
	// LargeTableCovers makes tables with at least this many covers a
	// priority. 0 turns it off.
	LargeTableCovers int `json:"large_table_covers"`
	// ReplanSeconds is how often screens ask for a fresh plan, on top of
	// re-planning after every change.
	ReplanSeconds int `json:"replan_seconds"`
}

// DefaultSettings returns the built-in scheduler settings.
func DefaultSettings() Settings {
	return Settings{
		Strategy: StrategyBackward, Batching: true, BatchWindowSeconds: 120, MaxPassWaitSeconds: 180,
		StationBuffers: []StationBuffer{}, PriorityLeadSeconds: 120, PriorityAllergy: true, ReplanSeconds: 15,
	}
}

// BuiltinSettingsID is the version ID of the built-in scheduler settings.
const BuiltinSettingsID = "builtin-scheduler-v1"

// Validate checks the settings.
func (s Settings) Validate() error {
	var errs []error
	if !slices.Contains(Strategies, s.Strategy) {
		errs = append(errs, fmt.Errorf("strategy must be one of %v, not %q", Strategies, s.Strategy))
	}
	check := func(name string, v, lo, hi int) {
		if v < lo || v > hi {
			errs = append(errs, fmt.Errorf("%s must be between %d and %d", name, lo, hi))
		}
	}
	check("batch_window_seconds", s.BatchWindowSeconds, 0, 900)
	check("max_pass_wait_seconds", s.MaxPassWaitSeconds, 0, 1800)
	check("priority_lead_seconds", s.PriorityLeadSeconds, 0, 900)
	check("large_table_covers", s.LargeTableCovers, 0, 100)
	check("replan_seconds", s.ReplanSeconds, 5, 300)
	seen := map[string]bool{}
	for _, b := range s.StationBuffers {
		if b.StationID == "" {
			errs = append(errs, errors.New("a station buffer needs a station_id"))
		}
		if seen[b.StationID] {
			errs = append(errs, fmt.Errorf("station %s has two buffers", b.StationID))
		}
		seen[b.StationID] = true
		check("a station buffer", b.Seconds, 0, 900)
	}
	return errors.Join(errs...)
}

// Normalize fills nil lists.
func (s Settings) Normalize() Settings {
	if s.StationBuffers == nil {
		s.StationBuffers = []StationBuffer{}
	}
	return s
}

// Config turns settings into the scheduler's configuration.
func (s Settings) Config(version string, loc *time.Location) Config {
	buffers := map[string]time.Duration{}
	for _, b := range s.StationBuffers {
		buffers[b.StationID] = time.Duration(b.Seconds) * time.Second
	}
	return Config{
		Strategy: s.Strategy, Batching: s.Batching, BatchWindow: secs(s.BatchWindowSeconds),
		MaxPassWait: secs(s.MaxPassWaitSeconds), StationBuffers: buffers, PriorityLead: secs(s.PriorityLeadSeconds),
		PriorityTakeaway: s.PriorityTakeaway, PriorityAllergy: s.PriorityAllergy, LargeTableCovers: s.LargeTableCovers,
		StartNowWindow: secs(s.ReplanSeconds) * 2, Location: loc, SettingsVersion: version,
	}
}

func secs(n int) time.Duration { return time.Duration(n) * time.Second }
