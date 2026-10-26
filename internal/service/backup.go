package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
}

const backupPrefix, backupExt = "kishtin-", ".db"

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
	name := backupPrefix + now.Format("20060102T150405Z") + backupExt
	path := filepath.Join(s.backups.Dir, name)
	if err := b.Backup(ctx, path); err != nil {
		return Backup{}, err
	}
	out := Backup{Name: name, Path: path, TakenAt: now}

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
		out.Records, out.HeadHash, out.Verified = rep.Chain.Checked, rep.Chain.HeadHash, true
	}

	sum, size, err := hashFile(path)
	if err != nil {
		return out, err
	}
	out.SHA256, out.Size = sum, size
	if err := os.WriteFile(path+".sha256", []byte(sum+"  "+name+"\n"), 0o640); err != nil {
		return out, fmt.Errorf("write backup checksum: %w", err)
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
	entries, err := os.ReadDir(s.backups.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, backupPrefix) && strings.HasSuffix(n, backupExt) {
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
	for len(names) > keep {
		old := filepath.Join(s.backups.Dir, names[0])
		for _, p := range []string{old, old + ".sha256", old + "-wal", old + "-shm"} {
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
	names, err := s.backupNames()
	if err != nil {
		return nil, err
	}
	out := []Backup{}
	for _, n := range slices.Backward(names) {
		path := filepath.Join(s.backups.Dir, n)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		b := Backup{Name: n, Path: path, Size: info.Size(), TakenAt: info.ModTime().UTC()}
		if t, err := time.Parse("20060102T150405Z", strings.TrimSuffix(strings.TrimPrefix(n, backupPrefix), backupExt)); err == nil {
			b.TakenAt = t
		}
		if sum, err := os.ReadFile(path + ".sha256"); err == nil {
			b.SHA256, _, _ = strings.Cut(string(sum), " ")
		}
		out = append(out, b)
	}
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
