package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

// compareCopy uploads a copy of a booklet's official PDF and compares it,
// or asks the local model to suggest columns from it.
func (c *cli) compareCopy(cmd *cobra.Command, book, path string, suggest bool) error {
	sha, err := c.upload(cmd, path)
	if err != nil {
		return err
	}
	body := &client.FileBody{File: sha}
	if suggest {
		res, err := c.api.SuggestColumns(cmd.Context(), &client.SuggestColumnsRequestOptions{PathParams: &client.SuggestColumnsPath{Book: book}, Body: body})
		if err != nil {
			return apiErr(err)
		}
		return c.emit(res, func(w io.Writer) {
			fmt.Fprintln(w, "The local model suggests these columns. Nothing is saved: review them in Setup › HACCP records, or set them with kishtin settings set record_books.")
			var rows [][]string
			for _, col := range res.Columns {
				rows = append(rows, []string{col.Label, deref(col.Group), deref(col.Field)})
			}
			table(w, "LABEL\tGROUP\tFIELD", rows)
			for _, n := range res.Notes {
				fmt.Fprintln(w, n)
			}
		})
	}
	res, err := c.api.CompareCopy(cmd.Context(), &client.CompareCopyRequestOptions{PathParams: &client.CompareCopyPath{Book: book}, Body: body})
	if err != nil {
		return apiErr(err)
	}
	return c.emit(res, func(w io.Writer) {
		if e := res.Edition; e != nil {
			if e.Matches {
				fmt.Fprintln(w, "Your copy is the edition this booklet was made from.")
			} else {
				fmt.Fprintln(w, "Your copy is not the edition this booklet was made from.")
			}
		}
		r := res.Report
		fmt.Fprintf(w, "%d of %d columns and questions found in the copy (%d pages).\n", r.Found, r.Total, r.Pages)
		var rows [][]string
		for _, l := range append(r.Columns, r.Questions...) {
			if !l.Found {
				rows = append(rows, []string{l.Label})
			}
		}
		if len(rows) > 0 {
			fmt.Fprintln(w)
			table(w, "NOT FOUND IN THE COPY", rows)
			fmt.Fprintln(w, "\nCheck these against your copy, and change the booklet in Setup › HACCP records if they changed.")
		}
	})
}

func (c *cli) editionsCmd() *cobra.Command {
	var check, yes bool
	cmd := &cobra.Command{
		Use:   "editions",
		Short: "List, or check, the official PDFs of the record books in use",
		Long: "editions lists the official PDF of each booklet in the bundle in use. With --check --yes it\n" +
			"downloads each one from its publisher and says whether it is the edition the booklet was made\n" +
			"from. Only public files are downloaded; nothing about the restaurant is sent. Needs the manager\n" +
			"password (a manager's API key).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if check && !yes {
				return fmt.Errorf("checking downloads the PDFs listed by kishtin haccp editions: add --yes to go ahead")
			}
			if check {
				res, err := c.api.CheckEditions(cmd.Context())
				if err != nil {
					return apiErr(err)
				}
				return c.emit(res, func(w io.Writer) {
					var rows [][]string
					for _, r := range res.Results {
						note := deref(r.ErrorData)
						if r.File != nil {
							note = "kept as " + (*r.File)[:12] + ": compare with kishtin haccp book " + r.Code + " --compare"
						}
						rows = append(rows, []string{r.Code, r.Published, r.Status, note})
					}
					table(w, "CODE\tEDITION\tRESULT\tNOTE", rows)
				})
			}
			res, err := c.api.EditionTargets(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, t := range res.Targets {
					rows = append(rows, []string{t.Code, t.Published, t.URL})
				}
				table(w, "CODE\tEDITION\tDOWNLOADED FROM", rows)
				fmt.Fprintln(w, "\nNothing was downloaded. Add --check --yes to download these and compare their editions.")
			})
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "download the PDFs and compare their editions")
	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead with --check")
	return cmd
}

func (c *cli) paperCmd() *cobra.Command {
	var photo, rowsFile string
	var read, yes bool
	cmd := &cobra.Command{
		Use:   "paper",
		Short: "Transcribe a photo of a paper record book page into records",
		Long: "paper uploads a photo of a paper page and checks the rows in --rows (a JSON list of records,\n" +
			"each with reading.taken_at and, if you like, reading.signed_by). With --yes it saves them, all\n" +
			"or none, each with the photo attached. With --read the local model reads the photo and prints\n" +
			"draft rows to check and save. Head chef or manager.",
		Example: "  kishtin haccp paper --photo sc2-april.jpg --read > rows.json\n  kishtin haccp paper --photo sc2-april.jpg --rows rows.json\n  kishtin haccp paper --photo sc2-april.jpg --rows rows.json --yes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if photo == "" {
				return fmt.Errorf("give the photo of the page with --photo")
			}
			sha, err := c.upload(cmd, photo)
			if err != nil {
				return err
			}
			if read {
				res, err := c.api.ReadPaper(cmd.Context(), &client.ReadPaperRequestOptions{Body: &client.FileBody{File: sha}})
				if err != nil {
					return apiErr(err)
				}
				for _, p := range res.Problems {
					fmt.Fprintln(c.errOut, p)
				}
				enc := json.NewEncoder(c.out)
				enc.SetIndent("", "  ")
				return enc.Encode(res.Rows)
			}
			if rowsFile == "" {
				return fmt.Errorf("give the rows with --rows rows.json, or use --read")
			}
			b, err := os.ReadFile(rowsFile)
			if err != nil {
				return fmt.Errorf("read rows: %w", err)
			}
			var rows []client.NewRecord
			if err := json.Unmarshal(b, &rows); err != nil {
				return fmt.Errorf("read rows: %w", err)
			}
			body := &client.PaperImport{File: sha, Rows: rows}
			if yes {
				res, err := c.api.ImportPaper(cmd.Context(), &client.ImportPaperRequestOptions{Body: body})
				if err != nil {
					return apiErr(err)
				}
				return c.emit(res, func(w io.Writer) { fmt.Fprintf(w, "%d records saved from the photo.\n", len(res.Records)) })
			}
			res, err := c.api.PreviewPaper(cmd.Context(), &client.PreviewPaperRequestOptions{Body: body})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { rowChecks(w, res.Rows) })
		},
	}
	cmd.Flags().StringVar(&photo, "photo", "", "photo (or PDF scan) of the paper page")
	cmd.Flags().StringVar(&rowsFile, "rows", "", "JSON file of the rows to save")
	cmd.Flags().BoolVar(&read, "read", false, "ask the local model to read the photo into draft rows")
	cmd.Flags().BoolVar(&yes, "yes", false, "save the rows")
	return cmd
}

// rowChecks prints the checks of rows to save, and what to do next.
func rowChecks(w io.Writer, rows []client.PaperCheck) {
	var tbl [][]string
	problems := 0
	for _, r := range rows {
		result := ""
		if r.Check != nil {
			result = strings.ToUpper(r.Check.Result.Outcome)
		}
		if r.Problem != nil {
			problems++
		}
		tbl = append(tbl, []string{fmt.Sprint(r.Row), result, deref(r.Problem)})
	}
	table(w, "ROW\tRESULT\tPROBLEM", tbl)
	if problems > 0 {
		fmt.Fprintf(w, "\n%d rows need fixing before they can be saved.\n", problems)
	} else {
		fmt.Fprintln(w, "\nNothing is saved. Add --yes to save every row.")
	}
}

func (c *cli) catchUpCmd() *cobra.Command {
	var from, to, rowsFile string
	var yes bool
	cmd := &cobra.Command{
		Use:   "catch-up",
		Short: "List daily checks with no record, or save checks done earlier",
		Long: "catch-up lists the fridge and freezer checks and cleaning tasks that were due and have no\n" +
			"record, by default for the last 14 days. With --rows it checks a JSON list of records, each\n" +
			"with reading.taken_at (when the check was done) and, if you like, reading.signed_by; with\n" +
			"--yes it saves them, all or none.",
		Example: "  kishtin haccp catch-up\n  kishtin haccp catch-up --from 2026-10-01 --to 2026-10-14 --json\n" +
			"  kishtin haccp catch-up --rows rows.json\n  kishtin haccp catch-up --rows rows.json --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if rowsFile == "" {
				q := &client.CatchUpGapsQuery{}
				if from != "" {
					q.From = &from
				}
				if to != "" {
					q.To = &to
				}
				res, err := c.api.CatchUpGaps(cmd.Context(), &client.CatchUpGapsRequestOptions{Query: q})
				if err != nil {
					return apiErr(err)
				}
				return c.emit(res, func(w io.Writer) {
					if len(res.Gaps) == 0 {
						fmt.Fprintf(w, "Nothing to catch up from %s to %s.\n", res.From, res.To)
						return
					}
					var tbl [][]string
					for _, g := range res.Gaps {
						what := deref(g.Unit) + " " + deref(g.Check)
						if g.Type == "cleaning" {
							what = deref(g.Task)
						}
						signed := ""
						if deref(g.SignedOff) {
							signed = "signed off"
						}
						tbl = append(tbl, []string{g.Date, g.Type, what, deref(g.Time), signed})
					}
					table(w, "DATE\tTYPE\tCHECK\tFROM\t", tbl)
					fmt.Fprintf(w, "\n%d checks from %s to %s have no record. Save the ones that were done with --rows.\n", len(res.Gaps), res.From, res.To)
				})
			}
			b, err := os.ReadFile(rowsFile)
			if err != nil {
				return fmt.Errorf("read rows: %w", err)
			}
			var rows []client.NewRecord
			if err := json.Unmarshal(b, &rows); err != nil {
				return fmt.Errorf("read rows: %w", err)
			}
			body := &client.CatchUpInput{Rows: rows}
			if yes {
				res, err := c.api.SaveCatchUp(cmd.Context(), &client.SaveCatchUpRequestOptions{Body: body})
				if err != nil {
					return apiErr(err)
				}
				return c.emit(res, func(w io.Writer) { fmt.Fprintf(w, "%d records saved.\n", len(res.Records)) })
			}
			res, err := c.api.PreviewCatchUp(cmd.Context(), &client.PreviewCatchUpRequestOptions{Body: body})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) { rowChecks(w, res.Rows) })
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "first day (2026-10-01); default 13 days before --to")
	cmd.Flags().StringVar(&to, "to", "", "last day; default today")
	cmd.Flags().StringVar(&rowsFile, "rows", "", "JSON file of records to save")
	cmd.Flags().BoolVar(&yes, "yes", false, "save the rows")
	return cmd
}
