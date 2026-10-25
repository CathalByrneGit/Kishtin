package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

func (c *cli) squareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "square",
		Short: "Orders from Square: status, catalog mapping and sync",
		Long: `Orders from Square. A manager turns import on with
  kishtin settings set square -f square.json --reason "..."
after reading the preview of what is sent to Square.`,
	}
	status := &cobra.Command{
		Use: "status", Short: "Show whether import is on and working, and recent till orders", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.SquareStatus(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				set := res.Settings.Value
				switch {
				case !res.Connected:
					fmt.Fprintln(w, "Square: not connected (kishtin connectors connect square --token-file FILE)")
				case set.Enabled:
					fmt.Fprintf(w, "Square: importing orders for location %s every %ds\n", set.LocationID, set.PollSeconds)
				default:
					fmt.Fprintln(w, "Square: connected, import is off")
				}
				if st := res.State; st != nil {
					if st.LastOkAt != nil {
						fmt.Fprintln(w, "Last worked:", hhmm(*st.LastOkAt))
					}
					if e := deref(st.LastError); e != "" {
						fmt.Fprintln(w, "Last problem:", e)
					}
				}
				var rows [][]string
				for _, o := range res.Recent {
					note := strings.Join(append(append([]string{}, o.Unmapped...), o.Notices...), "; ")
					if len(o.Unmapped) > 0 {
						note = "not mapped: " + note
					}
					rows = append(rows, []string{hhmm(o.LastSeen), o.Label, o.State, fmt.Sprint(o.Version), note})
				}
				if len(rows) > 0 {
					table(w, "SEEN\tORDER\tSTATE\tVERSION\tNEEDS CHECKING", rows)
				}
			})
		},
	}
	catalog := &cobra.Command{
		Use: "catalog", Short: "Read the catalog from Square with each entry's mapping (sends Square the access token)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.SquareCatalog(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			m, err := c.menu(cmd.Context())
			if err != nil {
				return err
			}
			names := map[string]string{}
			for _, d := range m.Dishes {
				names[d.ID] = d.Name
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, e := range res.Entries {
					mapped := "not mapped"
					if mp := e.Mapping; mp != nil {
						switch {
						case deref(mp.Ignore):
							mapped = "not for the kitchen"
						case deref(mp.DishID) != "":
							mapped = "→ " + names[deref(mp.DishID)]
						default:
							mapped = "→ " + deref(mp.Modifier)
						}
					} else if sg := deref(e.Suggested); sg != "" {
						if n, ok := names[sg]; ok {
							sg = n
						}
						mapped += " (suggest: " + sg + ")"
					}
					rows = append(rows, []string{e.Entry.Kind, e.Label, mapped, e.Entry.ID})
				}
				table(w, "KIND\tON THE TILL\tKITCHEN\tID", rows)
			})
		},
	}
	var dish, modifier string
	var ignore bool
	mapCmd := &cobra.Command{
		Use: "map ID --dish NAME | --modifier NAME | --ignore", Short: "Map a till item to a dish, or a till modifier to a dish modifier", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cat, err := c.api.SquareCatalog(ctx)
			if err != nil {
				return apiErr(err)
			}
			var entry *client.CatalogRow
			for i := range cat.Entries {
				if cat.Entries[i].Entry.ID == args[0] {
					entry = &cat.Entries[i]
				}
			}
			if entry == nil {
				return fmt.Errorf("no catalog entry %q on Square: see kishtin square catalog", args[0])
			}
			in := client.MappingInput{ExternalID: entry.Entry.ID, Kind: client.MappingInputKind(entry.Entry.Kind), Name: entry.Label}
			switch {
			case ignore:
				in.Ignore = &ignore
			case dish != "":
				m, err := c.menu(ctx)
				if err != nil {
					return err
				}
				d, err := findDish(m, dish)
				if err != nil {
					return err
				}
				in.DishID = &d.ID
			case modifier != "":
				in.Modifier = &modifier
			default:
				return fmt.Errorf("say what it is: --dish NAME, --modifier NAME, or --ignore")
			}
			res, err := c.api.SaveSquareMappings(ctx, &client.SaveSquareMappingsRequestOptions{Body: &client.MappingsIn{Mappings: []client.MappingInput{in}}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { fmt.Fprintf(w, "Mapped %s\n", entry.Label) })
		},
	}
	mapCmd.Flags().StringVar(&dish, "dish", "", "the dish this till item makes")
	mapCmd.Flags().StringVar(&modifier, "modifier", "", "the dish modifier this till modifier means")
	mapCmd.Flags().BoolVar(&ignore, "ignore", false, "not for the kitchen, such as drinks")
	sync := &cobra.Command{
		Use: "sync", Short: "Ask Square for new and changed orders now", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.SquareSync(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "%d changed orders: %d entered, %d added to, %d already up to date\n", res.Seen, res.Created, res.Updated, res.Skipped)
			})
		},
	}
	locations := &cobra.Command{
		Use: "locations", Short: "List the Square account's locations and their IDs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.SquareLocations(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { printLocations(w, res.Locations) })
		},
	}
	cmd.AddCommand(status, catalog, mapCmd, sync, locations)
	return cmd
}
