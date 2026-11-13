package state

import (
	"errors"
	"io/fs"
)

// LockFile is the file a serving remuda holds a lock on; the lock goes with the process, however it ends.
const LockFile = "serve.lock"

// errHeld is a lock another process holds now.
var errHeld = errors.New("the lock is held")

// Hold takes the state dir's lock for as long as this process serves it; held by another process already, it says so
// and holds nothing.
func Hold(dir string) (release func(), err error) {
	release, err = hold(dir)
	if errors.Is(err, errHeld) {
		return nil, errors.New("another remuda serves " + dir)
	}
	return release, err
}

// Held says whether a process holds the state dir's lock now, that is whether a remuda serves it. A dir that is not
// there is free; one whose lock cannot be taken for another reason (no permission) counts as held, so a restore never
// goes ahead blind.
func Held(dir string) bool {
	release, err := Hold(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	release()
	return false
}
