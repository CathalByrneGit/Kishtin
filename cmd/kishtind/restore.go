package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/config"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
)

// backupBeforeUpgrade copies the database into the backup folder before a
// new version of Kishtin changes it, so a failed upgrade can be undone with
// kishtind restore.
func backupBeforeUpgrade(cfg config.Config) func(ctx context.Context, from int64, backup func(dest string) error) error {
	return func(_ context.Context, from int64, backup func(dest string) error) error {
		if cfg.Backup.Dir == "" {
			return nil
		}
		if err := os.MkdirAll(cfg.Backup.Dir, 0o750); err != nil {
			return fmt.Errorf("create backup folder: %w", err)
		}
		path := service.SafetyBackupPath(cfg.Backup.Dir, service.BackupBeforeUpgrade, time.Now())
		if err := backup(path); err != nil {
			return err
		}
		if _, err := service.SealBackup(path); err != nil {
			return err
		}
		slog.Info("backed up the database before upgrading it", "path", path, "from_version", from)
		return service.PruneSafetyBackups(cfg.Backup.Dir)
	}
}

// restorePlan is what a restore would do, and what it did.
type restorePlan struct {
	Backup   string    `json:"backup"`
	TakenAt  time.Time `json:"taken_at"`
	Records  int       `json:"records"`
	HeadHash string    `json:"head_hash"`
	// Checksum is true when the backup matched its .sha256 file; false when
	// it had none.
	Checksum bool `json:"checksum"`
	// Current describes the database being replaced, when there is one.
	CurrentRecords *int `json:"current_records,omitempty"`
	// Kept is where the replaced database was put.
	Kept string `json:"kept,omitempty"`
	// Files is how many photos and files were put back.
	Files    int  `json:"files"`
	Restored bool `json:"restored"`
}

func restoreCmd(cfgPath *string) *cobra.Command {
	var yes, asJSON bool
	cmd := &cobra.Command{
		Use:   "restore [BACKUP]",
		Short: "Put a backup back as Kishtin's database (on the Kishtin computer, with Kishtin stopped)",
		Long: "restore replaces Kishtin's database with a backup, and puts back any photos and files kept with it.\n" +
			"With no BACKUP it uses the newest scheduled backup in the backup folder; otherwise give a file name\n" +
			"from that folder, or a path to a backup file.\n\n" +
			"It checks the backup first (its checksum and its HACCP chain), shows what will happen, and asks.\n" +
			"The database it replaces is kept in the backup folder as before-restore-….db, so a restore can be undone.\n" +
			"Anything recorded after the backup was made is not in it. Stop Kishtin first (kishtind startup stop,\n" +
			"or close its window); start it again afterwards. Till connections are not in backups: connect again.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
				return fmt.Errorf("create data folder %s: %w", cfg.DataDir, err)
			}
			lock := flock.New(cfg.LockPath())
			ok, err := lock.TryLock()
			if err != nil {
				return fmt.Errorf("lock data folder: %w", err)
			}
			if !ok {
				return fmt.Errorf("another Kishtin process (usually Kishtin is running) is using %s: stop it first (kishtind startup stop, or close its window), then run restore again", cfg.DataDir)
			}
			defer func() { _ = lock.Unlock() }()

			var name string
			if len(args) == 1 {
				name = args[0]
			}
			plan, src, err := planRestore(cmd.Context(), cfg, name)
			if err != nil {
				return err
			}
			errOut := cmd.ErrOrStderr()
			if !asJSON || !yes {
				describeRestore(errOut, cfg, plan)
			}
			if !yes {
				if asJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
				}
				fmt.Fprint(errOut, "Restore this backup? Type yes: ")
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if !strings.EqualFold(strings.TrimSpace(line), "yes") {
					fmt.Fprintln(errOut, "Nothing changed.")
					return nil
				}
			}
			if err := runRestore(cmd.Context(), cfg, src, &plan); err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Done: restored %d HACCP records and %d photos and files.\n", plan.Records, plan.Files)
			if plan.Kept != "" {
				fmt.Fprintf(out, "The database it replaced is kept as %s.\n", plan.Kept)
			}
			fmt.Fprintln(out, "Start Kishtin again (kishtind, or kishtind startup on), then run Exports › Verify.")
			fmt.Fprintln(out, "Till connections are not in backups: connect Square or SumUp again in Setup › Connections.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "restore without asking")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the plan (or, with --yes, the result) as JSON")
	return cmd
}

// planRestore finds the backup, checks it, and says what restoring it would
// do.
func planRestore(ctx context.Context, cfg config.Config, name string) (restorePlan, string, error) {
	var plan restorePlan
	src := name
	switch {
	case name == "":
		all, err := service.ListBackupFiles(cfg.Backup.Dir)
		if err != nil {
			return plan, "", err
		}
		for _, b := range all {
			if b.Kind == service.BackupScheduled {
				src = b.Path
				break
			}
		}
		if src == "" {
			return plan, "", fmt.Errorf("there are no backups in %s. Give the path to a backup file: kishtind restore /path/to/kishtin-….db", cfg.Backup.Dir)
		}
	case !strings.ContainsAny(name, `/\`):
		src = filepath.Join(cfg.Backup.Dir, name)
	}
	info, err := os.Stat(src)
	if err != nil {
		return plan, "", fmt.Errorf("no backup at %s", src)
	}
	if same, _ := sameFile(src, cfg.Store.DSN); same {
		return plan, "", errors.New("that is Kishtin's own database, not a backup")
	}
	plan.Backup, plan.TakenAt = src, info.ModTime().UTC()
	for _, b := range mustList(filepath.Dir(src)) {
		if b.Path == src {
			plan.TakenAt = b.TakenAt
		}
	}
	if plan.Checksum, err = service.CheckBackupFile(src); err != nil {
		return plan, "", err
	}
	st, err := sqlite.OpenSnapshot(ctx, src)
	if err != nil {
		return plan, "", fmt.Errorf("%s is not a Kishtin backup: %w", src, err)
	}
	plan.Records, plan.HeadHash, err = service.CheckBackup(ctx, st)
	_ = st.Close()
	if err != nil {
		return plan, "", fmt.Errorf("%s failed its check and was not restored: %w", src, err)
	}
	if _, err := os.Stat(cfg.Store.DSN); err == nil {
		if cur, err := sqlite.OpenSnapshot(ctx, cfg.Store.DSN); err == nil {
			if n, _, err := service.CheckBackup(ctx, cur); err == nil {
				plan.CurrentRecords = &n
			}
			_ = cur.Close()
		}
	}
	return plan, src, nil
}

func mustList(dir string) []service.Backup {
	all, _ := service.ListBackupFiles(dir)
	return all
}

func sameFile(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}

func describeRestore(w io.Writer, cfg config.Config, p restorePlan) {
	fmt.Fprintf(w, "Backup: %s\n", p.Backup)
	fmt.Fprintf(w, "  made %s, %d HACCP records, chain head %s\n", p.TakenAt.Local().Format("Mon 2 Jan 2006 15:04"), p.Records, shortHash(p.HeadHash))
	if p.Checksum {
		fmt.Fprintln(w, "  matches its checksum, and its HACCP chain checks out.")
	} else {
		fmt.Fprintln(w, "  has no checksum file to check against; its HACCP chain checks out.")
	}
	if p.CurrentRecords == nil {
		fmt.Fprintf(w, "It becomes Kishtin's database at %s.\n", cfg.Store.DSN)
	} else {
		fmt.Fprintf(w, "It replaces Kishtin's database at %s, which has %d HACCP records.\n", cfg.Store.DSN, *p.CurrentRecords)
		if *p.CurrentRecords > p.Records {
			fmt.Fprintf(w, "WARNING: the database now has %d more records than the backup. They will not be in Kishtin after the restore.\n", *p.CurrentRecords-p.Records)
		}
		fmt.Fprintf(w, "The database it replaces is kept in %s, so this can be undone.\n", cfg.Backup.Dir)
	}
	fmt.Fprintln(w, "Orders, staff, settings and everything else go back to how they were when the backup was made.")
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12] + "…"
	}
	if h == "" {
		return "(no records)"
	}
	return h
}

// runRestore keeps the current database, puts the backup in its place and
// puts back the photos and files kept beside the backup.
func runRestore(ctx context.Context, cfg config.Config, src string, plan *restorePlan) error {
	db := cfg.Store.DSN
	if _, err := os.Stat(db); err == nil {
		if err := os.MkdirAll(cfg.Backup.Dir, 0o750); err != nil {
			return fmt.Errorf("create backup folder: %w", err)
		}
		kept := service.SafetyBackupPath(cfg.Backup.Dir, service.BackupBeforeRestore, time.Now())
		if err := sqlite.SnapshotTo(ctx, db, kept); err != nil {
			return fmt.Errorf("keep the current database (nothing was changed): %w", err)
		}
		if _, err := service.SealBackup(kept); err != nil {
			return err
		}
		plan.Kept = kept
	}
	// Copy beside the database first, so a failed copy changes nothing.
	tmp := db + ".restoring"
	if err := copyFile(src, tmp); err != nil {
		return fmt.Errorf("copy the backup (nothing was changed): %w", err)
	}
	for _, p := range []string{db + "-wal", db + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(tmp)
			return fmt.Errorf("remove %s (nothing was changed): %w", p, err)
		}
	}
	if err := os.Rename(tmp, db); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("put the backup in place: %w", err)
	}
	fs, err := files.New(cfg.FilesDir())
	if err != nil {
		return err
	}
	plan.Files, err = fs.Restore(filepath.Join(filepath.Dir(src), service.BackupFilesDir))
	if err != nil {
		return fmt.Errorf("the database was restored, but putting back photos and files failed: %w", err)
	}
	plan.Restored = true
	return service.PruneSafetyBackups(cfg.Backup.Dir)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
