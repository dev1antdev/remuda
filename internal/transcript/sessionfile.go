package transcript

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// OpenSession opens a session file for reading, refusing a link, a named pipe or any other special file: a learned
// root is an agent's own folder, and a reader must not hang on a FIFO or follow a link out of it. The opened descriptor
// is checked too, so a path swapped between the check and the open is refused as well.
func OpenSession(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular session file", path)
	}
	f, err := openNoFollow(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		f.Close()
		return nil, fmt.Errorf("%s changed while it was opened", path)
	}
	return f, nil
}

// OpenInRoot opens a session file inside a session root with the filesystem's own confinement: a project folder that is
// a link out of the root is refused, as os.Root refuses any climb past it.
func OpenInRoot(root, path string) (*os.File, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s is not under %s", path, root)
	}
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rooted.Close()
	info, err := rooted.Lstat(rel)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular session file", path)
	}
	f, err := openRootFile(rooted, rel)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		f.Close()
		return nil, fmt.Errorf("%s changed while it was opened", path)
	}
	return f, nil
}

// ReadSession is the whole of a session file, opened as OpenSession does.
func ReadSession(path string) ([]byte, error) {
	f, err := OpenSession(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// sessionFile says a path names a regular session file, not a link or a special one.
func sessionFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
