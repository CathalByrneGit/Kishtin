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
	root    string
	maxSize int64
}

// New returns a store rooted at dir, creating it if needed.
func New(dir string, maxSize int64) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o750); err != nil {
		return nil, fmt.Errorf("create file store %s: %w", dir, err)
	}
	return &Store{root: dir, maxSize: maxSize}, nil
}

func (s *Store) path(sha string) string { return filepath.Join(s.root, sha[:2], sha) }

// Saved describes a stored file.
type Saved struct {
	SHA256 string
	Size   int64
	MIME   string
}

// Put stores the contents of r and returns its hash. Storing the same
// content twice keeps one copy.
func (s *Store) Put(r io.Reader) (Saved, error) {
	tmp, err := os.CreateTemp(filepath.Join(s.root, "tmp"), "upload-*")
	if err != nil {
		return Saved{}, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, s.maxSize+1))
	if err != nil {
		return Saved{}, fmt.Errorf("write upload: %w", err)
	}
	if n > s.maxSize {
		return Saved{}, fmt.Errorf("%w: the limit is %d MB", ErrTooLarge, s.maxSize>>20)
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
