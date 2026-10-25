package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "kishtin.toml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("KISHTIN_DATA_DIR", "/tmp/kishtin-test")
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"), false)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8080", cfg.Server.Listen)
	assert.Equal(t, "/tmp/kishtin-test/kishtin.db", cfg.Store.DSN)
	assert.Equal(t, "/tmp/kishtin-test/backups", cfg.Backup.Dir)
	assert.Equal(t, 6*time.Hour, cfg.Backup.Interval.Duration)

	_, err = Load(filepath.Join(t.TempDir(), "missing.toml"), true)
	assert.Error(t, err)
}

func TestLoadFile(t *testing.T) {
	p := write(t, `
data_dir = "/srv/kishtin"
[server]
listen = "0.0.0.0:8443"
tls_cert = "/etc/kishtin/cert.pem"
tls_key = "/etc/kishtin/key.pem"
[backup]
interval = "1h"
keep = 5
`)
	cfg, err := Load(p, true)
	require.NoError(t, err)
	assert.Equal(t, "/srv/kishtin", cfg.DataDir)
	assert.Equal(t, time.Hour, cfg.Backup.Interval.Duration)
	assert.Equal(t, 5, cfg.Backup.Keep)
}

func TestValidate(t *testing.T) {
	bad := map[string]string{
		"network without TLS": "[server]\nlisten = \"0.0.0.0:8080\"\n",
		"unknown setting":     "colour = \"red\"\n",
		"postgres not built":  "[store]\ndriver = \"postgres\"\n",
		"half TLS":            "[server]\ntls_cert = \"c.pem\"\n",
		"bad listen":          "[server]\nlisten = \"8080\"\n",
		"cloud speech":        "[speech]\nstt_url = \"https://speech.example.com\"\n",
		"zero speech timeout": "[speech]\ntimeout = \"0s\"\n",
		"bad square env":      "[square]\ntoken = \"x\"\nenvironment = \"staging\"\n",
	}
	for name, body := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, body), true)
			assert.Error(t, err)
		})
	}
	ok := "[server]\nlisten = \"192.168.1.10:8080\"\ntrusted_network = true\n[speech]\nstt_url = \"http://127.0.0.1:8178\"\ntts_url = \"http://kitchen-pc.local:5000\"\n"
	cfg, err := Load(write(t, ok), true)
	require.NoError(t, err)
	sp, err := cfg.SpeechClient()
	require.NoError(t, err)
	assert.True(t, sp.CanListen())
	assert.True(t, sp.CanSpeak())
	pos, err := cfg.SquareClient()
	require.NoError(t, err)
	assert.Nil(t, pos, "no token, no Square")
	t.Setenv("KISHTIN_SQUARE_TOKEN", "synthetic-token")
	cfg, err = Load(write(t, ok), true)
	require.NoError(t, err)
	pos, err = cfg.SquareClient()
	require.NoError(t, err)
	assert.Equal(t, "square", pos.Name())
	assert.Equal(t, "environment", cfg.Square.TokenFrom)
}
