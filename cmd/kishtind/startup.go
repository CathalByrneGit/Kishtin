package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/autostart"
	"github.com/cathalbyrnegit/kishtin/internal/config"
)

// startup is how this kishtind starts with the computer: the same program,
// with the same kishtin.toml.
type startup struct {
	m    autostart.Manager
	prog autostart.Program
	// warn names settings from the environment that a Kishtin started by
	// the computer would not see.
	warn string
}

func newStartup(cfg config.Config, cfgPath string) (*startup, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find kishtind: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find your home folder: %w", err)
	}
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	prog := autostart.Program{Executable: exe, LogFile: filepath.Join(cfg.DataDir, "kishtind.log")}
	if cfgPath == "" {
		cfgPath = os.Getenv("KISHTIN_CONFIG")
	}
	if cfgPath != "" {
		abs, err := filepath.Abs(cfgPath)
		if err != nil {
			return nil, err
		}
		prog.Args = []string{"--config", abs}
	}
	return &startup{
		m:    autostart.Manager{OS: runtime.GOOS, Home: home, User: name, UID: os.Getuid(), Run: autostart.Exec},
		prog: prog,
		warn: envWarning(os.Environ()),
	}, nil
}

// envWarning names KISHTIN_ server settings set in the environment, which
// a Kishtin the computer starts would not see.
func envWarning(env []string) string {
	skip := map[string]bool{
		// Read here and passed on, or for commands and the installer.
		"KISHTIN_CONFIG": true, "KISHTIN_API_KEY": true, "KISHTIN_URL": true,
		"KISHTIN_SETUP_PASSWORD": true, "KISHTIN_NEW_PASSWORD": true,
		"KISHTIN_VERSION": true, "KISHTIN_INSTALL_DIR": true, "KISHTIN_RELEASE_URL": true,
		"KISHTIN_NO_MODIFY_PATH": true, "KISHTIN_STARTUP": true,
	}
	var names []string
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "KISHTIN_") && !skip[k] {
			names = append(names, k)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return "Set where kishtind runs now, but not seen by a Kishtin the computer starts: " + strings.Join(names, ", ") + ". Put these settings in kishtin.toml instead."
}

func (s *startup) withWarning(st autostart.Status, err error) (autostart.Status, error) {
	if err == nil && st.On && s.warn != "" {
		st.Note = strings.TrimSpace(st.Note + " " + s.warn)
	}
	return st, err
}

// Status, Enable and Disable are for the running server: Enable never
// starts a second Kishtin.
func (s *startup) Status(ctx context.Context) (autostart.Status, error) {
	return s.withWarning(s.m.Status(ctx))
}

func (s *startup) Enable(ctx context.Context) (autostart.Status, error) {
	return s.withWarning(s.m.Enable(ctx, s.prog, false))
}

func (s *startup) Disable(ctx context.Context) (autostart.Status, error) {
	return s.m.Disable(ctx)
}

func startupCmd(cfgPath *string) *cobra.Command {
	var asJSON bool
	get := func() (config.Config, *startup, error) {
		cfg, err := loadConfig(*cfgPath)
		if err != nil {
			return cfg, nil, err
		}
		s, err := newStartup(cfg, *cfgPath)
		return cfg, s, err
	}
	show := func(cmd *cobra.Command, st autostart.Status, extra string) error {
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(st)
		}
		out := cmd.OutOrStdout()
		switch {
		case !st.Supported:
			fmt.Fprintln(out, "Kishtin cannot start by itself on this computer.")
		case st.On:
			fmt.Fprintf(out, "Kishtin starts by itself with this computer (%s: %s).\n", st.Method, st.Where)
		default:
			fmt.Fprintln(out, "Kishtin does not start by itself: someone has to run kishtind after the computer restarts.")
		}
		if st.Note != "" && (st.On || !st.Supported) {
			fmt.Fprintln(out, st.Note)
		}
		if extra != "" {
			fmt.Fprintln(out, extra)
		}
		return nil
	}
	cmd := &cobra.Command{
		Use:   "startup",
		Short: "Whether Kishtin starts by itself when this computer starts",
		Long: "startup shows whether Kishtin starts by itself when this computer starts, as this user, with this kishtin.toml.\n" +
			"It uses the computer's own way: a Task Scheduler task on Windows, a LaunchAgent on macOS, a systemd user service on Linux.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, s, err := get()
			if err != nil {
				return err
			}
			st, err := s.Status(cmd.Context())
			if err != nil {
				return err
			}
			return show(cmd, st, "")
		},
	}
	cmd.PersistentFlags().BoolVar(&asJSON, "json", false, "print JSON")
	on := &cobra.Command{
		Use:   "on",
		Short: "Start Kishtin by itself when this computer starts, and now if it isn't running",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, s, err := get()
			if err != nil {
				return err
			}
			// Start it now too, unless a Kishtin is already running.
			free, err := dataFolderFree(cfg)
			if err != nil {
				return err
			}
			st, err := s.withWarning(s.m.Enable(cmd.Context(), s.prog, free))
			if err != nil {
				return err
			}
			extra := "Kishtin is running now, and starts by itself from the next time the computer starts."
			if free {
				extra = "Kishtin is starting in the background. Open http://" + browserAddr(cfg.Server.Listen) + " in a browser on this computer."
			}
			return show(cmd, st, extra)
		},
	}
	off := &cobra.Command{
		Use:   "off",
		Short: "Stop Kishtin starting by itself (a running Kishtin keeps running)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, s, err := get()
			if err != nil {
				return err
			}
			st, err := s.Disable(cmd.Context())
			if err != nil {
				return err
			}
			return show(cmd, st, "A Kishtin running now keeps running until it is stopped or the computer restarts.")
		},
	}
	stop := &cobra.Command{
		Use:   "stop",
		Short: "Stop a Kishtin the computer started, for example before an update (start-up stays on)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, s, err := get()
			if err != nil {
				return err
			}
			if err := s.m.Stop(cmd.Context()); err != nil {
				return err
			}
			if !asJSON {
				fmt.Fprintln(cmd.OutOrStdout(), "Stopped Kishtin if the computer had started it. kishtind startup on starts it again.")
			}
			return nil
		},
	}
	cmd.AddCommand(on, off, stop)
	return cmd
}

// dataFolderFree reports whether no Kishtin is using the data folder.
func dataFolderFree(cfg config.Config) (bool, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return false, fmt.Errorf("create data folder %s: %w", cfg.DataDir, err)
	}
	lock := flock.New(cfg.LockPath())
	ok, err := lock.TryLock()
	if err != nil {
		return false, fmt.Errorf("lock data folder: %w", err)
	}
	if ok {
		_ = lock.Unlock()
	}
	return ok, nil
}
