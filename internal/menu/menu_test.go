package menu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func burger() Dish {
	return Dish{
		ID: "d1", Name: "Burger", Course: CourseMain, StationID: "grill", EquipmentID: "flat-top",
		CookSeconds: 480, RestSeconds: 120, PlateSeconds: 60, HoldSeconds: 180, Batchable: true,
		Modifiers: []Modifier{{Name: "Well done", AddSeconds: 120}, {Name: "Rare", AddSeconds: -120}},
		Available: true, Active: true,
	}
}

func setup() Setup {
	return Setup{
		Stations:  []Station{{ID: "grill", Name: "Grill", Active: true}, {ID: "cold", Name: "Cold", Active: true}},
		Equipment: []Equipment{{ID: "flat-top", StationID: "grill", Name: "Flat-top", Slots: 3, Active: true}},
		Dishes:    []Dish{burger()},
	}
}

func TestDishValidate(t *testing.T) {
	require.NoError(t, burger().Validate())
	cases := map[string]func(*Dish){
		"no name":            func(d *Dish) { d.Name = " " },
		"long name":          func(d *Dish) { d.Name = string(make([]rune, 61)) },
		"bad course":         func(d *Dish) { d.Course = "side" },
		"no station":         func(d *Dish) { d.StationID = "" },
		"negative cook":      func(d *Dish) { d.CookSeconds = -1 },
		"no time at all":     func(d *Dish) { d.CookSeconds, d.RestSeconds, d.PlateSeconds = 0, 0, 0 },
		"equipment, no cook": func(d *Dish) { d.CookSeconds = 0 },
		"duplicate modifier": func(d *Dish) { d.Modifiers = append(d.Modifiers, Modifier{Name: "well done"}) },
		"too much capacity":  func(d *Dish) { d.BatchCapacity = MaxSlots + 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := burger()
			mutate(&d)
			assert.Error(t, d.Validate())
		})
	}
}

func TestTimingFor(t *testing.T) {
	d := burger()
	tm, err := d.TimingFor(nil)
	require.NoError(t, err)
	assert.Equal(t, Timing{Cook: 8 * time.Minute, Rest: 2 * time.Minute, Plate: time.Minute}, tm)
	assert.Equal(t, 11*time.Minute, tm.Total())

	tm, err = d.TimingFor([]string{"well DONE"})
	require.NoError(t, err)
	assert.Equal(t, 10*time.Minute, tm.Cook)

	_, err = d.TimingFor([]string{"no onion"})
	assert.ErrorContains(t, err, `no modifier "no onion"`)

	d.CookSeconds = 60
	tm, err = d.TimingFor([]string{"Rare"})
	require.NoError(t, err)
	assert.Zero(t, tm.Cook, "a negative cook time is clamped to zero")
}

func TestSetupValidateDish(t *testing.T) {
	s := setup()
	require.NoError(t, s.ValidateDish(burger()))

	d := burger()
	d.StationID = "missing"
	assert.ErrorContains(t, s.ValidateDish(d), "station does not exist")

	d = burger()
	d.StationID = "cold"
	assert.ErrorContains(t, s.ValidateDish(d), "in another station")

	d = burger()
	d.EquipmentID = "missing"
	assert.ErrorContains(t, s.ValidateDish(d), "equipment does not exist")

	s.Stations[0].Active = false
	assert.ErrorContains(t, s.ValidateDish(burger()), "not in use")
}

func TestCapacity(t *testing.T) {
	s := setup()
	d := burger()
	assert.Equal(t, 3, s.Capacity(d), "defaults to the equipment's slots")
	d.BatchCapacity = 6
	assert.Equal(t, 6, s.Capacity(d))
	d.EquipmentID = ""
	assert.Equal(t, 0, s.Capacity(d), "no equipment, no limit")
}

func TestNormalize(t *testing.T) {
	d := burger()
	d.Name = "  Burger "
	d.EquipmentID = ""
	d.BatchCapacity = 4
	n := d.Normalize()
	assert.Equal(t, "Burger", n.Name)
	assert.Zero(t, n.BatchCapacity)
	assert.NotNil(t, Dish{}.Normalize().Modifiers)
}
