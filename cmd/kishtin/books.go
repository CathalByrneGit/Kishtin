package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

func (c *cli) booksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "books",
		Short: "List the official record books Kishtin can fill in",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.ListRecordBooks(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				for i, p := range res.Packs {
					if i > 0 {
						fmt.Fprintln(w)
					}
					fmt.Fprintf(w, "%s (%s)\n", p.Name, p.Issuer)
					var rows [][]string
					for _, t := range p.Templates {
						status := "can fill"
						if t.Builder == nil {
							status = "not yet: " + deref(t.Missing)
						}
						rows = append(rows, []string{t.Code, t.Title, t.Source.Published, status})
					}
					table(w, "CODE\tTITLE\tEDITION\tSTATUS", rows)
				}
			})
		},
	}
}

func (c *cli) bookCmd() *cobra.Command {
	var month, outPath string
	cmd := &cobra.Command{
		Use:   "book CODE",
		Short: "Fill in a record book for a month, such as SC2",
		Long: "book fills in an official record book from Kishtin's records, for one month (Dublin time).\n" +
			"Without -o it shows the checks that were due and have no reading, and the columns Kishtin\n" +
			"does not record. With -o FILE it writes the filled-in booklet as a PDF.",
		Example: "  kishtin haccp book SC2 --month 2026-10\n  kishtin haccp book SC2 --month 2026-10 -o sc2-october.pdf",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var q *client.GetRecordBookQuery
			if month != "" {
				if _, err := time.Parse("2006-01", month); err != nil {
					return fmt.Errorf("--month must look like 2026-10")
				}
				q = &client.GetRecordBookQuery{Month: &month}
			}
			if outPath != "" {
				opts := &client.ExportRecordBookRequestOptions{PathParams: &client.ExportRecordBookPath{Book: args[0]}}
				if q != nil {
					opts.Query = &client.ExportRecordBookQuery{Month: q.Month}
				}
				res, err := c.api.ExportRecordBookWithResponse(cmd.Context(), opts)
				if err != nil {
					return apiErr(err)
				}
				if outPath == "-" {
					_, err = c.out.Write(res.Body)
					return err
				}
				if err := os.WriteFile(outPath, res.Body, 0o640); err != nil {
					return fmt.Errorf("write %s: %w", outPath, err)
				}
				if h := res.Headers200; h != nil {
					chain := "OK"
					if h.XKishtinChainOK != "true" {
						chain = "BROKEN: run `kishtin haccp verify`"
					}
					fmt.Fprintf(c.errOut, "%s written. Records shown: %s. Hash chain %s, head %s.\n", outPath, h.XKishtinRecords, chain, h.XKishtinChainHead)
				}
				return nil
			}
			b, err := c.api.GetRecordBook(cmd.Context(), &client.GetRecordBookRequestOptions{PathParams: &client.GetRecordBookPath{Book: args[0]}, Query: q})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(b, func(w io.Writer) {
				m := b.Month
				if t, err := time.Parse("2006-01", b.Month); err == nil {
					m = t.Format("January 2006")
				}
				fmt.Fprintf(w, "%s %s, %s: filled from %d records", b.Template.Code, b.Template.Title, m, b.Records)
				if b.Corrected > 0 {
					fmt.Fprintf(w, " (%d corrected)", b.Corrected)
				}
				fmt.Fprintln(w, ".")
				if len(b.Gaps) == 0 {
					fmt.Fprintln(w, "No missed checks.")
				} else {
					fmt.Fprintf(w, "%d checks were due and have no reading:\n\n", len(b.Gaps))
					var rows [][]string
					for _, g := range b.Gaps {
						rows = append(rows, []string{g.Date, g.Unit, g.Check})
					}
					table(w, "DATE\tUNIT\tCHECK", rows)
				}
				if len(b.Unfilled) > 0 {
					fmt.Fprintf(w, "\nKishtin does not record: %s. These boxes print blank.\n", strings.Join(b.Unfilled, ", "))
				}
				fmt.Fprintln(w, "\nWrite the booklet with -o FILE.pdf.")
			})
		},
	}
	cmd.Flags().StringVar(&month, "month", "", "month as YYYY-MM (default this month)")
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "write the filled-in booklet as a PDF to this file")
	return cmd
}
