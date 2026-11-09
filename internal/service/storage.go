package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/diskfree"
)

// ActivityPlansPurged is logged when kept plans past their days are
// removed.
const ActivityPlansPurged = "plans.purged"

// StorageConfig says where Kishtin keeps its data, so it can say how much
// room is left.
type StorageConfig struct {
	// DataDir is Kishtin's data folder; Database the database file.
	DataDir  string
	Database string
	// Free reads the room on the disk holding a folder. Nil uses the
	// computer's own.
	Free func(path string) (diskfree.Space, error)
}

// Disk space levels.
const (
	StorageOK = "ok"
	// StorageLow is when there is room for now, but not for long.
	StorageLow = "low"
	// StorageCritical is when Kishtin may soon be unable to save.
	StorageCritical = "critical"

	lowBytes      = 2 << 30
	criticalBytes = 500 << 20
)

// Storage is how much room Kishtin's data takes and how much is left.
type Storage struct {
	// Level is "ok", "low" or "critical"; "unknown" when the computer would
	// not say.
	Level string `json:"level"`
	// Warning says, in plain words, what to do when the level is not ok.
	Warning string `json:"warning,omitempty"`
	// FreeBytes and TotalBytes are the disk that holds the data folder.
	FreeBytes  uint64 `json:"free_bytes"`
	TotalBytes uint64 `json:"total_bytes"`
	// DatabaseBytes, FilesBytes and BackupsBytes are what Kishtin's own
	// data takes. Photos kept with backups on the same disk share their
	// space with the originals and are counted once, under files.
	DatabaseBytes int64 `json:"database_bytes"`
	FilesBytes    int64 `json:"files_bytes"`
	BackupsBytes  int64 `json:"backups_bytes"`
	// PlanSnapshots is how many kept scheduler plans there are.
	PlanSnapshots int `json:"plan_snapshots"`
}

// Storage reports the room Kishtin's data takes and what is left. Anyone
// logged in can see it, so whoever notices can tell a manager.
func (s *Service) Storage(ctx context.Context, p *auth.Principal) (Storage, error) {
	if err := s.allow(p, auth.ActViewHACCP); err != nil {
		return Storage{}, err
	}
	out := s.storageLevel()
	out.DatabaseBytes = fileSize(s.storage.Database) + fileSize(s.storage.Database+"-wal")
	if s.storage.DataDir != "" {
		out.FilesBytes = dirSize(filepath.Join(s.storage.DataDir, "files"), "")
	}
	if s.backups.Dir != "" {
		out.BackupsBytes = dirSize(s.backups.Dir, filepath.Join(s.backups.Dir, BackupFilesDir))
	}
	n, err := s.st.Activity().CountPlanSnapshotsBefore(ctx, s.clk.Now().AddDate(1, 0, 0))
	if err != nil {
		return out, err
	}
	out.PlanSnapshots = n
	return out, nil
}

// storageLevel reads only the free space, which is quick enough for every
// load of Today.
func (s *Service) storageLevel() Storage {
	out := Storage{Level: "unknown"}
	if s.storage.DataDir == "" {
		return out
	}
	free := s.storage.Free
	if free == nil {
		free = diskfree.Of
	}
	sp, err := free(s.storage.DataDir)
	if err != nil {
		return out
	}
	out.FreeBytes, out.TotalBytes = sp.Free, sp.Total
	// A backup is a full copy of the database, so there must be room for
	// one more.
	need := uint64(max(fileSize(s.storage.Database), 0)) * 2
	switch {
	case sp.Free < criticalBytes || sp.Free < need:
		out.Level = StorageCritical
		out.Warning = fmt.Sprintf("The Kishtin computer's disk is almost full (%s left). Kishtin may soon be unable to save records or make backups. Tell a manager now: free up space on the computer, or keep fewer backups in Settings.", humanBytes(sp.Free))
	case sp.Free < lowBytes:
		out.Level = StorageLow
		out.Warning = fmt.Sprintf("The Kishtin computer's disk is getting full (%s left). Tell a manager: free up space on the computer, or keep fewer backups in Settings.", humanBytes(sp.Free))
	default:
		out.Level = StorageOK
	}
	return out
}

func fileSize(path string) int64 {
	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// dirSize adds up the files under dir, leaving out the folder skip.
func dirSize(dir, skip string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip != "" && path == skip {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil {
			n += info.Size()
		}
		return nil
	})
	return n
}

func humanBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d KB", max(n>>10, 1))
}

// PurgePlanSnapshots removes kept scheduler plans older than the days set
// in system settings (plan_snapshot_days). 0 keeps them all.
func (s *Service) PurgePlanSnapshots(ctx context.Context) (int, error) {
	sys, err := s.System().current(ctx, s.st)
	if err != nil || sys.Value.PlanSnapshotDays <= 0 {
		return 0, err
	}
	before := s.clk.Now().AddDate(0, 0, -sys.Value.PlanSnapshotDays)
	n, err := s.st.Activity().PurgePlanSnapshotsBefore(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("remove old plans: %w", err)
	}
	if n == 0 {
		return 0, nil
	}
	return n, s.note(ctx, s.st, nil, ActivityPlansPurged, "", map[string]any{"plans": n, "before": before, "days": sys.Value.PlanSnapshotDays})
}
