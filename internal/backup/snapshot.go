package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the driver tasks.db is written with, pure Go
)

// MaxSnapshot is the most a snapshot may weigh before it is packed: a state dir is a few megabytes, the cap is a guard.
const MaxSnapshot = 256 << 20

// ManifestName is the file at the archive's root that says what it is.
const ManifestName = "remuda-backup.json"

// Manifest is what a backup says about itself.
type Manifest struct {
	Format  int       `json:"format"`
	Host    string    `json:"host"`    // the cockpit it was taken on
	Version string    `json:"version"` // the remuda that took it
	At      time.Time `json:"at"`
	Files   int       `json:"files"`
}

// skippedDirs are the state dir's folders a snapshot leaves out: the ssh masters' sockets, and the backups this machine
// keeps for others (and for itself).
var skippedDirs = map[string]bool{"ssh": true, "backups": true}

// tempName is a file the atomic writer is still writing, "tasks.json.123456789", and an upload's partial copy
var tempName = regexp.MustCompile(`\.\d{6,}$|\.partial$|\.new$`)

// skipped says whether a file of the state dir stays out of the snapshot: a live database's side files (the database
// itself goes through SQLite), the lock, the tailnet node's logs, files half written.
func skipped(rel string) bool {
	name := filepath.Base(rel)
	switch {
	case name == "serve.lock", strings.HasSuffix(name, "-wal"), strings.HasSuffix(name, "-shm"), strings.HasSuffix(name, "-journal"):
		return true
	case strings.HasSuffix(name, ".log"), strings.HasPrefix(rel, "tailscale/") && strings.HasSuffix(name, ".txt"):
		return true
	}
	return tempName.MatchString(name)
}

// Snapshot packs the state dir as one gzipped tar: every file the cockpit keeps, each SQLite database copied through
// SQLite itself (VACUUM INTO: a consistent copy of a database a running remuda writes to), a manifest at the root.
func Snapshot(dir, host, version string, now time.Time) ([]byte, Manifest, error) {
	manifest := Manifest{Format: 1, Host: host, Version: version, At: now.UTC()}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	total := 0
	add := func(rel string, mode fs.FileMode, modTime time.Time, data []byte) error {
		total += len(data)
		if total > MaxSnapshot {
			return fmt.Errorf("the state dir is over %d MB; nothing was packed", MaxSnapshot>>20)
		}
		if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: int64(mode.Perm()), Size: int64(len(data)), ModTime: modTime, Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := tw.Write(data)
		manifest.Files++
		return err
	}
	scratch, err := os.MkdirTemp("", "remuda-backup-")
	if err != nil {
		return nil, manifest, err
	}
	defer os.RemoveAll(scratch)
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			if !strings.Contains(rel, "/") && skippedDirs[rel] {
				return filepath.SkipDir
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return tw.WriteHeader(&tar.Header{Name: rel + "/", Mode: int64(info.Mode().Perm()), ModTime: info.ModTime(), Typeflag: tar.TypeDir})
		}
		if !entry.Type().IsRegular() || skipped(rel) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		var data []byte
		if strings.HasSuffix(rel, ".db") {
			data, err = copyDatabase(path, filepath.Join(scratch, fmt.Sprintf("%d.db", manifest.Files)))
		} else {
			data, err = os.ReadFile(path)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil // gone between the listing and the read: a file renamed into place
		}
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		return add(rel, info.Mode(), info.ModTime(), data)
	})
	if err != nil {
		return nil, manifest, err
	}
	head, _ := json.MarshalIndent(manifest, "", "  ")
	if err := add(ManifestName, 0o600, now, head); err != nil {
		return nil, manifest, err
	}
	manifest.Files-- // the manifest does not count itself
	if err := tw.Close(); err != nil {
		return nil, manifest, err
	}
	if err := zw.Close(); err != nil {
		return nil, manifest, err
	}
	return buf.Bytes(), manifest, nil
}

// copyDatabase is a consistent copy of a SQLite database another process may be writing: VACUUM INTO a fresh file,
// read back whole.
func copyDatabase(path, into string) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.Exec("VACUUM INTO ?", into); err != nil {
		return nil, err
	}
	return os.ReadFile(into)
}
