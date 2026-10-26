// Command kishtind is the Kishtin server: it serves the API and runs
// scheduled backups. It also has offline commands for first-time setup.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/cathalbyrnegit/kishtin/frontend"
	"github.com/cathalbyrnegit/kishtin/internal/config"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
	"github.com/cathalbyrnegit/kishtin/internal/web"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "kishtind:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var cfgPath string
	root := &cobra.Command{
		Use:           "kishtind",
		Short:         "Kishtin server",
		Long:          "Kishtin, from cistin, Irish for kitchen.\n\nkishtind serves the Kishtin API and runs scheduled backups.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context(), cfgPath)
		},
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "", "path to kishtin.toml (default $KISHTIN_CONFIG, then <data dir>/kishtin.toml)")
	root.AddCommand(setupCmd(&cfgPath), migrateCmd(&cfgPath), openapiCmd())
	return root
}

func loadConfig(path string) (config.Config, error) {
	mustExist := path != ""
	if path == "" {
		path = os.Getenv("KISHTIN_CONFIG")
		mustExist = path != ""
	}
	if path == "" {
		path = filepath.Join(config.DefaultDataDir(), "kishtin.toml")
	}
	return config.Load(path, mustExist)
}

// app is an opened data directory.
type app struct {
	cfg   config.Config
	lock  *flock.Flock
	store *sqlite.Store
	svc   *service.Service
}

// open locks the data directory and opens the store. Only one process can
// hold the lock, so offline commands cannot run while the server does.
func open(ctx context.Context, cfg config.Config) (*app, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data folder %s: %w", cfg.DataDir, err)
	}
	lock := flock.New(cfg.LockPath())
	ok, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock data folder: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("another Kishtin process is using %s: stop it first", cfg.DataDir)
	}
	// fail releases what was opened so far.
	var st *sqlite.Store
	fail := func(err error) (*app, error) {
		if st != nil {
			_ = st.Close()
		}
		_ = lock.Unlock()
		return nil, err
	}
	st, err = sqlite.Open(ctx, cfg.Store.DSN)
	if err != nil {
		return fail(err)
	}
	fs, err := files.New(cfg.FilesDir())
	if err != nil {
		return fail(err)
	}
	sp, err := cfg.SpeechClient()
	if err != nil {
		return fail(err)
	}
	pos, err := cfg.SquareClient()
	if err != nil {
		return fail(err)
	}
	model, err := cfg.AIClient()
	if err != nil {
		return fail(err)
	}
	svc := service.New(service.Options{
		POS:         pos,
		POSSource:   cfg.Square.TokenFrom,
		Credentials: &integrations.CredentialStore{Dir: cfg.CredentialsDir()},
		NewSquare:   cfg.NewSquare,
		Speech:      sp,
		WakeWords:   cfg.Speech.WakeWords,
		AI:          model,
		Store:       st,
		Files:       fs,
		System:      cfg.SystemSettings(),
		// Kept voice recordings, when system settings say to keep them.
		VoiceAudioDir: filepath.Join(cfg.DataDir, "voice-audio"),
		Events:        events.NewHub(),
		Backups: service.BackupConfig{
			Dir:  cfg.Backup.Dir,
			Open: func(ctx context.Context, path string) (store.Store, error) { return sqlite.OpenSnapshot(ctx, path) },
		},
	})
	if err := svc.Init(ctx); err != nil {
		return fail(err)
	}
	return &app{cfg: cfg, lock: lock, store: st, svc: svc}, nil
}

func (a *app) close() error {
	return errors.Join(a.store.Close(), a.lock.Unlock())
}

func serve(ctx context.Context, cfgPath string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := a.close(); err != nil {
			log.Error("close data folder", "err", err)
		}
	}()

	staff, err := a.svc.StaffList(ctx)
	if err != nil {
		return err
	}
	if len(staff) == 0 {
		return errors.New("no staff yet: stop here and run `kishtind setup` to create the first manager")
	}

	h, err := web.New(a.svc, log, frontend.Files())
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		// Requests end when the server stops, which closes live streams.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	go a.svc.RunBackups(ctx, log)
	go a.svc.RunSquare(ctx, log)
	go a.svc.RunPlanRecorder(ctx, log)
	go cleanSessions(ctx, a.svc, log)

	errc := make(chan error, 1)
	go func() {
		tls := cfg.Server.TLSCert != ""
		log.Info("Kishtin is listening", "addr", cfg.Server.Listen, "tls", tls, "data", cfg.DataDir)
		if tls {
			errc <- srv.ListenAndServeTLS(cfg.Server.TLSCert, cfg.Server.TLSKey)
		} else {
			errc <- srv.ListenAndServe()
		}
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-ctx.Done():
	}
	log.Info("stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func cleanSessions(ctx context.Context, svc *service.Service, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := svc.CleanSessions(ctx); err != nil {
				log.Warn("clean sessions", "err", err)
			}
		}
	}
}

func setupCmd(cfgPath *string) *cobra.Command {
	var name, pin string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create the first manager and their API key (server must be stopped)",
		Long: "setup creates the first manager account and prints an API key for the kishtin CLI.\n" +
			"It works only while there are no staff, and only while the server is stopped.\n" +
			"You are asked for the manager password; set KISHTIN_SETUP_PASSWORD to skip the prompt.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			if name == "" {
				return errors.New("--name is required")
			}
			pw := os.Getenv("KISHTIN_SETUP_PASSWORD")
			if pw == "" {
				pw, err = readPassword(cmd)
				if err != nil {
					return err
				}
			}
			a, err := open(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer a.close()
			res, err := a.svc.Setup(cmd.Context(), service.NewUser{Name: name, Password: pw, PIN: pin})
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Created manager %s.\n\n", res.User.Name)
			fmt.Fprintf(out, "API key for the kishtin CLI (shown once, store it safely):\n\n  %s\n\n", res.Secret)
			fmt.Fprintf(out, "Next: start kishtind, then set KISHTIN_API_KEY to this key to use the CLI.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "manager's name")
	cmd.Flags().StringVar(&pin, "pin", "", "optional 4–8 digit PIN for kitchen devices")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func readPassword(cmd *cobra.Command) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("set KISHTIN_SETUP_PASSWORD, or run setup in a terminal to type the password")
	}
	fmt.Fprint(cmd.ErrOrStderr(), "Manager password (at least 10 characters): ")
	a, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Fprint(cmd.ErrOrStderr(), "Type it again: ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if string(a) != string(b) {
		return "", errors.New("the passwords do not match")
	}
	return string(a), nil
}

func migrateCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations without starting the server (server must be stopped)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			a, err := open(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Database is up to date: %s\n", cfg.Store.DSN)
			return a.close()
		},
	}
}

func openapiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "openapi",
		Short: "Print the API's OpenAPI description as JSON",
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := web.Spec()
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(append(b, '\n'))
			return err
		},
	}
}
