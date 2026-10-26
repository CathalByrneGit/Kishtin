// Package scheduler decides what each station should start cooking and
// when, so every item of a course reaches the pass together without going
// over any equipment's capacity. It is pure: no I/O, no global state and no
// clock reads. The same state, settings and time always give the same plan,
// and every recommendation carries its reason.
package scheduler

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
)

// Station is a station's name, for actions and reasons.
type Station struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Equipment is a piece of equipment's name and station.
type Equipment struct {
	ID        string `json:"id"`
	StationID string `json:"station_id"`
	Name      string `json:"name"`
}

// Item is one item of a fired course.
type Item struct {
	ID       string `json:"id"`
	DishID   string `json:"dish_id"`
	DishName string `json:"dish_name"`
	// Modifiers are shown with the dish and separate batches: a well-done
	// burger cooks longer than a medium one.
	Modifiers   []string `json:"modifiers,omitempty"`
	StationID   string   `json:"station_id"`
	EquipmentID string   `json:"equipment_id,omitempty"`
	// Capacity is how many of this dish the equipment cooks at once. One
	// unit uses 1/Capacity of the equipment. 0 means no limit.
	Capacity  int           `json:"capacity,omitempty"`
	Cook      time.Duration `json:"cook"`
	Rest      time.Duration `json:"rest"`
	Plate     time.Duration `json:"plate"`
	HoldMax   time.Duration `json:"hold_max"`
	Batchable bool          `json:"batchable"`
	State     orders.State  `json:"state"`
	// StateAt is when the item entered its state.
	StateAt time.Time `json:"state_at"`
}

func (it Item) total() time.Duration { return it.Cook + it.Rest + it.Plate }

// Course is a fired course waiting to reach the pass.
type Course struct {
	OrderID string      `json:"order_id"`
	Label   string      `json:"label"`
	Course  menu.Course `json:"course"`
	FiredAt time.Time   `json:"fired_at"`
	// Target is when every item should be at the pass.
	Target   time.Time `json:"target"`
	Urgent   bool      `json:"urgent,omitempty"`
	Allergy  bool      `json:"allergy,omitempty"`
	Takeaway bool      `json:"takeaway,omitempty"`
	Covers   int       `json:"covers,omitempty"`
	Items    []Item    `json:"items"`
}

// KitchenState is everything the scheduler plans from.
type KitchenState struct {
	Stations  []Station   `json:"stations"`
	Equipment []Equipment `json:"equipment"`
	Courses   []Course    `json:"courses"`
}

// Config is how the scheduler plans. Settings.Config builds one.
type Config struct {
	Strategy       string
	Batching       bool
	BatchWindow    time.Duration
	MaxPassWait    time.Duration
	StationBuffers map[string]time.Duration
	// PriorityLead moves a priority course this much earlier in the queue
	// for each priority it has.
	PriorityLead     time.Duration
	PriorityTakeaway bool
	PriorityAllergy  bool
	LargeTableCovers int
	// StartNowWindow: an action due within this of now says "start now".
	StartNowWindow time.Duration
	// Location is where times in reasons are written for.
	Location *time.Location
	// SettingsVersion is echoed in the plan.
	SettingsVersion string
}

// Causes say, as a code, why a course is at risk or late, for analysis
// after service. The reason says the same in words.
const (
	// CauseEquipmentFull: the equipment it needs is full.
	CauseEquipmentFull = "equipment_full"
	// CauseStartedLate: an item was not started by its latest start.
	CauseStartedLate = "started_late"
	// CauseCookingLong: an item already started will be ready late.
	CauseCookingLong = "cooking_long"
	// CauseWaitingAtPass: the course is ready and waiting too long at the pass.
	CauseWaitingAtPass = "waiting_at_pass"
	// CauseBusy: equipment is busy, with no single cause found.
	CauseBusy = "busy"
)

// Course statuses, always shown as text plus colour.
const (
	StatusOnTrack = "on_track"
	StatusAtRisk  = "at_risk"
	StatusLate    = "late"
	StatusReady   = "ready"
)

// Action is one recommendation for a station: start these items.
type Action struct {
	StationID     string    `json:"station_id"`
	EquipmentID   string    `json:"equipment_id,omitempty"`
	EquipmentName string    `json:"equipment_name,omitempty"`
	DishID        string    `json:"dish_id"`
	DishName      string    `json:"dish_name"`
	Modifiers     []string  `json:"modifiers,omitempty"`
	Quantity      int       `json:"quantity"`
	ItemIDs       []string  `json:"item_ids"`
	OrderIDs      []string  `json:"order_ids"`
	Labels        []string  `json:"labels"`
	StartAt       time.Time `json:"start_at"`
	// Now is set when the action should start now.
	Now bool `json:"now"`
	// Late is set when starting now is already after the latest start, so
	// its course will be late.
	Late   bool   `json:"late,omitempty"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// Progress is an item a station has started: when its phase should end.
type Progress struct {
	ItemID      string       `json:"item_id"`
	OrderID     string       `json:"order_id"`
	Label       string       `json:"label"`
	DishName    string       `json:"dish_name"`
	Modifiers   []string     `json:"modifiers,omitempty"`
	State       orders.State `json:"state"`
	PhaseEndsAt time.Time    `json:"phase_ends_at"`
	// Overdue is set when the phase should already have ended.
	Overdue bool `json:"overdue,omitempty"`
}

// StationPlan is one station's queue.
type StationPlan struct {
	StationID  string     `json:"station_id"`
	Name       string     `json:"name"`
	Actions    []Action   `json:"actions"`
	InProgress []Progress `json:"in_progress"`
}

// CourseStatus says whether a fired course will reach the pass on time.
type CourseStatus struct {
	OrderID string      `json:"order_id"`
	Label   string      `json:"label"`
	Course  menu.Course `json:"course"`
	Target  time.Time   `json:"target"`
	// PlannedReady is when the last item should reach the pass.
	PlannedReady time.Time `json:"planned_ready"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason"`
	// Cause is why the course is at risk or late, as a code; empty when it
	// is on track or ready.
	Cause string `json:"cause,omitempty"`
}

// ItemPlan is when one item should start and be ready.
type ItemPlan struct {
	ItemID      string    `json:"item_id"`
	OrderID     string    `json:"order_id"`
	LatestStart time.Time `json:"latest_start"`
	StartAt     time.Time `json:"start_at"`
	ReadyAt     time.Time `json:"ready_at"`
}

// Plan is the scheduler's output.
type Plan struct {
	At              time.Time      `json:"at"`
	Strategy        string         `json:"strategy"`
	SettingsVersion string         `json:"settings_version"`
	Stations        []StationPlan  `json:"stations"`
	Courses         []CourseStatus `json:"courses"`
	Items           []ItemPlan     `json:"items"`
}

// Strategy plans a kitchen. Each strategy is pure and deterministic.
type Strategy interface {
	Plan(state KitchenState, cfg Config, now time.Time) Plan
}

// For returns the named strategy, or backward when the name is unknown.
func For(name string) Strategy {
	if name == StrategyFIFO {
		return fifo{}
	}
	return backward{}
}

// Plan plans with the strategy named in cfg.
func PlanKitchen(state KitchenState, cfg Config, now time.Time) Plan {
	return For(cfg.Strategy).Plan(state, cfg, now)
}

type backward struct{}

func (backward) Plan(state KitchenState, cfg Config, now time.Time) Plan {
	return plan(state, cfg, now, StrategyBackward)
}

type fifo struct{}

func (fifo) Plan(state KitchenState, cfg Config, now time.Time) Plan {
	return plan(state, cfg, now, StrategyFIFO)
}

// --- planning ---

// job is an item still to start.
type job struct {
	course *Course
	ci     int
	item   Item
	// pos is the item's position in its course, for a stable order.
	pos int
	// latest0 is the latest start for the course's own target; latest
	// allows for the course being moved later.
	latest0 time.Time
	latest  time.Time
	desired time.Time
	rank    int
	start   time.Time
	group   *group
	// waitedFor is set when the item's start was pushed by full equipment.
	waitedFor *capacityWait
	// early is how much earlier than its latest start the item starts,
	// because the equipment is full then. It waits at the pass that long.
	early time.Duration
	// selfConflict is set when the item starts early, beyond what the dish
	// can wait, because its own course's items cannot all cook at once.
	selfConflict bool
}

type capacityWait struct {
	equipment string
	until     time.Time
	inUse     string
}

// group is items started together on one piece of equipment.
type group struct {
	equipment string
	dishKey   string
	start     time.Time
	cook      time.Duration
	jobs      []*job
}

// interval is equipment use: [start, end) using use of it (1 is full),
// by an item of course ci.
type interval struct {
	start, end time.Time
	use        float64
	ci         int
}

const epsilon = 1e-9

// slack is how late against a target still counts as on time: a plan made a
// few seconds after a latest start has passed should not call it late or
// move the course.
const slack = 30 * time.Second

func plan(state KitchenState, cfg Config, now time.Time, strategy string) Plan {
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}
	p := Plan{At: now, Strategy: strategy, SettingsVersion: cfg.SettingsVersion,
		Stations: []StationPlan{}, Courses: []CourseStatus{}, Items: []ItemPlan{}}

	courses := slices.Clone(state.Courses)
	sort.SliceStable(courses, func(i, j int) bool {
		if !courses[i].FiredAt.Equal(courses[j].FiredAt) {
			return courses[i].FiredAt.Before(courses[j].FiredAt)
		}
		if courses[i].OrderID != courses[j].OrderID {
			return courses[i].OrderID < courses[j].OrderID
		}
		return courses[i].Course.Order() < courses[j].Course.Order()
	})

	names := equipmentNames(state)
	jobs, shift, causes := assign(courses, cfg, now, strategy, names)

	stationNames := map[string]string{}
	for _, s := range state.Stations {
		stationNames[s.ID] = s.Name
	}

	// Item plans.
	for _, j := range jobs {
		p.Items = append(p.Items, ItemPlan{ItemID: j.item.ID, OrderID: j.course.OrderID, LatestStart: j.latest, StartAt: j.start, ReadyAt: j.start.Add(j.item.total())})
	}

	// Station plans: actions from groups, and items in progress.
	byStation := map[string]*StationPlan{}
	stationOf := func(id string) *StationPlan {
		if sp, ok := byStation[id]; ok {
			return sp
		}
		name := stationNames[id]
		if name == "" {
			name = id
		}
		sp := &StationPlan{StationID: id, Name: name, Actions: []Action{}, InProgress: []Progress{}}
		byStation[id] = sp
		return sp
	}
	for _, s := range state.Stations {
		stationOf(s.ID)
	}
	seen := map[*group]bool{}
	for _, j := range jobs {
		if seen[j.group] {
			continue
		}
		seen[j.group] = true
		a := action(j.group, shift, causes, cfg, now, names, strategy)
		sp := stationOf(a.StationID)
		sp.Actions = append(sp.Actions, a)
	}
	for ci := range courses {
		c := &courses[ci]
		for _, it := range c.Items {
			end, ok := phaseEnd(it)
			if !ok {
				continue
			}
			sp := stationOf(it.StationID)
			sp.InProgress = append(sp.InProgress, Progress{
				ItemID: it.ID, OrderID: c.OrderID, Label: c.Label, DishName: it.DishName, Modifiers: it.Modifiers,
				State: it.State, PhaseEndsAt: end, Overdue: end.Before(now),
			})
		}
	}
	for _, s := range state.Stations {
		sp := byStation[s.ID]
		sortStation(sp)
		p.Stations = append(p.Stations, *sp)
		delete(byStation, s.ID)
	}
	// Stations not in the state (removed from the menu) still show.
	var rest []string
	for id := range byStation {
		rest = append(rest, id)
	}
	sort.Strings(rest)
	for _, id := range rest {
		sp := byStation[id]
		sortStation(sp)
		p.Stations = append(p.Stations, *sp)
	}

	for ci := range courses {
		p.Courses = append(p.Courses, courseStatus(&courses[ci], ci, causes[ci], jobs, cfg, now))
	}
	return p
}

func sortStation(sp *StationPlan) {
	sort.SliceStable(sp.Actions, func(i, j int) bool { return sp.Actions[i].StartAt.Before(sp.Actions[j].StartAt) })
	sort.SliceStable(sp.InProgress, func(i, j int) bool { return sp.InProgress[i].PhaseEndsAt.Before(sp.InProgress[j].PhaseEndsAt) })
}

func equipmentNames(state KitchenState) map[string]string {
	m := map[string]string{}
	for _, e := range state.Equipment {
		m[e.ID] = e.Name
	}
	return m
}

// phaseEnd is when a started item's current phase should end.
func phaseEnd(it Item) (time.Time, bool) {
	switch it.State {
	case orders.StateCooking:
		return it.StateAt.Add(it.Cook), true
	case orders.StateResting:
		return it.StateAt.Add(it.Rest), true
	case orders.StatePlating:
		return it.StateAt.Add(it.Plate), true
	}
	return time.Time{}, false
}

// inProgressReady is when a started item should be ready at the pass. An
// overdue phase is taken to end now.
func inProgressReady(it Item, now time.Time) (time.Time, bool) {
	end, ok := phaseEnd(it)
	if !ok {
		if it.State == orders.StateReady {
			return it.StateAt, true
		}
		return time.Time{}, false
	}
	if end.Before(now) {
		end = now
	}
	switch it.State {
	case orders.StateCooking:
		return end.Add(it.Rest + it.Plate), true
	case orders.StateResting:
		return end.Add(it.Plate), true
	}
	return end, true
}

func (c *Course) rank(cfg Config) int {
	r := 0
	if c.Urgent {
		r += 2
	}
	if cfg.PriorityAllergy && c.Allergy {
		r++
	}
	if cfg.PriorityTakeaway && c.Takeaway {
		r++
	}
	if cfg.LargeTableCovers > 0 && c.Covers >= cfg.LargeTableCovers {
		r++
	}
	return r
}

func dishKey(it Item) string {
	return it.DishID + "|" + strings.Join(it.Modifiers, ",") + "|" + it.Cook.String()
}

// planner is equipment use and cooking groups planned so far.
type planner struct {
	busy   map[string][]interval
	groups []*group
}

// clone copies the planner so a course can be tried without changing it.
func (pl *planner) clone() *planner {
	c := &planner{busy: make(map[string][]interval, len(pl.busy)), groups: make([]*group, len(pl.groups))}
	for k, v := range pl.busy {
		c.busy[k] = slices.Clone(v)
	}
	for i, g := range pl.groups {
		cp := *g
		cp.jobs = slices.Clone(g.jobs)
		c.groups[i] = &cp
	}
	return c
}

// assign plans courses one at a time, in priority order. Each course gets
// the earliest target at or after its own at which every item fits around
// the courses already planned, so a course planned earlier is never pushed
// back by one planned later. It returns the item jobs, how much later than
// its own target each course will be ready, and why.
func assign(courses []Course, cfg Config, now time.Time, strategy string, names map[string]string) ([]*job, []time.Duration, []cause) {
	pl := &planner{busy: map[string][]interval{}}
	for ci := range courses {
		for _, it := range courses[ci].Items {
			if it.State != orders.StateCooking || it.EquipmentID == "" || it.Capacity <= 0 {
				continue
			}
			end := it.StateAt.Add(it.Cook)
			if end.Before(now) {
				end = now
			}
			pl.busy[it.EquipmentID] = append(pl.busy[it.EquipmentID], interval{start: it.StateAt, end: end, use: 1 / float64(it.Capacity), ci: ci})
		}
	}

	order := make([]int, len(courses))
	for i := range order {
		order[i] = i
	}
	key := func(ci int) time.Time {
		c := &courses[ci]
		lead := -time.Duration(c.rank(cfg)) * cfg.PriorityLead
		if strategy == StrategyFIFO {
			return c.FiredAt.Add(lead)
		}
		return c.Target.Add(lead)
	}
	sort.SliceStable(order, func(a, b int) bool {
		ka, kb := key(order[a]), key(order[b])
		if !ka.Equal(kb) {
			return ka.Before(kb)
		}
		return order[a] < order[b]
	})

	shift := make([]time.Duration, len(courses))
	causes := make([]cause, len(courses))
	var all []*job
	for _, ci := range order {
		c := &courses[ci]
		target := c.Target
		if strategy != StrategyFIFO {
			// Items already started set the earliest the course can be ready.
			for _, it := range c.Items {
				if r, ok := inProgressReady(it, now); ok && r.After(target.Add(slack)) {
					target = r
					causes[ci] = shiftCause(nil, &it, now, cfg, names)
				}
			}
		}
		var jobs []*job
		var try *planner
		for range 50 {
			try = pl.clone()
			var ready time.Time
			var worst *job
			jobs, ready, worst = placeCourse(c, ci, target, try, cfg, now, strategy)
			if strategy == StrategyFIFO || !ready.After(target.Add(slack)) {
				break
			}
			if causes[ci].text == "" {
				causes[ci] = shiftCause(worst, nil, now, cfg, names)
			}
			target = ready.Truncate(time.Second)
			if target.Before(ready) {
				target = target.Add(time.Second)
			}
		}
		pl = try
		all = append(all, jobs...)
		if strategy != StrategyFIFO {
			shift[ci] = target.Sub(c.Target)
		}
	}

	// Point each job at its final group: groups were copied while trying.
	for _, g := range pl.groups {
		for _, j := range g.jobs {
			j.group = g
		}
	}
	// Jobs in start order, for item plans and actions.
	sort.SliceStable(all, func(a, b int) bool { return all[a].start.Before(all[b].start) })
	return all, shift, causes
}

// placeCourse places one course's unstarted items for target on pl. It
// returns the jobs, when the last item will be ready, and the item that
// is ready last.
func placeCourse(c *Course, ci int, target time.Time, pl *planner, cfg Config, now time.Time, strategy string) ([]*job, time.Time, *job) {
	var jobs []*job
	for pos, it := range c.Items {
		if it.State != orders.StateFired {
			continue
		}
		j := &job{course: c, ci: ci, item: it, pos: pos, rank: c.rank(cfg)}
		buffer := cfg.StationBuffers[it.StationID]
		j.latest = target.Add(-it.total() - buffer)
		j.latest0 = c.Target.Add(-it.total() - buffer)
		j.desired = j.latest
		if strategy == StrategyFIFO || j.desired.Before(now) {
			j.desired = now
		}
		jobs = append(jobs, j)
	}
	// The longest items choose their time first.
	place := slices.Clone(jobs)
	if strategy != StrategyFIFO {
		sort.SliceStable(place, func(a, b int) bool { return place[a].item.total() > place[b].item.total() })
	}
	var ready time.Time
	var worst *job
	for _, j := range place {
		placeJob(j, pl, cfg, now, strategy)
		r := j.start.Add(j.item.total() + cfg.StationBuffers[j.item.StationID])
		if worst == nil || r.After(ready) {
			ready, worst = r, j
		}
	}
	for _, it := range c.Items {
		if r, ok := inProgressReady(it, now); ok && r.After(ready) {
			ready = r
		}
	}
	return jobs, ready, worst
}

// placeJob gives one item its start time and group.
func placeJob(j *job, pl *planner, cfg Config, now time.Time, strategy string) {
	it := j.item
	if it.EquipmentID == "" || it.Capacity <= 0 {
		// No equipment: no capacity limit. Plating-only items for the same
		// dish starting together still share an action.
		j.start = j.desired
		if g := findGroup(pl.groups, j, cfg, func(*group) bool { return true }); g != nil {
			g.jobs = append(g.jobs, j)
			j.group = g
			return
		}
		g := &group{equipment: "", dishKey: dishKey(it), start: j.start, cook: it.Cook, jobs: []*job{j}}
		pl.groups = append(pl.groups, g)
		j.group = g
		return
	}
	use := 1 / float64(it.Capacity)
	list := pl.busy[it.EquipmentID]
	add := func(start time.Time) {
		pl.busy[it.EquipmentID] = append(list, interval{start: start, end: start.Add(it.Cook), use: use, ci: j.ci})
	}
	if cfg.Batching && it.Batchable {
		g := findGroup(pl.groups, j, cfg, func(g *group) bool { return fits(list, g.start, it.Cook, use) })
		if g != nil {
			j.start = g.start
			j.early = j.desired.Sub(g.start)
			g.jobs = append(g.jobs, j)
			j.group = g
			add(g.start)
			return
		}
	}
	start, ok := j.desired, fits(list, j.desired, it.Cook, use)
	if !ok && strategy != StrategyFIFO {
		// Start earlier, as late as possible, and wait at the pass: within
		// the time the dish can wait, or a little further when the course's
		// own items alone fill the equipment (they cannot all start at
		// once). Past that, the course moves later instead.
		lo := j.desired.Add(-it.HoldMax)
		own := only(list, j.ci)
		self := !fits(own, j.desired, it.Cook, use)
		if self {
			// Early enough to finish before the course's own long cook.
			var longest time.Duration
			for _, iv := range own {
				longest = max(longest, iv.end.Sub(iv.start))
			}
			lo = j.desired.Add(-it.HoldMax - longest)
		}
		if lo.Before(now) {
			lo = now
		}
		if t, found := latestFit(list, lo, j.desired, it.Cook, use); found {
			start, ok = t, true
			j.early = j.desired.Sub(t)
			j.selfConflict = self && j.early > it.HoldMax
		}
	}
	if !ok {
		start = earliestFit(list, j.desired, it.Cook, use)
		j.waitedFor = &capacityWait{equipment: it.EquipmentID, until: start, inUse: inUseText(list, j.desired, it)}
	}
	j.start = start
	g := &group{equipment: it.EquipmentID, dishKey: dishKey(it), start: start, cook: it.Cook, jobs: []*job{j}}
	pl.groups = append(pl.groups, g)
	j.group = g
	add(start)
}

// findGroup finds a group j can join: same equipment and dish, starting
// within the batch window before j's desired start, early enough that the
// dish can wait for the rest of its course, and with room (ok).
func findGroup(groups []*group, j *job, cfg Config, ok func(*group) bool) *group {
	key := dishKey(j.item)
	for _, g := range groups {
		if g.equipment != j.item.EquipmentID || g.dishKey != key {
			continue
		}
		if g.equipment == "" {
			if g.start.Equal(j.desired) {
				return g
			}
			continue
		}
		if !cfg.Batching || !j.item.Batchable {
			continue
		}
		early := j.desired.Sub(g.start)
		if early < 0 || early > cfg.BatchWindow {
			continue
		}
		if early > j.item.HoldMax && early > 0 {
			continue
		}
		if len(g.jobs) >= j.item.Capacity {
			continue
		}
		if ok(g) {
			return g
		}
	}
	return nil
}

// useAt is the share of the equipment in use at t.
func useAt(list []interval, t time.Time) float64 {
	u := 0.0
	for _, iv := range list {
		if !t.Before(iv.start) && t.Before(iv.end) {
			u += iv.use
		}
	}
	return u
}

// fits reports whether use more of the equipment is free for [t, t+d).
func fits(list []interval, t time.Time, d time.Duration, use float64) bool {
	end := t.Add(d)
	if useAt(list, t)+use > 1+epsilon {
		return false
	}
	for _, iv := range list {
		if iv.start.After(t) && iv.start.Before(end) && useAt(list, iv.start)+use > 1+epsilon {
			return false
		}
	}
	return true
}

// only returns the intervals used by course ci.
func only(list []interval, ci int) []interval {
	out := make([]interval, 0, len(list))
	for _, iv := range list {
		if iv.ci == ci {
			out = append(out, iv)
		}
	}
	return out
}

// latestFit is the last time in [lo, hi] with room for [t, t+d).
func latestFit(list []interval, lo, hi time.Time, d time.Duration, use float64) (time.Time, bool) {
	cands := []time.Time{hi, lo}
	for _, iv := range list {
		cands = append(cands, iv.start.Add(-d), iv.end)
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].After(cands[j]) })
	for _, t := range cands {
		if t.After(hi) || t.Before(lo) {
			continue
		}
		if fits(list, t, d, use) {
			return t, true
		}
	}
	return time.Time{}, false
}

// earliestFit is the first time at or after from with room for [t, t+d).
func earliestFit(list []interval, from time.Time, d time.Duration, use float64) time.Time {
	if fits(list, from, d, use) {
		return from
	}
	var ends []time.Time
	for _, iv := range list {
		if iv.end.After(from) {
			ends = append(ends, iv.end)
		}
	}
	sort.Slice(ends, func(i, j int) bool { return ends[i].Before(ends[j]) })
	for _, e := range ends {
		if fits(list, e, d, use) {
			return e
		}
	}
	// Unreachable when every use is at most 1, but stay safe.
	if len(ends) > 0 {
		return ends[len(ends)-1]
	}
	return from
}

// inUseText says how full the equipment is at its busiest while the item
// would cook from t, such as "3 of 3".
func inUseText(list []interval, t time.Time, it Item) string {
	u := useAt(list, t)
	end := t.Add(it.Cook)
	for _, iv := range list {
		if iv.start.After(t) && iv.start.Before(end) {
			u = max(u, useAt(list, iv.start))
		}
	}
	n := int(math.Round(u * float64(it.Capacity)))
	return fmt.Sprintf("%d of %d", n, it.Capacity)
}

func (cfg Config) clock(t time.Time) string { return t.In(cfg.Location).Format("15:04") }

func mins(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%d s", int(d.Seconds()))
	}
	m := int(d / time.Minute)
	s := int((d % time.Minute) / time.Second)
	if s == 0 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%d min %d s", m, s)
}

func courseName(c *Course) string {
	switch c.Course {
	case menu.CourseStarter:
		return "starters"
	case menu.CourseMain:
		return "mains"
	case menu.CourseDessert:
		return "desserts"
	}
	return string(c.Course)
}

func joinAnd(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

func dishText(n int, name string, mods []string) string {
	s := fmt.Sprintf("%d× %s", n, name)
	if len(mods) > 0 {
		s += " (" + strings.Join(mods, ", ") + ")"
	}
	return s
}

func timingText(it Item) string {
	var parts []string
	if it.Cook > 0 {
		parts = append(parts, mins(it.Cook)+" cook")
	}
	if it.Rest > 0 {
		parts = append(parts, mins(it.Rest)+" rest")
	}
	if it.Plate > 0 {
		parts = append(parts, mins(it.Plate)+" plate")
	}
	return strings.Join(parts, " + ")
}

// action turns a group into a recommendation with its reason.
func action(g *group, shift []time.Duration, causes []cause, cfg Config, now time.Time, names map[string]string, strategy string) Action {
	first := g.jobs[0]
	it := first.item
	a := Action{
		StationID: it.StationID, EquipmentID: it.EquipmentID, EquipmentName: names[it.EquipmentID],
		DishID: it.DishID, DishName: it.DishName, Modifiers: it.Modifiers, Quantity: len(g.jobs),
		ItemIDs: []string{}, OrderIDs: []string{}, Labels: []string{}, StartAt: g.start,
	}
	a.Now = !g.start.After(now.Add(cfg.StartNowWindow))
	for _, j := range g.jobs {
		a.ItemIDs = append(a.ItemIDs, j.item.ID)
		if !slices.Contains(a.OrderIDs, j.course.OrderID) {
			a.OrderIDs = append(a.OrderIDs, j.course.OrderID)
			a.Labels = append(a.Labels, j.course.Label)
		}
		if g.start.After(j.latest0.Add(slack)) {
			a.Late = true
		}
	}
	where := ""
	if a.EquipmentName != "" {
		where = " on the " + strings.ToLower(a.EquipmentName)
	}
	when := "now"
	if !a.Now {
		when = "at " + cfg.clock(g.start)
	}
	a.Text = fmt.Sprintf("Start %s%s %s for %s", dishText(a.Quantity, a.DishName, a.Modifiers), where, when, joinAnd(a.Labels))

	var why []string
	c := first.course
	target := c.Target.Add(shift[first.ci])
	switch {
	case strategy == StrategyFIFO:
		why = append(why, fmt.Sprintf("%s %s were fired at %s: first in, first out.", c.Label, courseName(c), cfg.clock(c.FiredAt)))
	case shift[first.ci] > 0 && !causes[first.ci].together:
		// Running late: say what was due, and when it will now arrive.
		why = append(why, fmt.Sprintf("%s %s were due at %s. %s: %s, so it should have started by %s. Starting now, the course reaches the pass at %s.",
			c.Label, courseName(c), cfg.clock(c.Target), it.DishName, timingText(it), cfg.clock(first.latest0), cfg.clock(target)))
	default:
		why = append(why, fmt.Sprintf("%s %s are due at %s. %s: %s, so start by %s.",
			c.Label, courseName(c), cfg.clock(target), it.DishName, timingText(it), cfg.clock(first.latest)))
		if b := cfg.StationBuffers[it.StationID]; b > 0 {
			why = append(why, fmt.Sprintf("That includes the station's %s buffer.", mins(b)))
		}
	}
	if shift[first.ci] > 0 && strategy != StrategyFIFO {
		cs := causes[first.ci]
		if cs.together {
			why = append(why, fmt.Sprintf("%s The course moved from %s to %s so every item finishes together.", cs.text, cfg.clock(c.Target), cfg.clock(target)))
		}
	} else if first.waitedFor != nil {
		why = append(why, fmt.Sprintf("The %s is full until %s (%s in use).", strings.ToLower(names[first.waitedFor.equipment]), cfg.clock(first.waitedFor.until), first.waitedFor.inUse))
	}
	switch {
	case first.selfConflict:
		why = append(why, fmt.Sprintf("%s's items cannot all fit on the %s at once, so start this %s earlier; it waits at the pass longer than it should. Move the course later on the pass to avoid that.",
			c.Label, strings.ToLower(a.EquipmentName), mins(first.early)))
	case first.early > 0 && first.waitedFor == nil:
		why = append(why, fmt.Sprintf("The %s is full at %s, so start %s earlier; it waits at the pass.", strings.ToLower(a.EquipmentName), cfg.clock(first.latest), mins(first.early)))
	}
	if first.rank > 0 && strategy != StrategyFIFO {
		why = append(why, "It is a priority ("+priorityText(c, cfg)+"), so it goes ahead in the queue.")
	}
	if len(g.jobs) > 1 && g.equipment != "" {
		var others []string
		for _, j := range g.jobs[1:] {
			if j.course != c && !slices.Contains(others, j.course.Label) {
				others = append(others, j.course.Label)
			}
		}
		if len(others) > 0 {
			why = append(why, fmt.Sprintf("Cooked with %s: the same dish, starting within %s.", joinAnd(others), mins(cfg.BatchWindow)))
		}
	}
	if a.Late && strategy == StrategyFIFO {
		why = append(why, fmt.Sprintf("%s %s will reach the pass after %s.", c.Label, courseName(c), cfg.clock(c.Target)))
	}
	a.Reason = strings.Join(why, " ")
	return a
}

// cause is why a course moved later. together is set when the course moved
// so its items still finish together, rather than simply running late.
type cause struct {
	text     string
	together bool
	code     string
}

// shiftCause says why a course had to move later: full equipment, a start
// time already past, or an item already cooking that will be ready late.
func shiftCause(worst *job, slow *Item, now time.Time, cfg Config, names map[string]string) cause {
	switch {
	case worst != nil && worst.waitedFor != nil:
		w := worst.waitedFor
		return cause{fmt.Sprintf("The %s is full until %s (%s in use).", strings.ToLower(names[w.equipment]), cfg.clock(w.until), w.inUse), true, CauseEquipmentFull}
	case worst != nil && worst.latest0.Before(now):
		return cause{fmt.Sprintf("%s should have started by %s.", worst.item.DishName, cfg.clock(worst.latest0)), false, CauseStartedLate}
	case slow != nil:
		if end, ok := inProgressReady(*slow, now); ok {
			return cause{fmt.Sprintf("The %s already started will be ready at %s.", strings.ToLower(slow.DishName), cfg.clock(end)), true, CauseCookingLong}
		}
	}
	return cause{"Equipment is busy.", true, CauseBusy}
}

func priorityText(c *Course, cfg Config) string {
	var ps []string
	if c.Urgent {
		ps = append(ps, "flagged urgent")
	}
	if cfg.PriorityAllergy && c.Allergy {
		ps = append(ps, "allergy")
	}
	if cfg.PriorityTakeaway && c.Takeaway {
		ps = append(ps, "takeaway")
	}
	if cfg.LargeTableCovers > 0 && c.Covers >= cfg.LargeTableCovers {
		ps = append(ps, fmt.Sprintf("%d covers", c.Covers))
	}
	return strings.Join(ps, ", ")
}

// courseStatus says whether a course will reach the pass on time.
func courseStatus(c *Course, ci int, why cause, jobs []*job, cfg Config, now time.Time) CourseStatus {
	cs := CourseStatus{OrderID: c.OrderID, Label: c.Label, Course: c.Course, Target: c.Target}
	allReady, any := true, false
	var planned time.Time
	var firstReady time.Time
	for _, it := range c.Items {
		if it.State == orders.StateVoided || it.State == orders.StateServed {
			continue
		}
		any = true
		if it.State == orders.StateReady {
			if firstReady.IsZero() || it.StateAt.Before(firstReady) {
				firstReady = it.StateAt
			}
		} else {
			allReady = false
		}
		if r, ok := inProgressReady(it, now); ok && r.After(planned) {
			planned = r
		}
	}
	for _, j := range jobs {
		if j.ci == ci {
			if r := j.start.Add(j.item.total()); r.After(planned) {
				planned = r
			}
		}
	}
	cs.PlannedReady = planned
	switch {
	case !any:
		cs.Status, cs.Reason = StatusReady, "Nothing left to cook."
	case allReady:
		waited := now.Sub(firstReady)
		if waited > cfg.MaxPassWait {
			cs.Status, cs.Cause = StatusLate, CauseWaitingAtPass
			cs.Reason = fmt.Sprintf("Ready at the pass for %s, longer than %s: serve now.", mins(waited), mins(cfg.MaxPassWait))
		} else {
			cs.Status = StatusReady
			cs.Reason = fmt.Sprintf("Ready at the pass since %s.", cfg.clock(firstReady))
		}
	case now.After(c.Target):
		cs.Status = StatusLate
		cs.Reason = fmt.Sprintf("Due at %s; expected at the pass at %s.", cfg.clock(c.Target), cfg.clock(planned))
		if why.text != "" {
			cs.Reason += " " + why.text
		}
		cs.Cause = why.code
	case planned.After(c.Target.Add(slack)):
		cs.Status = StatusAtRisk
		cs.Reason = fmt.Sprintf("Due at %s; expected at %s.", cfg.clock(c.Target), cfg.clock(planned))
		if why.text != "" {
			cs.Reason += " " + why.text
		}
		cs.Cause = why.code
	default:
		cs.Status = StatusOnTrack
		cs.Reason = fmt.Sprintf("On track for %s.", cfg.clock(c.Target))
	}
	return cs
}
