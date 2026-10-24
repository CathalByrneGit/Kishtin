package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// UploadFile stores an attachment, such as a docket photo, and returns its
// metadata. Attach it to a delivery record with its SHA-256.
func (s *Service) UploadFile(ctx context.Context, p *auth.Principal, r io.Reader, name string) (store.FileMeta, error) {
	if err := allow(p, auth.ActLogHACCP); err != nil {
		return store.FileMeta{}, err
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return store.FileMeta{}, err
	}
	saved, err := s.files.Put(r, sys.Value.MaxUploadMB<<20)
	switch {
	case errors.Is(err, files.ErrNotAllowed), errors.Is(err, files.ErrTooLarge):
		return store.FileMeta{}, newErr(KindInvalid, "%s", err)
	case err != nil:
		return store.FileMeta{}, err
	}
	m := store.FileMeta{
		SHA256: saved.SHA256, MIME: saved.MIME, Size: saved.Size,
		OriginalName: cleanName(name), UploadedBy: p.User.ID, UploadedAt: s.clk.Now(),
	}
	if err := s.st.Files().PutFile(ctx, m); err != nil {
		return store.FileMeta{}, err
	}
	return s.st.Files().GetFile(ctx, saved.SHA256)
}

func cleanName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	if len(name) > 200 {
		name = strings.ToValidUTF8(name[:200], "")
	}
	return name
}

// OpenFile opens an attachment for reading.
func (s *Service) OpenFile(ctx context.Context, p *auth.Principal, sha string) (store.FileMeta, *os.File, error) {
	if err := allow(p, auth.ActViewHACCP); err != nil {
		return store.FileMeta{}, nil, err
	}
	sha = strings.ToLower(sha)
	m, err := s.st.Files().GetFile(ctx, sha)
	if err != nil {
		return store.FileMeta{}, nil, mapStoreErr(err, "file")
	}
	f, err := s.files.Open(sha)
	if errors.Is(err, files.ErrNotFound) {
		return store.FileMeta{}, nil, newErr(KindNotFound, "file %s is recorded but missing from disk: run a verify", sha)
	}
	if err != nil {
		return store.FileMeta{}, nil, err
	}
	return m, f, nil
}
