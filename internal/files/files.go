// Package files stores attachments (docket and invoice photos) on local
// disk under their SHA-256, so any change to a stored file can be detected.
package files

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/gabriel-vasile/mimetype"
)

// Allowed lists the file types that can be attached.
var Allowed = []string{"image/jpeg", "image/png", "image/webp", "image/heic", "application/pdf"}

// ErrNotAllowed is returned for a file type that cannot be attached.
var ErrNotAllowed = errors.New("file type not allowed")

// ErrTooLarge is returned when a file is over the size limit.
var ErrTooLarge = errors.New("file too large")

// ErrNotFound is returned when no file has the given hash.
var ErrNotFound = errors.New("file not found")

var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidHash reports whether s looks like a lower-case SHA-256.
func ValidHash(s string) bool { return hashRE.MatchString(s) }

// Store keeps files under root/ab/abcdef….
type Store struct {
	root string
}

// New returns a store rooted at dir, creating it if needed.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o750); err != nil {
		return nil, fmt.Errorf("create file store %s: %w", dir, err)
	}
	return &Store{root: dir}, nil
}

func (s *Store) path(sha string) string { return filepath.Join(s.root, sha[:2], sha) }

// Saved describes a stored file.
type Saved struct {
	SHA256 string
	Size   int64
	MIME   string
}

// Put stores the contents of r and returns its hash. A file over maxSize
// bytes is refused. Storing the same content twice keeps one copy.
func (s *Store) Put(r io.Reader, maxSize int64) (Saved, error) {
	tmp, err := os.CreateTemp(filepath.Join(s.root, "tmp"), "upload-*")
	if err != nil {
		return Saved{}, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, maxSize+1))
	if err != nil {
		return Saved{}, fmt.Errorf("write upload: %w", err)
	}
	if n > maxSize {
		return Saved{}, fmt.Errorf("%w: the limit is %d MB", ErrTooLarge, maxSize>>20)
	}
	if n == 0 {
		return Saved{}, fmt.Errorf("%w: the file is empty", ErrNotAllowed)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return Saved{}, fmt.Errorf("read upload: %w", err)
	}
	mt, err := mimetype.DetectReader(tmp)
	if err != nil {
		return Saved{}, fmt.Errorf("detect file type: %w", err)
	}
	var mime string
	for m := mt; m != nil; m = m.Parent() {
		if slices.Contains(Allowed, m.String()) {
			mime = m.String()
			break
		}
	}
	if mime == "" {
		return Saved{}, fmt.Errorf("%w: %s. Attach a JPEG, PNG, WebP, HEIC photo or a PDF", ErrNotAllowed, mt.String())
	}
	if err := tmp.Sync(); err != nil {
		return Saved{}, fmt.Errorf("sync upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Saved{}, fmt.Errorf("close upload: %w", err)
	}

	sha := hex.EncodeToString(h.Sum(nil))
	dest := s.path(sha)
	if _, err := os.Stat(dest); err == nil {
		return Saved{SHA256: sha, Size: n, MIME: mime}, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return Saved{}, fmt.Errorf("create file folder: %w", err)
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return Saved{}, fmt.Errorf("store file: %w", err)
	}
	if err := os.Chmod(dest, 0o440); err != nil {
		return Saved{}, fmt.Errorf("make file read-only: %w", err)
	}
	return Saved{SHA256: sha, Size: n, MIME: mime}, nil
}

// Open opens a stored file.
func (s *Store) Open(sha string) (*os.File, error) {
	if !ValidHash(sha) {
		return nil, ErrNotFound
	}
	f, err := os.Open(s.path(sha))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

// Verify re-hashes a stored file and reports whether it still matches.
func (s *Store) Verify(sha string) error {
	f, err := s.Open(sha)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("read %s: %w", sha, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != sha {
		return fmt.Errorf("file %s has changed: its content now hashes to %s", sha, got)
	}
	return nil
}

// Mirror makes dest hold every stored file, laid out as the store is, and
// returns how many it holds and how many it added. Files are named by their
// content, so one already there is left alone. A file is hard-linked when
// dest is on the same disk, which takes no extra space, and copied
// otherwise.
func (s *Store) Mirror(dest string) (total, added int, err error) {
	return mirror(s.root, dest)
}

// Restore copies into the store every file in a mirror that the store does
// not have, and returns how many it added.
func (s *Store) Restore(src string) (added int, err error) {
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	_, added, err = mirror(src, s.root)
	return added, err
}

func mirror(src, dest string) (total, added int, err error) {
	dirs, err := os.ReadDir(src)
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", src, err)
	}
	for _, d := range dirs {
		if !d.IsDir() || len(d.Name()) != 2 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(src, d.Name()))
		if err != nil {
			return total, added, fmt.Errorf("read %s: %w", filepath.Join(src, d.Name()), err)
		}
		for _, e := range entries {
			if e.IsDir() || !ValidHash(e.Name()) {
				continue
			}
			total++
			to := filepath.Join(dest, d.Name(), e.Name())
			if _, err := os.Stat(to); err == nil {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
				return total, added, fmt.Errorf("create %s: %w", filepath.Dir(to), err)
			}
			from := filepath.Join(src, d.Name(), e.Name())
			if err := linkOrCopy(from, to); err != nil {
				return total, added, err
			}
			added++
		}
	}
	return total, added, nil
}

func linkOrCopy(from, to string) error {
	if err := os.Link(from, to); err == nil {
		return nil
	}
	in, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("copy %s: %w", from, err)
	}
	defer in.Close()
	tmp := to + ".part"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o440)
	if err != nil {
		return fmt.Errorf("copy to %s: %w", to, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("copy to %s: %w", to, err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("copy to %s: %w", to, err)
	}
	if err := os.Rename(tmp, to); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("copy to %s: %w", to, err)
	}
	return nil
}
