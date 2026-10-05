// Package state is where herdr-ui keeps its own files: the access token,
// manually added machines, push keys and subscriptions. One directory, 0700,
// every file written atomically with 0600.
package state

import (
	"os"
	"path/filepath"
)

// Dir is <user config dir>/herdr-ui, created on first use.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "herdr-ui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// WriteFile writes data to path via a temp file + rename, mode 0600.
func WriteFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
