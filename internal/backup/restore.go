package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one file a backup holds.
type Entry struct {
	Name string
	Mode os.FileMode
	Data []byte
}

// Read opens a backup: its manifest and its files, every name a plain path inside the archive (none absolute, none
// leading out with ..), only folders and regular files, at most MaxSnapshot unpacked.
func Read(archive []byte) (Manifest, []Entry, error) {
	var manifest Manifest
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return manifest, nil, fmt.Errorf("not a remuda backup: %w", err)
	}
	tr := tar.NewReader(zr)
	var entries []Entry
	total := int64(0)
	found := false
	for {
		head, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return manifest, nil, fmt.Errorf("the backup is damaged: %w", err)
		}
		name := strings.TrimSuffix(head.Name, "/")
		// on Windows a backslash is a separator path.Clean does not know, and "C:" is a drive: either would lead out
		// of the state dir there, and the archive travels between the fleet's systems
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") ||
			strings.ContainsAny(name, `\:`) {
			return manifest, nil, fmt.Errorf("the backup names %q, a path outside the state dir", head.Name)
		}
		switch head.Typeflag {
		case tar.TypeDir:
			entries = append(entries, Entry{Name: name + "/", Mode: os.FileMode(head.Mode).Perm() | os.ModeDir})
			continue
		case tar.TypeReg:
		default:
			return manifest, nil, fmt.Errorf("the backup holds %q, which is no plain file", head.Name)
		}
		total += head.Size
		if total > MaxSnapshot {
			return manifest, nil, fmt.Errorf("the backup unpacks to over %d MB", MaxSnapshot>>20)
		}
		data, err := io.ReadAll(io.LimitReader(tr, head.Size))
		if err != nil {
			return manifest, nil, fmt.Errorf("the backup is damaged: %w", err)
		}
		if name == ManifestName {
			if err := json.Unmarshal(data, &manifest); err != nil {
				return manifest, nil, fmt.Errorf("the backup's manifest is damaged: %w", err)
			}
			found = true
			continue
		}
		entries = append(entries, Entry{Name: name, Mode: os.FileMode(head.Mode).Perm(), Data: data})
	}
	if !found || manifest.Format != 1 {
		return manifest, nil, errors.New("not a remuda backup: no manifest of a format this remuda reads")
	}
	return manifest, entries, nil
}

// Restore puts a backup in the state dir's place: the dir as it is moved aside first, beside it as
// <dir>.before-restore-<time>, the backup unpacked into a fresh one, and the backups this machine keeps carried over;
// the answer is where the old dir went. The caller makes sure no remuda serves the dir.
func Restore(archive []byte, dir string, now time.Time) (Manifest, string, error) {
	manifest, entries, err := Read(archive)
	if err != nil {
		return manifest, "", err
	}
	// a state dir that is a symlink (a small disk's /var is one) is restored where it points: a rename of the link
	// would leave the real dir in place, and the next start would serve what restore meant to replace
	if fi, err := os.Lstat(dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
	}
	aside := ""
	if _, err := os.Stat(dir); err == nil {
		aside = dir + ".before-restore-" + now.UTC().Format("20060102T150405Z")
		if err := os.Rename(dir, aside); err != nil {
			return manifest, "", fmt.Errorf("moving %s aside: %w", dir, err)
		}
	}
	undo := func(cause error) (Manifest, string, error) {
		if aside != "" {
			os.RemoveAll(dir)
			if err := os.Rename(aside, dir); err != nil {
				return manifest, "", fmt.Errorf("%w; the old state dir is at %s", cause, aside)
			}
		}
		return manifest, "", cause
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return undo(err)
	}
	for _, entry := range entries {
		target := filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(entry.Name, "/")))
		if entry.Mode.IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil { // the person's own alone, whatever the archive said
				return undo(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return undo(err)
		}
		if err := writeSynced(target, entry.Data, entry.Mode.Perm()&0o700|0o600); err != nil {
			return undo(err)
		}
	}
	if aside != "" {
		for _, folder := range keptHere {
			if kept := filepath.Join(aside, folder); dirExists(kept) {
				if err := os.Rename(kept, filepath.Join(dir, folder)); err != nil {
					return manifest, aside, fmt.Errorf("restored, but the %s kept here stayed at %s: %w", folder, kept, err)
				}
			}
		}
	}
	return manifest, aside, nil
}

// writeSynced writes a restored file and puts it on the disk before the answer: a power cut right after a restore must
// not leave its state half written.
func writeSynced(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
