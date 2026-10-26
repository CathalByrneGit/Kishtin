package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

// tillCmd is the command for one till: kishtin square or kishtin sumup.
func (c *cli) tillCmd(id, name, short, extra string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   id,
		Short: short,
		Long: fmt.Sprintf(`%s. A manager turns import on with
  kishtin settings set %s -f %s.json --reason "..."
after reading the preview of what is sent to %s.%s`, short, id, id, name, extra),
	}
	status := &cobra.Command{
		Use: "status", Short: "Show whether import is on and working, and recent till orders", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.TillStatus(cmd.Context(), &client.TillStatusRequestOptions{PathParams: &client.TillStatusPath{Till: client.TillStatusPathTill(id)}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				set := res.Settings.Value
				switch {
				case !res.Connected:
					fmt.Fprintf(w, "%s: not connected (kishtin connectors connect %s --token-file FILE)\n", name, id)
				case set.Enabled:
					fmt.Fprintf(w, "%s: importing orders for location %s every %ds\n", name, set.LocationID, set.PollSeconds)
				default:
					fmt.Fprintf(w, "%s: connected, import is off\n", name)
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
		Use: "catalog", Short: fmt.Sprintf("Read the catalog from %s with each entry's mapping (sends %s its token)", name, name), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.TillCatalog(cmd.Context(), &client.TillCatalogRequestOptions{PathParams: &client.TillCatalogPath{Till: client.TillCatalogPathTill(id)}})
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
			cat, err := c.api.TillCatalog(ctx, &client.TillCatalogRequestOptions{PathParams: &client.TillCatalogPath{Till: client.TillCatalogPathTill(id)}})
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
				return fmt.Errorf("no catalog entry %q on %s: see kishtin %s catalog", args[0], name, id)
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
			res, err := c.api.SaveTillMappings(ctx, &client.SaveTillMappingsRequestOptions{
				PathParams: &client.SaveTillMappingsPath{Till: client.SaveTillMappingsPathTill(id)},
				Body:       &client.SaveTillMappingsBody{Mappings: []client.MappingInput{in}},
			})
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
		Use: "sync", Short: fmt.Sprintf("Ask %s for new and changed orders now", name), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.TillSync(cmd.Context(), &client.TillSyncRequestOptions{PathParams: &client.TillSyncPath{Till: client.TillSyncPathTill(id)}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "%d changed orders: %d entered, %d added to, %d already up to date\n", res.Seen, res.Created, res.Updated, res.Skipped)
			})
		},
	}
	locations := &cobra.Command{
		Use: "locations", Short: fmt.Sprintf("List the %s account's locations and their IDs", name), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.TillLocations(cmd.Context(), &client.TillLocationsRequestOptions{PathParams: &client.TillLocationsPath{Till: client.TillLocationsPathTill(id)}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { printLocations(w, res.Locations) })
		},
	}
	cmd.AddCommand(status, catalog, mapCmd, sync, locations, c.tillVersionsCmd(id))
	return cmd
}
