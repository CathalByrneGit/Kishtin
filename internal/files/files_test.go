package files

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pngBytes is a 1×1 PNG.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestPutOpenVerify(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 1<<20)
	require.NoError(t, err)

	saved, err := s.Put(bytes.NewReader(pngBytes))
	require.NoError(t, err)
	assert.Equal(t, "image/png", saved.MIME)
	assert.True(t, ValidHash(saved.SHA256))
	assert.Equal(t, int64(len(pngBytes)), saved.Size)

	again, err := s.Put(bytes.NewReader(pngBytes))
	require.NoError(t, err)
	assert.Equal(t, saved, again)

	f, err := s.Open(saved.SHA256)
	require.NoError(t, err)
	f.Close()
	require.NoError(t, s.Verify(saved.SHA256))

	p := filepath.Join(dir, saved.SHA256[:2], saved.SHA256)
	require.NoError(t, os.Chmod(p, 0o640))
	require.NoError(t, os.WriteFile(p, []byte("tampered"), 0o640))
	assert.ErrorContains(t, s.Verify(saved.SHA256), "has changed")
}

func TestPutRejects(t *testing.T) {
	s, err := New(t.TempDir(), 32)
	require.NoError(t, err)
	_, err = s.Put(bytes.NewReader([]byte("plain text is not a docket")))
	assert.ErrorIs(t, err, ErrNotAllowed)
	_, err = s.Put(bytes.NewReader(pngBytes))
	assert.ErrorIs(t, err, ErrTooLarge)
	_, err = s.Put(bytes.NewReader(nil))
	assert.ErrorIs(t, err, ErrNotAllowed)
}

func TestOpenMissing(t *testing.T) {
	s, err := New(t.TempDir(), 1<<20)
	require.NoError(t, err)
	_, err = s.Open("../../etc/passwd")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.Open(string(bytes.Repeat([]byte("a"), 64)))
	assert.ErrorIs(t, err, ErrNotFound)
}
