// Package state is where remuda keeps its own files: the access token,
// manually added machines, push keys and subscriptions, the radar's
// stamps, review baselines, schedules, quick prompts, the task database.
// One directory, 0700, every file written atomically with 0600.
package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
)

// Dir is <user config dir>/remuda, created on first use — or
// REMUDA_STATE_DIR, for a second instance beside the real one (a test,
// a trial of -tailscale) that must not share its token, tasks and pushes.
func Dir() (string, error) {
	dir := os.Getenv("REMUDA_STATE_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "remuda")
		// the state kept under the previous name (herdr-ui) is carried
		// over once: token, machines, tasks, radar — the same files
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if old := filepath.Join(base, "herdr-ui"); isDir(old) {
				if err := os.Rename(old, dir); err == nil {
					log.Printf("state: %s → %s", old, dir)
				}
			}
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// WriteFile writes data to path via a private temp file + rename, mode
// 0600 — two writers at once each get their own temp file, so the rename
// always installs a whole document.
func WriteFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// LoadJSON reads <dir>/<name> into v; a missing file is no error (v stays
// as it was), a damaged one is.
func LoadJSON(dir, name string, v any) error {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// SaveJSON writes v to <dir>/<name>, indented, atomically.
func SaveJSON(dir, name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(filepath.Join(dir, name), b)
}

// NewID is an opaque id: 12 random hex characters.
func NewID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
