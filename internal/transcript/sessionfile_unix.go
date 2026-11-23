//go:build !windows

package transcript

import (
	"os"
	"syscall"
)

// openNoFollow is OpenSession's open on a Unix machine: O_NOFOLLOW refuses a link swapped in, O_NONBLOCK keeps a FIFO
// swapped in from hanging the open, and the regular-file check after it refuses that FIFO.
func openNoFollow(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// openRootFile opens inside an os.Root; O_NONBLOCK keeps a FIFO swapped in after the type check from hanging.
func openRootFile(root *os.Root, rel string) (*os.File, error) {
	return root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
