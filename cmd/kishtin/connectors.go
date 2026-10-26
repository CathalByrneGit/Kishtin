package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

func (c *cli) connectorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connectors",
		Short: "What Kishtin can connect to (Square), and connecting it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.ListConnectors(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				for _, k := range res.Connectors {
					state := "not connected"
					if k.Connected {
						state = "connected (" + deref(k.Source)
						if e := deref(k.Environment); e != "" {
							state += ", " + e
						}
						state += ")"
					}
					if k.Enabled {
						state += ", in use"
					}
					fmt.Fprintf(w, "%s: %s\n  %s\n  Sends: %s\n", k.Name, state, k.Summary, k.Sends)
					if e := deref(k.LastError); e != "" {
						fmt.Fprintln(w, "  Last problem:", e)
					}
				}
			})
		},
	}
	var env, tokenFile string
	connect := &cobra.Command{
		Use:   "connect square --token-file FILE",
		Short: "Connect Square with an access token, read from a file or standard input (-)",
		Long: `Connect Square. The token is read from a file, or from standard input
with --token-file -, never from the command line, so it does not end up in
shell history. Kishtin sends it to Square to check it, and saves it only if
Square accepts it, in a file only the server can read (not the database).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "square" {
				return fmt.Errorf("no connector %q: Kishtin connects to square", args[0])
			}
			var raw []byte
			var err error
			switch tokenFile {
			case "":
				return fmt.Errorf("give the token with --token-file FILE, or --token-file - to read it from standard input")
			case "-":
				raw, err = io.ReadAll(c.in)
			default:
				raw, err = os.ReadFile(tokenFile)
			}
			if err != nil {
				return err
			}
			res, err := c.api.ConnectSquare(cmd.Context(), &client.ConnectSquareRequestOptions{Body: &client.ConnectInput{
				Environment: client.ConnectInputEnvironment(env), Token: strings.TrimSpace(string(raw)),
			}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "Connected to Square (%s).\n", env)
				printLocations(w, res.Locations)
				fmt.Fprintln(w, "Next: map till items with kishtin square catalog and map, then turn import on with kishtin settings set square.")
			})
		},
	}
	connect.Flags().StringVar(&env, "environment", "production", "production or sandbox")
	connect.Flags().StringVar(&tokenFile, "token-file", "", "file holding the access token, or - for standard input")
	disconnect := &cobra.Command{
		Use: "disconnect square", Short: "Disconnect Square and delete the saved token (turn import off first)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "square" {
				return fmt.Errorf("no connector %q: Kishtin connects to square", args[0])
			}
			if _, err := c.api.DisconnectSquare(cmd.Context()); err != nil {
				return apiErr(err)
			}
			fmt.Fprintln(c.out, "Disconnected from Square. The saved token is deleted.")
			return nil
		},
	}
	cmd.AddCommand(connect, disconnect)
	return cmd
}

func printLocations(w io.Writer, locs []client.Location) {
	var rows [][]string
	for _, l := range locs {
		state := "active"
		if !l.Active {
			state = "closed"
		}
		rows = append(rows, []string{l.Name, l.ID, state})
	}
	table(w, "LOCATION\tID\tSTATE", rows)
}
