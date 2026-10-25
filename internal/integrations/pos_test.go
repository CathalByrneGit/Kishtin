package integrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var maps = map[string]Mapping{
	"var-burger":   {ExternalID: "var-burger", Kind: EntryItem, DishID: "d-burger"},
	"var-fries":    {ExternalID: "var-fries", Kind: EntryItem, DishID: "d-fries"},
	"var-cola":     {ExternalID: "var-cola", Kind: EntryItem, Ignore: true},
	"mod-welldone": {ExternalID: "mod-welldone", Kind: EntryModifier, Modifier: "Well done"},
	"mod-bag":      {ExternalID: "mod-bag", Kind: EntryModifier, Ignore: true},
}

func TestTranslate(t *testing.T) {
	o := TillOrder{
		ID: "sq-1", Version: 1, State: OrderOpen, Ticket: "Table 4",
		Lines: []TillLine{
			{UID: "l1", CatalogID: "var-burger", Name: "Burger", Quantity: 2, Note: "no onion",
				Modifiers: []TillModifier{{CatalogID: "mod-welldone", Name: "Well done"}, {CatalogID: "mod-x", Name: "Extra pickles"}}},
			{UID: "l2", CatalogID: "var-cola", Name: "Cola", Quantity: 2},
			{UID: "l3", CatalogID: "var-lamb", Name: "Lamb special", Quantity: 1},
			{UID: "l4", Name: "Custom amount", Quantity: 1},
		},
	}
	tk, reduced := Translate(o, maps, nil)
	assert.Empty(t, reduced)
	assert.False(t, tk.Takeaway)
	assert.Equal(t, "4", tk.Table)
	assert.Equal(t, []TicketLine{{LineUID: "l1", DishID: "d-burger", Modifiers: []string{"Well done"}, Notes: "Extra pickles; no onion", Quantity: 2}}, tk.Lines)
	assert.Equal(t, []string{"1 × Lamb special", "1 × Custom amount"}, tk.Unmapped, "drinks are not for the kitchen; unknown items are listed")

	// Version 2: one more burger, fries added, and a line removed.
	o.Version = 2
	o.Lines[0].Quantity = 3
	o.Lines = append(o.Lines[:3], TillLine{UID: "l5", CatalogID: "var-fries", Name: "Fries", Quantity: 1, Modifiers: []TillModifier{{CatalogID: "mod-bag", Name: "Bag"}}})
	tk, reduced = Translate(o, maps, map[string]int{"l1": 2, "l9": 1})
	assert.Equal(t, []TicketLine{
		{LineUID: "l1", DishID: "d-burger", Modifiers: []string{"Well done"}, Notes: "Extra pickles; no onion", Quantity: 1},
		{LineUID: "l5", DishID: "d-fries", Quantity: 1},
	}, tk.Lines)
	assert.Equal(t, []string{"a line removed on the till (1 added before)"}, reduced)

	// Fewer than before: nothing is voided automatically.
	o.Lines[0].Quantity = 1
	tk, reduced = Translate(o, maps, map[string]int{"l1": 2, "l5": 1})
	assert.Empty(t, tk.Lines)
	assert.Equal(t, []string{"Burger now 1, was 2"}, reduced)
}

func TestTranslateTakeaway(t *testing.T) {
	tk, _ := Translate(TillOrder{Takeaway: true, Customer: "Synthetic customer", Ticket: "#88"}, maps, nil)
	assert.True(t, tk.Takeaway)
	assert.Equal(t, "Synthetic customer", tk.Name)
	tk, _ = Translate(TillOrder{Ticket: "Bar tab"}, maps, nil)
	assert.True(t, tk.Takeaway, "no table to be found")
	assert.Equal(t, "Bar tab", tk.Name)
}

func TestTableName(t *testing.T) {
	for in, want := range map[string]string{"Table 4": "4", "table 12A": "12A", "T4": "4", "Tbl. 9": "9", "7": "7", " Table #3 ": "3"} {
		got, ok := TableName(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "Takeaway", "Tom", "Table", "Bar tab", "Table for two"} {
		_, ok := TableName(in)
		assert.False(t, ok, in)
	}
}

func TestMappingAndSettings(t *testing.T) {
	assert.NoError(t, Mapping{ExternalID: "x", Kind: EntryItem, DishID: "d"}.Validate())
	assert.NoError(t, Mapping{ExternalID: "x", Kind: EntryModifier, Ignore: true}.Validate())
	assert.Error(t, Mapping{ExternalID: "x", Kind: EntryItem}.Validate())
	assert.Error(t, Mapping{ExternalID: "x", Kind: EntryItem, DishID: "d", Ignore: true}.Validate())
	assert.Error(t, Mapping{Kind: EntryItem, DishID: "d"}.Validate())
	assert.Error(t, Mapping{ExternalID: "x", Kind: "drink", DishID: "d"}.Validate())

	assert.NoError(t, DefaultPOSSettings().Validate())
	assert.ErrorContains(t, POSSettings{Enabled: true, PollSeconds: 5}.Validate(), "location ID")
	assert.ErrorContains(t, POSSettings{PollSeconds: 1}.Validate(), "between 2 and 300")

	assert.Equal(t, "Burger", CatalogEntry{Kind: EntryItem, Name: "Regular", Group: "Burger"}.Label())
	assert.Equal(t, "Burger (Large)", CatalogEntry{Kind: EntryItem, Name: "Large", Group: "Burger"}.Label())
	assert.Equal(t, "Well done", CatalogEntry{Kind: EntryModifier, Name: "Well done", Group: "Cooking"}.Label())
}
