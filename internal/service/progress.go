package service

import (
	"context"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// ProgressStep is one step of getting Kishtin going, worked out from what
// the restaurant has set up and recorded, so it ticks itself off.
type ProgressStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Done   bool   `json:"done"`
	// Href is the screen where the step is done.
	Href string `json:"href"`
	// Optional steps matter only to some kitchens, such as the menu for a
	// kitchen that only keeps HACCP records.
	Optional bool `json:"optional,omitempty"`
}

// SetupProgress is the getting-started checklist.
type SetupProgress struct {
	Steps []ProgressStep `json:"steps"`
	// Done counts the steps done that are not optional, out of Total.
	Done  int `json:"done"`
	Total int `json:"total"`
}

// SetupProgress works out how far the restaurant has got with setting
// Kishtin up. Managers see it.
func (s *Service) SetupProgress(ctx context.Context, p *auth.Principal) (SetupProgress, error) {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return SetupProgress{}, err
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return SetupProgress{}, err
	}
	users, err := s.st.Users().ListUsers(ctx)
	if err != nil {
		return SetupProgress{}, err
	}
	keys, err := s.st.Auth().ListAPIKeys(ctx)
	if err != nil {
		return SetupProgress{}, err
	}
	lists, err := s.Lists().current(ctx, s.st)
	if err != nil {
		return SetupProgress{}, err
	}
	dishes, err := s.st.Menu().ListDishes(ctx)
	if err != nil {
		return SetupProgress{}, err
	}
	last, err := s.st.HACCP().LastRecord(ctx)
	if err != nil {
		return SetupProgress{}, err
	}

	staff, managers := 0, 0
	for _, u := range users {
		if !u.Active {
			continue
		}
		staff++
		if u.Role == auth.RoleManager && u.PasswordHash != "" {
			managers++
		}
	}
	devices := 0
	for _, k := range keys {
		if k.Kind == auth.KeyDevice && k.RevokedAt == nil {
			devices++
		}
	}
	// Setting up from the screens pairs that computer's browser; that one
	// is not a tablet.
	setups, err := s.st.Activity().ListActivity(ctx, store.ActivityFilter{Kinds: []string{ActivitySetup}})
	if err != nil {
		return SetupProgress{}, err
	}
	tablets := devices
	for _, a := range setups {
		if a.Detail["from"] == "screen" {
			tablets--
		}
	}
	active, unchecked := 0, 0
	for _, d := range dishes {
		if d.Active {
			active++
			if !d.AllergensChecked {
				unchecked++
			}
		}
	}
	startupOn := false
	if s.startup != nil {
		if st, err := s.startup.Status(ctx); err == nil {
			startupOn = st.On || !st.Supported
		}
	} else {
		// Kishtin started some other way, such as a service the owner set up.
		startupOn = true
	}

	steps := []ProgressStep{
		{ID: "name", Title: "Name the restaurant", Detail: "Shown on every screen and printout.", Done: sys.Value.RestaurantName != "", Href: "/setup/settings"},
		{ID: "staff", Title: "Add your staff", Detail: "Each person with their role and a PIN, so records show who did what.", Done: staff > 1, Href: "/setup/staff"},
		{ID: "second_manager", Title: "Add a second manager", Detail: "Someone who can reset a forgotten password and sign off records when you are away.", Done: managers > 1, Href: "/setup/staff"},
		{ID: "lists", Title: "Name your fridges, freezers and cleaning tasks", Detail: "Kishtin starts with examples: change them to match your kitchen.", Done: !lists.Builtin, Href: "/setup/lists"},
		{ID: "tablet", Title: "Connect a tablet or phone", Detail: "Scan a QR code with each device staff will use.", Done: tablets > 0, Href: "/setup/keys"},
		{ID: "first_check", Title: "Log your first check", Detail: "A fridge temperature is a good start.", Done: last != nil, Href: "/today"},
		{ID: "startup", Title: "Start Kishtin with the computer", Detail: "So nobody has to start it after a restart.", Done: startupOn, Href: "/setup/settings"},
		{ID: "menu", Title: "Add your menu", Detail: "Only needed for the kitchen screens, till orders and the allergen chart.", Done: active > 0, Href: "/setup/menu", Optional: true},
		{ID: "allergens", Title: "Check every dish's allergens", Detail: "So staff can answer customers from the allergen chart.", Done: active > 0 && unchecked == 0, Href: "/setup/allergens", Optional: active == 0},
	}
	out := SetupProgress{Steps: steps}
	for _, st := range steps {
		if st.Optional {
			continue
		}
		out.Total++
		if st.Done {
			out.Done++
		}
	}
	return out, nil
}
