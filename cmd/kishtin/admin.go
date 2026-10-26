package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/doordash-oss/oapi-codegen-dd/v3/pkg/runtime"
	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
)

func num(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', 1, 64) + "°C"
}

// shortDur writes 2m0s as 2m and 2h30m0s as 2h30m.
func shortDur(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

func limitLabel(l client.LimitVersion) string {
	scope := l.Scope.Kind
	if ref := deref(l.Scope.Ref); ref != "" {
		scope += " " + ref
	}
	var parts []string
	if l.MaxC != nil {
		parts = append(parts, "max "+num(l.MaxC))
	}
	if l.BorderlineMaxC != nil {
		parts = append(parts, "borderline to "+num(l.BorderlineMaxC))
	}
	if l.MinC != nil {
		parts = append(parts, "min "+num(l.MinC))
	}
	for _, c := range l.Combos {
		if c.HoldSeconds == 0 {
			parts = append(parts, fmt.Sprintf("core %.1f°C", c.MinC))
		} else {
			parts = append(parts, fmt.Sprintf("core %.1f°C for %s", c.MinC, shortDur(time.Duration(c.HoldSeconds)*time.Second)))
		}
	}
	if c := l.Cooling; c != nil {
		parts = append(parts, fmt.Sprintf("into fridge within %s, else serve or discard if above %.1f°C at %s",
			shortDur(time.Duration(c.IntoFridgeWithinSeconds)*time.Second), c.CutoffMaxC, shortDur(time.Duration(c.CutoffAfterSeconds)*time.Second)))
	}
	if deref(l.NonFood) {
		parts = append(parts, "non-food")
	}
	return fmt.Sprintf("%s (%s): %s [%s]", l.Check, scope, strings.Join(parts, " or "), l.ID)
}

func (c *cli) limitsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "limits", Short: "Show and change critical limits"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the limits in force (--history for every version)",
		Args:  cobra.NoArgs,
	}
	var history bool
	list.Flags().BoolVar(&history, "history", false, "show every version, oldest first")
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		res, err := c.api.ListLimits(cmd.Context())
		if err != nil {
			return apiErr(err)
		}
		return c.emit(res, func(w io.Writer) {
			show := res.Current
			if history {
				show = res.History
			}
			for _, l := range show {
				fmt.Fprintln(w, limitLabel(l))
				if history {
					fmt.Fprintf(w, "    %s by %s: %s", local(l.CreatedAt), l.CreatedBy, deref(l.Reason))
					if p := deref(l.PlanRef); p != "" {
						fmt.Fprintf(w, " (plan: %s)", p)
					}
					if s := deref(l.Supersedes); s != "" {
						fmt.Fprintf(w, " replaces %s", s)
					}
					fmt.Fprintln(w)
				}
			}
		})
	}

	cmd.AddCommand(list, c.setLimitCmd())
	return cmd
}

func (c *cli) setLimitCmd() *cobra.Command {
	var check, scope, reason, planRef string
	var maxC, borderline, minC, cutoffMax float64
	var combos []string
	var within, cutoffAfter time.Duration
	var nonFood, yes bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Preview and save a new limit version",
		Long: "set shows how recent records would have scored under the new limit, then asks before saving.\n" +
			"Saved records are never re-graded. A limit below your reference guidance (see kishtin settings)\n" +
			"needs --plan-ref.\n\n" +
			"Examples:\n" +
			"  kishtin limits set --check fridge --scope \"unit:Fish fridge\" --max 2 --reason \"Fish kept at 0–2°C\"\n" +
			"  kishtin limits set --check cook --scope category:poultry --combo 75 --combo 70:2m --reason \"Plan 4.1\"",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			nl := client.NewLimit{Check: check, Reason: reason}
			kind, ref, _ := strings.Cut(scope, ":")
			nl.Scope = client.Scope{Kind: kind}
			if ref != "" {
				nl.Scope.Ref = &ref
			}
			if planRef != "" {
				nl.PlanRef = &planRef
			}
			if nonFood {
				nl.NonFood = runtime.Ptr(true)
			}
			fl := cmd.Flags()
			if fl.Changed("max") {
				nl.MaxC = runtime.Ptr(maxC)
			}
			if fl.Changed("borderline") {
				nl.BorderlineMaxC = runtime.Ptr(borderline)
			}
			if fl.Changed("min") {
				nl.MinC = runtime.Ptr(minC)
			}
			for _, s := range combos {
				tt, err := parseCombo(s)
				if err != nil {
					return err
				}
				nl.Combos = append(nl.Combos, tt)
			}
			if fl.Changed("into-fridge-within") || fl.Changed("cutoff-after") || fl.Changed("cutoff-max") {
				nl.Cooling = &client.CoolingLimit{
					IntoFridgeWithinSeconds: int64(within / time.Second),
					CutoffAfterSeconds:      int64(cutoffAfter / time.Second),
					CutoffMaxC:              cutoffMax,
				}
			}

			pv, err := c.api.PreviewLimit(cmd.Context(), &client.PreviewLimitRequestOptions{Body: &nl})
			if err != nil {
				return apiErr(err)
			}
			if c.asJSON && !yes {
				return c.emit(pv, nil)
			}
			w := c.errOut
			fmt.Fprintf(w, "New limit: %s\n", limitLabel(pv.Limit))
			if pv.Replaces != nil {
				fmt.Fprintf(w, "Replaces:  %s\n", limitLabel(*pv.Replaces))
			}
			for _, b := range pv.BelowGuidance {
				fmt.Fprintf(w, "WARNING: below your reference guidance (%s): %s\n", pv.Guidance, b)
			}
			if len(pv.BelowGuidance) == 0 {
				for _, b := range pv.BelowFsai {
					fmt.Fprintf(w, "Note: below the FSAI defaults, but allowed by %s: %s\n", pv.Guidance, b)
				}
			}
			fmt.Fprintf(w, "Recent records this limit applies to: %d. Would score differently: %d.\n", len(pv.Recent), pv.Changed)
			for _, r := range pv.Recent {
				if r.Changed {
					fmt.Fprintf(w, "  %s  %s: saved %s, would be %s (%s)\n", local(r.LoggedAt), r.Subject, outcomeText(r.Saved), outcomeText(r.Proposed), r.ProposedReason)
				}
			}
			fmt.Fprintln(w, "Saved records keep their result.")
			if p := deref(pv.Problem); p != "" {
				return fmt.Errorf("cannot save this limit: %s", p)
			}
			if !yes {
				ok, err := confirm(c.in, w, "Save this limit?")
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(w, "Not saved.")
					return nil
				}
			}
			lv, err := c.api.CreateLimit(cmd.Context(), &client.CreateLimitRequestOptions{Body: &nl})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(lv, func(out io.Writer) { fmt.Fprintf(out, "Saved %s\n", limitLabel(*lv)) })
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&check, "check", "", "fridge, freezer, cook, reheat, hot_hold, cooling, delivery_chilled or delivery_frozen")
	fl.StringVar(&scope, "scope", "default", "default, unit:NAME, category:NAME or dish:NAME")
	fl.Float64Var(&maxC, "max", 0, "highest passing °C (fridge, freezer, deliveries)")
	fl.Float64Var(&borderline, "borderline", 0, "highest borderline °C (deliveries)")
	fl.Float64Var(&minC, "min", 0, "lowest passing °C (hot holding)")
	fl.StringSliceVar(&combos, "combo", nil, "cooking: core °C, optionally held for a time, such as 75 or 70:2m (repeat for alternatives)")
	fl.DurationVar(&within, "into-fridge-within", 0, "cooling: time allowed to get into the fridge, such as 2h")
	fl.DurationVar(&cutoffAfter, "cutoff-after", 0, "cooling: serve-or-discard check time, such as 2h30m")
	fl.Float64Var(&cutoffMax, "cutoff-max", 0, "cooling: highest °C allowed at the cutoff")
	fl.BoolVar(&nonFood, "non-food", false, "unit holds no food, such as a drinks fridge")
	fl.StringVar(&reason, "reason", "", "why the limit is changing (required)")
	fl.StringVar(&planRef, "plan-ref", "", "HACCP plan section or validation (required below your reference guidance)")
	fl.BoolVar(&yes, "yes", false, "save without asking")
	return cmd
}

func parseCombo(s string) (client.TimeTemp, error) {
	tempS, holdS, _ := strings.Cut(s, ":")
	t, err := strconv.ParseFloat(tempS, 64)
	if err != nil {
		return client.TimeTemp{}, fmt.Errorf("--combo %q: the temperature is not a number", s)
	}
	var hold time.Duration
	if holdS != "" {
		if hold, err = time.ParseDuration(holdS); err != nil {
			return client.TimeTemp{}, fmt.Errorf("--combo %q: the hold time should look like 2m or 90s", s)
		}
	}
	return client.TimeTemp{MinC: t, HoldSeconds: int64(hold / time.Second)}, nil
}

func confirm(in io.Reader, w io.Writer, question string) (bool, error) {
	fmt.Fprintf(w, "%s [y/N] ", question)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

func (c *cli) staffCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "staff", Short: "Manage staff (manager only)"}

	list := &cobra.Command{
		Use: "list", Short: "List staff", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.ListUsers(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, u := range res.Users {
					status := "active"
					if !u.Active {
						status = "deactivated"
					}
					rows = append(rows, []string{u.Name, u.Role, status, u.ID})
				}
				table(w, "NAME\tROLE\tSTATUS\tID", rows)
			})
		},
	}

	var name, role, pin, password string
	add := &cobra.Command{
		Use: "add", Short: "Add a staff member", Args: cobra.NoArgs,
		Long: "add creates a staff member. Kitchen staff need a --pin; managers also need a --password.\n" +
			"Roles: head_chef, line_cook, front_of_house, manager.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			nu := client.NewUser{Name: name, Role: role}
			if pin != "" {
				nu.Pin = &pin
			}
			if password != "" {
				nu.Password = &password
			}
			u, err := c.api.CreateUser(cmd.Context(), &client.CreateUserRequestOptions{Body: &nu})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(u, func(w io.Writer) { fmt.Fprintf(w, "Added %s (%s), ID %s\n", u.Name, u.Role, u.ID) })
		},
	}
	add.Flags().StringVar(&name, "name", "", "name")
	add.Flags().StringVar(&role, "role", "line_cook", "role")
	add.Flags().StringVar(&pin, "pin", "", "4–8 digit PIN")
	add.Flags().StringVar(&password, "password", "", "password, at least 10 characters (managers)")

	var newName, newRole, newPIN, newPassword string
	var deactivate, activate bool
	update := &cobra.Command{
		Use: "update ID", Short: "Change a staff member", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ch := client.UserChange{}
			fl := cmd.Flags()
			if fl.Changed("name") {
				ch.Name = &newName
			}
			if fl.Changed("role") {
				ch.Role = &newRole
			}
			if fl.Changed("pin") {
				ch.Pin = &newPIN
			}
			if fl.Changed("password") {
				ch.Password = &newPassword
			}
			switch {
			case activate && deactivate:
				return errors.New("use --activate or --deactivate, not both")
			case activate:
				ch.Active = runtime.Ptr(true)
			case deactivate:
				ch.Active = runtime.Ptr(false)
			}
			u, err := c.api.UpdateUser(cmd.Context(), &client.UpdateUserRequestOptions{
				PathParams: &client.UpdateUserPath{ID: args[0]}, Body: &ch,
			})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(u, func(w io.Writer) { fmt.Fprintf(w, "Updated %s (%s, active: %t)\n", u.Name, u.Role, u.Active) })
		},
	}
	update.Flags().StringVar(&newName, "name", "", "new name")
	update.Flags().StringVar(&newRole, "role", "", "new role")
	update.Flags().StringVar(&newPIN, "pin", "", "new PIN")
	update.Flags().StringVar(&newPassword, "password", "", "new password")
	update.Flags().BoolVar(&deactivate, "deactivate", false, "deactivate: they can no longer log in")
	update.Flags().BoolVar(&activate, "activate", false, "reactivate")

	cmd.AddCommand(list, add, update, c.anonymiseCmd())
	return cmd
}

func (c *cli) keysCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "keys", Short: "Manage API keys (manager only)"}

	list := &cobra.Command{
		Use: "list", Short: "List API keys", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.ListKeys(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, k := range res.Keys {
					status := "active"
					if k.RevokedAt != nil {
						status = "revoked " + local(*k.RevokedAt)
					}
					rows = append(rows, []string{k.Name, k.Kind, status, local(k.CreatedAt), k.ID})
				}
				table(w, "NAME\tKIND\tSTATUS\tCREATED\tID", rows)
			})
		},
	}

	var kind, name, userID string
	create := &cobra.Command{
		Use: "create", Short: "Create a device or user API key", Args: cobra.NoArgs,
		Long: "A device key lets a shared kitchen tablet show the staff login. A user key acts as one\n" +
			"person, for the CLI and scripts. The secret is shown once.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := client.NewKeyInBody{Kind: client.NewKeyInBodyKind(kind), Name: name}
			if userID != "" {
				body.UserID = &userID
			}
			res, err := c.api.CreateKey(cmd.Context(), &client.CreateKeyRequestOptions{Body: &body})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "Created %s key %q. Secret (shown once, store it safely):\n\n  %s\n", res.Key.Kind, res.Key.Name, res.Secret)
				if res.Key.Kind == "device" {
					fmt.Fprintf(w, "\nOpen this link once on the device to pair it:\n\n  %s/#pair=%s\n", strings.TrimRight(c.url, "/"), res.Secret)
				}
			})
		},
	}
	create.Flags().StringVar(&kind, "kind", "device", "device or user")
	create.Flags().StringVar(&name, "name", "", "name, such as \"Pass tablet\"")
	create.Flags().StringVar(&userID, "user", "", "user key: the staff ID it acts as (default you)")

	revoke := &cobra.Command{
		Use: "revoke ID", Short: "Revoke an API key", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := c.api.RevokeKey(cmd.Context(), &client.RevokeKeyRequestOptions{PathParams: &client.RevokeKeyPath{ID: args[0]}}); err != nil {
				return apiErr(err)
			}
			return c.emit(map[string]string{"revoked": args[0]}, func(w io.Writer) { fmt.Fprintf(w, "Revoked %s\n", args[0]) })
		},
	}

	var joinName string
	var invert bool
	join := &cobra.Command{
		Use: "join", Short: "Make a QR code that pairs a tablet or phone", Args: cobra.NoArgs,
		Long: "join makes a one-time code, good for 15 minutes, and prints a QR code for it. Scan it with\n" +
			"the tablet's camera, or open the link: the tablet installs Kishtin's certificate if it needs to,\n" +
			"then pairs itself. Needs a manager.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.CreateJoinCode(cmd.Context(), &client.CreateJoinCodeRequestOptions{Body: &client.JoinCodeInBody{Name: joinName}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "Scan this with %q, or open the link on it, by %s:\n\n", res.Name, res.ExpiresAt.In(clock.Dublin).Format("15:04"))
				printQR(w, res.Links[0].Qr, invert)
				fmt.Fprintf(w, "\n  %s\n", res.Links[0].URL)
				for _, l := range res.Links[1:] {
					fmt.Fprintf(w, "  %s  (another network this computer is on)\n", l.URL)
				}
				fmt.Fprintf(w, "\nCode: %s. It works once.\n", res.Code)
				if res.Fingerprint != nil {
					fmt.Fprintf(w, "Kishtin's certificate fingerprint (SHA-256), as the tablet shows it:\n  %s\n", *res.Fingerprint)
				}
			})
		},
	}
	join.Flags().StringVar(&joinName, "name", "", "name for the tablet or phone, such as \"Pass tablet\"")
	join.Flags().BoolVar(&invert, "invert", false, "draw the QR code for a terminal with a light background")

	cmd.AddCommand(list, create, revoke, join)
	return cmd
}

func (c *cli) backupCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "backup", Short: "Back up the database (manager only)"}
	now := &cobra.Command{
		Use: "now", Short: "Back up now and verify the copy", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := c.api.CreateBackup(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(b, func(w io.Writer) {
				fmt.Fprintf(w, "Backup written: %s (%d bytes)\n", b.Path, b.Size)
				fmt.Fprintf(w, "SHA-256: %s\n", b.Sha256)
				if b.Verified {
					fmt.Fprintf(w, "Verified: %d HACCP records, chain head %s\n", b.Records, b.HeadHash)
				}
				fmt.Fprintln(w, "This copy is on the server machine. Copy it somewhere else to protect against losing that machine.")
			})
		},
	}
	list := &cobra.Command{
		Use: "list", Short: "List backups", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.ListBackups(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, b := range res.Backups {
					rows = append(rows, []string{local(b.TakenAt), strconv.FormatInt(b.Size, 10), b.Name, b.Sha256})
				}
				table(w, "TAKEN\tBYTES\tFILE\tSHA-256", rows)
			})
		},
	}
	cmd.AddCommand(now, list)
	return cmd
}

// printQR draws a QR code with half-height blocks, two rows to a line, in a
// four-module quiet zone. By default light modules are drawn, which scans on
// a terminal with a dark background; invert draws the dark ones instead.
func printQR(w io.Writer, rows []string, invert bool) {
	const quiet = 4
	size := len(rows)
	dark := func(x, y int) bool {
		if x < 0 || y < 0 || x >= size || y >= size {
			return false
		}
		return rows[y][x] == '1'
	}
	for y := -quiet; y < size+quiet; y += 2 {
		var b strings.Builder
		b.WriteString("  ")
		for x := -quiet; x < size+quiet; x++ {
			top, bottom := dark(x, y) == invert, dark(x, y+1) == invert
			switch {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		fmt.Fprintln(w, b.String())
	}
}
