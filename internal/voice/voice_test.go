package voice

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vocab is a synthetic kitchen: open tables 4, 7 and 14 and takeaway #3.
var vocab = Vocab{
	Dishes: []Name{
		{ID: "d-burger", Name: "Burger"}, {ID: "d-fries", Name: "Fries"}, {ID: "d-steak", Name: "Sirloin steak"},
		{ID: "d-supreme", Name: "Chicken supreme"}, {ID: "d-wings", Name: "Chicken wings"}, {ID: "d-soup", Name: "Soup of the day"},
	},
	Stations: []Name{{ID: "s-grill", Name: "Grill"}, {ID: "s-fryer", Name: "Fryer"}, {ID: "s-cold", Name: "Cold section"}},
	Units: []Unit{
		{Name: "Walk-in fridge", Kind: "fridge"}, {Name: "Prep fridge", Kind: "fridge"}, {Name: "Chest freezer", Kind: "freezer"},
	},
	Orders: []OrderRef{
		{ID: "o-4", Table: "4", Number: 1, Label: "Table 4"},
		{ID: "o-7", Table: "7", Number: 2, Label: "Table 7"},
		{ID: "o-14", Table: "14", Number: 5, Label: "Table 14"},
		{ID: "o-t3", Number: 3, Label: "Takeaway #3"},
	},
	Tasks: []string{
		"Food-contact surfaces sanitised", "Chopping boards washed & sanitised", "Fridge handles & seals wiped",
		"Floors swept & mopped", "Bins emptied & cleaned", "Hand-wash sinks stocked (soap, towels)",
	},
}

type fixture struct {
	Say     string   `json:"say"`
	Intent  Intent   `json:"intent"`
	Orders  []string `json:"orders"`
	Course  string   `json:"course"`
	Dish    string   `json:"dish"`
	Station string   `json:"station"`
	Unit    string   `json:"unit"`
	Check   string   `json:"check"`
	Food    string   `json:"food"`
	Temp    *float64 `json:"temp"`
	Count   int      `json:"count"`
	Ask     string   `json:"ask"`
	Options []string `json:"options"`
	Problem string   `json:"problem"`

	Reason       string `json:"reason"`
	Supplier     string `json:"supplier"`
	Product      string `json:"product"`
	DeliveryKind string `json:"delivery_kind"`
	PackagingOK  *bool  `json:"packaging_ok"`
	InDate       *bool  `json:"in_date"`
	CookedClock  string `json:"cooked_clock"`
	CookedAgoMin int    `json:"cooked_ago_min"`
	ShelfDays    int    `json:"shelf_days"`
	Quantity     string `json:"quantity"`
	Task         string `json:"task"`
}

func label(id string) string {
	for _, o := range vocab.Orders {
		if o.ID == id {
			return o.Label
		}
	}
	for _, n := range append(vocab.Dishes, vocab.Stations...) {
		if n.ID == id {
			return n.Name
		}
	}
	return id
}

func TestTranscripts(t *testing.T) {
	b, err := os.ReadFile("testdata/transcripts.json")
	require.NoError(t, err)
	var cases []fixture
	require.NoError(t, json.Unmarshal(b, &cases))
	for _, tc := range cases {
		t.Run(tc.Say, func(t *testing.T) {
			r := Parse(tc.Say, vocab)
			if tc.Problem != "" {
				assert.Equal(t, tc.Problem, r.Problem)
				assert.Nil(t, r.Command)
				return
			}
			if tc.Ask != "" {
				require.Empty(t, r.Problem)
				assert.Equal(t, tc.Ask, r.Ask)
				var got []string
				for _, o := range r.Options {
					got = append(got, o.Label)
				}
				assert.Equal(t, tc.Options, got)
				return
			}
			require.Empty(t, r.Problem)
			require.Empty(t, r.Ask)
			require.NotNil(t, r.Command)
			c := r.Command
			assert.Equal(t, tc.Intent, c.Intent)
			var orders []string
			for _, id := range c.OrderIDs {
				orders = append(orders, label(id))
			}
			assert.Equal(t, tc.Orders, orders, "orders")
			assert.Equal(t, tc.Course, c.Course, "course")
			assert.Equal(t, tc.Dish, label(c.DishID), "dish")
			assert.Equal(t, tc.Station, label(c.StationID), "station")
			assert.Equal(t, tc.Unit, c.Unit, "unit")
			assert.Equal(t, tc.Check, c.Check, "check")
			assert.Equal(t, tc.Food, c.Food, "food")
			assert.Equal(t, tc.Count, c.Count, "count")
			if tc.Temp != nil {
				require.NotNil(t, c.TempC)
				assert.InDelta(t, *tc.Temp, *c.TempC, 0.001)
			} else if tc.Intent == IntentDelivery {
				assert.Nil(t, c.TempC, "temp")
			}
			assert.Equal(t, tc.Reason, c.Reason, "reason")
			assert.Equal(t, tc.Supplier, c.Supplier, "supplier")
			assert.Equal(t, tc.Product, c.Product, "product")
			assert.Equal(t, tc.DeliveryKind, c.DeliveryKind, "delivery kind")
			assert.Equal(t, tc.PackagingOK, c.PackagingOK, "packaging")
			assert.Equal(t, tc.InDate, c.InDate, "in date")
			assert.Equal(t, tc.ShelfDays, c.ShelfDays, "shelf days")
			assert.Equal(t, tc.Quantity, c.Quantity, "quantity")
			assert.Equal(t, tc.Task, c.Task, "task")
			switch {
			case tc.CookedClock != "":
				require.NotNil(t, c.Cooked)
				assert.Equal(t, tc.CookedClock, c.Cooked.Clock)
			case tc.CookedAgoMin != 0:
				require.NotNil(t, c.Cooked)
				assert.Equal(t, time.Duration(tc.CookedAgoMin)*time.Minute, c.Cooked.Ago)
			default:
				assert.Nil(t, c.Cooked, "cooked")
			}
		})
	}
}

func TestAskKeepsTheRestOfTheCommand(t *testing.T) {
	r := Parse("burgers on tables 41 and 7", vocab)
	require.Len(t, r.Options, 2)
	c := r.Options[1].Command
	assert.Equal(t, IntentStart, c.Intent)
	assert.Equal(t, "d-burger", c.DishID)
	assert.Equal(t, []string{"o-14", "o-7"}, c.OrderIDs)

	r = Parse("fire table 41 mains", vocab)
	assert.Equal(t, IntentFire, r.Options[0].Command.Intent)
	assert.Equal(t, "main", r.Options[0].Command.Course)
}

func TestChoose(t *testing.T) {
	opts := []Option{{Label: "Table 4"}, {Label: "Table 14"}}
	for reply, want := range map[string]int{
		"table 14": 1, "14": 1, "fourteen": 1, "four": 0, "for": 0, "the first one": 0, "second": 1, "Table 4.": 0,
	} {
		got, ok := Choose(reply, opts)
		assert.True(t, ok, reply)
		assert.Equal(t, want, got, reply)
	}
	_, ok := Choose("table 9", opts)
	assert.False(t, ok)
	_, ok = Choose("", opts)
	assert.False(t, ok)

	dishes := []Option{{Label: "Chicken supreme"}, {Label: "Chicken wings"}}
	got, ok := Choose("wings", dishes)
	assert.True(t, ok)
	assert.Equal(t, 1, got)
	_, ok = Choose("chicken", dishes)
	assert.False(t, ok, "still ambiguous")

	got, ok = Choose("yes", []Option{{Label: "Table 14"}})
	assert.True(t, ok)
	assert.Equal(t, 0, got)
}

func TestNumbers(t *testing.T) {
	for in, want := range map[string]string{
		"seventy eight":         "78",
		"three point four":      "3.4",
		"minus eighteen":        "-18",
		"twenty one point five": "21.5",
		"3,4":                   "3.4",
		"-18":                   "-18",
		"fourteen":              "14",
		"eighty-six":            "86",
	} {
		assert.Equal(t, []string{want}, normalize(in), in)
	}
	assert.Equal(t, []string{"tables", "4", "and", "7"}, normalize("tables 4, and 7"))
}

func TestReaders(t *testing.T) {
	for in, want := range map[string]bool{"yes": true, "Yeah, all good": true, "fine": true, "no": false, "No, the box was torn": false, "out of date": false} {
		got, ok := ReadYesNo(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	_, ok := ReadYesNo("maybe")
	assert.False(t, ok)

	for in, want := range map[string]Cooked{
		"14:30": {Clock: "14:30"}, "at 2 pm": {Clock: "14:00"}, "half two": {}, "1430": {Clock: "14:30"},
		"ninety minutes ago": {Ago: 90 * time.Minute}, "an hour ago": {Ago: time.Hour}, "2 hours ago": {Ago: 2 * time.Hour},
		"cooked at 12 15 am": {Clock: "00:15"},
	} {
		got, ok := ReadCooked(in)
		if want == (Cooked{}) {
			assert.False(t, ok, in)
			continue
		}
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	_, ok = ReadCooked("25 70")
	assert.False(t, ok)

	for in, want := range map[string]int{"5": 5, "five days": 5, "a week": 7, "2 weeks": 14} {
		got, ok := ReadDays(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	k, ok := ReadDeliveryKind("it's frozen")
	assert.True(t, ok)
	assert.Equal(t, "frozen", k)
	n, ok := ReadNumber("minus eighteen")
	assert.True(t, ok)
	assert.InDelta(t, -18, n, 0.01)
	task, ok := ChooseTask("the chopping boards", vocab.Tasks)
	assert.True(t, ok)
	assert.Equal(t, "Chopping boards washed & sanitised", task)
	assert.Equal(t, "Synthetic Farm", ReadText("synthetic Farm."))
}
