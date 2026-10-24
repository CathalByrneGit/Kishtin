package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// StationInput is a new station, or a station's new values.
type StationInput struct {
	Name     string `json:"name"`
	Position int    `json:"position,omitempty"`
	// Active false takes the station out of use. Left out, it stays as it
	// is; new stations are active.
	Active *bool `json:"active,omitempty"`
}

// EquipmentInput is new equipment, or its new values.
type EquipmentInput struct {
	StationID string `json:"station_id"`
	Name      string `json:"name"`
	Slots     int    `json:"slots"`
	// Active false takes the equipment out of use. Left out, it stays as
	// it is; new equipment is active.
	Active *bool `json:"active,omitempty"`
}

// DishInput is a new dish, or a dish's new values.
type DishInput struct {
	Name          string          `json:"name"`
	Course        menu.Course     `json:"course"`
	StationID     string          `json:"station_id"`
	EquipmentID   string          `json:"equipment_id,omitempty"`
	BatchCapacity int             `json:"batch_capacity,omitempty"`
	CookSeconds   int             `json:"cook_seconds,omitempty"`
	RestSeconds   int             `json:"rest_seconds,omitempty"`
	PlateSeconds  int             `json:"plate_seconds,omitempty"`
	HoldSeconds   int             `json:"hold_seconds,omitempty"`
	Batchable     bool            `json:"batchable,omitempty"`
	Modifiers     []menu.Modifier `json:"modifiers,omitempty"`
	FoodCategory  string          `json:"food_category,omitempty"`
	// Available false marks the dish sold out; Active false takes it off
	// the menu. Left out, they stay as they are; a new dish is on the menu
	// and available.
	Available *bool `json:"available,omitempty"`
	Active    *bool `json:"active,omitempty"`
}

func orBool(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func (in DishInput) dish(old menu.Dish) menu.Dish {
	return menu.Dish{
		Name: in.Name, Course: in.Course, StationID: in.StationID, EquipmentID: in.EquipmentID,
		BatchCapacity: in.BatchCapacity, CookSeconds: in.CookSeconds, RestSeconds: in.RestSeconds,
		PlateSeconds: in.PlateSeconds, HoldSeconds: in.HoldSeconds, Batchable: in.Batchable,
		Modifiers: in.Modifiers, FoodCategory: in.FoodCategory,
		Available: orBool(in.Available, old.Available), Active: orBool(in.Active, old.Active),
	}.Normalize()
}

// MenuSetup reads the whole menu through st.
func menuSetup(ctx context.Context, st store.Store) (menu.Setup, error) {
	stations, err := st.Menu().ListStations(ctx)
	if err != nil {
		return menu.Setup{}, fmt.Errorf("list stations: %w", err)
	}
	equipment, err := st.Menu().ListEquipment(ctx)
	if err != nil {
		return menu.Setup{}, fmt.Errorf("list equipment: %w", err)
	}
	dishes, err := st.Menu().ListDishes(ctx)
	if err != nil {
		return menu.Setup{}, fmt.Errorf("list dishes: %w", err)
	}
	return menu.Setup{Stations: stations, Equipment: equipment, Dishes: dishes}, nil
}

// Menu returns the stations, equipment and dishes. Any staff member can see
// it.
func (s *Service) Menu(ctx context.Context, p *auth.Principal) (menu.Setup, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return menu.Setup{}, err
	}
	return menuSetup(ctx, s.st)
}

func (s *Service) menuChanged(id string) { s.publish(events.KindMenu, id) }

// CreateStation adds a station.
func (s *Service) CreateStation(ctx context.Context, p *auth.Principal, in StationInput) (menu.Station, error) {
	if err := allow(p, auth.ActManageMenu); err != nil {
		return menu.Station{}, err
	}
	now := s.clk.Now()
	st := menu.Station{ID: newID(), Name: strings.TrimSpace(in.Name), Position: in.Position, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := st.Validate(); err != nil {
		return menu.Station{}, newErr(KindInvalid, "%s", err)
	}
	if err := s.st.Menu().CreateStation(ctx, st); err != nil {
		return menu.Station{}, mapStoreErr(err, "a station called "+st.Name)
	}
	s.menuChanged(st.ID)
	return st, nil
}

// UpdateStation changes a station. A station with active dishes or
// equipment cannot be taken out of use.
func (s *Service) UpdateStation(ctx context.Context, p *auth.Principal, id string, in StationInput) (menu.Station, error) {
	if err := allow(p, auth.ActManageMenu); err != nil {
		return menu.Station{}, err
	}
	var out menu.Station
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		st, ok := setup.Station(id)
		if !ok {
			return newErr(KindNotFound, "station not found")
		}
		st.Name, st.Position, st.Active, st.UpdatedAt = strings.TrimSpace(in.Name), in.Position, orBool(in.Active, st.Active), s.clk.Now()
		if err := st.Validate(); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		if !st.Active {
			var using []string
			for _, d := range setup.Dishes {
				if d.Active && d.StationID == id {
					using = append(using, d.Name)
				}
			}
			for _, e := range setup.Equipment {
				if e.Active && e.StationID == id {
					using = append(using, e.Name)
				}
			}
			if len(using) > 0 {
				return newErr(KindConflict, "%s is still used by %s: move or take those out of use first", st.Name, strings.Join(using, ", "))
			}
		}
		if err := tx.Menu().UpdateStation(ctx, st); err != nil {
			return mapStoreErr(err, "a station called "+st.Name)
		}
		out = st
		return nil
	})
	if err != nil {
		return menu.Station{}, err
	}
	s.menuChanged(id)
	return out, nil
}

// CreateEquipment adds equipment to a station.
func (s *Service) CreateEquipment(ctx context.Context, p *auth.Principal, in EquipmentInput) (menu.Equipment, error) {
	if err := allow(p, auth.ActManageMenu); err != nil {
		return menu.Equipment{}, err
	}
	var out menu.Equipment
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		now := s.clk.Now()
		e := menu.Equipment{ID: newID(), StationID: in.StationID, Name: strings.TrimSpace(in.Name), Slots: in.Slots, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := e.Validate(); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		if st, ok := setup.Station(e.StationID); !ok || !st.Active {
			return newErr(KindInvalid, "choose a station that is in use")
		}
		if err := tx.Menu().CreateEquipment(ctx, e); err != nil {
			return mapStoreErr(err, e.Name+" in that station")
		}
		out = e
		return nil
	})
	if err != nil {
		return menu.Equipment{}, err
	}
	s.menuChanged(out.ID)
	return out, nil
}

// UpdateEquipment changes equipment. Equipment with active dishes cannot be
// taken out of use or moved to another station.
func (s *Service) UpdateEquipment(ctx context.Context, p *auth.Principal, id string, in EquipmentInput) (menu.Equipment, error) {
	if err := allow(p, auth.ActManageMenu); err != nil {
		return menu.Equipment{}, err
	}
	var out menu.Equipment
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		e, ok := setup.EquipmentByID(id)
		if !ok {
			return newErr(KindNotFound, "equipment not found")
		}
		moved := e.StationID != in.StationID
		e.StationID, e.Name, e.Slots, e.Active, e.UpdatedAt = in.StationID, strings.TrimSpace(in.Name), in.Slots, orBool(in.Active, e.Active), s.clk.Now()
		if err := e.Validate(); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		if st, ok := setup.Station(e.StationID); !ok || (!st.Active && e.Active) {
			return newErr(KindInvalid, "choose a station that is in use")
		}
		if !e.Active || moved {
			var using []string
			for _, d := range setup.Dishes {
				if d.Active && d.EquipmentID == id {
					using = append(using, d.Name)
				}
			}
			if len(using) > 0 {
				return newErr(KindConflict, "%s is used by %s: change those dishes first", e.Name, strings.Join(using, ", "))
			}
		}
		if err := tx.Menu().UpdateEquipment(ctx, e); err != nil {
			return mapStoreErr(err, e.Name+" in that station")
		}
		out = e
		return nil
	})
	if err != nil {
		return menu.Equipment{}, err
	}
	s.menuChanged(id)
	return out, nil
}

// checkCategory checks a dish's food category against the HACCP lists, so
// its cooking limit applies.
func (s *Service) checkCategory(ctx context.Context, st store.Store, d *menu.Dish) error {
	if d.FoodCategory == "" {
		return nil
	}
	lists, err := s.Lists().current(ctx, st)
	if err != nil {
		return err
	}
	cats := lists.Value.FoodCategories
	if len(cats) == 0 {
		return nil
	}
	i := slices.IndexFunc(cats, func(c string) bool { return strings.EqualFold(c, d.FoodCategory) })
	if i < 0 {
		return newErr(KindInvalid, "food category %q is not on the list: choose one of %s", d.FoodCategory, strings.Join(cats, ", "))
	}
	d.FoodCategory = cats[i]
	return nil
}

// CreateDish adds a dish to the menu.
func (s *Service) CreateDish(ctx context.Context, p *auth.Principal, in DishInput) (menu.Dish, error) {
	if err := allow(p, auth.ActManageMenu); err != nil {
		return menu.Dish{}, err
	}
	var out menu.Dish
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		now := s.clk.Now()
		d := in.dish(menu.Dish{Available: true, Active: true})
		d.ID, d.CreatedAt, d.UpdatedAt = newID(), now, now
		if err := setup.ValidateDish(d); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		if err := s.checkCategory(ctx, tx, &d); err != nil {
			return err
		}
		if err := tx.Menu().CreateDish(ctx, d); err != nil {
			return mapStoreErr(err, "a dish called "+d.Name)
		}
		out = d
		return nil
	})
	if err != nil {
		return menu.Dish{}, err
	}
	s.menuChanged(out.ID)
	return out, nil
}

// UpdateDish replaces a dish's values. Orders already in the kitchen keep
// the times they were ordered with.
func (s *Service) UpdateDish(ctx context.Context, p *auth.Principal, id string, in DishInput) (menu.Dish, error) {
	if err := allow(p, auth.ActManageMenu); err != nil {
		return menu.Dish{}, err
	}
	var out menu.Dish
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		old, ok := setup.Dish(id)
		if !ok {
			return newErr(KindNotFound, "dish not found")
		}
		d := in.dish(old)
		d.ID, d.CreatedAt, d.UpdatedAt = id, old.CreatedAt, s.clk.Now()
		if err := setup.ValidateDish(d); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		if err := s.checkCategory(ctx, tx, &d); err != nil {
			return err
		}
		if err := tx.Menu().UpdateDish(ctx, d); err != nil {
			return mapStoreErr(err, "a dish called "+d.Name)
		}
		out = d
		return nil
	})
	if err != nil {
		return menu.Dish{}, err
	}
	s.menuChanged(id)
	return out, nil
}

// SetAvailability marks a dish sold out ("86 salmon") or back on.
func (s *Service) SetAvailability(ctx context.Context, p *auth.Principal, id string, available bool) (menu.Dish, error) {
	if err := allow(p, auth.ActSetAvailability); err != nil {
		return menu.Dish{}, err
	}
	var out menu.Dish
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		d, err := tx.Menu().GetDish(ctx, id)
		if err != nil {
			return mapStoreErr(err, "dish")
		}
		d.Available, d.UpdatedAt = available, s.clk.Now()
		if err := tx.Menu().UpdateDish(ctx, d); err != nil {
			return err
		}
		out = d
		return nil
	})
	if err != nil {
		return menu.Dish{}, err
	}
	s.menuChanged(id)
	return out, nil
}
