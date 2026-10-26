package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

func optStr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (c *cli) plansCmd() *cobra.Command {
	var from, to string
	var limit int64
	cmd := &cobra.Command{
		Use:   "plans",
		Short: "List the scheduler's plans as they changed",
		Long: "plans lists the snapshots Kishtin keeps each time the scheduler's plan changes: what each\n" +
			"station was told to start, and each course's status. A course at risk or late says why\n" +
			"(equipment_full, started_late, cooking_long, waiting_at_pass, busy). Head chef or manager.",
		Example: "  kishtin plans --from 2026-10-24\n  kishtin plans --from 2026-10-24 --to 2026-10-24 --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := &client.PlanSnapshotsQuery{From: optStr(from), To: optStr(to)}
			if limit > 0 {
				q.Limit = &limit
			}
			res, err := c.api.PlanSnapshots(cmd.Context(), &client.PlanSnapshotsRequestOptions{Query: q})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, s := range res.Snapshots {
					actions := 0
					for _, st := range s.Plan.Stations {
						actions += len(st.Actions)
					}
					count := map[string]int{}
					var causes []string
					for _, co := range s.Plan.Courses {
						count[co.Status]++
						if co.Cause != nil {
							causes = append(causes, co.Label+" "+*co.Cause)
						}
					}
					rows = append(rows, []string{
						local(s.At), s.Trigger, fmt.Sprint(actions),
						fmt.Sprintf("%d on track, %d at risk, %d late, %d ready", count["on_track"], count["at_risk"], count["late"], count["ready"]),
						strings.Join(causes, "; "),
					})
				}
				table(w, "AT\tAFTER\tACTIONS\tCOURSES\tWHY", rows)
				if len(rows) == 0 {
					fmt.Fprintln(w, "No plans kept in this period.")
				}
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start date (2026-10-24) or RFC 3339 time")
	cmd.Flags().StringVar(&to, "to", "", "end date, inclusive")
	cmd.Flags().Int64Var(&limit, "limit", 0, "at most this many")
	return cmd
}

func (c *cli) activityCmd() *cobra.Command {
	var from, to string
	var kinds []string
	var limit int64
	cmd := &cobra.Command{
		Use:   "activity",
		Short: "List the activity log",
		Long: "activity lists what staff and screens did that is not a HACCP record or an order event:\n" +
			"dishes sold out and back on, logins and logouts, screens connecting and dropping off, how\n" +
			"each HACCP record was entered, and leavers anonymised. Head chef or manager.",
		Example: "  kishtin activity --from 2026-10-24 --kind dish.sold_out\n  kishtin activity --kind screen.disconnected --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := &client.ListActivityQuery{From: optStr(from), To: optStr(to), Kind: kinds}
			if limit > 0 {
				q.Limit = &limit
			}
			res, err := c.api.ListActivity(cmd.Context(), &client.ListActivityRequestOptions{Query: q})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, a := range res.Entries {
					keys := make([]string, 0, len(a.Detail))
					for k := range a.Detail {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					var detail []string
					for _, k := range keys {
						detail = append(detail, k+"="+a.Detail[k])
					}
					rows = append(rows, []string{local(a.At), a.Kind, deref(a.UserName), deref(a.DeviceName), deref(a.Subject), strings.Join(detail, " ")})
				}
				table(w, "AT\tKIND\tWHO\tDEVICE\tSUBJECT\tDETAIL", rows)
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start date (2026-10-24) or RFC 3339 time")
	cmd.Flags().StringVar(&to, "to", "", "end date, inclusive")
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "only these kinds (repeat or comma-separate)")
	cmd.Flags().Int64Var(&limit, "limit", 0, "at most this many")
	return cmd
}

func (c *cli) anonymiseCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "anonymise NAME-OR-ID",
		Short: "Anonymise a leaver's name in the kitchen's logs (cannot be undone)",
		Long: "anonymise replaces a deactivated member of staff's name with \"Former staff N\" in order\n" +
			"events, the voice log, the activity log and the staff list, removes their PIN and password\n" +
			"and revokes their API keys. HACCP records keep the name they were logged under. Shows what\n" +
			"will change and asks first. Manager password (a manager's API key).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			users, err := c.api.ListUsers(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			i := slices.IndexFunc(users.Users, func(u client.User) bool { return u.ID == args[0] || strings.EqualFold(u.Name, args[0]) })
			if i < 0 {
				return fmt.Errorf("no member of staff is called %q", args[0])
			}
			id := users.Users[i].ID
			pv, err := c.api.PreviewAnonymise(cmd.Context(), &client.PreviewAnonymiseRequestOptions{PathParams: &client.PreviewAnonymisePath{ID: id}})
			if err != nil {
				return apiErr(err)
			}
			if c.asJSON && !yes {
				return c.emit(pv, nil)
			}
			for _, n := range pv.Notes {
				fmt.Fprintln(c.errOut, n)
			}
			if pv.Problem != nil {
				return fmt.Errorf("cannot anonymise %s: %s", pv.User.Name, *pv.Problem)
			}
			if !yes {
				ok, err := confirm(c.in, c.errOut, "Anonymise "+pv.User.Name+"?")
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(c.errOut, "Nothing changed.")
					return nil
				}
			}
			res, err := c.api.AnonymiseUser(cmd.Context(), &client.AnonymiseUserRequestOptions{PathParams: &client.AnonymiseUserPath{ID: id}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "Done: now %s in %d order events, %d voice commands and %d activity entries.\n",
					res.User.Name, res.Changed.OrderEvents, res.Changed.VoiceLog, res.Changed.Activity)
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "anonymise without asking")
	return cmd
}

func (c *cli) recordingCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "recording ID",
		Short: "Save a kept voice recording to a file (manager password)",
		Long:  "recording saves the kept recording of a voice command, by its ID from kishtin voice log --json.\nRecordings are kept only when system settings say to (voice_audio_days).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" {
				return fmt.Errorf("give a file to save it to with -o")
			}
			res, err := c.api.VoiceRecording(cmd.Context(), &client.VoiceRecordingRequestOptions{PathParams: &client.VoiceRecordingPath{ID: args[0]}})
			if err != nil {
				return apiErr(err)
			}
			if err := os.WriteFile(out, []byte(*res), 0o600); err != nil {
				return err
			}
			fmt.Fprintf(c.out, "%s written.\n", out)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "file to write")
	return cmd
}

func (c *cli) tillVersionsCmd(id string) *cobra.Command {
	return &cobra.Command{
		Use:   "versions TILL-ORDER-ID",
		Short: "List every version of a till order Kishtin saw",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := c.api.TillOrderVersions(cmd.Context(), &client.TillOrderVersionsRequestOptions{PathParams: &client.TillOrderVersionsPath{Till: client.TillOrderVersionsPathTill(id), ID: args[0]}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, v := range res.Versions {
					var lines []string
					for _, l := range v.Payload.Lines {
						lines = append(lines, fmt.Sprintf("%d × %s", l.Quantity, l.Name))
					}
					rows = append(rows, []string{fmt.Sprint(v.Version), local(v.SeenAt), v.Payload.State, strings.Join(lines, ", ")})
				}
				table(w, "VERSION\tSEEN\tSTATE\tLINES", rows)
			})
		},
	}
}
