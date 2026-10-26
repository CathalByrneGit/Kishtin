package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

// minsSecs says a number of seconds as minutes and seconds.
func minsSecs(s int64) string {
	sign := ""
	if s < 0 {
		sign, s = "-", -s
	}
	if s < 60 {
		return fmt.Sprintf("%s%ds", sign, s)
	}
	return fmt.Sprintf("%s%dm %02ds", sign, s/60, s%60)
}

func spreadText(s client.Spread) string {
	if s.Count == 0 {
		return "none"
	}
	return fmt.Sprintf("median %s, 9 in 10 within %s (%d)", minsSecs(s.MedianSeconds), minsSecs(s.P90Seconds), s.Count)
}

func (c *cli) reportCmd() *cobra.Command {
	var from, to string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Reports on HACCP checks and pass times",
		Long:  "report works out a report for a period, by default the 28 days up to today.",
	}
	cmd.PersistentFlags().StringVar(&from, "from", "", "first day (2026-10-01)")
	cmd.PersistentFlags().StringVar(&to, "to", "", "last day (default today)")

	haccp := &cobra.Command{
		Use:   "haccp",
		Short: "How well the daily HACCP checks were kept up",
		Long:  "haccp reports fridge and freezer checks due, done and missed, cleaning tasks done, records out of\nlimit, and each unit's temperatures. Head chef or manager.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.ReportHaccp(cmd.Context(), &client.ReportHaccpRequestOptions{Query: &client.ReportHaccpQuery{From: optStr(from), To: optStr(to)}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "HACCP checks, %s to %s\n\n", res.From, res.To)
				fmt.Fprintf(w, "Fridge and freezer checks: %d of %d done (%d%%), %d missed\n", res.ChecksDone, res.ChecksDue, res.ChecksPercent, res.ChecksMissed)
				fmt.Fprintf(w, "Cleaning tasks: %d of %d done (%d%%)\n", res.CleaningDone, res.CleaningDue, res.CleaningPercent)
				fmt.Fprintf(w, "Records: %d, %d out of limit, %d without an action\n\n", res.Records, res.OutOfLimit, res.WithoutAction)
				var rows [][]string
				for _, d := range res.MissedByWeekday {
					rows = append(rows, []string{d.Label, fmt.Sprint(d.Count), fmt.Sprint(deref(d.Of))})
				}
				table(w, "DAY\tMISSED\tDUE", rows)
				if len(res.MissedByUnit) > 0 {
					rows = nil
					for _, u := range res.MissedByUnit {
						rows = append(rows, []string{u.Label, fmt.Sprint(u.Count)})
					}
					fmt.Fprintln(w)
					table(w, "UNIT\tMISSED", rows)
				}
				rows = nil
				for _, u := range res.Temperatures {
					var hi *float64
					n := int64(0)
					for _, d := range u.Days {
						n += d.Count
						if d.MaxC != nil && (hi == nil || *d.MaxC > *hi) {
							hi = d.MaxC
						}
					}
					high, limit := "-", "-"
					if hi != nil {
						high = fmt.Sprintf("%.1f°C", *hi)
					}
					if u.MaxC != nil {
						limit = fmt.Sprintf("%.1f°C", *u.MaxC)
					}
					rows = append(rows, []string{u.Unit, fmt.Sprint(n), high, limit})
				}
				fmt.Fprintln(w)
				table(w, "UNIT\tREADINGS\tHIGHEST\tLIMIT", rows)
			})
		},
	}

	pass := &cobra.Command{
		Use:   "pass",
		Short: "How fast courses reached the pass",
		Long: "pass reports fired courses that reached the pass: the share on time (within a minute of target),\n" +
			"time from fire to pass and from pass to served, late courses and why, and each dish's actual time\n" +
			"against its set time. Head chef or manager.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.ReportPass(cmd.Context(), &client.ReportPassRequestOptions{Query: &client.ReportPassQuery{From: optStr(from), To: optStr(to)}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "Pass times, %s to %s\n\n", res.From, res.To)
				fmt.Fprintf(w, "Courses: %d, %d on time (%d%%), %d late\n", res.Courses, res.OnTime, res.OnTimePercent, res.Late)
				fmt.Fprintf(w, "Fire to pass: %s\n", spreadText(res.Ticket))
				fmt.Fprintf(w, "Pass to served: %s\n", spreadText(res.PassWait))
				if len(res.LateByCause) > 0 {
					var rows [][]string
					for _, l := range res.LateByCause {
						rows = append(rows, []string{l.Label, fmt.Sprint(l.Count)})
					}
					fmt.Fprintln(w)
					table(w, "WHY LATE\tCOURSES", rows)
				}
				if len(res.Dishes) > 0 {
					var rows [][]string
					for _, d := range res.Dishes {
						rows = append(rows, []string{d.DishName, minsSecs(d.SetSeconds), minsSecs(d.Actual.MedianSeconds), minsSecs(d.OverSeconds), fmt.Sprint(d.Actual.Count)})
					}
					fmt.Fprintln(w)
					table(w, "DISH\tSET\tACTUAL (MEDIAN)\tOVER\tCOOKED", rows)
				}
			})
		},
	}
	cmd.AddCommand(haccp, pass)
	return cmd
}
