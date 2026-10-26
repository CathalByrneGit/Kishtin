// Command kishtin is the Kishtin command line. It is a client of the
// server's HTTP API and never opens the database itself, so people,
// scripts and agents work under the same rules as the kitchen screens.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/doordash-oss/oapi-codegen-dd/v3/pkg/runtime"
	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
)

func main() {
	if err := newRoot(os.Stdout, os.Stderr, os.Stdin).Execute(); err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, "kishtin:", err)
		os.Exit(1)
	}
}

// exitError ends the program with a code after output was already written.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

// cli holds global options and the API client.
type cli struct {
	url    string
	key    string
	asJSON bool
	out    io.Writer
	errOut io.Writer
	in     io.Reader
	api    *client.Client
}

func newRoot(out, errOut io.Writer, in io.Reader) *cobra.Command {
	c := &cli{out: out, errOut: errOut, in: in}
	root := &cobra.Command{
		Use:   "kishtin",
		Short: "Kishtin command line",
		Long: "Kishtin, from cistin, Irish for kitchen.\n\n" +
			"kishtin talks to a running kishtind over its API. Set KISHTIN_API_KEY to a user API key\n" +
			"(from `kishtind setup` or `kishtin keys create`). Set KISHTIN_URL if the server is not on\n" +
			"http://127.0.0.1:8080.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return c.connect()
		},
	}
	root.SetOut(out)
	root.SetErr(errOut)
	root.PersistentFlags().StringVar(&c.url, "url", envOr("KISHTIN_URL", "http://127.0.0.1:8080"), "server URL ($KISHTIN_URL)")
	root.PersistentFlags().BoolVar(&c.asJSON, "json", false, "print JSON instead of text")
	root.AddCommand(c.whoamiCmd(), c.permissionsCmd(), c.plansCmd(), c.activityCmd(), c.haccpCmd(), c.limitsCmd(), c.settingsCmd(), c.staffCmd(), c.keysCmd(), c.backupCmd(),
		c.menuCmd(), c.orderCmd(), c.passCmd(), c.stationCmd(), c.itemsCmd(), c.sayCmd(), c.voiceCmd(), c.squareCmd(), c.connectorsCmd())
	return root
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (c *cli) connect() error {
	if c.api != nil {
		return nil
	}
	c.key = os.Getenv("KISHTIN_API_KEY")
	if c.key == "" {
		return errors.New("set KISHTIN_API_KEY to a Kishtin user API key")
	}
	base := strings.TrimRight(c.url, "/") + "/api"
	key := c.key
	api, err := client.NewDefaultClient(base,
		runtime.WithStdHTTPClient(&http.Client{Timeout: 2 * time.Minute}),
		runtime.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+key)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", c.url, err)
	}
	c.api = api
	return nil
}

// apiErr makes an API error readable.
func apiErr(err error) error {
	if err == nil {
		return nil
	}
	var em client.ErrorModel
	if errors.As(err, &em) && em.Detail != nil {
		msg := *em.Detail
		for _, d := range em.Errors {
			if d.Message != nil {
				msg += "; " + strings.TrimPrefix(deref(d.Location), "body.") + " " + *d.Message
			}
		}
		return errors.New(msg)
	}
	var ce *runtime.ClientAPIError
	if errors.As(err, &ce) {
		return fmt.Errorf("%s (HTTP %d)", strings.TrimPrefix(ce.Error(), "API error "), ce.StatusCode())
	}
	if strings.Contains(err.Error(), "connection refused") {
		return errors.New("cannot reach the Kishtin server: is kishtind running, and is --url right?")
	}
	return err
}

// emit prints v as JSON when --json is set, otherwise calls text.
func (c *cli) emit(v any, text func(w io.Writer)) error {
	if c.asJSON {
		enc := json.NewEncoder(c.out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	text(c.out)
	return nil
}

func table(w io.Writer, header string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, header)
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}

// pairs prints "Label: value" lines, skipping empty values.
func pairs(w io.Writer, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		if r[1] != "" {
			fmt.Fprintf(tw, "%s:\t%s\n", r[0], r[1])
		}
	}
	tw.Flush()
}

func local(t time.Time) string { return t.In(clock.Dublin).Format("2006-01-02 15:04") }

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func (c *cli) whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show who the API key acts as",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			me, err := c.api.Me(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(me, func(w io.Writer) {
				if me.User == nil {
					fmt.Fprintf(w, "Device key %q (no staff member)\n", me.Key.Name)
					return
				}
				fmt.Fprintf(w, "%s (%s), using key %q\n", me.User.Name, me.User.Role, me.Key.Name)
			})
		},
	}
}
