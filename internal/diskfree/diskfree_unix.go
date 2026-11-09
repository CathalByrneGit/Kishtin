//go:build !windows

package diskfree

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Of returns the room on the disk that holds path.
func Of(path string) (Space, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Space{}, fmt.Errorf("read free disk space of %s: %w", path, err)
	}
	bs := uint64(st.Bsize)                                                         //nolint:gosec,unconvert // block size is positive; its type differs by system
	return Space{Free: uint64(st.Bavail) * bs, Total: uint64(st.Blocks) * bs}, nil //nolint:unconvert // types differ by system
}
