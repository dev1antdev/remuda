package state

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// LockFile is the file a serving remuda holds a lock on; the lock goes with the process, however it ends.
const LockFile = "serve.lock"

// Hold takes the state dir's lock for as long as this process serves it; held by another process already, it says so
// and holds nothing.
func Hold(dir string) (release func(), err error) {
	f, err := os.OpenFile(filepath.Join(dir, LockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("another remuda serves " + dir)
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// Held says whether a process holds the state dir's lock now, that is whether a remuda serves it.
func Held(dir string) bool {
	release, err := Hold(dir)
	if err != nil {
		_, missing := err.(*os.PathError)
		return !missing
	}
	release()
	return false
}
