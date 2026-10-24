// Package settings holds the system settings and a readable diff for any
// settings change. It does no I/O.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Kind names a group of settings that is versioned as a whole.
type Kind string

const (
	// KindGuidance is the reference guidance limits are measured against.
	KindGuidance Kind = "guidance"
	// KindRules is the food-safety rules.
	KindRules Kind = "rules"
	// KindLists is the managed lists of fridges, categories and tasks.
	KindLists Kind = "lists"
	// KindSystem is the system settings.
	KindSystem Kind = "system"
	// KindScheduler is the cook scheduler's settings.
	KindScheduler Kind = "scheduler"
)

// Kinds lists every kind.
var Kinds = []Kind{KindGuidance, KindRules, KindLists, KindSystem, KindScheduler}

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return slices.Contains(Kinds, k) }

// System holds settings for the server itself. The backup folder, listen
// address and TLS stay in kishtin.toml: they decide where files go and who
// can connect, so they are not changeable over the API.
type System struct {
	// RestaurantName is shown at the top of every screen and on exports.
	RestaurantName string `json:"restaurant_name"`
	// BackupIntervalMinutes is how often to back up. 0 turns scheduled
	// backups off.
	BackupIntervalMinutes int64 `json:"backup_interval_minutes"`
	// BackupKeep is how many backups to keep. 0 keeps every backup.
	BackupKeep int `json:"backup_keep"`
	// MaxUploadMB is the largest docket or invoice file accepted.
	MaxUploadMB int64 `json:"max_upload_mb"`
	// PreviewRecords is how many recent records a limit preview re-scores.
	PreviewRecords int `json:"preview_records"`
}

// Upper bounds for system settings.
const (
	MaxUploadMBLimit    = 100
	PreviewRecordsLimit = 500
)

// Validate checks the system settings.
func (s System) Validate() error {
	var errs []error
	if len([]rune(s.RestaurantName)) > 80 {
		errs = append(errs, errors.New("restaurant_name is at most 80 characters"))
	}
	if s.BackupIntervalMinutes != 0 && s.BackupIntervalMinutes < 15 {
		errs = append(errs, errors.New("backup_interval_minutes must be 0 (off) or at least 15"))
	}
	if s.BackupIntervalMinutes < 0 {
		errs = append(errs, errors.New("backup_interval_minutes cannot be negative"))
	}
	if s.BackupKeep < 0 {
		errs = append(errs, errors.New("backup_keep cannot be negative"))
	}
	if s.MaxUploadMB < 1 || s.MaxUploadMB > MaxUploadMBLimit {
		errs = append(errs, fmt.Errorf("max_upload_mb must be between 1 and %d", MaxUploadMBLimit))
	}
	if s.PreviewRecords < 1 || s.PreviewRecords > PreviewRecordsLimit {
		errs = append(errs, fmt.Errorf("preview_records must be between 1 and %d", PreviewRecordsLimit))
	}
	return errors.Join(errs...)
}

// Changes lists, in plain words, what differs between two settings values
// of the same type. Lists of objects with a "check", "name" or "task" field
// (or a "type" and "text" pair) are compared by those fields, so adding one
// entry reads as one change.
func Changes(before, after any) ([]string, error) {
	a, err := flatten(before)
	if err != nil {
		return nil, err
	}
	b, err := flatten(after)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	slices.Sort(sorted)
	out := []string{}
	for _, k := range sorted {
		av, inA := a[k]
		bv, inB := b[k]
		switch {
		case !inA:
			out = append(out, fmt.Sprintf("%s: added %s", k, bv))
		case !inB:
			out = append(out, fmt.Sprintf("%s: removed (was %s)", k, av))
		case av != bv:
			out = append(out, fmt.Sprintf("%s: %s → %s", k, av, bv))
		}
	}
	return out, nil
}

func flatten(v any) (map[string]string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		return nil, fmt.Errorf("decode settings: %w", err)
	}
	out := map[string]string{}
	walk("", tree, out)
	return out, nil
}

var listKeys = []string{"check", "name", "task"}

func walk(path string, v any, out map[string]string) {
	join := func(k string) string {
		if path == "" {
			return k
		}
		return path + "." + k
	}
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			walk(join(k), child, out)
		}
	case []any:
		for i, child := range t {
			key := fmt.Sprintf("[%d]", i)
			switch c := child.(type) {
			case map[string]any:
				typ, _ := c["type"].(string)
				text, _ := c["text"].(string)
				if typ != "" && text != "" {
					key = fmt.Sprintf("[%s: %s]", typ, text)
					break
				}
				for _, lk := range listKeys {
					if s, ok := c[lk].(string); ok && s != "" {
						key = fmt.Sprintf("[%s]", s)
						break
					}
				}
			case string:
				key = fmt.Sprintf("[%s]", c)
			}
			walk(path+key, child, out)
		}
	default:
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(t)
		out[path] = strings.TrimSpace(buf.String())
	}
}
