package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

func (c *cli) startupCmd() *cobra.Command {
	show := func(st *client.StartupStatus) func(io.Writer) {
		return func(w io.Writer) {
			switch {
			case !st.Supported:
				fmt.Fprintln(w, "Kishtin cannot change how it starts on this computer.")
			case st.On:
				fmt.Fprintf(w, "Kishtin starts by itself with this computer (%s: %s).\n", deref(st.Method), deref(st.Where))
			default:
				fmt.Fprintln(w, "Kishtin does not start by itself: someone has to run kishtind after the computer restarts.")
			}
			if st.Note != "" && (st.On || !st.Supported) {
				fmt.Fprintln(w, st.Note)
			}
		}
	}
	set := func(on bool) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.SetStartup(cmd.Context(), &client.SetStartupRequestOptions{Body: &client.SetStartupBody{On: on}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, show(res))
		}
	}
	cmd := &cobra.Command{
		Use:   "startup",
		Short: "Whether Kishtin starts by itself when the Kishtin computer starts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.GetStartup(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, show(res))
		},
	}
	cmd.AddCommand(
		&cobra.Command{Use: "on", Short: "Start Kishtin by itself when the Kishtin computer starts", Args: cobra.NoArgs, RunE: set(true)},
		&cobra.Command{Use: "off", Short: "Stop Kishtin starting by itself (it keeps running until stopped)", Args: cobra.NoArgs, RunE: set(false)},
	)
	return cmd
}

func (c *cli) gettingStartedCmd() *cobra.Command {
	return &cobra.Command{
		Use: "getting-started", Short: "How far the restaurant has got with setting Kishtin up", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.GetSetupProgress(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "%d of %d steps done.\n\n", res.Done, res.Total)
				for _, s := range res.Steps {
					mark := "[ ]"
					if s.Done {
						mark = "[x]"
					}
					opt := ""
					if s.Optional != nil && *s.Optional {
						opt = " (optional)"
					}
					fmt.Fprintf(w, "%s %s%s\n    %s\n", mark, s.Title, opt, s.Detail)
				}
			})
		},
	}
}
