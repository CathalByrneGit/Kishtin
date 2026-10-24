// Package config loads kishtin.toml.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the server configuration.
type Config struct {
	// DataDir holds the database, attached files and backups.
	DataDir string `toml:"data_dir"`
	Server  Server `toml:"server"`
	Store   Store  `toml:"store"`
	Backup  Backup `toml:"backup"`
	Files   Files  `toml:"files"`
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
	if c.Files.MaxSizeMB <= 0 {
		return errors.New("files.max_size_mb must be positive")
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
