//go:build windows

package transcript

import "os"

// openNoFollow is OpenSession's open on Windows: the link and regular-file checks around it are OpenSession's.
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}

// openRootFile opens inside an os.Root, whose own checks keep a link from leaving it.
func openRootFile(root *os.Root, rel string) (*os.File, error) {
	return root.Open(rel)
}
