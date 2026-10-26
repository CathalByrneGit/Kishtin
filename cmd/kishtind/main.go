// Command kishtind is the Kishtin server: it serves the API and runs
// scheduled backups. It also has offline commands for first-time setup.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/cathalbyrnegit/kishtin/frontend"
	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/config"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/legal"
	"github.com/cathalbyrnegit/kishtin/internal/localtls"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
	"github.com/cathalbyrnegit/kishtin/internal/web"
)

// version is the release, set at build time with
// -ldflags "-X main.version=v1.2.3"; "dev" for a local build.
var version = "dev"

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
		Version:       version,
		Short:         "Kishtin server",
		Long:          "Kishtin, from cistin, Irish for kitchen.\n\nkishtind serves the Kishtin API and runs scheduled backups.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context(), cfgPath)
		},
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "", "path to kishtin.toml (default $KISHTIN_CONFIG, then <data dir>/kishtin.toml)")
	root.AddCommand(setupCmd(&cfgPath), migrateCmd(&cfgPath), openapiCmd(), licensesCmd())
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
	// ca and certs serve tablets over HTTPS with Kishtin's own certificate
	// authority, when the server runs with tablets on.
	ca    *localtls.CA
	certs *localtls.Server
}

// open locks the data directory and opens the store. Only one process can
// hold the lock, so offline commands cannot run while the server does.
func open(ctx context.Context, cfg config.Config, serving bool) (*app, error) {
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
	sq, err := cfg.SquareClient()
	if err != nil {
		return fail(err)
	}
	su, err := cfg.SumUpClient()
	if err != nil {
		return fail(err)
	}
	model, err := cfg.AIClient()
	if err != nil {
		return fail(err)
	}
	var tablets *service.TabletNetwork
	var ca *localtls.CA
	var certs *localtls.Server
	if serving {
		tablets, ca, certs, err = tabletsFor(cfg)
		if err != nil {
			return fail(err)
		}
	}
	svc := service.New(service.Options{
		Tablets: tablets,
		Tills: map[string]service.TillConnection{
			service.TillSquare: {Adapter: sq, Source: cfg.Square.TokenFrom, New: cfg.NewSquare},
			service.TillSumUp:  {Adapter: su, Source: cfg.SumUp.TokenFrom, New: cfg.NewSumUp},
		},
		Credentials: &integrations.CredentialStore{Dir: cfg.CredentialsDir()},
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
	return &app{cfg: cfg, lock: lock, store: st, svc: svc, ca: ca, certs: certs}, nil
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

	a, err := open(ctx, cfg, true)
	if err != nil {
		return err
	}
	defer func() {
		if err := a.close(); err != nil {
			log.Error("close data folder", "err", err)
		}
	}()

	h, err := web.New(a.svc, log, frontend.Files())
	if err != nil {
		return err
	}
	needed, err := a.svc.SetupNeeded(ctx)
	if err != nil {
		return err
	}
	if needed {
		if err := startSetup(cfg, h, log); err != nil {
			return err
		}
	}
	newServer := func(addr string, handler http.Handler) *http.Server {
		return &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       2 * time.Minute,
			WriteTimeout:      2 * time.Minute,
			IdleTimeout:       2 * time.Minute,
			// Requests end when the server stops, which closes live streams.
			BaseContext: func(net.Listener) context.Context { return ctx },
		}
	}
	servers := []*http.Server{newServer(cfg.Server.Listen, h)}
	if a.ca != nil {
		_, httpsPort, _ := net.SplitHostPort(cfg.Server.TabletListen)
		h.SetTablets(&web.Tablets{CA: a.ca.DER(), MobileConfig: a.ca.MobileConfig(a.ca.Cert.Subject.CommonName), Fingerprint: a.ca.Fingerprint(), HTTPSPort: httpsPort})
		// Plain HTTP answers every network address, so a tablet can open
		// the join page before it trusts Kishtin; other devices get only
		// that page. This computer still gets everything.
		if host, port, _ := net.SplitHostPort(cfg.Server.Listen); isLoopbackHost(host) {
			servers[0] = newServer(net.JoinHostPort("", port), h.ForNetwork())
		}
		tablet := newServer(cfg.Server.TabletListen, h)
		tablet.TLSConfig = &tls.Config{GetCertificate: a.certs.GetCertificate, MinVersion: tls.VersionTLS12}
		servers = append(servers, tablet)
		go refreshCertificate(ctx, a.certs, cfg.TabletIPs, log)
	}

	go a.svc.RunBackups(ctx, log)
	go a.svc.RunTills(ctx, log)
	go a.svc.RunPlanRecorder(ctx, log)
	go cleanSessions(ctx, a.svc, log)

	errc := make(chan error, len(servers))
	go func() {
		ownCert := cfg.Server.TLSCert != ""
		log.Info("Kishtin is listening", "version", version, "addr", servers[0].Addr, "tls", ownCert, "data", cfg.DataDir)
		if ownCert {
			errc <- servers[0].ListenAndServeTLS(cfg.Server.TLSCert, cfg.Server.TLSKey)
		} else {
			errc <- servers[0].ListenAndServe()
		}
	}()
	if len(servers) > 1 {
		go func() {
			_, port, _ := net.SplitHostPort(cfg.Server.TabletListen)
			var at []string
			for _, ip := range cfg.TabletIPs() {
				at = append(at, "https://"+net.JoinHostPort(ip.String(), port))
			}
			log.Info("Tablets connect over HTTPS: pair them with a QR code in Setup › Staff and access › Device keys", "addr", servers[1].Addr, "at", strings.Join(at, " "), "certificate", a.ca.Fingerprint())
			if err := servers[1].ListenAndServeTLS("", ""); err != nil {
				errc <- fmt.Errorf("serve tablets on %s: %w (set server.tablet_listen to a free port, or server.tablets = false)", cfg.Server.TabletListen, err)
			}
		}()
	}

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
	var errs []error
	for _, srv := range servers {
		errs = append(errs, srv.Shutdown(shutdownCtx))
	}
	return errors.Join(errs...)
}

// tabletsFor sets up how tablets reach Kishtin. With tablets on and no
// certificate in kishtin.toml, Kishtin makes its own certificate authority
// in the data folder (once) and a server certificate for this computer's
// local addresses.
func tabletsFor(cfg config.Config) (*service.TabletNetwork, *localtls.CA, *localtls.Server, error) {
	if !cfg.Server.Tablets {
		return nil, nil, nil, nil
	}
	if !cfg.LocalTLS() {
		// kishtin.toml names a certificate: listen serves tablets itself,
		// if it is on the network.
		host, port, _ := net.SplitHostPort(cfg.Server.Listen)
		if isLoopbackHost(host) {
			return nil, nil, nil, nil
		}
		return &service.TabletNetwork{Addresses: cfg.TabletIPs, JoinPort: port, HTTPSPort: port}, nil, nil, nil
	}
	hostname, _ := os.Hostname()
	ca, err := localtls.LoadOrCreateCA(cfg.TLSDir(), "Kishtin on "+hostname, hostname, time.Now())
	if err != nil {
		return nil, nil, nil, err
	}
	certs, err := localtls.NewServer(ca, hostname, cfg.TabletIPs(), time.Now)
	if err != nil {
		return nil, nil, nil, err
	}
	_, joinPort, _ := net.SplitHostPort(cfg.Server.Listen)
	_, httpsPort, _ := net.SplitHostPort(cfg.Server.TabletListen)
	return &service.TabletNetwork{Addresses: cfg.TabletIPs, JoinPort: joinPort, HTTPSPort: httpsPort, Fingerprint: ca.Fingerprint()}, ca, certs, nil
}

// refreshCertificate issues a new server certificate when the computer's
// addresses change (a router can hand out a new one) or it nears its end.
func refreshCertificate(ctx context.Context, certs *localtls.Server, addresses func() []net.IP, log *slog.Logger) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			issued, err := certs.Refresh(addresses())
			if err != nil {
				log.Warn("renew tablet certificate", "err", err)
			} else if issued {
				log.Info("issued a new tablet certificate for this computer's addresses")
			}
		}
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// startSetup lets the screens set up a new install. On the server machine
// itself anyone can; another device needs a one-time code, shown here and
// kept in the data folder until setup is done.
func startSetup(cfg config.Config, h *web.Server, log *slog.Logger) error {
	code, err := auth.NewCode()
	if err != nil {
		return fmt.Errorf("make setup code: %w", err)
	}
	path := filepath.Join(cfg.DataDir, "setup-code.txt")
	if err := os.WriteFile(path, []byte(code+"\n"), 0o600); err != nil {
		return fmt.Errorf("save setup code: %w", err)
	}
	h.SetSetupCode(code, func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warn("remove setup code", "err", err)
		}
		log.Info("Kishtin is set up")
	})
	scheme := "http"
	if cfg.Server.TLSCert != "" {
		scheme = "https"
	}
	log.Info("Kishtin needs setting up: open it in a browser on this computer", "url", scheme+"://"+browserAddr(cfg.Server.Listen)+"/")
	log.Info("From another device, give this setup code", "code", code, "also_in", path)
	return nil
}

// browserAddr is the listen address as a browser on this machine reaches
// it.
func browserAddr(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
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

func licensesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "licenses",
		Short: "Show Kishtin's licence and the licences of the software it includes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), legal.Notices())
			return err
		},
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
			a, err := open(cmd.Context(), cfg, false)
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
			a, err := open(cmd.Context(), cfg, false)
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
