// Package autostart makes kishtind start by itself when the computer
// starts, using the computer's own way of doing that: a Task Scheduler task
// on Windows, a LaunchAgent on macOS, and a systemd user service on Linux.
// Each starts Kishtin as the user who turned it on, with their data folder.
package autostart

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Runner runs a system command (schtasks, launchctl, systemctl, loginctl)
// and returns what it printed. Tests replace it.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Exec runs commands for real.
func Exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return out, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// Names Kishtin uses for its start-up entry.
const (
	TaskName   = "Kishtin"
	AgentLabel = "kishtin.kishtind"
	UnitName   = "kishtind.service"
)

// Program is what starts: kishtind and its arguments.
type Program struct {
	Executable string
	Args       []string
	// LogFile is where macOS writes Kishtin's output (systemd keeps it in
	// the journal; on Windows it shows in Kishtin's window).
	LogFile string
}

// Status is whether Kishtin starts by itself, and how.
type Status struct {
	// Supported is false on a computer Kishtin cannot set this up on.
	Supported bool `json:"supported"`
	On        bool `json:"on"`
	// Method is how: "Windows Task Scheduler", "macOS LaunchAgent" or
	// "systemd user service".
	Method string `json:"method,omitempty"`
	// Where is the task's name or the file that holds the entry.
	Where string `json:"where,omitempty"`
	// Note says plainly when Kishtin starts once this is on, and what it
	// does not do; or, when not Supported, why.
	Note string `json:"note"`
}

// Manager turns start-up on and off for one user.
type Manager struct {
	OS   string // runtime.GOOS
	Home string // the user's home folder
	// User is the user's name; on Windows with its domain (PC\name).
	User string
	UID  int // macOS only
	Run  Runner
}

// ErrUnsupported is a system Kishtin cannot set start-up on.
var ErrUnsupported = errors.New("this system cannot start Kishtin by itself: start kishtind from the system's own start-up settings")

// Status reports whether Kishtin starts by itself.
func (m Manager) Status(ctx context.Context) (Status, error) {
	switch m.OS {
	case "windows":
		st := Status{Supported: true, Method: "Windows Task Scheduler", Where: "Task " + TaskName, Note: m.note()}
		_, err := m.Run(ctx, "schtasks", "/Query", "/TN", TaskName)
		st.On = err == nil
		return st, nil
	case "darwin":
		st := Status{Supported: true, Method: "macOS LaunchAgent", Where: m.agentPath(), Note: m.note()}
		st.On = exists(st.Where)
		return st, nil
	case "linux":
		st := Status{Method: "systemd user service", Where: m.unitPath()}
		if _, err := m.Run(ctx, "systemctl", "--user", "show-environment"); err != nil {
			st.Note = "systemd is not running for this user, so Kishtin cannot set itself to start here. Start kishtind from the system's own start-up settings."
			return st, nil
		}
		st.Supported = true
		out, _ := m.Run(ctx, "systemctl", "--user", "is-enabled", UnitName)
		st.On = exists(st.Where) && strings.TrimSpace(string(out)) == "enabled"
		st.Note = m.note()
		if !m.lingering(ctx) {
			st.Note = "Kishtin starts in the background when " + m.User + " logs in, and again if it stops by mistake. To start it at power-on without anyone logging in, an administrator can run: sudo loginctl enable-linger " + m.User
		}
		return st, nil
	default:
		return Status{Note: ErrUnsupported.Error()}, nil
	}
}

func (m Manager) note() string {
	switch m.OS {
	case "windows":
		return "Kishtin starts when " + m.User + " logs in to Windows, in its own window: leave that window open, or minimise it. For Kishtin to start after a power cut with nobody there, set Windows to log in to this account by itself."
	case "darwin":
		return "Kishtin starts in the background when " + m.User + " logs in, and again if it stops by mistake. For Kishtin to start after a power cut with nobody there, turn on automatic login: System Settings, Users & Groups, Automatically log in as."
	default:
		return "Kishtin starts in the background when the computer starts, and again if it stops by mistake."
	}
}

// Enable makes Kishtin start by itself. With startNow, it also starts it
// now in the background; only do that when no server is running.
func (m Manager) Enable(ctx context.Context, p Program, startNow bool) (Status, error) {
	if !m.absolute(p.Executable) {
		return Status{}, fmt.Errorf("start-up needs kishtind's full path, not %q", p.Executable)
	}
	if strings.Contains(p.Executable, "go-build") {
		return Status{}, errors.New("this kishtind is a temporary build (go run): install it first, then turn start-up on")
	}
	var err error
	switch m.OS {
	case "windows":
		err = m.enableWindows(ctx, p, startNow)
	case "darwin":
		err = m.enableMac(ctx, p, startNow)
	case "linux":
		err = m.enableLinux(ctx, p, startNow)
	default:
		return Status{}, ErrUnsupported
	}
	if err != nil {
		return Status{}, err
	}
	return m.Status(ctx)
}

// Disable stops Kishtin starting by itself. A running Kishtin keeps
// running until it is stopped or the computer restarts.
func (m Manager) Disable(ctx context.Context) (Status, error) {
	switch m.OS {
	case "windows":
		if st, _ := m.Status(ctx); st.On {
			if _, err := m.Run(ctx, "schtasks", "/Delete", "/TN", TaskName, "/F"); err != nil {
				return Status{}, fmt.Errorf("remove the start-up task: %w", err)
			}
		}
	case "darwin":
		if err := os.Remove(m.agentPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Status{}, fmt.Errorf("remove the LaunchAgent: %w", err)
		}
	case "linux":
		_, _ = m.Run(ctx, "systemctl", "--user", "disable", UnitName)
		if err := os.Remove(m.unitPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Status{}, fmt.Errorf("remove the systemd service: %w", err)
		}
		_, _ = m.Run(ctx, "systemctl", "--user", "daemon-reload")
	default:
		return Status{}, ErrUnsupported
	}
	return m.Status(ctx)
}

// Stop stops a Kishtin that start-up started, for example before an
// update. It does nothing to a Kishtin started by hand, or when none runs.
// Start-up stays on.
func (m Manager) Stop(ctx context.Context) error {
	switch m.OS {
	case "windows":
		if st, _ := m.Status(ctx); st.On {
			// schtasks says so when the task is not running; that's fine.
			_, _ = m.Run(ctx, "schtasks", "/End", "/TN", TaskName)
		}
	case "darwin":
		// bootout fails when the agent is not loaded; that's fine. It loads
		// again at the next login.
		_, _ = m.Run(ctx, "launchctl", "bootout", "gui/"+strconv.Itoa(m.UID)+"/"+AgentLabel)
	case "linux":
		if _, err := m.Run(ctx, "systemctl", "--user", "show-environment"); err != nil {
			return nil
		}
		if _, err := m.Run(ctx, "systemctl", "--user", "stop", UnitName); err != nil {
			return fmt.Errorf("stop Kishtin: %w", err)
		}
	default:
		return ErrUnsupported
	}
	return nil
}

// Windows: a Task Scheduler task that runs at logon. It is made from XML
// because schtasks' own flags stop a task after 72 hours and don't start
// it on battery.

func (m Manager) enableWindows(ctx context.Context, p Program, startNow bool) error {
	f, err := os.CreateTemp("", "kishtin-task-*.xml")
	if err != nil {
		return fmt.Errorf("write the start-up task: %w", err)
	}
	defer os.Remove(f.Name())
	_, err = f.Write(utf16le(TaskXML(m.User, p)))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("write the start-up task: %w", err)
	}
	if _, err := m.Run(ctx, "schtasks", "/Create", "/TN", TaskName, "/XML", f.Name(), "/F"); err != nil {
		return fmt.Errorf("add the start-up task: %w", err)
	}
	if startNow {
		if _, err := m.Run(ctx, "schtasks", "/Run", "/TN", TaskName); err != nil {
			return fmt.Errorf("start Kishtin: %w", err)
		}
	}
	return nil
}

// TaskXML is the Task Scheduler task for a user.
func TaskXML(user string, p Program) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Starts Kishtin, the kitchen server, when ` + xmlText(user) + ` logs in.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + xmlText(user) + `</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + xmlText(user) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>999</Count>
    </RestartOnFailure>
    <StartWhenAvailable>true</StartWhenAvailable>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + xmlText(p.Executable) + `</Command>
`)
	if len(p.Args) > 0 {
		b.WriteString("      <Arguments>" + xmlText(windowsArgs(p.Args)) + "</Arguments>\n")
	}
	b.WriteString(`    </Exec>
  </Actions>
</Task>
`)
	return b.String()
}

// windowsArgs quotes arguments the way Windows programs read them.
func windowsArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\"") {
			out[i] = a
			continue
		}
		out[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
	}
	return strings.Join(out, " ")
}

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2, 2+2*len(u))
	b[0], b[1] = 0xFF, 0xFE
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

// macOS: a LaunchAgent, started at login and kept running.

func (m Manager) agentPath() string {
	return filepath.Join(m.Home, "Library", "LaunchAgents", AgentLabel+".plist")
}

func (m Manager) enableMac(ctx context.Context, p Program, startNow bool) error {
	path := m.agentPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(AgentPlist(p)), 0o644); err != nil {
		return fmt.Errorf("write the LaunchAgent: %w", err)
	}
	if startNow {
		domain := "gui/" + strconv.Itoa(m.UID)
		// Unload an older copy first; it is not running, or startNow
		// would not be set.
		_, _ = m.Run(ctx, "launchctl", "bootout", domain+"/"+AgentLabel)
		if _, err := m.Run(ctx, "launchctl", "bootstrap", domain, path); err != nil {
			return fmt.Errorf("start Kishtin: %w", err)
		}
	}
	return nil
}

// AgentPlist is the LaunchAgent for a program.
func AgentPlist(p Program) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + AgentLabel + `</string>
  <key>ProgramArguments</key>
  <array>
`)
	for _, a := range append([]string{p.Executable}, p.Args...) {
		b.WriteString("    <string>" + xmlText(a) + "</string>\n")
	}
	b.WriteString(`  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key>
    <false/>
  </dict>
  <key>ThrottleInterval</key>
  <integer>10</integer>
`)
	if p.LogFile != "" {
		b.WriteString("  <key>StandardOutPath</key>\n  <string>" + xmlText(p.LogFile) + "</string>\n")
		b.WriteString("  <key>StandardErrorPath</key>\n  <string>" + xmlText(p.LogFile) + "</string>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

// Linux: a systemd user service. With lingering on, user services start at
// boot without anyone logging in.

func (m Manager) unitPath() string {
	return filepath.Join(m.Home, ".config", "systemd", "user", UnitName)
}

func (m Manager) enableLinux(ctx context.Context, p Program, startNow bool) error {
	if _, err := m.Run(ctx, "systemctl", "--user", "show-environment"); err != nil {
		return ErrUnsupported
	}
	path := m.unitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(Unit(p)), 0o644); err != nil {
		return fmt.Errorf("write the systemd service: %w", err)
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", UnitName}} {
		if _, err := m.Run(ctx, "systemctl", append([]string{"--user"}, args...)...); err != nil {
			return fmt.Errorf("turn on the systemd service: %w", err)
		}
	}
	// Lingering starts user services at boot. Many systems let a user turn
	// it on for themselves; where they don't, Status says how.
	if !m.lingering(ctx) {
		_, _ = m.Run(ctx, "loginctl", "enable-linger", m.User)
	}
	if startNow {
		if _, err := m.Run(ctx, "systemctl", "--user", "start", UnitName); err != nil {
			return fmt.Errorf("start Kishtin: %w", err)
		}
	}
	return nil
}

func (m Manager) lingering(ctx context.Context) bool {
	out, err := m.Run(ctx, "loginctl", "show-user", m.User, "--property=Linger")
	return err == nil && bytes.Contains(out, []byte("Linger=yes"))
}

// Unit is the systemd user service for a program.
func Unit(p Program) string {
	args := make([]string, 0, len(p.Args)+1)
	for _, a := range append([]string{p.Executable}, p.Args...) {
		a = strings.ReplaceAll(a, `\`, `\\`)
		a = strings.ReplaceAll(a, `"`, `\"`)
		a = strings.ReplaceAll(a, "%", "%%")
		args = append(args, `"`+a+`"`)
	}
	return `[Unit]
Description=Kishtin kitchen server
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=` + strings.Join(args, " ") + `
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`
}

func xmlText(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// absolute reports whether a path is a full path on the manager's system,
// which need not be the one the code runs on.
func (m Manager) absolute(p string) bool {
	if m.OS == "windows" {
		return strings.HasPrefix(p, `\\`) || len(p) > 2 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
	}
	return strings.HasPrefix(p, "/")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
