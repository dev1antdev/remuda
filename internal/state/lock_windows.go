package state

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// hold opens the lock file shared with nobody: a second open fails while this one stands, and Windows closes it with the process.
func hold(dir string) (func(), error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, LockFile))
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		const sharingViolation = syscall.Errno(32)
		if errors.Is(err, sharingViolation) {
			return nil, errHeld
		}
		return nil, err
	}
	return func() { syscall.CloseHandle(handle) }, nil
}
