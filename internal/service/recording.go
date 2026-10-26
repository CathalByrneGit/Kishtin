package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Activity kinds. Each is something staff or a screen did that is not a
// HACCP record or an order event, kept for looking back on later.
const (
	ActivityLogin              = "login"
	ActivityLogout             = "logout"
	ActivityScreenConnected    = "screen.connected"
	ActivityScreenDisconnected = "screen.disconnected"
	ActivitySoldOut            = "dish.sold_out"
	ActivityBackOn             = "dish.back_on"
	ActivityHACCPEntered       = "haccp.entered"
	ActivityStaffAnonymised    = "staff.anonymised"
	ActivitySetup              = "setup"
	ActivityDevicePaired       = "device.paired"
)

// ActivityKinds lists every activity kind.
var ActivityKinds = []string{
	ActivityLogin, ActivityLogout, ActivityScreenConnected, ActivityScreenDisconnected,
	ActivitySoldOut, ActivityBackOn, ActivityHACCPEntered, ActivityStaffAnonymised, ActivitySetup, ActivityDevicePaired,
}

// How a HACCP record was entered, kept in the activity log.
const (
	SourceForm    = "form"
	SourceAPI     = "api"
	SourceVoice   = "voice"
	SourceCatchUp = "catch_up"
	SourcePaper   = "paper"
)

type sourceKey struct{}

// withSource marks the HACCP records saved under ctx as entered a given
// way.
func withSource(ctx context.Context, src string) context.Context {
	return context.WithValue(ctx, sourceKey{}, src)
}

// sourceOf is how a record is being entered: as marked, or else from the
// screens (a device key) or the API (a user key).
func sourceOf(ctx context.Context, p *auth.Principal) string {
	if src, ok := ctx.Value(sourceKey{}).(string); ok {
		return src
	}
	if p != nil && p.Method == auth.MethodKey {
		return SourceAPI
	}
	return SourceForm
}

// note appends to the activity log through st. p may be nil for things
// the server does itself.
func (s *Service) note(ctx context.Context, st store.Store, p *auth.Principal, kind, subject string, detail map[string]any) error {
	a := store.Activity{ID: newID(), At: s.clk.Now(), Kind: kind, Subject: subject, Detail: map[string]string{}}
	if p != nil {
		a.UserID, a.UserName, a.DeviceID, a.DeviceName = p.User.ID, p.User.Name, p.Device, p.DeviceName
	}
	for k, v := range detail {
		switch v := v.(type) {
		case time.Time:
			a.Detail[k] = v.UTC().Format(time.RFC3339Nano)
		case string:
			a.Detail[k] = v
		default:
			a.Detail[k] = fmt.Sprint(v)
		}
	}
	if err := st.Activity().AppendActivity(ctx, a); err != nil {
		return fmt.Errorf("log %s activity: %w", kind, err)
	}
	return nil
}

// noteEntered logs how a HACCP record was entered and when its check was
// done, next to when it was saved.
func (s *Service) noteEntered(ctx context.Context, st store.Store, p *auth.Principal, r haccp.Record) error {
	return s.note(ctx, st, p, ActivityHACCPEntered, r.ID, map[string]any{
		"type": string(r.Type), "source": sourceOf(ctx, p), "done_at": r.TakenAt(), "saved_at": r.LoggedAt, "seq": r.Seq,
	})
}

// ScreenConnected logs a screen opening its live stream, and returns a
// function that logs it closing, with how long it stayed connected. A
// screen that drops off the Wi-Fi shows as disconnected once the server
// notices.
func (s *Service) ScreenConnected(ctx context.Context, c Caller) func() {
	p := c.Principal
	if p == nil {
		p = &auth.Principal{Device: c.Key.ID, DeviceName: c.Key.Name}
	}
	from := s.clk.Now()
	_ = s.note(ctx, s.st, p, ActivityScreenConnected, c.Key.ID, nil)
	return func() {
		// The request's context has ended by now.
		_ = s.note(context.WithoutCancel(ctx), s.st, p, ActivityScreenDisconnected, c.Key.ID,
			map[string]any{"seconds": int(s.clk.Now().Sub(from) / time.Second)})
	}
}

// ActivityQuery selects activity to list.
type ActivityQuery struct {
	From, To time.Time
	Kinds    []string
	Limit    int
}

// MaxActivityLimit caps one page of activity.
const MaxActivityLimit = 5000

// Activity lists the activity log, oldest first.
func (s *Service) Activity(ctx context.Context, p *auth.Principal, q ActivityQuery) ([]store.Activity, error) {
	if err := s.allow(p, auth.ActExportHACCP); err != nil {
		return nil, err
	}
	for _, k := range q.Kinds {
		if !contains(ActivityKinds, k) {
			return nil, newErr(KindInvalid, "unknown activity kind %q: use one of %s", k, strings.Join(ActivityKinds, ", "))
		}
	}
	if q.Limit <= 0 || q.Limit > MaxActivityLimit {
		q.Limit = MaxActivityLimit
	}
	return s.st.Activity().ListActivity(ctx, store.ActivityFilter{From: q.From, To: q.To, Kinds: q.Kinds, Limit: q.Limit})
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// audioExt is the file extension for a recording's content type.
func audioExt(contentType string) string {
	switch {
	case strings.Contains(contentType, "webm"):
		return ".webm"
	case strings.Contains(contentType, "ogg"):
		return ".ogg"
	case strings.Contains(contentType, "wav"):
		return ".wav"
	case strings.Contains(contentType, "mp4"), strings.Contains(contentType, "m4a"), strings.Contains(contentType, "aac"):
		return ".m4a"
	}
	return ".audio"
}

// keepAudio saves a voice command's recording when the restaurant keeps
// them, and returns the file name. It keeps nothing when the setting is
// off or there is no folder for recordings.
func (s *Service) keepAudio(ctx context.Context, id string, audio []byte, contentType string) (string, error) {
	if len(audio) == 0 || s.voiceAudioDir == "" {
		return "", nil
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil || sys.Value.VoiceAudioDays <= 0 {
		return "", err
	}
	if err := os.MkdirAll(s.voiceAudioDir, 0o700); err != nil {
		return "", fmt.Errorf("make the voice recordings folder: %w", err)
	}
	name := id + audioExt(contentType)
	if err := os.WriteFile(filepath.Join(s.voiceAudioDir, name), audio, 0o600); err != nil {
		return "", fmt.Errorf("keep the recording: %w", err)
	}
	return name, nil
}

// PurgeVoiceAudio deletes kept voice recordings older than the days set in
// system settings, and every one when keeping them is turned off. The log
// entries stay; only the recordings go.
func (s *Service) PurgeVoiceAudio(ctx context.Context) (int, error) {
	if s.voiceAudioDir == "" {
		return 0, nil
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(s.voiceAudioDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the voice recordings folder: %w", err)
	}
	cutoff := s.clk.Now().AddDate(0, 0, -sys.Value.VoiceAudioDays)
	n := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		if sys.Value.VoiceAudioDays <= 0 || info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(s.voiceAudioDir, e.Name())); err == nil {
				n++
			}
		}
	}
	return n, nil
}

// VoiceRecording returns a kept voice recording. Recordings are staff
// voices, so only a manager with a password can play them.
func (s *Service) VoiceRecording(ctx context.Context, p *auth.Principal, id string) ([]byte, string, error) {
	if err := s.allow(p, auth.ActManageUsers); err != nil {
		return nil, "", err
	}
	e, err := s.st.Voice().GetVoiceLog(ctx, id)
	if err != nil {
		return nil, "", mapStoreErr(err, "voice command")
	}
	if e.AudioFile == "" || s.voiceAudioDir == "" {
		return nil, "", newErr(KindNotFound, "no recording was kept for this command")
	}
	b, err := os.ReadFile(filepath.Join(s.voiceAudioDir, filepath.Base(e.AudioFile)))
	if os.IsNotExist(err) {
		return nil, "", newErr(KindNotFound, "the recording was deleted after the days set in system settings")
	}
	if err != nil {
		return nil, "", fmt.Errorf("read the recording: %w", err)
	}
	ct := map[string]string{".webm": "audio/webm", ".ogg": "audio/ogg", ".wav": "audio/wav", ".m4a": "audio/mp4"}[filepath.Ext(e.AudioFile)]
	if ct == "" {
		ct = "application/octet-stream"
	}
	return b, ct, nil
}
