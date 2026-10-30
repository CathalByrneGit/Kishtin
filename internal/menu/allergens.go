package menu

import (
	"fmt"
	"slices"
	"strings"
)

// Allergen is one of the allergens a food business must tell customers
// about.
type Allergen struct {
	// Code is how Kishtin stores it.
	Code string `json:"code"`
	// Name is the allergen as the regulation names it.
	Name string `json:"name"`
	// Detail lists what it covers, where the regulation says.
	Detail string `json:"detail,omitempty"`
}

// Allergens are the 14 allergens that must be declared for food served
// without packaging, in the order of Annex II of Regulation (EU) No
// 1169/2011 on food information to consumers. Ireland applies it to
// restaurants, cafés and pubs through S.I. No. 489 of 2014 (Health
// (Provision of Food Allergen Information to Consumers in respect of
// Non-Prepacked Food) Regulations 2014); see the FSAI's "Allergen
// information for non-prepacked food" guidance. The list is fixed in law,
// so it is code, not a setting.
var Allergens = []Allergen{
	{"gluten", "Cereals containing gluten", "wheat (such as spelt and khorasan), rye, barley and oats"},
	{"crustaceans", "Crustaceans", "such as prawns, crab and lobster"},
	{"eggs", "Eggs", ""},
	{"fish", "Fish", ""},
	{"peanuts", "Peanuts", ""},
	{"soybeans", "Soybeans", ""},
	{"milk", "Milk", "including lactose"},
	{"nuts", "Nuts", "almonds, hazelnuts, walnuts, cashews, pecans, Brazil nuts, pistachios and macadamia nuts"},
	{"celery", "Celery", ""},
	{"mustard", "Mustard", ""},
	{"sesame", "Sesame seeds", ""},
	{"sulphites", "Sulphur dioxide and sulphites", "above 10 mg per kg or 10 mg per litre"},
	{"lupin", "Lupin", ""},
	{"molluscs", "Molluscs", "such as mussels, oysters and squid"},
}

// AllergenName returns an allergen's name, or the code if it is unknown.
func AllergenName(code string) string {
	if i := slices.IndexFunc(Allergens, func(a Allergen) bool { return a.Code == code }); i >= 0 {
		return Allergens[i].Name
	}
	return code
}

// CleanAllergens checks allergen codes, ignoring case, and returns them
// once each, in the regulation's order.
func CleanAllergens(codes []string) ([]string, error) {
	seen := map[string]bool{}
	for _, c := range codes {
		c = strings.ToLower(strings.TrimSpace(c))
		if !slices.ContainsFunc(Allergens, func(a Allergen) bool { return a.Code == c }) {
			return nil, fmt.Errorf("%q is not one of the 14 allergens: use %s", c, strings.Join(AllergenCodes(), ", "))
		}
		seen[c] = true
	}
	out := []string{}
	for _, a := range Allergens {
		if seen[a.Code] {
			out = append(out, a.Code)
		}
	}
	return out, nil
}

// AllergenCodes lists every allergen's code, in order.
func AllergenCodes() []string {
	out := make([]string, len(Allergens))
	for i, a := range Allergens {
		out[i] = a.Code
	}
	return out
}
