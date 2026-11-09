//go:build windows

package diskfree

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Of returns the room on the disk that holds path.
func Of(path string) (Space, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Space{}, fmt.Errorf("read free disk space of %s: %w", path, err)
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return Space{}, fmt.Errorf("read free disk space of %s: %w", path, err)
	}
	return Space{Free: free, Total: total}, nil
}
