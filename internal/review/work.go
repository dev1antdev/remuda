package review

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// What the badge sweep keeps of each folder's work beyond its counts: the profile the review queue's risk weighs, the
// remote two checkouts of one repository share, and when the work last moved, so a secret check of it stays the
// answer until the work changes.

// work is a folder's uncommitted work as the sweeps read it.
type work struct {
	profile     Profile
	fingerprint string    // every uncommitted file's blob, hashed: the work as it stood
	readAt      time.Time // the start of the sweep that read it last
	movedAfter  time.Time // the work changed after this moment: the start of the sweep before the one that saw it change
}

// scanNote is the last secret check of a folder: how much it found, and when.
type scanNote struct {
	findings int
	at       time.Time
}

// numstatScript prints git's added and removed lines per file changed against HEAD, relative to the folder.
const numstatScript = `git -c core.quotepath=off diff --no-ext-diff --no-renames --relative --numstat HEAD 2>/dev/null`

// newBytesScript prints the untracked files' bytes and how many they are: wc reads a file's size off its inode, so
// the files are not read; a name starting with a dash reaches it as ./-name.
const newBytesScript = `git ls-files -z --others --exclude-standard 2>/dev/null | tr '\0' '\n' | sed 's|^|./|' | tr '\n' '\0' | ` +
	`xargs -0 -r wc -c 2>/dev/null | awk '$2 != "total" { s += $1; n++ } END { print s + 0, n + 0 }'`

// remoteScript prints the address of the folder's origin, else of its first remote, on a line of its own.
const remoteScript = `u=$(git remote get-url origin 2>/dev/null) || { r=$(git remote 2>/dev/null | head -n 1); [ -n "$r" ] && u=$(git remote get-url "$r" 2>/dev/null); }; printf '%s\n' "$u"`

// readWork reads a folder's sections of the sweep: its blob lines, its numstat, and the untracked files' size.
func readWork(blobLines, numstat, newBytes string) (Profile, string) {
	var paths []string
	gone := map[string]bool{}
	for _, line := range strings.Split(strings.TrimRight(blobLines, "\n"), "\n") {
		path, blob, ok := strings.Cut(line, "\x1f")
		if !ok {
			continue
		}
		paths = append(paths, path)
		if blob == "gone" {
			gone[path] = true
		}
	}
	added, removed := 0, 0
	for _, line := range strings.Split(strings.TrimRight(numstat, "\n"), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		plus, _ := strconv.Atoi(fields[0]) // a binary file counts "-": no lines
		minus, _ := strconv.Atoi(fields[1])
		added += plus
		removed += minus
	}
	var bytes int64
	var files int
	if fields := strings.Fields(newBytes); len(fields) == 2 {
		bytes, _ = strconv.ParseInt(fields[0], 10, 64)
		files, _ = strconv.Atoi(fields[1])
	}
	sum := sha256.Sum256([]byte(blobLines))
	return profileOf(paths, gone, added, removed, files, bytes), hex.EncodeToString(sum[:12])
}

// setWork keeps what a sweep begun at began read of a folder's work.
func (store *Store) setWork(storeKey string, profile Profile, fingerprint string, began time.Time) {
	store.mu.Lock()
	defer store.mu.Unlock()
	previous, known := store.works[storeKey]
	next := work{profile: profile, fingerprint: fingerprint, readAt: began, movedAfter: previous.movedAfter}
	if known && previous.fingerprint != fingerprint {
		next.movedAfter = previous.readAt
	}
	store.works[storeKey] = next
}

// Profile is the folder's uncommitted work as the last sweep weighed it.
func (store *Store) Profile(machineID, dir string) (Profile, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	work, ok := store.works[key(machineID, dir)]
	return work.profile, ok
}

// Remote is the address of the folder's origin as the last sweep read it, "" for a checkout with no remote.
func (store *Store) Remote(machineID, dir string) string {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.remotes[key(machineID, dir)]
}

func (store *Store) setRemote(storeKey, remote string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if remote == "" {
		delete(store.remotes, storeKey)
		return
	}
	store.remotes[storeKey] = remote
}

// NoteScan keeps the secret check of a folder the person just ran, for the review queue's risk.
func (store *Store) NoteScan(machineID, dir string, scan SecretScan) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.scans[key(machineID, dir)] = scanNote{findings: len(scan.Findings) + scan.More, at: scan.At}
}

// Secrets is how many secrets the last check of the folder found, while the work is still the one it read: once a
// sweep sees the work move after the check, it is no longer known.
func (store *Store) Secrets(machineID, dir string) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	note, ok := store.scans[key(machineID, dir)]
	if !ok {
		return 0
	}
	if work, read := store.works[key(machineID, dir)]; read && !note.at.After(work.movedAfter) {
		return 0
	}
	return note.findings
}

func (store *Store) forgetWork(storeKey string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.works, storeKey)
	delete(store.remotes, storeKey)
	delete(store.badges, storeKey)
}
