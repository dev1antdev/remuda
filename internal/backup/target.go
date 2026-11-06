package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Target is a machine of the fleet a backup goes to: its shell, and a file written whole there.
type Target interface {
	shell.Host
	PutFile(path string, data []byte, timeout time.Duration) error
}

// Timeout is how long one step on the target may take: a snapshot is megabytes, over ssh.
var Timeout = 2 * time.Minute

// fileName is a backup's name on the target, its moment in UTC: names sort as times do.
var fileName = regexp.MustCompile(`^remuda-(\d{8}T\d{6}Z)\.tar\.gz$`)

const stamp = "20060102T150405Z"

// NameAt is the name of a backup taken at that moment.
func NameAt(at time.Time) string {
	return "remuda-" + at.UTC().Format(stamp) + ".tar.gz"
}

// HostFolder is a cockpit's name as the folder its backups go to: letters, digits, dots, dashes.
func HostFolder(host string) string {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".local")
	host = regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(host, "-")
	host = strings.Trim(host, ".-")
	if host == "" {
		return "remuda"
	}
	return host
}

// FolderScript prints the folder the backups of a cockpit go to on a machine — <the machine's remuda state dir>/
// backups/<cockpit> — made private to the person first.
func FolderScript(host string) string {
	return `umask 077
case "$(uname -s)" in
  Darwin) d="$HOME/Library/Application Support/remuda" ;;
  *) d="${XDG_CONFIG_HOME:-$HOME/.config}/remuda" ;;
esac
d="$d/backups/"` + script.Quote(HostFolder(host)) + `
mkdir -p "$d" && chmod 700 "$d" && printf '%s\n' "$d"
`
}

// FinishScript puts a backup written as <name>.partial in place, private, clears the partial copies earlier runs left
// when their write was cut off, and removes every backup of the folder past the newest keep, a line "pruned <name>" each.
func FinishScript(folder, name string, keep int) string {
	return script.PATH + "\ncd " + script.Quote(folder) + " || exit 3\n" +
		"mv -f " + script.Quote(name+".partial") + " " + script.Quote(name) + " && chmod 600 " + script.Quote(name) + " || exit 4\n" +
		"rm -f -- remuda-*.tar.gz.partial\n" +
		"ls -1 | grep -E '^remuda-[0-9]{8}T[0-9]{6}Z\\.tar\\.gz$' | sort -r | tail -n +" + strconv.Itoa(keep+1) +
		" | while IFS= read -r f; do rm -f -- \"$f\" && printf 'pruned %s\\n' \"$f\"; done\n"
}

// ListScript prints the folder's backups, a name and its size in bytes a line.
func ListScript(folder string) string {
	return "cd " + script.Quote(folder) + " 2>/dev/null || exit 0\n" +
		"for f in remuda-*.tar.gz; do [ -f \"$f\" ] || continue; printf '%s\\037%s\\n' \"$f\" \"$(wc -c < \"$f\" | tr -d ' ')\"; done\n"
}

// File is one backup on the target.
type File struct {
	Name string    `json:"name"`
	Size int64     `json:"size"`
	At   time.Time `json:"at"`
}

// ParseList reads ListScript's lines, newest first; a name that is no backup's is left out.
func ParseList(out string) []File {
	files := []File{}
	for _, line := range script.Lines(out) {
		fields := script.Fields(line)
		if len(fields) != 2 {
			continue
		}
		m := fileName.FindStringSubmatch(fields[0])
		if m == nil {
			continue
		}
		at, err := time.Parse(stamp, m[1])
		if err != nil {
			continue
		}
		size, _ := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
		files = append(files, File{Name: fields[0], Size: size, At: at})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	return files
}

// pruned counts FinishScript's lines about the backups it removed.
func pruned(out string) int {
	n := 0
	for _, line := range script.Lines(out) {
		if strings.HasPrefix(line, "pruned ") {
			n++
		}
	}
	return n
}

// folderOn is the folder a cockpit's backups go to on the target: under this cockpit's own state dir when the target
// is the machine it runs on (a second instance's dir is its own), else the target's remuda state dir.
func folderOn(target Target, dir, host string, create bool) (string, error) {
	if target.Local() {
		folder := filepath.Join(dir, "backups", HostFolder(host))
		if create {
			if err := os.MkdirAll(folder, 0o700); err != nil {
				return "", err
			}
		}
		return folder, nil
	}
	out, err := shell.Text(target, FolderScript(host), Timeout)
	folder := strings.TrimSpace(out)
	if err != nil || folder == "" {
		return "", fmt.Errorf("no backups folder on %s: %v", target.Label(), firstLine(out, err))
	}
	return folder, nil
}

// firstLine is a script's complaint: its first line of output, else the error.
func firstLine(out string, err error) string {
	if line, _, _ := strings.Cut(strings.TrimSpace(out), "\n"); line != "" {
		return line
	}
	if err != nil {
		return err.Error()
	}
	return "no answer"
}
