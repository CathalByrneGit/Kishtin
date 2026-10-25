package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/doordash-oss/oapi-codegen-dd/v3/pkg/runtime"
	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
)

var recordTypes = []string{"fridge", "freezer", "delivery", "cook", "reheat", "cooling", "hot_hold", "batch_label", "cleaning"}

func (c *cli) haccpCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "haccp", Short: "Log, list, verify and export HACCP records"}
	cmd.AddCommand(c.todayCmd(), c.logCmd(), c.listCmd(), c.showCmd(), c.verifyCmd(), c.exportCmd(), c.booksCmd(), c.bookCmd())
	return cmd
}

// readingFlags are the flags shared by every record type.
type readingFlags struct {
	unit, food, dish, category                 string
	temp, tempAtCutoff                         float64
	hold                                       time.Duration
	cookedAt, intoFridgeAt                     string
	supplier, product, kind, docket            string
	madeOn, useBy, quantity, task, area, notes string
	done, notDone                              bool
	packagingOK, inDate                        bool
	shelfLife                                  int
	action, corrects, reason                   string
	dryRun                                     bool
}

func (f *readingFlags) register(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.unit, "unit", "", "fridge or freezer name")
	fl.StringVar(&f.food, "food", "", "food cooked, reheated, cooled or held")
	fl.StringVar(&f.dish, "dish", "", "dish, to use a dish limit")
	fl.StringVar(&f.category, "category", "", "food category, to use a category limit (poultry, pork, …)")
	fl.Float64Var(&f.temp, "temp", 0, "temperature in °C")
	fl.DurationVar(&f.hold, "hold", 0, "how long the core temperature was held, such as 2m")
	fl.StringVar(&f.cookedAt, "cooked-at", "", "cooling: when cooking finished (15:04 today, or RFC 3339)")
	fl.StringVar(&f.intoFridgeAt, "into-fridge-at", "", "cooling: when it went into the fridge")
	fl.Float64Var(&f.tempAtCutoff, "temp-at-cutoff", 0, "cooling: temperature at 2h30 if not in the fridge by 2h")
	fl.StringVar(&f.supplier, "supplier", "", "delivery: supplier")
	fl.StringVar(&f.product, "product", "", "delivery or batch label: product")
	fl.StringVar(&f.kind, "kind", "", "delivery: chilled, frozen or ambient")
	fl.StringVar(&f.docket, "docket", "", "delivery: path to a docket photo or PDF to attach")
	fl.BoolVar(&f.packagingOK, "packaging-ok", false, "delivery: packaging intact (required: --packaging-ok or --packaging-ok=false)")
	fl.BoolVar(&f.inDate, "in-date", false, "delivery: in date (required: --in-date or --in-date=false)")
	fl.IntVar(&f.shelfLife, "shelf-life", 0, "batch label: shelf life in days, instead of --use-by")
	fl.StringVar(&f.madeOn, "made-on", "", "batch label: date made (2026-05-01)")
	fl.StringVar(&f.useBy, "use-by", "", "batch label: use-by date")
	fl.StringVar(&f.quantity, "quantity", "", "batch label: quantity")
	fl.StringVar(&f.task, "task", "", "cleaning: task")
	fl.StringVar(&f.area, "area", "", "cleaning: area")
	fl.BoolVar(&f.done, "done", false, "cleaning: the task was done")
	fl.BoolVar(&f.notDone, "not-done", false, "cleaning: the task was not done")
	fl.StringVar(&f.notes, "notes", "", "notes")
	fl.StringVar(&f.action, "action", "", "corrective action (needed when a reading fails or is borderline)")
	fl.StringVar(&f.corrects, "corrects", "", "ID of an earlier record this one corrects")
	fl.StringVar(&f.reason, "reason", "", "why the earlier record was wrong (with --corrects)")
	fl.BoolVar(&f.dryRun, "dry-run", false, "check the reading and show the result without saving")
}

func str(cmd *cobra.Command, name, v string) *string {
	if !cmd.Flags().Changed(name) && v == "" {
		return nil
	}
	return &v
}

// parseClock reads "15:04" as today in Dublin, or an RFC 3339 time.
func parseClock(v string, now time.Time) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	hm, err := time.Parse("15:04", v)
	if err != nil {
		return nil, fmt.Errorf("%q is not a time like 14:30 or 2026-05-01T14:30:00+01:00", v)
	}
	d := now.In(clock.Dublin)
	t := time.Date(d.Year(), d.Month(), d.Day(), hm.Hour(), hm.Minute(), 0, 0, clock.Dublin)
	return &t, nil
}

func (f *readingFlags) build(cmd *cobra.Command, c *cli, typ string) (*client.NewRecord, error) {
	r := client.Reading{
		Unit: str(cmd, "unit", f.unit), Food: str(cmd, "food", f.food), Dish: str(cmd, "dish", f.dish),
		FoodCategory: str(cmd, "category", f.category), Supplier: str(cmd, "supplier", f.supplier),
		Product: str(cmd, "product", f.product), DeliveryKind: str(cmd, "kind", f.kind),
		MadeOn: str(cmd, "made-on", f.madeOn), UseBy: str(cmd, "use-by", f.useBy),
		Quantity: str(cmd, "quantity", f.quantity), Task: str(cmd, "task", f.task),
		Area: str(cmd, "area", f.area), Notes: str(cmd, "notes", f.notes),
	}
	if cmd.Flags().Changed("temp") {
		r.TempC = runtime.Ptr(f.temp)
	}
	if cmd.Flags().Changed("temp-at-cutoff") {
		r.TempAtCutoffC = runtime.Ptr(f.tempAtCutoff)
	}
	if cmd.Flags().Changed("packaging-ok") {
		r.PackagingOk = runtime.Ptr(f.packagingOK)
	}
	if cmd.Flags().Changed("in-date") {
		r.InDate = runtime.Ptr(f.inDate)
	}
	if cmd.Flags().Changed("shelf-life") {
		r.ShelfLifeDays = runtime.Ptr(int64(f.shelfLife))
	}
	if cmd.Flags().Changed("hold") {
		r.HoldSeconds = runtime.Ptr(int64(f.hold / time.Second))
	}
	now := time.Now()
	var err error
	if r.CookedAt, err = parseClock(f.cookedAt, now); err != nil {
		return nil, err
	}
	if r.IntoFridgeAt, err = parseClock(f.intoFridgeAt, now); err != nil {
		return nil, err
	}
	switch {
	case f.done && f.notDone:
		return nil, fmt.Errorf("use --done or --not-done, not both")
	case f.done:
		r.Done = runtime.Ptr(true)
	case f.notDone:
		r.Done = runtime.Ptr(false)
	}
	if f.docket != "" && !f.dryRun {
		sha, err := c.upload(cmd, f.docket)
		if err != nil {
			return nil, err
		}
		r.DocketFile = &sha
	}
	return &client.NewRecord{
		Type: typ, Reading: r,
		CorrectiveAction: str(cmd, "action", f.action),
		Corrects:         str(cmd, "corrects", f.corrects),
		CorrectionReason: str(cmd, "reason", f.reason),
	}, nil
}

func (c *cli) upload(cmd *cobra.Command, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read docket: %w", err)
	}
	var f runtime.File
	f.InitFromBytes(data, filepath.Base(path))
	m, err := c.api.UploadFile(cmd.Context(), &client.UploadFileRequestOptions{
		Query: &client.UploadFileQuery{Name: runtime.Ptr(filepath.Base(path))},
		Body:  &f,
	})
	if err != nil {
		return "", apiErr(err)
	}
	fmt.Fprintf(c.errOut, "Attached %s (%s, sha256 %s)\n", filepath.Base(path), m.Mime, m.Sha256)
	return m.Sha256, nil
}

func outcomeText(o string) string {
	switch o {
	case "pass":
		return "PASS"
	case "fail":
		return "FAIL"
	case "borderline":
		return "BORDERLINE"
	case "recorded":
		return "RECORDED"
	}
	return strings.ToUpper(o)
}

func (c *cli) todayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "today",
		Short: "Show today's checks: done, due, missed and out of limit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			td, err := c.api.Today(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(td, func(w io.Writer) {
				title := td.Date
				if td.RestaurantName != "" {
					title = td.RestaurantName + ", " + title
				}
				fmt.Fprintf(w, "%s: %d of %d checks done (%d%%)\n", title, td.Done, td.Total, td.Percent)
				switch {
				case td.OutOfLimit == 0:
					fmt.Fprintln(w, "No out-of-limit readings today.")
				case td.OutOfLimit == 1 && td.WithoutAction == 0:
					fmt.Fprintln(w, "1 out-of-limit reading today, with an action recorded.")
				case td.WithoutAction == 0:
					fmt.Fprintf(w, "%d out-of-limit readings today, each with an action recorded.\n", td.OutOfLimit)
				default:
					fmt.Fprintf(w, "%d out-of-limit readings today, %d without an action.\n", td.OutOfLimit, td.WithoutAction)
				}
				fmt.Fprintln(w)
				var rows [][]string
				for _, u := range td.Units {
					for _, sl := range u.Slots {
						state := strings.ToUpper(sl.Status)
						if sl.Record != nil {
							state = tempOf(*sl.Record) + " " + outcomeText(sl.Record.Outcome)
						}
						rows = append(rows, []string{u.Name, u.Kind, sl.Label, state})
					}
				}
				table(w, "UNIT\tKIND\tCHECK\tSTATUS", rows)
				fmt.Fprintln(w)
				rows = nil
				for _, ct := range td.Cleaning {
					state := "not done"
					if ct.Record != nil {
						state = "done " + local(ct.Record.LoggedAt) + " by " + ct.Record.LoggedByName
					}
					rows = append(rows, []string{ct.Task, state})
				}
				table(w, "CLEANING\tSTATUS", rows)
				if len(td.Batches) > 0 {
					fmt.Fprintln(w)
					rows = nil
					for _, b := range td.Batches {
						rows = append(rows, []string{deref(b.Record.Reading.Product), b.UseBy, strings.ToUpper(strings.ReplaceAll(b.Status, "_", " "))})
					}
					table(w, "BATCH\tUSE BY\tSTATUS", rows)
				}
			})
		},
	}
}

func (c *cli) logCmd() *cobra.Command {
	var f readingFlags
	cmd := &cobra.Command{
		Use:   "log TYPE",
		Short: "Save a HACCP record (" + strings.Join(recordTypes, ", ") + ")",
		Long: "log checks a reading against its limit and saves it. A failed or borderline reading\n" +
			"needs --action. Records cannot be changed: fix a mistake with a new record and --corrects.\n\n" +
			"Examples:\n" +
			"  kishtin haccp log fridge --unit \"Walk-in\" --temp 3.4\n" +
			"  kishtin haccp log cook --food \"Chicken thighs\" --category poultry --temp 78\n" +
			"  kishtin haccp log delivery --supplier \"Synthetic Meats\" --product Chicken --kind chilled --temp 3.1 \\\n" +
			"      --packaging-ok --in-date --docket docket.jpg\n" +
			"  kishtin haccp log cooling --food \"Beef stew\" --cooked-at 14:00 --into-fridge-at 15:30\n" +
			"  kishtin haccp log batch_label --product \"Grape soda syrup\" --made-on 2026-05-01 --shelf-life 5\n" +
			"  kishtin haccp log cleaning --task \"Floors swept & mopped\" --done",
		Args:      cobra.ExactArgs(1),
		ValidArgs: recordTypes,
		RunE: func(cmd *cobra.Command, args []string) error {
			nr, err := f.build(cmd, c, args[0])
			if err != nil {
				return err
			}
			if f.dryRun {
				chk, err := c.api.CheckRecord(cmd.Context(), &client.CheckRecordRequestOptions{Body: nr})
				if err != nil {
					return apiErr(err)
				}
				return c.emit(chk, func(w io.Writer) {
					fmt.Fprintf(w, "%s  %s\n", outcomeText(chk.Result.Outcome), chk.Result.Reason)
					if chk.Limit != nil {
						fmt.Fprintf(w, "Limit: %s\n", limitLabel(*chk.Limit))
					}
					if chk.NeedsCorrectiveAction {
						fmt.Fprintln(w, "Needs a corrective action (--action) to save.")
					}
					fmt.Fprintln(w, "Not saved (dry run).")
				})
			}
			rec, err := c.api.LogRecord(cmd.Context(), &client.LogRecordRequestOptions{Body: nr})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(rec, func(w io.Writer) {
				fmt.Fprintf(w, "%s  %s\n", outcomeText(rec.Outcome), rec.OutcomeReason)
				fmt.Fprintf(w, "Saved record %d (%s) at %s by %s.\n", rec.Seq, rec.ID, local(rec.LoggedAt), rec.LoggedByName)
			})
		},
	}
	f.register(cmd)
	return cmd
}

func subjectOf(r client.Record) string {
	rd := r.Reading
	for _, s := range []*string{rd.Unit, rd.Food, rd.Product, rd.Task} {
		if s != nil && *s != "" {
			if rd.Supplier != nil && *rd.Supplier != "" {
				return *rd.Supplier + ": " + *s
			}
			return *s
		}
	}
	return ""
}

func tempOf(r client.Record) string {
	if r.Reading.TempC != nil {
		return strconv.FormatFloat(*r.Reading.TempC, 'f', 1, 64) + "°C"
	}
	if r.Reading.TempAtCutoffC != nil {
		return strconv.FormatFloat(*r.Reading.TempAtCutoffC, 'f', 1, 64) + "°C"
	}
	return ""
}

func (c *cli) listCmd() *cobra.Command {
	var from, to string
	var types []string
	var limit int64
	var oldest bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List HACCP records, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := &client.ListRecordsQuery{Type: types, Newest: runtime.Ptr(!oldest)}
			if from != "" {
				q.From = &from
			}
			if to != "" {
				q.To = &to
			}
			if limit > 0 {
				q.Limit = &limit
			}
			res, err := c.api.ListRecords(cmd.Context(), &client.ListRecordsRequestOptions{Query: q})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				if len(res.Records) == 0 {
					fmt.Fprintln(w, "No records.")
					return
				}
				var rows [][]string
				for _, r := range res.Records {
					note := ""
					if deref(r.Corrects) != "" {
						note = "corrects " + deref(r.Corrects)
					}
					rows = append(rows, []string{
						strconv.FormatInt(r.Seq, 10), local(r.LoggedAt), r.Type, subjectOf(r), tempOf(r),
						outcomeText(r.Outcome), r.LoggedByName, note,
					})
				}
				table(w, "SEQ\tLOGGED\tTYPE\tWHAT\tTEMP\tRESULT\tBY\tNOTE", rows)
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start date (2026-05-01) or RFC 3339 time")
	cmd.Flags().StringVar(&to, "to", "", "end date, inclusive")
	cmd.Flags().StringSliceVar(&types, "type", nil, "only these record types")
	cmd.Flags().Int64Var(&limit, "limit", 50, "at most this many records (up to 1000)")
	cmd.Flags().BoolVar(&oldest, "oldest", false, "oldest first")
	return cmd
}

func (c *cli) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show one HACCP record in full",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := c.api.GetRecord(cmd.Context(), &client.GetRecordRequestOptions{PathParams: &client.GetRecordPath{ID: args[0]}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(r, func(w io.Writer) {
				rows := [][]string{
					{"Record", fmt.Sprintf("%d (%s)", r.Seq, r.ID)},
					{"Type", r.Type},
					{"What", subjectOf(*r)},
					{"Temperature", tempOf(*r)},
					{"Result", outcomeText(r.Outcome) + ": " + r.OutcomeReason},
					{"Corrective action", deref(r.CorrectiveAction)},
					{"Limit version", deref(r.LimitVersionID)},
					{"Logged", local(r.LoggedAt) + " by " + r.LoggedByName},
					{"Corrects", deref(r.Corrects)},
					{"Correction reason", deref(r.CorrectionReason)},
					{"Docket file", deref(r.Reading.DocketFile)},
					{"Notes", deref(r.Reading.Notes)},
					{"Hash", r.Hash},
					{"Previous hash", r.PrevHash},
				}
				pairs(w, rows)
			})
		},
	}
}

func (c *cli) verifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Check every HACCP record and attached file for tampering",
		Long:  "verify re-checks the hash chain from the first record, the limit versions records refer to,\nand every attached file. It exits with status 2 if anything fails.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := c.api.Verify(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			if err := c.emit(rep, func(w io.Writer) {
				if rep.Chain.Ok {
					fmt.Fprintf(w, "Hash chain OK: %d records, head %s\n", rep.Chain.Checked, rep.Chain.HeadHash)
				} else {
					b := rep.Chain.Break
					fmt.Fprintf(w, "Hash chain BROKEN at record %d: %s\n", b.Seq, b.Problem)
					fmt.Fprintf(w, "%d records before it checked out.\n", rep.Chain.Checked)
				}
				for _, id := range rep.UnknownLimits {
					fmt.Fprintf(w, "Record %s refers to a limit version that does not exist.\n", id)
				}
				fmt.Fprintf(w, "Files checked: %d\n", rep.FilesChecked)
				for _, p := range rep.FileProblems {
					fmt.Fprintf(w, "File %s: %s\n", p.Sha256, p.Problem)
				}
				if rep.Ok {
					fmt.Fprintln(w, "Everything checks out.")
				}
			}); err != nil {
				return err
			}
			if !rep.Ok {
				return exitError{2}
			}
			return nil
		},
	}
}

func (c *cli) exportCmd() *cobra.Command {
	var from, to, outPath string
	var limits, fails, pdf bool
	var types []string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export HACCP records as CSV for an inspection",
		Long: "export writes HACCP records as CSV, oldest first, with record IDs and hashes so the\n" +
			"file can be checked later. --from and --to are inclusive dates; leave them out for all\n" +
			"records. With --limits it writes the full limit history instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var body []byte
			var summary string
			switch {
			case limits:
				res, err := c.api.ExportLimitsWithResponse(cmd.Context())
				if err != nil {
					return apiErr(err)
				}
				body, summary = res.Body, "Limit history exported."
			case pdf:
				q := &client.ExportPackQuery{Type: types}
				if from != "" {
					q.From = &from
				}
				if to != "" {
					q.To = &to
				}
				if fails {
					q.Outcome = []string{"borderline", "fail"}
				}
				if outPath == "" || outPath == "-" {
					return fmt.Errorf("a PDF needs -o FILE")
				}
				res, err := c.api.ExportPackWithResponse(cmd.Context(), &client.ExportPackRequestOptions{Query: q})
				if err != nil {
					return apiErr(err)
				}
				body = res.Body
				if h := res.Headers200; h != nil {
					summary = fmt.Sprintf("Inspection pack with %s records written. Hash chain OK: %s, head %s.", h.XKishtinRecords, h.XKishtinChainOK, h.XKishtinChainHead)
				}
			default:
				q := &client.ExportRecordsQuery{Type: types}
				if from != "" {
					q.From = &from
				}
				if to != "" {
					q.To = &to
				}
				if fails {
					q.Outcome = []string{"borderline", "fail"}
				}
				res, err := c.api.ExportRecordsWithResponse(cmd.Context(), &client.ExportRecordsRequestOptions{Query: q})
				if err != nil {
					return apiErr(err)
				}
				body = res.Body
				if h := res.Headers200; h != nil {
					chain := "OK"
					if h.XKishtinChainOK != "true" {
						chain = "BROKEN: run `kishtin haccp verify`"
					}
					summary = fmt.Sprintf("%s records exported. Hash chain %s, head %s.", h.XKishtinRecords, chain, h.XKishtinChainHead)
				}
			}
			if outPath == "" || outPath == "-" {
				if _, err := c.out.Write(body); err != nil {
					return err
				}
			} else if err := os.WriteFile(outPath, body, 0o640); err != nil {
				return fmt.Errorf("write %s: %w", outPath, err)
			}
			fmt.Fprintln(c.errOut, summary)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start date (2026-05-01)")
	cmd.Flags().StringVar(&to, "to", "", "end date, inclusive")
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "write to this file instead of standard output")
	cmd.Flags().BoolVar(&limits, "limits", false, "export the limit history instead of records")
	cmd.Flags().StringSliceVar(&types, "type", nil, "only these record types")
	cmd.Flags().BoolVar(&fails, "fails", false, "only failed and borderline records")
	cmd.Flags().BoolVar(&pdf, "pdf", false, "write the PDF inspection pack instead of CSV (needs -o)")
	return cmd
}
