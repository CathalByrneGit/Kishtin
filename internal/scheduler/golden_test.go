package scheduler

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
)

// Run `go test ./internal/scheduler -update` to rewrite the golden plans
// after a deliberate change to plan output. Explain the change in the
// commit.
var update = flag.Bool("update", false, "rewrite golden plans")

// fixture is a kitchen state written for people: durations as "8m" and
// dishes defined once.
type fixture struct {
	Description string            `json:"description"`
	Now         time.Time         `json:"now"`
	Settings    *Settings         `json:"settings,omitempty"`
	Stations    []Station         `json:"stations"`
	Equipment   []Equipment       `json:"equipment"`
	Dishes      map[string]fxDish `json:"dishes"`
	Courses     []fxCourse        `json:"courses"`
}

type fxDish struct {
	Name      string `json:"name"`
	Station   string `json:"station"`
	Equipment string `json:"equipment,omitempty"`
	Capacity  int    `json:"capacity,omitempty"`
	Cook      string `json:"cook,omitempty"`
	Rest      string `json:"rest,omitempty"`
	Plate     string `json:"plate,omitempty"`
	Hold      string `json:"hold,omitempty"`
	Batchable bool   `json:"batchable,omitempty"`
}

type fxCourse struct {
	OrderID  string      `json:"order_id"`
	Label    string      `json:"label"`
	Course   menu.Course `json:"course"`
	FiredAt  time.Time   `json:"fired_at"`
	Target   time.Time   `json:"target"`
	Urgent   bool        `json:"urgent,omitempty"`
	Allergy  bool        `json:"allergy,omitempty"`
	Takeaway bool        `json:"takeaway,omitempty"`
	Covers   int         `json:"covers,omitempty"`
	Items    []fxItem    `json:"items"`
}

type fxItem struct {
	ID        string       `json:"id"`
	Dish      string       `json:"dish"`
	Modifiers []string     `json:"modifiers,omitempty"`
	ExtraCook string       `json:"extra_cook,omitempty"`
	State     orders.State `json:"state"`
	StateAt   time.Time    `json:"state_at,omitzero"`
}

func dur(t *testing.T, s string) time.Duration {
	t.Helper()
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	require.NoError(t, err, s)
	return d
}

func (f fixture) state(t *testing.T) KitchenState {
	ks := KitchenState{Stations: f.Stations, Equipment: f.Equipment}
	for _, fc := range f.Courses {
		c := Course{OrderID: fc.OrderID, Label: fc.Label, Course: fc.Course, FiredAt: fc.FiredAt, Target: fc.Target,
			Urgent: fc.Urgent, Allergy: fc.Allergy, Takeaway: fc.Takeaway, Covers: fc.Covers}
		for _, fi := range fc.Items {
			d, ok := f.Dishes[fi.Dish]
			require.True(t, ok, "unknown dish %s", fi.Dish)
			stateAt := fi.StateAt
			if stateAt.IsZero() {
				stateAt = fc.FiredAt
			}
			c.Items = append(c.Items, Item{
				ID: fi.ID, DishID: fi.Dish, DishName: d.Name, Modifiers: fi.Modifiers, StationID: d.Station,
				EquipmentID: d.Equipment, Capacity: d.Capacity, Cook: dur(t, d.Cook) + dur(t, fi.ExtraCook),
				Rest: dur(t, d.Rest), Plate: dur(t, d.Plate), HoldMax: dur(t, d.Hold), Batchable: d.Batchable,
				State: fi.State, StateAt: stateAt,
			})
		}
		ks.Courses = append(ks.Courses, c)
	}
	return ks
}

func TestGoldenPlans(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.input.json"))
	require.NoError(t, err)
	require.NotEmpty(t, inputs)
	loc, err := time.LoadLocation("Europe/Dublin")
	require.NoError(t, err)
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".input.json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(in)
			require.NoError(t, err)
			var f fixture
			require.NoError(t, json.Unmarshal(raw, &f))
			s := DefaultSettings()
			if f.Settings != nil {
				s = *f.Settings
			}
			require.NoError(t, s.Validate())
			p := PlanKitchen(f.state(t), s.Config("test-settings", loc), f.Now)

			got, err := json.MarshalIndent(p, "", "  ")
			require.NoError(t, err)
			got = append(got, '\n')
			golden := filepath.Join("testdata", name+".golden.json")
			if *update {
				require.NoError(t, os.WriteFile(golden, got, 0o644))
			}
			want, err := os.ReadFile(golden)
			require.NoError(t, err, "run go test ./internal/scheduler -update to create it")
			assert.Equal(t, string(want), string(got))

			// The same input always gives the same plan.
			again, err := json.MarshalIndent(PlanKitchen(f.state(t), s.Config("test-settings", loc), f.Now), "", "  ")
			require.NoError(t, err)
			assert.Equal(t, string(got), string(append(again, '\n')))
		})
	}
}
