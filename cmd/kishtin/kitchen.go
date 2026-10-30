package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
)

func ptr[T any](v T) *T { return &v }

func hhmm(t time.Time) string { return t.In(clock.Dublin).Format("15:04") }

func secs(n int64) string {
	if n == 0 {
		return "0"
	}
	return shortDur(time.Duration(n) * time.Second)
}

func (c *cli) menu(ctx context.Context) (*client.Setup, error) {
	m, err := c.api.GetMenu(ctx)
	return m, apiErr(err)
}

// findStation finds a station by name (any case) or ID.
func findStation(m *client.Setup, ref string) (client.Station, error) {
	for _, s := range m.Stations {
		if s.ID == ref || strings.EqualFold(s.Name, ref) {
			return s, nil
		}
	}
	return client.Station{}, fmt.Errorf("no station called %q", ref)
}

// findEquipment finds equipment by name or ID, within a station when one
// is given.
func findEquipment(m *client.Setup, stationID, ref string) (client.Equipment, error) {
	for _, e := range m.Equipment {
		if (e.ID == ref || strings.EqualFold(e.Name, ref)) && (stationID == "" || e.StationID == stationID) {
			return e, nil
		}
	}
	return client.Equipment{}, fmt.Errorf("no equipment called %q in that station", ref)
}

// findDish finds a dish by name (any case) or ID.
func findDish(m *client.Setup, ref string) (client.Dish, error) {
	for _, d := range m.Dishes {
		if d.ID == ref || strings.EqualFold(d.Name, ref) {
			return d, nil
		}
	}
	var names []string
	for _, d := range m.Dishes {
		if d.Active {
			names = append(names, d.Name)
		}
	}
	return client.Dish{}, fmt.Errorf("no dish called %q: dishes are %s", ref, strings.Join(names, ", "))
}

func stationName(m *client.Setup, id string) string {
	for _, s := range m.Stations {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

func equipmentName(m *client.Setup, id string) string {
	for _, e := range m.Equipment {
		if e.ID == id {
			return e.Name
		}
	}
	return id
}

func (c *cli) menuCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "menu", Short: "Show and change stations, equipment and dishes", Args: cobra.NoArgs,
		Long: "menu shows every station, its equipment and its dishes. Changing the menu needs a manager key.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(m, func(w io.Writer) {
				for _, s := range m.Stations {
					status := ""
					if !s.Active {
						status = " (not in use)"
					}
					fmt.Fprintf(w, "%s%s\n", s.Name, status)
					for _, e := range m.Equipment {
						if e.StationID == s.ID {
							fmt.Fprintf(w, "  equipment: %s, %d slots\n", e.Name, e.Slots)
						}
					}
					var rows [][]string
					for _, d := range m.Dishes {
						if d.StationID != s.ID || !d.Active {
							continue
						}
						avail := "available"
						if !d.Available {
							avail = "86"
						}
						on := ""
						if d.EquipmentID != nil {
							on = equipmentName(m, *d.EquipmentID)
							if d.BatchCapacity != nil && *d.BatchCapacity > 0 {
								on += fmt.Sprintf(" (%d at once)", *d.BatchCapacity)
							}
						}
						var mods []string
						for _, md := range d.Modifiers {
							mods = append(mods, fmt.Sprintf("%s %+ds", md.Name, md.AddSeconds))
						}
						rows = append(rows, []string{"  " + d.Name, d.Course, on, secs(d.CookSeconds), secs(d.RestSeconds), secs(d.PlateSeconds), secs(d.HoldSeconds), avail, strings.Join(mods, ", ")})
					}
					if len(rows) > 0 {
						table(w, "  DISH\tCOURSE\tON\tCOOK\tREST\tPLATE\tHOLD\tSTATUS\tMODIFIERS", rows)
					}
				}
			})
		},
	}
	cmd.AddCommand(c.stationCmds(), c.equipmentCmds(), c.dishCmds(), c.availabilityCmd(true), c.availabilityCmd(false), c.learnedCmd(), c.allergensCmd())
	return cmd
}

func (c *cli) stationCmds() *cobra.Command {
	cmd := &cobra.Command{Use: "station", Short: "Add or change stations (manager)"}
	var name string
	var position int64
	var active bool
	add := &cobra.Command{
		Use: "add", Short: "Add a station", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := c.api.CreateStation(cmd.Context(), &client.CreateStationRequestOptions{Body: &client.StationInput{Name: name, Position: &position}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(s, func(w io.Writer) { fmt.Fprintf(w, "Added station %s, ID %s\n", s.Name, s.ID) })
		},
	}
	add.Flags().StringVar(&name, "name", "", "station name, such as Grill")
	add.Flags().Int64Var(&position, "position", 0, "order on screens")
	update := &cobra.Command{
		Use: "update STATION", Short: "Change a station's name, position or whether it is in use", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			st, err := findStation(m, args[0])
			if err != nil {
				return err
			}
			in := client.StationInput{Name: st.Name, Position: &st.Position}
			if cmd.Flags().Changed("name") {
				in.Name = name
			}
			if cmd.Flags().Changed("position") {
				in.Position = &position
			}
			if cmd.Flags().Changed("active") {
				in.Active = &active
			}
			s, err := c.api.UpdateStation(cmd.Context(), &client.UpdateStationRequestOptions{PathParams: &client.UpdateStationPath{ID: st.ID}, Body: &in})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(s, func(w io.Writer) { fmt.Fprintf(w, "Saved station %s\n", s.Name) })
		},
	}
	update.Flags().StringVar(&name, "name", "", "new name")
	update.Flags().Int64Var(&position, "position", 0, "order on screens")
	update.Flags().BoolVar(&active, "active", true, "false takes the station out of use")
	cmd.AddCommand(add, update)
	return cmd
}

func (c *cli) equipmentCmds() *cobra.Command {
	cmd := &cobra.Command{Use: "equipment", Short: "Add or change equipment (manager)"}
	var station, name string
	var slots int64
	var active bool
	add := &cobra.Command{
		Use: "add", Short: "Add equipment to a station", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			st, err := findStation(m, station)
			if err != nil {
				return err
			}
			e, err := c.api.CreateEquipment(cmd.Context(), &client.CreateEquipmentRequestOptions{Body: &client.EquipmentInput{StationID: st.ID, Name: name, Slots: slots}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(e, func(w io.Writer) { fmt.Fprintf(w, "Added %s to %s: %d slots, ID %s\n", e.Name, st.Name, e.Slots, e.ID) })
		},
	}
	add.Flags().StringVar(&station, "station", "", "station name or ID")
	add.Flags().StringVar(&name, "name", "", "equipment name, such as Flat-top")
	add.Flags().Int64Var(&slots, "slots", 1, "how many units it holds at once")
	update := &cobra.Command{
		Use: "update EQUIPMENT", Short: "Change equipment", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			eq, err := findEquipment(m, "", args[0])
			if err != nil {
				return err
			}
			in := client.EquipmentInput{StationID: eq.StationID, Name: eq.Name, Slots: eq.Slots}
			fl := cmd.Flags()
			if fl.Changed("station") {
				st, err := findStation(m, station)
				if err != nil {
					return err
				}
				in.StationID = st.ID
			}
			if fl.Changed("name") {
				in.Name = name
			}
			if fl.Changed("slots") {
				in.Slots = slots
			}
			if fl.Changed("active") {
				in.Active = &active
			}
			e, err := c.api.UpdateEquipment(cmd.Context(), &client.UpdateEquipmentRequestOptions{PathParams: &client.UpdateEquipmentPath{ID: eq.ID}, Body: &in})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(e, func(w io.Writer) { fmt.Fprintf(w, "Saved %s: %d slots\n", e.Name, e.Slots) })
		},
	}
	update.Flags().StringVar(&station, "station", "", "move to this station")
	update.Flags().StringVar(&name, "name", "", "new name")
	update.Flags().Int64Var(&slots, "slots", 1, "how many units it holds at once")
	update.Flags().BoolVar(&active, "active", true, "false takes it out of use")
	cmd.AddCommand(add, update)
	return cmd
}

// dishFlags are the flags shared by dish add and dish update.
type dishFlags struct {
	name, course, station, equipment, category string
	cook, rest, plate, hold                    time.Duration
	capacity                                   int64
	batchable, active, noAllergens             bool
	modifiers, allergens                       []string
}

func (f *dishFlags) register(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.name, "name", "", "dish name")
	fl.StringVar(&f.course, "course", "main", "starter, main or dessert")
	fl.StringVar(&f.station, "station", "", "station name or ID")
	fl.StringVar(&f.equipment, "equipment", "", "equipment it cooks on (leave out for none)")
	fl.StringVar(&f.category, "category", "", "HACCP food category, such as poultry")
	fl.DurationVar(&f.cook, "cook", 0, "cook time, such as 8m")
	fl.DurationVar(&f.rest, "rest", 0, "rest time, such as 2m")
	fl.DurationVar(&f.plate, "plate", 0, "plate time, such as 1m")
	fl.DurationVar(&f.hold, "hold", 0, "how long it can wait at the pass")
	fl.Int64Var(&f.capacity, "capacity", 0, "how many the equipment cooks at once (default: its slots)")
	fl.BoolVar(&f.batchable, "batchable", false, "allow cooking for several tables at once")
	fl.BoolVar(&f.active, "active", true, "false takes the dish off the menu")
	fl.StringArrayVar(&f.modifiers, "modifier", nil, `a modifier and its extra cook time, such as "Well done=2m" or "Rare=-1m" (repeatable)`)
	fl.StringSliceVar(&f.allergens, "allergens", nil, "the allergens it contains, such as gluten,milk (marks them checked); see kishtin menu allergens")
	fl.BoolVar(&f.noAllergens, "no-allergens", false, "it contains none of the 14 allergens (marks them checked)")
}

func parseModifiers(specs []string) ([]client.Modifier, error) {
	var out []client.Modifier
	for _, s := range specs {
		name, d, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("modifier %q: write it as Name=2m", s)
		}
		dur, err := time.ParseDuration(strings.TrimSpace(d))
		if err != nil {
			return nil, fmt.Errorf("modifier %q: %w", s, err)
		}
		out = append(out, client.Modifier{Name: strings.TrimSpace(name), AddSeconds: int64(dur / time.Second)})
	}
	return out, nil
}

// input builds a dish from flags. With base set, only changed flags
// replace the dish's current values.
func (f *dishFlags) input(cmd *cobra.Command, m *client.Setup, base *client.Dish) (*client.DishInput, error) {
	fl := cmd.Flags()
	in := &client.DishInput{Name: f.name, Course: f.course}
	if base != nil {
		in = &client.DishInput{
			Name: base.Name, Course: base.Course, StationID: base.StationID, EquipmentID: base.EquipmentID,
			BatchCapacity: base.BatchCapacity, Batchable: &base.Batchable, CookSeconds: &base.CookSeconds,
			RestSeconds: &base.RestSeconds, PlateSeconds: &base.PlateSeconds, HoldSeconds: &base.HoldSeconds,
			Modifiers: base.Modifiers, FoodCategory: base.FoodCategory,
		}
	}
	changed := func(n string) bool { return base == nil || fl.Changed(n) }
	if changed("name") {
		in.Name = f.name
	}
	if changed("course") {
		in.Course = f.course
	}
	if changed("station") {
		st, err := findStation(m, f.station)
		if err != nil {
			return nil, err
		}
		in.StationID = st.ID
	}
	if changed("equipment") {
		in.EquipmentID = nil
		if f.equipment != "" {
			eq, err := findEquipment(m, in.StationID, f.equipment)
			if err != nil {
				return nil, err
			}
			in.EquipmentID = &eq.ID
		}
	}
	for name, dst := range map[string]**int64{"cook": &in.CookSeconds, "rest": &in.RestSeconds, "plate": &in.PlateSeconds, "hold": &in.HoldSeconds} {
		if changed(name) {
			d := map[string]time.Duration{"cook": f.cook, "rest": f.rest, "plate": f.plate, "hold": f.hold}[name]
			*dst = ptr(int64(d / time.Second))
		}
	}
	if changed("capacity") {
		in.BatchCapacity = &f.capacity
	}
	if changed("batchable") {
		in.Batchable = &f.batchable
	}
	if changed("category") {
		in.FoodCategory = &f.category
	}
	if changed("modifier") {
		mods, err := parseModifiers(f.modifiers)
		if err != nil {
			return nil, err
		}
		in.Modifiers = mods
	}
	if base != nil && fl.Changed("active") {
		in.Active = &f.active
	}
	if fl.Changed("allergens") || f.noAllergens {
		if fl.Changed("allergens") && f.noAllergens {
			return nil, fmt.Errorf("give --allergens or --no-allergens, not both")
		}
		in.Allergens, in.AllergensNone, in.AllergensChecked = f.allergens, &f.noAllergens, ptr(true)
	}
	return in, nil
}

func (c *cli) dishCmds() *cobra.Command {
	cmd := &cobra.Command{Use: "dish", Short: "Add or change dishes (manager)"}
	var add, update dishFlags
	addCmd := &cobra.Command{
		Use: "add", Short: "Add a dish", Args: cobra.NoArgs,
		Example: `  kishtin menu dish add --name "Burger" --station Grill --equipment Flat-top \
    --cook 8m --rest 2m --plate 1m --hold 3m --batchable --modifier "Well done=2m"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			in, err := add.input(cmd, m, nil)
			if err != nil {
				return err
			}
			d, err := c.api.CreateDish(cmd.Context(), &client.CreateDishRequestOptions{Body: in})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(d, func(w io.Writer) {
				fmt.Fprintf(w, "Added %s to %s, ID %s\n", d.Name, stationName(m, d.StationID), d.ID)
			})
		},
	}
	add.register(addCmd)
	updateCmd := &cobra.Command{
		Use: "update DISH", Short: "Change a dish: only the flags you give change", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			cur, err := findDish(m, args[0])
			if err != nil {
				return err
			}
			in, err := update.input(cmd, m, &cur)
			if err != nil {
				return err
			}
			d, err := c.api.UpdateDish(cmd.Context(), &client.UpdateDishRequestOptions{PathParams: &client.UpdateDishPath{ID: cur.ID}, Body: in})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(d, func(w io.Writer) { fmt.Fprintf(w, "Saved %s\n", d.Name) })
		},
	}
	update.register(updateCmd)
	cmd.AddCommand(addCmd, updateCmd)
	return cmd
}

func (c *cli) availabilityCmd(soldOut bool) *cobra.Command {
	use, short := "on DISH", "Put a sold-out dish back on"
	if soldOut {
		use, short = "86 DISH", "Mark a dish sold out (86)"
	}
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			d, err := findDish(m, args[0])
			if err != nil {
				return err
			}
			res, err := c.api.SetDishAvailability(cmd.Context(), &client.SetDishAvailabilityRequestOptions{
				PathParams: &client.SetDishAvailabilityPath{ID: d.ID}, Body: &client.AvailabilityBody{Available: !soldOut},
			})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				if soldOut {
					fmt.Fprintf(w, "%s is 86: it cannot be ordered\n", res.Name)
				} else {
					fmt.Fprintf(w, "%s is back on\n", res.Name)
				}
			})
		},
	}
}

// --- Orders ---

var itemSpec = regexp.MustCompile(`^\s*(?:(\d+)\s*[x×]\s*)?([^:@]+?)\s*(?::\s*([^@]*?))?\s*(?:@\s*(\w+))?\s*$`)

// parseItem reads "2x Burger: well done @main".
func parseItem(m *client.Setup, s string) (client.NewItem, error) {
	g := itemSpec.FindStringSubmatch(s)
	if g == nil {
		return client.NewItem{}, fmt.Errorf("item %q: write it as \"2x Burger: well done @main\"", s)
	}
	d, err := findDish(m, g[2])
	if err != nil {
		return client.NewItem{}, err
	}
	ni := client.NewItem{DishID: d.ID}
	if g[1] != "" {
		n, _ := strconv.ParseInt(g[1], 10, 64)
		ni.Quantity = &n
	}
	if g[3] != "" {
		for _, mod := range strings.Split(g[3], ",") {
			if mod = strings.TrimSpace(mod); mod != "" {
				ni.Modifiers = append(ni.Modifiers, mod)
			}
		}
	}
	if g[4] != "" {
		ni.Course = &g[4]
	}
	return ni, nil
}

func orderLabel(o client.Order) string {
	l := fmt.Sprintf("Takeaway #%d", o.Number)
	if o.Kind == "table" {
		l = "Table " + deref(o.Table)
	}
	if n := deref(o.Name); n != "" {
		l += " (" + n + ")"
	}
	return l
}

// findOrder finds an open order by ID, ticket number ("#12"), table ("4",
// "table 4") or name.
func (c *cli) findOrder(ctx context.Context, ref string) (client.Order, error) {
	res, err := c.api.ListOrders(ctx, &client.ListOrdersRequestOptions{Query: &client.ListOrdersQuery{}})
	if err != nil {
		return client.Order{}, apiErr(err)
	}
	r := strings.TrimSpace(strings.ToLower(ref))
	r = strings.TrimPrefix(r, "table ")
	for _, o := range res.Orders {
		if o.ID == ref || "#"+strconv.FormatInt(o.Number, 10) == r || (o.Kind == "table" && strings.EqualFold(deref(o.Table), r)) {
			return o, nil
		}
	}
	var named []client.Order
	for _, o := range res.Orders {
		if n := deref(o.Name); n != "" && strings.EqualFold(n, strings.TrimSpace(ref)) {
			named = append(named, o)
		}
	}
	if len(named) == 1 {
		return named[0], nil
	}
	if len(named) > 1 {
		return client.Order{}, fmt.Errorf("more than one open order is called %q: use the ticket number (#12)", ref)
	}
	return client.Order{}, fmt.Errorf("no open order %q: use its ID, ticket number (#12), table or name", ref)
}

func printOrder(w io.Writer, o client.Order) {
	fmt.Fprintf(w, "%s  #%d  %s  (ID %s)\n", orderLabel(o), o.Number, o.Status, o.ID)
	if deref(o.Allergy) {
		fmt.Fprintln(w, "ALLERGY")
	}
	if n := deref(o.Notes); n != "" {
		fmt.Fprintln(w, "Notes:", n)
	}
	for _, co := range o.Courses {
		line := fmt.Sprintf("%s: %s", co.Course, co.State)
		if co.TargetAt != nil && co.State == "fired" {
			line += ", due " + hhmm(*co.TargetAt)
		}
		if deref(co.Urgent) {
			line += ", urgent"
		}
		fmt.Fprintln(w, line)
		var rows [][]string
		for _, it := range o.Items {
			if it.Course != co.Course {
				continue
			}
			name := it.DishName
			if len(it.Modifiers) > 0 {
				name += " (" + strings.Join(it.Modifiers, ", ") + ")"
			}
			if n := deref(it.Notes); n != "" {
				name += ": " + n
			}
			state := it.State
			if r := deref(it.VoidReason); r != "" {
				state += " (" + r + ")"
			}
			rows = append(rows, []string{"  " + name, state, it.ID})
		}
		table(w, "  ITEM\tSTATE\tID", rows)
	}
}

func (c *cli) orderCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "order", Short: "Enter orders, fire and serve courses"}

	var tableName, name, notes string
	var takeaway, allergy bool
	var covers int64
	var items, fire []string
	newCmd := &cobra.Command{
		Use: "new", Short: "Enter an order", Args: cobra.NoArgs,
		Long: "new enters an order. Each --item is \"[N x] Dish[: modifier, modifier][@course]\".\n" +
			"Courses are held until fired; --fire fires them straight away (head chef or manager).",
		Example: `  kishtin order new --table 4 --covers 2 --item "Salad" --item "2x Burger: well done"
  kishtin order new --takeaway --name "Aoife" --item "Burger" --fire main`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			body := client.NewOrder{Kind: client.Table, Covers: &covers, Allergy: &allergy, Fire: fire}
			if takeaway {
				body.Kind = client.Takeaway
			}
			if tableName != "" {
				body.Table = &tableName
			}
			if name != "" {
				body.Name = &name
			}
			if notes != "" {
				body.Notes = &notes
			}
			for _, s := range items {
				ni, err := parseItem(m, s)
				if err != nil {
					return err
				}
				body.Items = append(body.Items, ni)
			}
			if len(body.Items) == 0 {
				return errors.New("add at least one --item")
			}
			o, err := c.api.CreateOrder(cmd.Context(), &client.CreateOrderRequestOptions{Body: &body})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(o, func(w io.Writer) { printOrder(w, *o) })
		},
	}
	newCmd.Flags().StringVar(&tableName, "table", "", "table number or name")
	newCmd.Flags().BoolVar(&takeaway, "takeaway", false, "a takeaway order")
	newCmd.Flags().StringVar(&name, "name", "", "a name for the order, such as the guest's")
	newCmd.Flags().Int64Var(&covers, "covers", 0, "number of guests")
	newCmd.Flags().BoolVar(&allergy, "allergy", false, "the order has an allergy")
	newCmd.Flags().StringVar(&notes, "notes", "", "notes for the kitchen")
	newCmd.Flags().StringArrayVar(&items, "item", nil, "an item (repeatable)")
	newCmd.Flags().StringSliceVar(&fire, "fire", nil, "courses to fire now")

	var addItems []string
	addCmd := &cobra.Command{
		Use: "add ORDER", Short: "Add items to an open order", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			o, err := c.findOrder(ctx, args[0])
			if err != nil {
				return err
			}
			m, err := c.menu(ctx)
			if err != nil {
				return err
			}
			body := client.AddItemsBody{}
			for _, s := range addItems {
				ni, err := parseItem(m, s)
				if err != nil {
					return err
				}
				body.Items = append(body.Items, ni)
			}
			res, err := c.api.AddItems(ctx, &client.AddItemsRequestOptions{PathParams: &client.AddItemsPath{ID: o.ID}, Body: &body})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { printOrder(w, *res) })
		},
	}
	addCmd.Flags().StringArrayVar(&addItems, "item", nil, "an item (repeatable)")

	var date string
	list := &cobra.Command{
		Use: "list", Short: "List open orders, or every order of a day", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := &client.ListOrdersQuery{}
			if date != "" {
				q.Date = &date
			}
			res, err := c.api.ListOrders(cmd.Context(), &client.ListOrdersRequestOptions{Query: q})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, o := range res.Orders {
					var courses []string
					for _, co := range o.Courses {
						courses = append(courses, co.Course+" "+co.State)
					}
					rows = append(rows, []string{fmt.Sprintf("#%d", o.Number), orderLabel(o), hhmm(o.CreatedAt), strconv.Itoa(len(o.Items)), strings.Join(courses, ", "), o.Status, o.ID})
				}
				table(w, "#\tORDER\tAT\tITEMS\tCOURSES\tSTATUS\tID", rows)
			})
		},
	}
	list.Flags().StringVar(&date, "date", "", "a day, YYYY-MM-DD (default: open orders)")

	var history bool
	show := &cobra.Command{
		Use: "show ORDER", Short: "Show an order (ID, #ticket or table)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			o, err := c.orderByRef(ctx, args[0])
			if err != nil {
				return err
			}
			if !history {
				return c.emit(o, func(w io.Writer) { printOrder(w, o) })
			}
			evs, err := c.api.OrderEvents(ctx, &client.OrderEventsRequestOptions{PathParams: &client.OrderEventsPath{ID: o.ID}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(evs, func(w io.Writer) {
				var rows [][]string
				for _, e := range evs.Events {
					change := deref(e.To)
					if f := deref(e.From); f != "" {
						change = f + " → " + change
					}
					rows = append(rows, []string{hhmm(e.At), e.Kind, deref(e.Course), change, deref(e.Detail), e.ByName})
				}
				table(w, "AT\tWHAT\tCOURSE\tCHANGE\tDETAIL\tBY", rows)
			})
		},
	}
	show.Flags().BoolVar(&history, "history", false, "show every change, who made it and when")

	courseCmd := func(use, short string, run func(ctx context.Context, id, course string) (*client.Order, error)) *cobra.Command {
		return &cobra.Command{
			Use: use + " ORDER COURSE", Short: short, Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				o, err := c.findOrder(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				res, err := run(cmd.Context(), o.ID, args[1])
				if err != nil {
					return apiErr(err)
				}
				return c.emit(res, func(w io.Writer) { printOrder(w, *res) })
			},
		}
	}
	fireCmd := courseCmd("fire", "Fire a course", func(ctx context.Context, id, course string) (*client.Order, error) {
		return c.api.FireCourse(ctx, &client.FireCourseRequestOptions{PathParams: &client.FireCoursePath{ID: id, Course: client.FireCoursePathCourse(course)}})
	})
	readyCmd := courseCmd("ready", "Mark a fired course ready at the pass, once every item has been started", func(ctx context.Context, id, course string) (*client.Order, error) {
		return c.api.ReadyCourse(ctx, &client.ReadyCourseRequestOptions{PathParams: &client.ReadyCoursePath{ID: id, Course: client.ReadyCoursePathCourse(course)}})
	})
	serveCmd := courseCmd("serve", "Serve a course once every item is ready", func(ctx context.Context, id, course string) (*client.Order, error) {
		return c.api.ServeCourse(ctx, &client.ServeCourseRequestOptions{PathParams: &client.ServeCoursePath{ID: id, Course: client.ServeCoursePathCourse(course)}})
	})

	var reason string
	target := &cobra.Command{
		Use: "target ORDER COURSE HH:MM", Short: "Change when a fired course should reach the pass (logged)", Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			o, err := c.findOrder(ctx, args[0])
			if err != nil {
				return err
			}
			at, err := parseClock(args[2], time.Now())
			if err != nil {
				return err
			}
			body := client.TargetBody{TargetAt: *at}
			if reason != "" {
				body.Reason = &reason
			}
			res, err := c.api.SetCourseTarget(ctx, &client.SetCourseTargetRequestOptions{
				PathParams: &client.SetCourseTargetPath{ID: o.ID, Course: client.SetCourseTargetPathCourse(args[1])}, Body: &body,
			})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { printOrder(w, *res) })
		},
	}
	target.Flags().StringVar(&reason, "reason", "", "why, for the log")

	var holdReason string
	hold := &cobra.Command{
		Use: "hold ORDER COURSE", Short: "Hold a fired course again (un-fire it), while nothing in it has started", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			o, err := c.findOrder(ctx, args[0])
			if err != nil {
				return err
			}
			body := client.HoldBody{}
			if holdReason != "" {
				body.Reason = &holdReason
			}
			res, err := c.api.HoldCourse(ctx, &client.HoldCourseRequestOptions{
				PathParams: &client.HoldCoursePath{ID: o.ID, Course: client.HoldCoursePathCourse(args[1])}, Body: &body,
			})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { printOrder(w, *res) })
		},
	}
	hold.Flags().StringVar(&holdReason, "reason", "", "why, for the log")

	var off bool
	urgent := &cobra.Command{
		Use: "urgent ORDER COURSE", Short: "Flag a course urgent so it goes first (--off to clear)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			o, err := c.findOrder(ctx, args[0])
			if err != nil {
				return err
			}
			res, err := c.api.SetCourseUrgent(ctx, &client.SetCourseUrgentRequestOptions{
				PathParams: &client.SetCourseUrgentPath{ID: o.ID, Course: client.SetCourseUrgentPathCourse(args[1])},
				Body:       &client.UrgentBody{Urgent: !off},
			})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { printOrder(w, *res) })
		},
	}
	urgent.Flags().BoolVar(&off, "off", false, "clear the flag")

	closeCmd := &cobra.Command{
		Use: "close ORDER", Short: "Close an order once everything is served or voided", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := c.findOrder(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			res, err := c.api.CloseOrder(cmd.Context(), &client.CloseOrderRequestOptions{PathParams: &client.CloseOrderPath{ID: o.ID}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { fmt.Fprintf(w, "Closed %s\n", orderLabel(*res)) })
		},
	}
	rename := &cobra.Command{
		Use: "name ORDER NAME", Short: `Name an order, such as a guest's name ("" clears it)`, Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := c.findOrder(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			res, err := c.api.RenameOrder(cmd.Context(), &client.RenameOrderRequestOptions{
				PathParams: &client.RenameOrderPath{ID: o.ID}, Body: &client.NameBody{Name: args[1]},
			})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { fmt.Fprintf(w, "%s\n", orderLabel(*res)) })
		},
	}
	cmd.AddCommand(newCmd, addCmd, list, show, fireCmd, hold, readyCmd, serveCmd, target, urgent, rename, closeCmd)
	return cmd
}

// orderByRef finds an open order by reference, or any order by ID.
func (c *cli) orderByRef(ctx context.Context, ref string) (client.Order, error) {
	o, err := c.findOrder(ctx, ref)
	if err == nil {
		return o, nil
	}
	got, gerr := c.api.GetOrder(ctx, &client.GetOrderRequestOptions{PathParams: &client.GetOrderPath{ID: ref}})
	if gerr != nil {
		return client.Order{}, err
	}
	return *got, nil
}

// --- Pass, stations and items ---

func statusText(s string) string {
	switch s {
	case "on_track":
		return "on track"
	case "at_risk":
		return "AT RISK"
	case "late":
		return "LATE"
	case "ready":
		return "ready"
	}
	return s
}

func (c *cli) passCmd() *cobra.Command {
	var why bool
	cmd := &cobra.Command{
		Use: "pass", Short: "Show every fired course: due, expected and on track, at risk or late", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			k, err := c.api.GetKitchen(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(k, func(w io.Writer) {
				fmt.Fprintf(w, "Plan at %s, strategy %s, settings %s\n", hhmm(k.Plan.At), k.Plan.Strategy, k.Plan.SettingsVersion)
				var rows [][]string
				for _, cs := range k.Plan.Courses {
					row := []string{cs.Label, cs.Course, hhmm(cs.Target), hhmm(cs.PlannedReady), statusText(cs.Status)}
					if why {
						row = append(row, cs.Reason)
					}
					rows = append(rows, row)
				}
				header := "ORDER\tCOURSE\tDUE\tEXPECTED\tSTATUS"
				if why {
					header += "\tWHY"
				}
				table(w, header, rows)
				var held []string
				for _, o := range k.Orders {
					for _, co := range o.Courses {
						if co.State == "held" {
							held = append(held, orderLabel(o)+" "+co.Course)
						}
					}
				}
				if len(held) > 0 {
					fmt.Fprintln(w, "Held:", strings.Join(held, ", "))
				}
			})
		},
	}
	cmd.Flags().BoolVar(&why, "why", false, "show the reason for each status")
	return cmd
}

func (c *cli) stationCmd() *cobra.Command {
	return &cobra.Command{
		Use: "station STATION", Short: "Show what a station should start next, and why", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			k, err := c.api.GetKitchen(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			var sp *client.StationPlan
			for i := range k.Plan.Stations {
				s := &k.Plan.Stations[i]
				if s.StationID == args[0] || strings.EqualFold(s.Name, args[0]) {
					sp = s
				}
			}
			if sp == nil {
				return fmt.Errorf("no station called %q", args[0])
			}
			return c.emit(sp, func(w io.Writer) {
				fmt.Fprintln(w, sp.Name)
				if len(sp.Actions) == 0 {
					fmt.Fprintln(w, "Nothing to start.")
				}
				for _, a := range sp.Actions {
					fmt.Fprintf(w, "- %s\n  Why: %s\n  Items: %s\n", a.Text, a.Reason, strings.Join(a.ItemIds, " "))
				}
				if len(sp.InProgress) > 0 {
					fmt.Fprintln(w, "In progress:")
					var rows [][]string
					for _, p := range sp.InProgress {
						due := hhmm(p.PhaseEndsAt)
						if deref(p.Overdue) {
							due += " (overdue)"
						}
						rows = append(rows, []string{"  " + p.DishName, p.Label, p.State, due, p.ItemID})
					}
					table(w, "  ITEM\tORDER\tSTATE\tDONE AT\tID", rows)
				}
			})
		},
	}
}

func (c *cli) itemsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "items", Short: "Move items through the kitchen"}
	var to, reason, from string
	move := func(use, short string, state *string) *cobra.Command {
		return &cobra.Command{
			Use: use + " ITEM...", Short: short, Args: cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				body := client.ItemMove{ItemIds: args}
				if state != nil && *state != "" {
					body.To = state
				}
				if reason != "" {
					body.Reason = &reason
				}
				if from != "" {
					body.From = &from
				}
				res, err := c.api.MoveItems(cmd.Context(), &client.MoveItemsRequestOptions{Body: &body})
				if err != nil {
					return apiErr(err)
				}
				return c.emit(res, func(w io.Writer) {
					for _, o := range res.Orders {
						for _, it := range o.Items {
							for _, id := range args {
								if id == it.ID {
									fmt.Fprintf(w, "%s %s: %s\n", orderLabel(o), it.DishName, it.State)
								}
							}
						}
					}
				})
			},
		}
	}
	next := move("next", "Move each item one step forward (start, rested, plated, ready, served)", nil)
	set := move("move", "Move items to a state (--to cooking|resting|plating|ready|served|voided, or back one step)", &to)
	set.Flags().StringVar(&to, "to", "", "the state to move to")
	set.Flags().StringVar(&reason, "reason", "", "why (required to void)")
	for _, sub := range []*cobra.Command{next, set} {
		sub.Flags().StringVar(&from, "from", "", "the state you saw the items in; nothing moves if any has already changed")
	}
	var with, why string
	replace := &cobra.Command{
		Use: "replace ITEM --with \"Burger: well done\"", Short: "Replace an item with the right one (void it and add the new one in one step)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if with == "" {
				return fmt.Errorf("say what to replace it with: --with \"Burger: well done\"")
			}
			m, err := c.menu(ctx)
			if err != nil {
				return err
			}
			ni, err := parseItem(m, with)
			if err != nil {
				return err
			}
			body := client.ItemReplace{Item: ni}
			if why != "" {
				body.Reason = &why
			}
			res, err := c.api.ReplaceItem(ctx, &client.ReplaceItemRequestOptions{PathParams: &client.ReplaceItemPath{ID: args[0]}, Body: &body})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { printOrder(w, *res) })
		},
	}
	replace.Flags().StringVar(&with, "with", "", "the right item, such as \"Burger: well done\"")
	replace.Flags().StringVar(&why, "reason", "", "why, for the log")
	cmd.AddCommand(next, set, replace)
	return cmd
}

func (c *cli) learnedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "learned", Short: "Each dish's cook time as set and as learned from past cooks", Args: cobra.NoArgs,
		Long: "learned shows each dish's cook time as set on the menu and as learned from its cooks in the last 90 days:\n" +
			"the time from started to ready, less rest, plate and modifiers, smoothed so recent cooks count most.\n" +
			"New orders use a learned time once the dish has enough cooks and the scheduler setting learned_times is on.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.ListLearnedTimes(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				if res.On {
					fmt.Fprintf(w, "New orders use learned cook times for dishes with %d cooks or more.\n\n", res.MinCooks)
				} else {
					fmt.Fprintf(w, "New orders use the cook times set on the menu. Turn on learned_times in the scheduler settings to use learned ones.\n\n")
				}
				var rows [][]string
				for _, d := range res.Dishes {
					rows = append(rows, []string{d.DishName, secs(d.SetSeconds), learnedText(d, res.MinCooks), fmt.Sprint(d.Cooks), usedText(d)})
				}
				table(w, "DISH\tSET\tLEARNED\tCOOKS\tNEW ORDERS USE", rows)
			})
		},
	}
	reset := &cobra.Command{
		Use: "reset DISH", Short: "Start a dish's learned cook time again from its next cook (manager)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			d, err := findDish(m, args[0])
			if err != nil {
				return err
			}
			res, err := c.api.ResetLearnedTime(cmd.Context(), &client.ResetLearnedTimeRequestOptions{PathParams: &client.ResetLearnedTimePath{ID: d.ID}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "%s learns its cook time again from its next cook. Until it has enough cooks, new orders use the set time, %s.\n", res.DishName, secs(res.SetSeconds))
			})
		},
	}
	cmd.AddCommand(reset)
	return cmd
}

func learnedText(d client.LearnedDish, minCooks int64) string {
	switch {
	case d.Cooks == 0:
		return "no cooks yet"
	case !d.Ready:
		return fmt.Sprintf("%s (needs %d cooks)", secs(d.LearnedSeconds), minCooks)
	default:
		return secs(d.LearnedSeconds)
	}
}

func usedText(d client.LearnedDish) string {
	if d.InUse {
		return "learned"
	}
	return "set"
}

func (c *cli) allergensCmd() *cobra.Command {
	return &cobra.Command{
		Use: "allergens", Short: "The allergen chart: every dish and the 14 allergens it contains", Args: cobra.NoArgs,
		Long: "allergens lists each dish on the menu with the regulated allergens it contains (Regulation (EU) No 1169/2011,\n" +
			"Annex II). A dish marked NOT CHECKED has not had its allergens checked by a manager and may contain any of them.\n" +
			"Set a dish's allergens with: kishtin menu dish update DISH --allergens gluten,milk (or --no-allergens).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.GetAllergenChart(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				names := map[string]string{}
				for _, a := range res.Allergens {
					names[a.Code] = a.Name
				}
				var rows [][]string
				for _, d := range res.Dishes {
					contains := "none"
					if len(d.Allergens) > 0 {
						var ns []string
						for _, a := range d.Allergens {
							ns = append(ns, names[a])
						}
						contains = strings.Join(ns, ", ")
					}
					if !d.Checked {
						contains = "NOT CHECKED (listed: " + contains + ")"
					}
					rows = append(rows, []string{d.DishName, d.Course, contains})
				}
				table(w, "DISH\tCOURSE\tCONTAINS", rows)
				if res.Unchecked > 0 {
					fmt.Fprintf(w, "\n%d dishes have not had their allergens checked. Do not tell customers they are allergen-free.\n", res.Unchecked)
				}
				fmt.Fprintf(w, "\nAllergen codes: %s\n", strings.Join(func() []string {
					out := make([]string, len(res.Allergens))
					for i, a := range res.Allergens {
						out[i] = a.Code
					}
					return out
				}(), ", "))
			})
		},
	}
}

func (c *cli) collectionCmd() *cobra.Command {
	return &cobra.Command{
		Use: "collection", Short: "Takeaways being prepared and ready to collect, as the collection screen shows them", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.GetCollection(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				list := func(os []client.CollectionOrder) string {
					if len(os) == 0 {
						return "none"
					}
					var out []string
					for _, o := range os {
						s := fmt.Sprintf("#%d", o.Number)
						if o.Name != nil && *o.Name != "" {
							s += " (" + *o.Name + ")"
						}
						out = append(out, s)
					}
					return strings.Join(out, ", ")
				}
				fmt.Fprintf(w, "Ready to collect: %s\nPreparing: %s\n", list(res.Ready), list(res.Preparing))
			})
		},
	}
}
