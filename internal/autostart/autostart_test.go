package autostart

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fake records commands and answers them from a table.
type fake struct {
	calls   []string
	answers map[string]string
	fail    map[string]bool
	// taskXML is the Windows task file as schtasks saw it.
	taskXML []byte
}

func (f *fake) run(_ context.Context, name string, args ...string) ([]byte, error) {
	c := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, c)
	if name == "schtasks" && len(args) > 4 && args[0] == "/Create" {
		f.taskXML, _ = os.ReadFile(args[4])
	}
	for prefix, failed := range f.fail {
		if failed && strings.HasPrefix(c, prefix) {
			return nil, errors.New("failed")
		}
	}
	return []byte(f.answers[c]), nil
}

var prog = Program{Executable: "/home/synth/.local/bin/kishtind", Args: []string{"--config", "/srv/kitchen & co/kishtin.toml"}, LogFile: "/home/synth/.kishtin/kishtind.log"}

func TestLinux(t *testing.T) {
	ctx := context.Background()
	f := &fake{answers: map[string]string{}, fail: map[string]bool{}}
	m := Manager{OS: "linux", Home: t.TempDir(), User: "synth", Run: f.run}

	st, err := m.Status(ctx)
	require.NoError(t, err)
	assert.True(t, st.Supported)
	assert.False(t, st.On)
	assert.Contains(t, st.Note, "sudo loginctl enable-linger synth", "says how to start at power-on")

	_, err = m.Enable(ctx, prog, true)
	require.NoError(t, err)
	unit, err := os.ReadFile(filepath.Join(m.Home, ".config/systemd/user/kishtind.service"))
	require.NoError(t, err)
	assert.Contains(t, string(unit), `ExecStart="/home/synth/.local/bin/kishtind" "--config" "/srv/kitchen & co/kishtin.toml"`)
	assert.Contains(t, string(unit), "Restart=on-failure")
	assert.Contains(t, f.calls, "systemctl --user enable kishtind.service")
	assert.Contains(t, f.calls, "loginctl enable-linger synth")
	assert.Equal(t, "systemctl --user start kishtind.service", f.calls[len(f.calls)-4], "started now, then status")
	assert.Equal(t, "systemctl --user show-environment", f.calls[0], "checks systemd runs for this user before writing anything")

	f.answers["systemctl --user is-enabled kishtind.service"] = "enabled\n"
	f.answers["loginctl show-user synth --property=Linger"] = "Linger=yes\n"
	st, err = m.Status(ctx)
	require.NoError(t, err)
	assert.True(t, st.On)
	assert.Contains(t, st.Note, "when the computer starts")

	f.calls = nil
	_, err = m.Enable(ctx, prog, false)
	require.NoError(t, err)
	assert.NotContains(t, f.calls, "systemctl --user start kishtind.service", "not started while a server runs")
	assert.NotContains(t, f.calls, "loginctl enable-linger synth", "already lingering")

	f.calls = nil
	require.NoError(t, m.Stop(ctx))
	assert.Contains(t, f.calls, "systemctl --user stop kishtind.service")

	f.answers["systemctl --user is-enabled kishtind.service"] = "disabled\n"
	f.calls = nil
	st, err = m.Disable(ctx)
	require.NoError(t, err)
	assert.False(t, st.On)
	assert.NoFileExists(t, filepath.Join(m.Home, ".config/systemd/user/kishtind.service"))
	assert.NotContains(t, f.calls, "systemctl --user stop kishtind.service", "a running Kishtin keeps running")
}

func TestLinuxWithoutSystemd(t *testing.T) {
	f := &fake{fail: map[string]bool{"systemctl": true}}
	m := Manager{OS: "linux", Home: t.TempDir(), User: "synth", Run: f.run}
	st, err := m.Status(context.Background())
	require.NoError(t, err)
	assert.False(t, st.Supported)
	_, err = m.Enable(context.Background(), prog, false)
	assert.ErrorIs(t, err, ErrUnsupported)
	assert.NoFileExists(t, filepath.Join(m.Home, ".config/systemd/user/kishtind.service"), "nothing written")
}

func TestMac(t *testing.T) {
	ctx := context.Background()
	f := &fake{}
	m := Manager{OS: "darwin", Home: t.TempDir(), User: "synth", UID: 501, Run: f.run}
	st, err := m.Enable(ctx, prog, true)
	require.NoError(t, err)
	assert.True(t, st.On)
	assert.Contains(t, st.Note, "Automatically log in")
	path := filepath.Join(m.Home, "Library/LaunchAgents/kishtin.kishtind.plist")
	assert.Equal(t, path, st.Where)
	plist, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, xml.Unmarshal(plist, new(struct{})), "well-formed")
	assert.Contains(t, string(plist), "<string>/srv/kitchen &amp; co/kishtin.toml</string>")
	assert.Contains(t, string(plist), "<key>RunAtLoad</key>\n  <true/>")
	assert.Contains(t, string(plist), "/home/synth/.kishtin/kishtind.log")
	assert.Equal(t, []string{"launchctl bootout gui/501/kishtin.kishtind", "launchctl bootstrap gui/501 " + path}, f.calls)

	f.calls = nil
	require.NoError(t, m.Stop(ctx))
	assert.Equal(t, []string{"launchctl bootout gui/501/kishtin.kishtind"}, f.calls)

	f.calls = nil
	st, err = m.Disable(ctx)
	require.NoError(t, err)
	assert.False(t, st.On)
	assert.Empty(t, f.calls, "a running Kishtin keeps running")
}

func TestWindows(t *testing.T) {
	ctx := context.Background()
	f := &fake{fail: map[string]bool{"schtasks /Query": true}}
	m := Manager{OS: "windows", User: `KITCHEN-PC\synth`, Run: f.run}
	exe := Program{Executable: `C:\Users\synth\AppData\Local\Programs\kishtin\bin\kishtind.exe`, Args: []string{"--config", `C:\Kishtin data\kishtin.toml`}}
	st, err := m.Status(ctx)
	require.NoError(t, err)
	assert.False(t, st.On)

	f.fail["schtasks /Query"] = false
	st, err = m.Enable(ctx, exe, true)
	require.NoError(t, err)
	assert.True(t, st.On)
	assert.Contains(t, st.Note, "leave that window open")
	require.Len(t, f.calls, 4)
	assert.True(t, strings.HasPrefix(f.calls[1], "schtasks /Create /TN Kishtin /XML "))
	assert.Equal(t, "schtasks /Run /TN Kishtin", f.calls[2])

	require.True(t, len(f.taskXML) > 2 && f.taskXML[0] == 0xFF && f.taskXML[1] == 0xFE, "UTF-16 with a byte-order mark, as schtasks wants")
	task := TaskXML(m.User, exe)
	assert.Contains(t, task, "<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>", "never stopped after 72 hours")
	assert.Contains(t, task, "<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>")
	assert.Contains(t, task, `<UserId>KITCHEN-PC\synth</UserId>`)
	assert.Contains(t, task, `<Arguments>--config &quot;C:\Kishtin data\kishtin.toml&quot;</Arguments>`)
	require.NoError(t, xml.Unmarshal([]byte(strings.Replace(task, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)), new(struct{})))

	f.calls = nil
	require.NoError(t, m.Stop(ctx))
	assert.Contains(t, f.calls, "schtasks /End /TN Kishtin")

	f.calls = nil
	_, err = m.Disable(ctx)
	require.NoError(t, err)
	assert.Contains(t, f.calls, "schtasks /Delete /TN Kishtin /F")
}

func TestEnableNeedsAnInstalledProgram(t *testing.T) {
	m := Manager{OS: "linux", Home: t.TempDir(), Run: (&fake{}).run}
	_, err := m.Enable(context.Background(), Program{Executable: "kishtind"}, false)
	assert.Error(t, err)
	_, err = m.Enable(context.Background(), Program{Executable: "/tmp/go-build123/b001/exe/kishtind"}, false)
	assert.ErrorContains(t, err, "temporary build")
	_, err = Manager{OS: "plan9"}.Enable(context.Background(), prog, false)
	assert.ErrorIs(t, err, ErrUnsupported)
}
