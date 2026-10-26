package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/config"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

func resetPasswordCmd(cfgPath *string) *cobra.Command {
	var name string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "reset-password",
		Short: "Set a new password for a manager who cannot log in (on the Kishtin computer)",
		Long: "reset-password sets a new password for a manager, for when it is forgotten and no other manager can reset it.\n" +
			"Run it on the Kishtin computer, as the user Kishtin runs as: being able to use that computer's account and\n" +
			"Kishtin's data folder is what allows it. It works whether or not the server is running.\n\n" +
			"With one manager, it resets theirs. With several, name one with --name. If no manager is left (the last one\n" +
			"was deactivated), --name makes that staff member a manager, or adds a new manager with that name.\n" +
			"Every reset is noted in the activity log. You are asked for the new password; set KISHTIN_NEW_PASSWORD to skip the prompt.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			pw := os.Getenv("KISHTIN_NEW_PASSWORD")
			if pw == "" {
				pw, err = readPassword(cmd, "New manager password", "KISHTIN_NEW_PASSWORD", "reset-password")
				if err != nil {
					return err
				}
			}
			in := service.Recovery{Name: name, Password: pw}
			res, err := recoverHere(cmd.Context(), cfg, in)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			out := cmd.OutOrStdout()
			switch {
			case res.Created:
				fmt.Fprintf(out, "Added %s as a manager with this password.\n", res.User.Name)
			case res.Promoted:
				fmt.Fprintf(out, "Made %s a manager, with this password.\n", res.User.Name)
			default:
				fmt.Fprintf(out, "Done: %s can log in with the new password.\n", res.User.Name)
			}
			fmt.Fprintln(out, "This is noted in the activity log.")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "the manager whose password to reset")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

// recoverHere resets the password straight in the data folder when the
// server is stopped, or through the running server otherwise.
func recoverHere(ctx context.Context, cfg config.Config, in service.Recovery) (service.RecoveryResult, error) {
	a, err := open(ctx, cfg, false, nil)
	var busy busyError
	if errors.As(err, &busy) {
		return recoverRunning(ctx, cfg, in)
	}
	if err != nil {
		return service.RecoveryResult{}, err
	}
	defer a.close()
	return a.svc.RecoverManager(ctx, in)
}

// recoverRunning asks the running server to reset the password. It leaves
// a one-time code in the data folder, which only someone who can use that
// folder could do, and gives the server the same code.
func recoverRunning(ctx context.Context, cfg config.Config, in service.Recovery) (service.RecoveryResult, error) {
	code, err := auth.NewCode()
	if err != nil {
		return service.RecoveryResult{}, fmt.Errorf("make reset code: %w", err)
	}
	path := cfg.RecoveryCodePath()
	if err := os.WriteFile(path, []byte(code+"\n"), 0o600); err != nil {
		return service.RecoveryResult{}, fmt.Errorf("save reset code in %s: %w", path, err)
	}
	defer os.Remove(path)

	body, err := json.Marshal(struct {
		service.Recovery
		Code string `json:"code"`
	}{in, code})
	if err != nil {
		return service.RecoveryResult{}, err
	}
	scheme := "http"
	client := &http.Client{Timeout: 30 * time.Second}
	if cfg.Server.TLSCert != "" {
		scheme = "https"
		// The server is this computer's own Kishtin, reached on loopback,
		// with a certificate made for other names.
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	url := scheme + "://" + browserAddr(cfg.Server.Listen) + "/api/recover"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return service.RecoveryResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return service.RecoveryResult{}, fmt.Errorf("the Kishtin server is running (its data folder is in use) but did not answer at %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Detail == "" {
			e.Detail = resp.Status
		}
		return service.RecoveryResult{}, errors.New(e.Detail)
	}
	var res service.RecoveryResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return service.RecoveryResult{}, fmt.Errorf("read the server's answer: %w", err)
	}
	return res, nil
}
