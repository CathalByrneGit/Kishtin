package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// BackupConfig controls database backups.
type BackupConfig struct {
	// Dir is where backups are written.
	Dir string
	// Open opens a backup file as a store, so it can be verified.
	Open func(ctx context.Context, path string) (store.Store, error)
}

// Backup describes one backup file.
type Backup struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	SHA256  string    `json:"sha256"`
	TakenAt time.Time `json:"taken_at"`
	// Records and HeadHash come from verifying the backup's own chain.
	Records  int    `json:"records"`
	HeadHash string `json:"head_hash"`
	Verified bool   `json:"verified"`
	// Checked is true when Kishtin checked the backup's chain when it was
	// made, so Records, HeadHash and Verified say what it found.
	Checked bool `json:"checked"`
	// Kind is why the backup was made: "scheduled" (on the schedule, or
	// with Back up now), "before_upgrade" (before a new version of Kishtin
	// changed the database) or "before_restore" (the database a restore
	// replaced).
	Kind string `json:"kind"`
	// Files is how many attached photos and files the backup folder holds
	// after this backup; FilesAdded how many this backup added.
	Files      int `json:"files"`
	FilesAdded int `json:"files_added"`
}

const backupPrefix, backupExt, infoExt = "kishtin-", ".db", ".check.json"

// backupInfo is what checking a backup found, kept beside it.
type backupInfo struct {
	Records  int    `json:"records"`
	HeadHash string `json:"head_hash"`
	Verified bool   `json:"verified"`
}

// Kinds of backup, and the start of each kind's file name.
const (
	BackupScheduled     = "scheduled"
	BackupBeforeUpgrade = "before_upgrade"
	BackupBeforeRestore = "before_restore"

	beforeUpgradePrefix = "before-upgrade-"
	beforeRestorePrefix = "before-restore-"
	// safetyKeep is how many before-upgrade and before-restore backups are
	// kept of each.
	safetyKeep = 3
	// BackupFilesDir is the folder inside the backup folder that holds a
	// copy of every attached photo and file.
	BackupFilesDir  = "files"
	backupTimestamp = "20060102T150405Z"
)

func backupKind(name string) string {
	switch {
	case strings.HasPrefix(name, beforeUpgradePrefix):
		return BackupBeforeUpgrade
	case strings.HasPrefix(name, beforeRestorePrefix):
		return BackupBeforeRestore
	}
	return BackupScheduled
}

// SafetyBackupPath names a backup made before an upgrade or a restore.
func SafetyBackupPath(dir, kind string, now time.Time) string {
	prefix := beforeUpgradePrefix
	if kind == BackupBeforeRestore {
		prefix = beforeRestorePrefix
	}
	base := filepath.Join(dir, prefix+now.UTC().Format(backupTimestamp))
	path := base + backupExt
	// Two in the same second get different names.
	for i := 2; ; i++ {
		if _, err := os.Stat(path); err != nil {
			return path
		}
		path = fmt.Sprintf("%s-%d%s", base, i, backupExt)
	}
}

// SealBackup writes the .sha256 file next to a backup and returns its
// checksum.
func SealBackup(path string) (string, error) {
	sum, _, err := hashFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path+".sha256", []byte(sum+"  "+filepath.Base(path)+"\n"), 0o640); err != nil {
		return "", fmt.Errorf("write backup checksum: %w", err)
	}
	return sum, nil
}

// PruneSafetyBackups keeps the newest few before-upgrade and before-restore
// backups in dir.
func PruneSafetyBackups(dir string) error {
	for _, prefix := range []string{beforeUpgradePrefix, beforeRestorePrefix} {
		names, err := namesWith(dir, prefix)
		if err != nil {
			return err
		}
		if err := removeOldest(dir, names, safetyKeep); err != nil {
			return err
		}
	}
	return nil
}

// CheckBackupFile checks a backup against its .sha256 file, when it has
// one, and reports whether there was one to check.
func CheckBackupFile(path string) (bool, error) {
	want, err := os.ReadFile(path + ".sha256")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s.sha256: %w", path, err)
	}
	got, _, err := hashFile(path)
	if err != nil {
		return true, err
	}
	if w, _, _ := strings.Cut(strings.TrimSpace(string(want)), " "); w != got {
		return true, fmt.Errorf("%s does not match its checksum: the file has changed or is damaged (expected %s, got %s)", path, w, got)
	}
	return true, nil
}

// BackupNow writes a backup, verifies its HACCP chain, writes a .sha256
// file next to it, and removes backups beyond the keep count.
func (s *Service) BackupNow(ctx context.Context, p *auth.Principal) (Backup, error) {
	if err := s.allow(p, auth.ActBackup); err != nil {
		return Backup{}, err
	}
	return s.backup(ctx)
}

func (s *Service) backup(ctx context.Context) (Backup, error) {
	b, ok := s.st.(store.Backuper)
	if !ok {
		return Backup{}, newErr(KindInvalid, "this storage backend does not support backups from Kishtin; use the database's own tools")
	}
	if s.backups.Dir == "" {
		return Backup{}, newErr(KindInvalid, "no backup folder is set: set backup.dir in kishtin.toml")
	}
	if err := os.MkdirAll(s.backups.Dir, 0o750); err != nil {
		return Backup{}, fmt.Errorf("create backup folder: %w", err)
	}
	now := s.clk.Now()
	name := backupPrefix + now.Format(backupTimestamp) + backupExt
	path := filepath.Join(s.backups.Dir, name)
	if err := b.Backup(ctx, path); err != nil {
		return Backup{}, err
	}
	out := Backup{Name: name, Path: path, TakenAt: now, Kind: BackupScheduled}

	if s.backups.Open != nil {
		copyStore, err := s.backups.Open(ctx, path)
		if err != nil {
			return out, fmt.Errorf("open backup to verify it: %w", err)
		}
		rep, err := s.verifyStore(ctx, copyStore, false)
		closeErr := copyStore.Close()
		if err != nil {
			return out, fmt.Errorf("verify backup: %w", err)
		}
		if closeErr != nil {
			return out, fmt.Errorf("close backup: %w", closeErr)
		}
		if !rep.Chain.OK {
			return out, fmt.Errorf("backup %s failed its HACCP chain check at record %d: %s", name, rep.Chain.Break.Seq, rep.Chain.Break.Problem)
		}
		out.Records, out.HeadHash, out.Verified, out.Checked = rep.Chain.Checked, rep.Chain.HeadHash, true, true
	}

	sum, size, err := hashFile(path)
	if err != nil {
		return out, err
	}
	out.SHA256, out.Size = sum, size
	if err := os.WriteFile(path+".sha256", []byte(sum+"  "+name+"\n"), 0o640); err != nil {
		return out, fmt.Errorf("write backup checksum: %w", err)
	}
	// What the check found is kept beside the backup, for the list.
	if info, err := json.Marshal(backupInfo{Records: out.Records, HeadHash: out.HeadHash, Verified: out.Verified}); err == nil {
		if err := os.WriteFile(path+infoExt, info, 0o640); err != nil {
			return out, fmt.Errorf("write backup check result: %w", err)
		}
	}
	// Photos and files are kept beside the database backups, so copying the
	// backup folder takes everything.
	if s.files != nil {
		out.Files, out.FilesAdded, err = s.files.Mirror(filepath.Join(s.backups.Dir, BackupFilesDir))
		if err != nil {
			return out, fmt.Errorf("back up attached files: %w", err)
		}
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return out, err
	}
	if err := s.prune(sys.Value.BackupKeep); err != nil {
		return out, err
	}
	return out, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func (s *Service) backupNames() ([]string, error) {
	return namesWith(s.backups.Dir, backupPrefix)
}

// namesWith lists the backups in dir whose names start with prefix, oldest
// first.
func namesWith(dir, prefix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, prefix) && strings.HasSuffix(n, backupExt) {
			names = append(names, n)
		}
	}
	slices.Sort(names) // timestamped names sort oldest first
	return names, nil
}

func (s *Service) prune(keep int) error {
	if keep <= 0 {
		return nil
	}
	names, err := s.backupNames()
	if err != nil {
		return err
	}
	return removeOldest(s.backups.Dir, names, keep)
}

func removeOldest(dir string, names []string, keep int) error {
	for len(names) > keep {
		old := filepath.Join(dir, names[0])
		for _, p := range []string{old, old + ".sha256", old + infoExt, old + "-wal", old + "-shm"} {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove old backup: %w", err)
			}
		}
		names = names[1:]
	}
	return nil
}

// ListBackups lists backup files, newest first.
func (s *Service) ListBackups(ctx context.Context, p *auth.Principal) ([]Backup, error) {
	if err := s.allow(p, auth.ActBackup); err != nil {
		return nil, err
	}
	return ListBackupFiles(s.backups.Dir)
}

// ListBackupFiles lists the backups in a folder, of every kind, newest
// first.
func ListBackupFiles(dir string) ([]Backup, error) {
	out := []Backup{}
	for _, prefix := range []string{backupPrefix, beforeUpgradePrefix, beforeRestorePrefix} {
		names, err := namesWith(dir, prefix)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			path := filepath.Join(dir, n)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			b := Backup{Name: n, Path: path, Size: info.Size(), TakenAt: info.ModTime().UTC(), Kind: backupKind(n)}
			if t, err := time.Parse(backupTimestamp, strings.TrimSuffix(strings.TrimPrefix(n, prefix), backupExt)); err == nil {
				b.TakenAt = t
			}
			if sum, err := os.ReadFile(path + ".sha256"); err == nil {
				b.SHA256, _, _ = strings.Cut(string(sum), " ")
			}
			if raw, err := os.ReadFile(path + infoExt); err == nil {
				var info backupInfo
				if json.Unmarshal(raw, &info) == nil {
					b.Records, b.HeadHash, b.Verified, b.Checked = info.Records, info.HeadHash, info.Verified, true
				}
			}
			out = append(out, b)
		}
	}
	slices.SortStableFunc(out, func(a, b Backup) int { return b.TakenAt.Compare(a.TakenAt) })
	return out, nil
}

// RunBackups backs up on the schedule in the system settings until ctx is
// done. It re-reads the schedule after every backup and every minute while
// backups are off, so a change takes effect without a restart. Failures
// are logged and retried at the next interval.
func (s *Service) RunBackups(ctx context.Context, log *slog.Logger) {
	last := s.clk.Now()
	for {
		wait := time.Minute
		sys, err := s.System().current(ctx, s.st)
		if err != nil {
			log.Error("read backup schedule", "err", err)
		} else if every := time.Duration(sys.Value.BackupIntervalMinutes) * time.Minute; every > 0 {
			if due := last.Add(every).Sub(s.clk.Now()); due < wait {
				wait = max(due, 0)
			}
			if wait == 0 {
				b, err := s.backup(ctx)
				last = s.clk.Now()
				if err != nil {
					log.Error("scheduled backup failed", "err", err)
				} else {
					log.Info("backup written", "path", b.Path, "records", b.Records, "sha256", b.SHA256)
				}
				continue
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// CheckBackup re-checks a backup's HACCP hash chain and returns how many
// records it holds and the hash of the last one.
func CheckBackup(ctx context.Context, st store.Store) (records int, head string, err error) {
	var v haccp.ChainVerifier
	err = st.HACCP().EachRecord(ctx, func(r haccp.Record) error {
		if !v.Add(r) {
			return errStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return 0, "", fmt.Errorf("read records: %w", err)
	}
	rep := v.Report()
	if !rep.OK {
		return rep.Checked, rep.HeadHash, fmt.Errorf("the HACCP chain breaks at record %d: %s", rep.Break.Seq, rep.Break.Problem)
	}
	return rep.Checked, rep.HeadHash, nil
}
