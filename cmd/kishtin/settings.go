package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

// settingsOps calls the API for one settings group. Results are passed on
// as JSON so one set of commands handles every group.
type settingsOps struct {
	get     func(context.Context) (any, error)
	history func(context.Context) (any, error)
	preview func(context.Context, []byte) (any, error)
	save    func(context.Context, []byte) (any, error)
}

func decodeInto[T any](body []byte) (*T, error) {
	v := new(T)
	if err := json.Unmarshal(body, v); err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	return v, nil
}

// result turns a typed API result into (any, error) without wrapping a nil
// pointer in a non-nil interface.
func result[T any](v *T, err error) (any, error) {
	if err != nil {
		return nil, apiErr(err)
	}
	return v, nil
}

func (c *cli) settingsGroups() map[string]settingsOps {
	return map[string]settingsOps{
		"guidance": {
			get:     func(ctx context.Context) (any, error) { return result(c.api.GetGuidance(ctx)) },
			history: func(ctx context.Context) (any, error) { return result(c.api.GuidanceHistory(ctx)) },
			preview: func(ctx context.Context, b []byte) (any, error) {
				body, err := decodeInto[client.SettingsChangeGuidance](b)
				if err != nil {
					return nil, err
				}
				return result(c.api.PreviewGuidance(ctx, &client.PreviewGuidanceRequestOptions{Body: body}))
			},
			save: func(ctx context.Context, b []byte) (any, error) {
				body, err := decodeInto[client.SettingsChangeGuidance](b)
				if err != nil {
					return nil, err
				}
				return result(c.api.SaveGuidance(ctx, &client.SaveGuidanceRequestOptions{Body: body}))
			},
		},
		"rules": {
			get:     func(ctx context.Context) (any, error) { return result(c.api.GetRules(ctx)) },
			history: func(ctx context.Context) (any, error) { return result(c.api.RulesHistory(ctx)) },
			preview: func(ctx context.Context, b []byte) (any, error) {
				body, err := decodeInto[client.SettingsChangeRules](b)
				if err != nil {
					return nil, err
				}
				return result(c.api.PreviewRules(ctx, &client.PreviewRulesRequestOptions{Body: body}))
			},
			save: func(ctx context.Context, b []byte) (any, error) {
				body, err := decodeInto[client.SettingsChangeRules](b)
				if err != nil {
					return nil, err
				}
				return result(c.api.SaveRules(ctx, &client.SaveRulesRequestOptions{Body: body}))
			},
		},
		"lists": {
			get:     func(ctx context.Context) (any, error) { return result(c.api.GetLists(ctx)) },
			history: func(ctx context.Context) (any, error) { return result(c.api.ListsHistory(ctx)) },
			preview: func(ctx context.Context, b []byte) (any, error) {
				body, err := decodeInto[client.SettingsChangeLists](b)
				if err != nil {
					return nil, err
				}
				return result(c.api.PreviewLists(ctx, &client.PreviewListsRequestOptions{Body: body}))
			},
			save: func(ctx context.Context, b []byte) (any, error) {
				body, err := decodeInto[client.SettingsChangeLists](b)
				if err != nil {
					return nil, err
				}
				return result(c.api.SaveLists(ctx, &client.SaveListsRequestOptions{Body: body}))
			},
		},
		"system": {
			get:     func(ctx context.Context) (any, error) { return result(c.api.GetSystem(ctx)) },
			history: func(ctx context.Context) (any, error) { return result(c.api.SystemHistory(ctx)) },
			preview: func(ctx context.Context, b []byte) (any, error) {
				body, err := decodeInto[client.SettingsChangeSystem](b)
				if err != nil {
					return nil, err
				}
				return result(c.api.PreviewSystem(ctx, &client.PreviewSystemRequestOptions{Body: body}))
			},
			save: func(ctx context.Context, b []byte) (any, error) {
				body, err := decodeInto[client.SettingsChangeSystem](b)
				if err != nil {
					return nil, err
				}
				return result(c.api.SaveSystem(ctx, &client.SaveSystemRequestOptions{Body: body}))
			},
		},
	}
}

var settingsKinds = []string{"guidance", "rules", "lists", "system"}

// versioned is the JSON shape of a settings version with its value.
type versioned struct {
	Version struct {
		ID         string    `json:"id"`
		Supersedes string    `json:"supersedes"`
		CreatedBy  string    `json:"created_by"`
		CreatedAt  time.Time `json:"created_at"`
		Reason     string    `json:"reason"`
		PlanRef    string    `json:"plan_ref"`
	} `json:"version"`
	Value   json.RawMessage `json:"value"`
	Builtin bool            `json:"builtin"`
}

func roundTrip[T any](v any) (T, error) {
	var out T
	b, err := json.Marshal(v)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(b, &out)
	return out, err
}

func indentJSON(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func versionLine(v versioned) string {
	line := fmt.Sprintf("Version %s, saved %s by %s: %s", v.Version.ID, local(v.Version.CreatedAt), v.Version.CreatedBy, v.Version.Reason)
	if v.Version.PlanRef != "" {
		line += " (plan: " + v.Version.PlanRef + ")"
	}
	if v.Builtin {
		line += " [built in]"
	}
	return line
}

func (c *cli) settingsCmd() *cobra.Command {
	kindsHelp := strings.Join(settingsKinds, ", ")
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Show and change reference guidance, food-safety rules, lists and system settings",
		Long: "Settings groups: " + kindsHelp + ".\n\n" +
			"  guidance  the reference limits are measured against (starts as FSAI)\n" +
			"  rules     whether borderline readings need a corrective action, and listed names only\n" +
			"  lists     fridges, freezers, food categories and cleaning tasks\n" +
			"  system    backup schedule and retention, upload size, limit preview length\n\n" +
			"To change a group, save its value, edit it, and set it:\n\n" +
			"  kishtin settings show lists --value > lists.json\n" +
			"  # edit lists.json\n" +
			"  kishtin settings set lists -f lists.json --reason \"Add the fish fridge\"",
	}

	pick := func(kind string) (settingsOps, error) {
		ops, ok := c.settingsGroups()[kind]
		if !ok {
			return settingsOps{}, fmt.Errorf("unknown settings group %q: use one of %s", kind, kindsHelp)
		}
		return ops, nil
	}

	var valueOnly bool
	show := &cobra.Command{
		Use: "show GROUP", Short: "Show a settings group in force", Args: cobra.ExactArgs(1), ValidArgs: settingsKinds,
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := pick(args[0])
			if err != nil {
				return err
			}
			res, err := ops.get(cmd.Context())
			if err != nil {
				return err
			}
			v, err := roundTrip[versioned](res)
			if err != nil {
				return err
			}
			if valueOnly {
				_, err := fmt.Fprintln(c.out, indentJSON(v.Value))
				return err
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintln(w, versionLine(v))
				fmt.Fprintln(w, indentJSON(v.Value))
			})
		},
	}
	show.Flags().BoolVar(&valueOnly, "value", false, "print only the value, as JSON to edit and pass to set")

	history := &cobra.Command{
		Use: "history GROUP", Short: "List every saved version of a settings group", Args: cobra.ExactArgs(1), ValidArgs: settingsKinds,
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := pick(args[0])
			if err != nil {
				return err
			}
			res, err := ops.history(cmd.Context())
			if err != nil {
				return err
			}
			h, err := roundTrip[struct {
				Versions []versioned `json:"versions"`
			}](res)
			if err != nil {
				return err
			}
			return c.emit(res, func(w io.Writer) {
				if len(h.Versions) == 0 {
					fmt.Fprintln(w, "No saved versions: the defaults from kishtin.toml are in force.")
				}
				for _, v := range h.Versions {
					fmt.Fprintln(w, versionLine(v))
				}
			})
		},
	}

	var file, reason, planRef string
	var yes bool
	set := &cobra.Command{
		Use: "set GROUP -f FILE", Short: "Preview and save a new version of a settings group", Args: cobra.ExactArgs(1), ValidArgs: settingsKinds,
		Long: "set reads the new value from a JSON file (- for standard input), shows what would change,\n" +
			"then asks before saving. A change that makes food safety less strict needs --plan-ref.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := pick(args[0])
			if err != nil {
				return err
			}
			if file == "" {
				return errors.New("set -f to a JSON file with the new value (see kishtin settings show " + args[0] + " --value)")
			}
			var raw []byte
			if file == "-" {
				if !yes {
					return errors.New("reading the value from standard input needs --yes, because there is no way to ask")
				}
				raw, err = io.ReadAll(c.in)
			} else {
				raw, err = os.ReadFile(file)
			}
			if err != nil {
				return fmt.Errorf("read %s: %w", file, err)
			}
			if !json.Valid(raw) {
				return fmt.Errorf("%s is not valid JSON", file)
			}
			body, err := json.Marshal(map[string]any{"value": json.RawMessage(raw), "reason": reason, "plan_ref": planRef})
			if err != nil {
				return err
			}

			res, err := ops.preview(cmd.Context(), body)
			if err != nil {
				return err
			}
			pv, err := roundTrip[struct {
				Changes    []string `json:"changes"`
				LessStrict []string `json:"less_strict"`
				Notes      []string `json:"notes"`
				Problem    string   `json:"problem"`
			}](res)
			if err != nil {
				return err
			}
			if c.asJSON && !yes {
				return c.emit(res, nil)
			}
			w := c.errOut
			if len(pv.Changes) > 0 {
				fmt.Fprintln(w, "Changes:")
				for _, ch := range pv.Changes {
					fmt.Fprintln(w, "  "+ch)
				}
			}
			for _, ls := range pv.LessStrict {
				fmt.Fprintln(w, "WARNING: less strict: "+ls)
			}
			for _, n := range slices.Compact(pv.Notes) {
				fmt.Fprintln(w, n)
			}
			if pv.Problem != "" {
				return fmt.Errorf("cannot save: %s", pv.Problem)
			}
			if !yes {
				ok, err := confirm(c.in, w, "Save these settings?")
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(w, "Not saved.")
					return nil
				}
			}
			saved, err := ops.save(cmd.Context(), body)
			if err != nil {
				return err
			}
			v, err := roundTrip[versioned](saved)
			if err != nil {
				return err
			}
			return c.emit(saved, func(out io.Writer) { fmt.Fprintf(out, "Saved %s version %s.\n", args[0], v.Version.ID) })
		},
	}
	set.Flags().StringVarP(&file, "file", "f", "", "JSON file with the new value, or - for standard input")
	set.Flags().StringVar(&reason, "reason", "", "why the settings are changing (required)")
	set.Flags().StringVar(&planRef, "plan-ref", "", "HACCP plan section or validation (required if less strict)")
	set.Flags().BoolVar(&yes, "yes", false, "save without asking")

	cmd.AddCommand(show, history, set)
	return cmd
}
