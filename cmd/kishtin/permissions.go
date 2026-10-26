package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

var roleNames = map[string]string{"head_chef": "head chefs", "line_cook": "line cooks", "front_of_house": "front of house", "manager": "managers"}

func (c *cli) permissionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permissions",
		Short: "Show who may do what, or change it",
		Long: "permissions lists every action, who may take it, and whether a PIN is enough. Managers may\n" +
			"always do everything. Staff, API keys, permissions and fitness to work are always managers\n" +
			"with a password.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			actions, err := c.api.ListActions(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			cur, err := c.api.GetPermissions(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			staff, err := c.api.ListStaff(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			names := map[string]string{}
			for _, s := range staff.Staff {
				names[s.ID] = s.Name
			}
			return c.emit(cur, func(w io.Writer) {
				var rows [][]string
				for _, a := range *actions {
					roles, users, pin, changed := a.Roles, []string(nil), a.Pin, ""
					for _, g := range cur.Value.Grants {
						if g.Action == a.Action {
							roles, users, pin, changed = g.Roles, g.Users, deref(g.Pin), "changed"
						}
					}
					if deref(a.Fixed) {
						changed = "fixed"
					}
					who := []string{"managers"}
					for _, r := range roles {
						if r != "manager" {
							who = append(who, roleNames[r])
						}
					}
					for _, u := range users {
						n := names[u]
						if n == "" {
							n = u + " (inactive)"
						}
						who = append(who, n)
					}
					login := "password"
					if pin {
						login = "PIN"
					}
					rows = append(rows, []string{a.Action, a.Label, strings.Join(who, ", "), login, changed})
				}
				table(w, "ACTION\tWHAT\tWHO\tLOGIN\t", rows)
				fmt.Fprintf(w, "\nVersion %s. Change one with: kishtin permissions set ACTION --role head_chef --staff \"Name\" --pin --reason \"...\"\n", cur.Version.ID)
			})
		},
	}
	cmd.AddCommand(c.permissionsSetCmd())
	return cmd
}

func (c *cli) permissionsSetCmd() *cobra.Command {
	var roles, staff []string
	var pin, password, reset, yes bool
	var reason string
	cmd := &cobra.Command{
		Use:   "set ACTION",
		Short: "Change who may take one action",
		Long: "set replaces who may take an action: the roles in --role and the staff named in --staff.\n" +
			"Managers always may. --pin lets a PIN login do it; --password needs a password login.\n" +
			"--reset goes back to Kishtin's default. Shows the change and asks before saving. Needs a\n" +
			"manager password (a manager's API key).",
		Example: "  kishtin permissions set haccp.sign_off --role head_chef --pin --reason \"Head chef signs off weekly\"\n" +
			"  kishtin permissions set haccp.sign_off --staff \"Aoife Chef\" --reason \"Aoife covers sign-off\"\n" +
			"  kishtin permissions set haccp.sign_off --reset --reason \"Back to managers only\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if pin && password {
				return fmt.Errorf("choose --pin or --password, not both")
			}
			actions, err := c.api.ListActions(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			i := slices.IndexFunc(*actions, func(a client.ActionInfo) bool { return a.Action == args[0] })
			if i < 0 {
				var known []string
				for _, a := range *actions {
					known = append(known, a.Action)
				}
				return fmt.Errorf("unknown action %q: use one of %s", args[0], strings.Join(known, ", "))
			}
			info := (*actions)[i]
			cur, err := c.api.GetPermissions(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			grants := slices.DeleteFunc(slices.Clone(cur.Value.Grants), func(g client.Grant) bool { return g.Action == info.Action })
			if !reset {
				g := client.Grant{Action: info.Action, Roles: info.Roles, Pin: &info.Pin}
				for _, old := range cur.Value.Grants {
					if old.Action == info.Action {
						g = old
					}
				}
				if cmd.Flags().Changed("role") {
					for _, r := range roles {
						if _, ok := roleNames[r]; !ok {
							return fmt.Errorf("unknown role %q: use head_chef, line_cook or front_of_house", r)
						}
					}
					g.Roles = roles
				}
				if cmd.Flags().Changed("staff") {
					list, err := c.api.ListStaff(cmd.Context())
					if err != nil {
						return apiErr(err)
					}
					g.Users = nil
					for _, name := range staff {
						j := slices.IndexFunc(list.Staff, func(s client.StaffEntry) bool { return strings.EqualFold(s.Name, name) })
						if j < 0 {
							return fmt.Errorf("no active member of staff is called %q", name)
						}
						g.Users = append(g.Users, list.Staff[j].ID)
					}
				}
				if pin || password {
					g.Pin = &pin
				}
				grants = append(grants, g)
			}
			raw, err := json.Marshal(client.Permissions{Grants: grants})
			if err != nil {
				return err
			}
			return c.saveSettings(cmd.Context(), "permissions", c.settingsGroups()["permissions"], raw, reason, "", yes)
		},
	}
	cmd.Flags().StringSliceVar(&roles, "role", nil, "roles allowed (head_chef, line_cook, front_of_house); repeat or comma-separate; --role= for none")
	cmd.Flags().StringSliceVar(&staff, "staff", nil, "names of staff allowed whatever their role; --staff= for none")
	cmd.Flags().BoolVar(&pin, "pin", false, "a PIN login is enough")
	cmd.Flags().BoolVar(&password, "password", false, "needs a password login")
	cmd.Flags().BoolVar(&reset, "reset", false, "go back to Kishtin's default for this action")
	cmd.Flags().StringVar(&reason, "reason", "", "why (required)")
	cmd.Flags().BoolVar(&yes, "yes", false, "save without asking")
	return cmd
}
