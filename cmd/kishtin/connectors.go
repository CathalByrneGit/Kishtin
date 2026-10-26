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
		Short: "What Kishtin can connect to (Square, SumUp), and connecting it",
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
		Use:   "connect square|sumup --token-file FILE",
		Short: "Connect a till with its token (a Square access token or a SumUp API key), read from a file or standard input (-)",
		Long: `Connect a till. The token is read from a file, or from standard input
with --token-file -, never from the command line, so it does not end up in
shell history. Kishtin sends it to the till to check it, and saves it only if
the till accepts it, in a file only the server can read (not the database).
A SumUp API key comes from SumUp's dashboard: Settings, For Developers,
Toolkit, API Keys.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := tillName(args[0])
			if err != nil {
				return err
			}
			var raw []byte
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
			environment := client.ConnectInputEnvironment(env)
			res, err := c.api.ConnectTill(cmd.Context(), &client.ConnectTillRequestOptions{
				PathParams: &client.ConnectTillPath{Till: client.ConnectTillPathTill(args[0])},
				Body:       &client.ConnectTillBody{Environment: &environment, Token: strings.TrimSpace(string(raw))},
			})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				fmt.Fprintf(w, "Connected to %s (%s).\n", name, env)
				printLocations(w, res.Locations)
				fmt.Fprintf(w, "Next: map till items with kishtin %s catalog and map, then turn import on with kishtin settings set %s.\n", args[0], args[0])
			})
		},
	}
	connect.Flags().StringVar(&env, "environment", "production", "production, or sandbox for Square")
	connect.Flags().StringVar(&tokenFile, "token-file", "", "file holding the token, or - for standard input")
	disconnect := &cobra.Command{
		Use: "disconnect square|sumup", Short: "Disconnect a till and delete its saved token (turn import off first)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := tillName(args[0])
			if err != nil {
				return err
			}
			if _, err := c.api.DisconnectTill(cmd.Context(), &client.DisconnectTillRequestOptions{PathParams: &client.DisconnectTillPath{Till: client.DisconnectTillPathTill(args[0])}}); err != nil {
				return apiErr(err)
			}
			fmt.Fprintf(c.out, "Disconnected from %s. The saved token is deleted.\n", name)
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

// tillName checks a till's connector ID and names it.
func tillName(id string) (string, error) {
	switch id {
	case "square":
		return "Square", nil
	case "sumup":
		return "SumUp", nil
	}
	return "", fmt.Errorf("no connector %q: Kishtin connects to square and sumup", id)
}
