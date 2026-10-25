package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

func (c *cli) signOffCmd() *cobra.Command {
	var from, to, note string
	var yes bool
	cmd := &cobra.Command{
		Use:   "sign-off",
		Short: "Sign off the HACCP records for a period (manager password)",
		Long: "sign-off shows what a period holds: its records, readings out of limit, readings without\n" +
			"a corrective action and missed checks. With --yes it saves a sign-off record, which fills the\n" +
			"manager or supervisor check line of every record book. Without --from it starts the day after\n" +
			"the last sign-off; without --to it ends yesterday. A period with missed checks or readings\n" +
			"without an action needs --note.",
		Example: "  kishtin haccp sign-off\n  kishtin haccp sign-off --from 2026-10-01 --to 2026-10-07 --note \"Closed on the 6th\" --yes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opt := func(s string) *string {
				if s == "" {
					return nil
				}
				return &s
			}
			if yes {
				rec, err := c.api.SignOff(cmd.Context(), &client.SignOffRequestOptions{Body: &client.SignOffBody{From: opt(from), To: opt(to), Note: opt(note)}})
				if err != nil {
					return apiErr(err)
				}
				return c.emit(rec, func(w io.Writer) {
					fmt.Fprintf(w, "Signed off as record %d by %s.\n%s\n", rec.Seq, rec.LoggedByName, rec.OutcomeReason)
				})
			}
			pv, err := c.api.PreviewSignOff(cmd.Context(), &client.PreviewSignOffRequestOptions{Query: &client.PreviewSignOffQuery{From: opt(from), To: opt(to)}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(pv, func(w io.Writer) {
				if pv.UpToDate {
					fmt.Fprintln(w, "Every day up to today is signed off. Give --from and --to to sign off a period again.")
					return
				}
				fmt.Fprintf(w, "Period %s to %s\n\n", pv.From, pv.To)
				r := pv.Review
				pairs(w, [][]string{
					{"Records", fmt.Sprint(r.Records)},
					{"Out of limit", fmt.Sprint(r.OutOfLimit)},
					{"Without a corrective action", fmt.Sprint(r.WithoutAction)},
					{"Missed checks", fmt.Sprint(r.Missed)},
				})
				if len(pv.WithoutAction) > 0 {
					fmt.Fprintln(w)
					var rows [][]string
					for _, x := range pv.WithoutAction {
						rows = append(rows, []string{fmt.Sprint(x.Seq), local(x.LoggedAt), subjectOf(x), tempOf(x), outcomeText(x.Outcome)})
					}
					table(w, "RECORD\tLOGGED\tWHAT\tREADING\tRESULT", rows)
				}
				if len(pv.Missed) > 0 {
					fmt.Fprintln(w)
					var rows [][]string
					for _, g := range pv.Missed {
						rows = append(rows, []string{g.Date, g.Unit, g.Check})
					}
					table(w, "MISSED\tUNIT\tCHECK", rows)
				}
				for _, o := range pv.Overlaps {
					fmt.Fprintf(w, "\nRecord %d already signs off %s.\n", o.Seq, subjectOf(o))
				}
				fmt.Fprintln(w)
				if pv.NoteRequired {
					fmt.Fprintln(w, "This period has missed checks or readings without an action: add --note saying what was done about them.")
				}
				fmt.Fprintln(w, "Nothing is saved. Add --yes to sign off.")
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "first day (2026-10-01); default: the day after the last sign-off")
	cmd.Flags().StringVar(&to, "to", "", "last day, inclusive; default: yesterday")
	cmd.Flags().StringVar(&note, "note", "", "what was done about missed checks or readings without an action")
	cmd.Flags().BoolVar(&yes, "yes", false, "save the sign-off")
	return cmd
}
