// Package config loads kishtin.toml.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/integrations/square"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/speech"
)

// Config is the server configuration.
type Config struct {
	// DataDir holds the database, attached files and backups.
	DataDir string `toml:"data_dir"`
	Server  Server `toml:"server"`
	Store   Store  `toml:"store"`
	Backup  Backup `toml:"backup"`
	Files   Files  `toml:"files"`
	Speech  Speech `toml:"speech"`
	Square  Square `toml:"square"`
	// RestaurantName is the starting restaurant name for the screens.
	RestaurantName string `toml:"restaurant_name"`
}

// Server is the HTTP listener.
type Server struct {
	Listen  string `toml:"listen"`
	TLSCert string `toml:"tls_cert"`
	TLSKey  string `toml:"tls_key"`
	// TrustedNetwork allows listening beyond loopback without TLS. Set it
	// only on a private kitchen network you control.
	TrustedNetwork bool `toml:"trusted_network"`
}

// Store selects the storage backend.
type Store struct {
	Driver string `toml:"driver"`
	DSN    string `toml:"dsn"`
}

// Backup controls scheduled backups.
type Backup struct {
	Dir      string   `toml:"dir"`
	Interval Duration `toml:"interval"`
	Keep     int      `toml:"keep"`
}

// Files controls attachments.
type Files struct {
	MaxSizeMB int64 `toml:"max_size_mb"`
}

// Speech points at the local speech sidecars. Both are optional: without
// stt_url, typed voice commands still work; without tts_url, replies are
// shown but not spoken by the server. Both must be on this machine or the
// local network.
type Speech struct {
	// STTURL is a whisper.cpp server, such as http://127.0.0.1:8178.
	STTURL string `toml:"stt_url"`
	// TTSURL is a Piper HTTP server, such as http://127.0.0.1:5000.
	TTSURL  string   `toml:"tts_url"`
	Timeout Duration `toml:"timeout"`
	// WakeWords start a hands-free command. Default: kishtin.
	WakeWords []string `toml:"wake_words"`
}

// Square connects to Square for importing orders. Whether import is on,
// and for which location, is a manager's setting in Kishtin; this only
// holds how to reach Square. The access token belongs in the environment
// (KISHTIN_SQUARE_TOKEN) or this file, never in the database.
type Square struct {
	// Environment is "production" or "sandbox", or a URL for testing
	// against a local stand-in.
	Environment string `toml:"environment"`
	// Token is the access token. KISHTIN_SQUARE_TOKEN overrides it. Without
	// either, a manager can connect Square on the screens instead.
	Token   string   `toml:"token"`
	Timeout Duration `toml:"timeout"`
	// TokenFrom says where Token came from: "environment" or
	// "kishtin.toml".
	TokenFrom string `toml:"-"`
}

// Duration is a time.Duration written as a string such as "6h".
type Duration struct{ time.Duration }

// UnmarshalText parses a duration string.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// MarshalText writes the duration as a string.
func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// DefaultDataDir is ~/.kishtin, or /var/lib/kishtin if there is no home.
func DefaultDataDir() string {
	if v := os.Getenv("KISHTIN_DATA_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/var/lib/kishtin"
	}
	return filepath.Join(home, ".kishtin")
}

// Default returns the default configuration.
func Default() Config {
	return Config{
		DataDir: DefaultDataDir(),
		Server:  Server{Listen: "127.0.0.1:8080"},
		Store:   Store{Driver: "sqlite"},
		Backup:  Backup{Interval: Duration{6 * time.Hour}, Keep: 28},
		Files:   Files{MaxSizeMB: 15},
		Speech:  Speech{Timeout: Duration{20 * time.Second}},
		Square:  Square{Environment: "production", Timeout: Duration{15 * time.Second}},
	}
}

// Load reads path over the defaults. A missing file is not an error when
// path is the default location; the defaults are used.
func Load(path string, mustExist bool) (Config, error) {
	cfg := Default()
	if path != "" {
		md, err := toml.DecodeFile(path, &cfg)
		switch {
		case errors.Is(err, os.ErrNotExist) && !mustExist:
		case err != nil:
			return cfg, fmt.Errorf("read config %s: %w", path, err)
		default:
			if undecoded := md.Undecoded(); len(undecoded) > 0 {
				return cfg, fmt.Errorf("read config %s: unknown setting %q", path, undecoded[0].String())
			}
		}
	}
	if v := os.Getenv("KISHTIN_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("KISHTIN_LISTEN"); v != "" {
		cfg.Server.Listen = v
	}
	if cfg.Square.Token != "" {
		cfg.Square.TokenFrom = "kishtin.toml"
	}
	if v := os.Getenv("KISHTIN_SQUARE_TOKEN"); v != "" {
		cfg.Square.Token, cfg.Square.TokenFrom = v, "environment"
	}
	if v := os.Getenv("KISHTIN_STT_URL"); v != "" {
		cfg.Speech.STTURL = v
	}
	if v := os.Getenv("KISHTIN_TTS_URL"); v != "" {
		cfg.Speech.TTSURL = v
	}
	cfg.fill()
	return cfg, cfg.Validate()
}

func (c *Config) fill() {
	if c.Store.DSN == "" && c.Store.Driver == "sqlite" {
		c.Store.DSN = filepath.Join(c.DataDir, "kishtin.db")
	}
	if c.Backup.Dir == "" {
		c.Backup.Dir = filepath.Join(c.DataDir, "backups")
	}
}

// SystemSettings returns the starting system settings. Changes saved in
// Kishtin (kishtin settings set system) take their place.
func (c Config) SystemSettings() settings.System {
	return settings.System{
		RestaurantName:        c.RestaurantName,
		BackupIntervalMinutes: int64(c.Backup.Interval.Minutes()),
		BackupKeep:            c.Backup.Keep,
		MaxUploadMB:           c.Files.MaxSizeMB,
		PreviewRecords:        50,
	}
}

// SpeechClient returns the client for the speech sidecars.
func (c Config) SpeechClient() (*speech.Client, error) {
	for _, w := range c.Speech.WakeWords {
		if len([]rune(strings.TrimSpace(w))) < 4 {
			return nil, fmt.Errorf("speech.wake_words: %q is too short: use a word of 4 letters or more that is not said in normal kitchen talk", w)
		}
	}
	if c.Speech.Timeout.Duration <= 0 {
		return nil, errors.New("speech.timeout must be positive, such as \"20s\"")
	}
	return speech.New(c.Speech.STTURL, c.Speech.TTSURL, c.Speech.Timeout.Duration)
}

// SquareClient returns the Square client, or nil when no token is set.
func (c Config) SquareClient() (integrations.POSAdapter, error) {
	if c.Square.Token == "" {
		return nil, nil
	}
	return c.NewSquare(c.Square.Environment, c.Square.Token)
}

// NewSquare makes a Square client for an environment and token, with the
// configured timeout. It is used for tokens saved on the screens too. When
// square.environment is a URL (a local stand-in for testing), it is used
// whatever environment was chosen.
func (c Config) NewSquare(environment, token string) (integrations.POSAdapter, error) {
	if strings.HasPrefix(c.Square.Environment, "http://") || strings.HasPrefix(c.Square.Environment, "https://") {
		environment = c.Square.Environment
	}
	if c.Square.Timeout.Duration <= 0 {
		return nil, errors.New("square.timeout must be positive, such as \"15s\"")
	}
	return square.New(environment, token, c.Square.Timeout.Duration)
}

// CredentialsDir holds connector tokens saved on the screens: outside the
// database, so they are never in a backup or export.
func (c Config) CredentialsDir() string { return filepath.Join(c.DataDir, "connectors") }

// FilesDir is where attachments are stored.
func (c Config) FilesDir() string { return filepath.Join(c.DataDir, "files") }

// LockPath is the single-instance lock file.
func (c Config) LockPath() string { return filepath.Join(c.DataDir, "kishtin.lock") }

// Validate checks the configuration.
func (c Config) Validate() error {
	if c.DataDir == "" {
		return errors.New("data_dir is empty")
	}
	switch c.Store.Driver {
	case "sqlite":
	case "postgres", "duckdb":
		return fmt.Errorf("store.driver %q is planned but not built yet: use sqlite", c.Store.Driver)
	default:
		return fmt.Errorf("store.driver %q is not known: use sqlite", c.Store.Driver)
	}
	if (c.Server.TLSCert == "") != (c.Server.TLSKey == "") {
		return errors.New("set both server.tls_cert and server.tls_key, or neither")
	}
	host, _, err := net.SplitHostPort(c.Server.Listen)
	if err != nil {
		return fmt.Errorf("server.listen %q: %w", c.Server.Listen, err)
	}
	if !isLoopback(host) && c.Server.TLSCert == "" && !c.Server.TrustedNetwork {
		return fmt.Errorf("server.listen %q is reachable from the network: set server.tls_cert and server.tls_key, or set server.trusted_network = true if this is a private kitchen network you control", c.Server.Listen)
	}
	if c.Backup.Keep < 0 {
		return errors.New("backup.keep cannot be negative")
	}
	if c.Backup.Interval.Duration < 0 {
		return errors.New("backup.interval cannot be negative")
	}
	if _, err := c.SpeechClient(); err != nil {
		return err
	}
	if _, err := c.SquareClient(); err != nil {
		return err
	}
	if err := c.SystemSettings().Validate(); err != nil {
		return fmt.Errorf("backup or files settings: %w", err)
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
