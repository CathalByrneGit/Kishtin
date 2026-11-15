// Package diskfree reports how much room is left on a disk.
package diskfree

// Space is the room on the disk that holds a path, in bytes.
type Space struct {
	// Free is what the user Kishtin runs as can still write.
	Free uint64
	// Total is the disk's size.
	Total uint64
}
