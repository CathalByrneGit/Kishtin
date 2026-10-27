// Package menu holds the kitchen's setup: stations, the equipment in them,
// and dishes with their timings and capacities. It does no I/O.
package menu

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Course is when a dish is served. Courses are held until the expo fires
// them.
type Course string

const (
	CourseStarter Course = "starter"
	CourseMain    Course = "main"
	CourseDessert Course = "dessert"
)

// Courses lists every course in service order.
var Courses = []Course{CourseStarter, CourseMain, CourseDessert}

// Valid reports whether c is a known course.
func (c Course) Valid() bool { return slices.Contains(Courses, c) }

// Order is the course's position in service, starting at 0.
func (c Course) Order() int { return slices.Index(Courses, c) }

// Station is a cooking area with its own queue, such as the grill or the
// cold section.
type Station struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Position orders stations on screens.
	Position int `json:"position"`
	// Active is false for a station no longer in use. Stations are never
	// deleted, because past orders refer to them.
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Equipment is a capacity-limited resource inside a station, such as a
// grill or a fryer.
type Equipment struct {
	ID        string `json:"id"`
	StationID string `json:"station_id"`
	Name      string `json:"name"`
	// Slots is how many units the equipment holds at once, for example 3
	// burgers on the grill or 2 fryer baskets. A dish can say it fits a
	// different number (its batch capacity).
	Slots     int       `json:"slots"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Modifier changes a dish's timing, such as "well done" adding 2 minutes.
type Modifier struct {
	Name string `json:"name"`
	// AddSeconds is added to the cook time. It can be negative ("rare").
	AddSeconds int `json:"add_seconds"`
}

// Dish is something on the menu, with what the kitchen needs to cook it.
type Dish struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Course is the dish's usual course. An order can put it in another.
	Course    Course `json:"course"`
	StationID string `json:"station_id"`
	// EquipmentID is the equipment the dish cooks on. Empty means it needs
	// none, such as a salad plated in the cold section: it has no capacity
	// limit.
	EquipmentID string `json:"equipment_id,omitempty"`
	// BatchCapacity is how many of this dish the equipment can cook at once
	// when it is used for nothing else. One unit uses 1/BatchCapacity of
	// the equipment, so different dishes share it fairly. 0 means the
	// equipment's slots.
	BatchCapacity int `json:"batch_capacity,omitempty"`
	CookSeconds   int `json:"cook_seconds"`
	RestSeconds   int `json:"rest_seconds"`
	PlateSeconds  int `json:"plate_seconds"`
	// HoldSeconds is how long the dish can wait at the pass before quality
	// drops. 0 means it cannot wait: it must go out as soon as it is ready.
	HoldSeconds int `json:"hold_seconds"`
	// Batchable allows the scheduler to cook this dish for several tables
	// at once.
	Batchable bool       `json:"batchable"`
	Modifiers []Modifier `json:"modifiers"`
	// FoodCategory links the dish to a HACCP food category, such as
	// poultry, for its cooking limit.
	FoodCategory string `json:"food_category,omitempty"`
	// Available is false while the dish is 86'd (sold out).
	Available bool `json:"available"`
	// Active is false for a dish taken off the menu. Dishes are never
	// deleted, because past orders refer to them.
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Limits on menu values.
const (
	MaxNameLen   = 60
	MaxSlots     = 100
	MaxPhaseSecs = 6 * 3600
	MaxModifiers = 20
)

// CleanName trims a name and checks its length and characters.
func CleanName(what, s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("a %s needs a name", what)
	}
	if len([]rune(s)) > MaxNameLen {
		return "", fmt.Errorf("a %s name is at most %d characters", what, MaxNameLen)
	}
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("a %s name cannot contain control characters", what)
	}
	return s, nil
}

// Validate checks a station.
func (s Station) Validate() error {
	_, err := CleanName("station", s.Name)
	return err
}

// Validate checks equipment.
func (e Equipment) Validate() error {
	if _, err := CleanName("piece of equipment", e.Name); err != nil {
		return err
	}
	if e.StationID == "" {
		return errors.New("equipment needs a station")
	}
	if e.Slots < 1 || e.Slots > MaxSlots {
		return fmt.Errorf("slots must be between 1 and %d", MaxSlots)
	}
	return nil
}

// Validate checks a dish's own fields. Use Setup.ValidateDish to also check
// its station and equipment exist.
func (d Dish) Validate() error {
	var errs []error
	if _, err := CleanName("dish", d.Name); err != nil {
		errs = append(errs, err)
	}
	if !d.Course.Valid() {
		errs = append(errs, fmt.Errorf("course must be one of starter, main, dessert, not %q", d.Course))
	}
	if d.StationID == "" {
		errs = append(errs, errors.New("a dish needs a station"))
	}
	for _, p := range []struct {
		name string
		v    int
	}{{"cook_seconds", d.CookSeconds}, {"rest_seconds", d.RestSeconds}, {"plate_seconds", d.PlateSeconds}, {"hold_seconds", d.HoldSeconds}} {
		if p.v < 0 || p.v > MaxPhaseSecs {
			errs = append(errs, fmt.Errorf("%s must be between 0 and %d", p.name, MaxPhaseSecs))
		}
	}
	if d.CookSeconds+d.RestSeconds+d.PlateSeconds == 0 {
		errs = append(errs, errors.New("a dish needs a cook, rest or plate time"))
	}
	if d.EquipmentID != "" && d.CookSeconds == 0 {
		errs = append(errs, errors.New("a dish that uses equipment needs a cook time"))
	}
	if d.BatchCapacity < 0 || d.BatchCapacity > MaxSlots {
		errs = append(errs, fmt.Errorf("batch_capacity must be between 0 and %d", MaxSlots))
	}
	if len(d.Modifiers) > MaxModifiers {
		errs = append(errs, fmt.Errorf("a dish has at most %d modifiers", MaxModifiers))
	}
	seen := map[string]bool{}
	for _, m := range d.Modifiers {
		n, err := CleanName("modifier", m.Name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if seen[strings.ToLower(n)] {
			errs = append(errs, fmt.Errorf("modifier %q is listed twice", n))
		}
		seen[strings.ToLower(n)] = true
		if m.AddSeconds < -MaxPhaseSecs || m.AddSeconds > MaxPhaseSecs {
			errs = append(errs, fmt.Errorf("modifier %q changes the cook time by too much", n))
		}
	}
	return errors.Join(errs...)
}

// Normalize trims names and fills defaults.
func (d Dish) Normalize() Dish {
	d.Name = strings.TrimSpace(d.Name)
	d.FoodCategory = strings.TrimSpace(d.FoodCategory)
	mods := make([]Modifier, 0, len(d.Modifiers))
	for _, m := range d.Modifiers {
		m.Name = strings.TrimSpace(m.Name)
		mods = append(mods, m)
	}
	d.Modifiers = mods
	if d.EquipmentID == "" {
		d.BatchCapacity = 0
	}
	return d
}

// Timing is a dish's three timed phases for one item, after modifiers.
type Timing struct {
	Cook  time.Duration `json:"cook"`
	Rest  time.Duration `json:"rest"`
	Plate time.Duration `json:"plate"`
}

// Total is the time from starting to ready at the pass.
func (t Timing) Total() time.Duration { return t.Cook + t.Rest + t.Plate }

// TimingFor applies the named modifiers to the dish's times. It returns an
// error naming any modifier the dish does not have.
func (d Dish) TimingFor(modifiers []string) (Timing, error) {
	return d.TimingWithCook(d.CookSeconds, modifiers)
}

// TimingWithCook is TimingFor with another cook time in place of the
// dish's, such as one learned from past cooks.
func (d Dish) TimingWithCook(cookSeconds int, modifiers []string) (Timing, error) {
	cook := cookSeconds
	for _, name := range modifiers {
		i := slices.IndexFunc(d.Modifiers, func(m Modifier) bool { return strings.EqualFold(m.Name, strings.TrimSpace(name)) })
		if i < 0 {
			return Timing{}, fmt.Errorf("%s has no modifier %q", d.Name, name)
		}
		cook += d.Modifiers[i].AddSeconds
	}
	if cook < 0 {
		cook = 0
	}
	return Timing{
		Cook:  time.Duration(cook) * time.Second,
		Rest:  time.Duration(d.RestSeconds) * time.Second,
		Plate: time.Duration(d.PlateSeconds) * time.Second,
	}, nil
}

// Setup is the whole menu: stations, equipment and dishes.
type Setup struct {
	Stations  []Station   `json:"stations"`
	Equipment []Equipment `json:"equipment"`
	Dishes    []Dish      `json:"dishes"`
}

// Station returns the station with the given ID.
func (s Setup) Station(id string) (Station, bool) {
	i := slices.IndexFunc(s.Stations, func(x Station) bool { return x.ID == id })
	if i < 0 {
		return Station{}, false
	}
	return s.Stations[i], true
}

// EquipmentByID returns the equipment with the given ID.
func (s Setup) EquipmentByID(id string) (Equipment, bool) {
	i := slices.IndexFunc(s.Equipment, func(x Equipment) bool { return x.ID == id })
	if i < 0 {
		return Equipment{}, false
	}
	return s.Equipment[i], true
}

// Dish returns the dish with the given ID.
func (s Setup) Dish(id string) (Dish, bool) {
	i := slices.IndexFunc(s.Dishes, func(x Dish) bool { return x.ID == id })
	if i < 0 {
		return Dish{}, false
	}
	return s.Dishes[i], true
}

// ValidateDish checks a dish against the rest of the setup: its station
// exists and is active, and its equipment belongs to that station.
func (s Setup) ValidateDish(d Dish) error {
	if err := d.Validate(); err != nil {
		return err
	}
	st, ok := s.Station(d.StationID)
	if !ok {
		return errors.New("the dish's station does not exist")
	}
	if !st.Active && d.Active {
		return fmt.Errorf("station %s is not in use", st.Name)
	}
	if d.EquipmentID == "" {
		return nil
	}
	eq, ok := s.EquipmentByID(d.EquipmentID)
	if !ok {
		return errors.New("the dish's equipment does not exist")
	}
	if eq.StationID != d.StationID {
		return fmt.Errorf("%s is in another station", eq.Name)
	}
	if !eq.Active && d.Active {
		return fmt.Errorf("%s is not in use", eq.Name)
	}
	return nil
}

// Capacity is how many of a dish its equipment cooks at once: its batch
// capacity, or the equipment's slots when it has none. 0 means no limit.
func (s Setup) Capacity(d Dish) int {
	if d.EquipmentID == "" {
		return 0
	}
	if d.BatchCapacity > 0 {
		return d.BatchCapacity
	}
	if eq, ok := s.EquipmentByID(d.EquipmentID); ok {
		return eq.Slots
	}
	return 1
}
