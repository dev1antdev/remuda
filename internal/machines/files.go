package machines

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/sshx"
)

// File access for the viewer: what the agent wrote, read back from the
// machine it lives on. Local reads use the filesystem; remote reads go over
// the SSH master. Nothing here writes.

const (
	MaxTextBytes = 512 * 1024       // a text file is served up to this much
	MaxRawBytes  = 24 * 1024 * 1024 // images / media / pdf
	maxFind      = 8
)

type FileInfo struct {
	Path       string   `json:"path"`
	Kind       string   `json:"kind"` // text | image | media | pdf | binary | dir | missing
	Size       int64    `json:"size"`
	MIME       string   `json:"-"`
	Candidates []string `json:"candidates,omitempty"` // when a relative name matched several files
}

// MaxBytes is how much of this kind of file the viewer gets.
func (fi FileInfo) MaxBytes() int64 {
	if fi.Kind == "text" {
		return MaxTextBytes
	}
	return MaxRawBytes
}

// fileKinds is the one table of extension → kind + MIME; anything else is
// text or binary by content.
var fileKinds = map[string]struct{ kind, mime string }{
	".png": {"image", "image/png"}, ".jpg": {"image", "image/jpeg"}, ".jpeg": {"image", "image/jpeg"},
	".gif": {"image", "image/gif"}, ".webp": {"image", "image/webp"}, ".svg": {"image", "image/svg+xml"},
	".mp4": {"media", "video/mp4"}, ".webm": {"media", "video/webm"}, ".mov": {"media", "video/quicktime"},
	".mp3": {"media", "audio/mpeg"}, ".m4a": {"media", "audio/mp4"}, ".wav": {"media", "audio/wav"},
	".pdf": {"pdf", "application/pdf"},
}

func classify(p string, sniffBinary func() bool) (kind, mime string) {
	if k, ok := fileKinds[strings.ToLower(filepath.Ext(p))]; ok {
		return k.kind, k.mime
	}
	if sniffBinary() {
		return "binary", "application/octet-stream"
	}
	return "text", "text/plain; charset=utf-8"
}

// ResolvePath turns what an agent printed — absolute, ~/…, or relative to the
// pane's folder — into a real path. A relative miss is searched for under the
// pane folder by suffix, because agents say "demo.mp4" when they mean
// "docs/screenshots/demo.mp4".
func (mc *Machine) ResolvePath(cwd, raw string) (FileInfo, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return FileInfo{}, fmt.Errorf("path required")
	}
	if strings.HasPrefix(p, "~/") || p == "~" {
		p = mc.Home() + strings.TrimPrefix(p, "~")
	}
	if !strings.HasPrefix(p, "/") && cwd != "" {
		p = cwd + "/" + p
	}
	p = cleanPath(p)
	info, err := mc.stat(p)
	if err == nil {
		return info, nil
	}
	// relative miss: look under the pane folder
	if !strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "~") && cwd != "" {
		found := mc.findBySuffix(cwd, strings.TrimPrefix(strings.TrimPrefix(raw, "./"), "/"))
		if len(found) == 1 {
			if info, err := mc.stat(found[0]); err == nil {
				return info, nil
			}
		}
		if len(found) > 1 {
			return FileInfo{Path: p, Kind: "missing", Candidates: found}, nil
		}
	}
	return FileInfo{Path: p, Kind: "missing"}, nil
}

func cleanPath(p string) string {
	c := filepath.ToSlash(filepath.Clean(p))
	if !strings.HasPrefix(c, "/") {
		c = "/" + c
	}
	return c
}

func (mc *Machine) stat(p string) (FileInfo, error) {
	if mc.Local {
		fi, err := os.Stat(p)
		if err != nil {
			return FileInfo{}, err
		}
		if fi.IsDir() {
			return FileInfo{Path: p, Kind: "dir"}, nil
		}
		kind, mime := classify(p, func() bool { return sniffLocal(p) })
		return FileInfo{Path: p, Kind: kind, MIME: mime, Size: fi.Size()}, nil
	}
	q := sshx.Quote(p)
	out, err := mc.exec(`if [ -d `+q+` ]; then echo dir; elif [ -f `+q+` ]; then `+
		`printf 'file %s ' "$(`+statScript(p, "%s", "%z")+`)"; `+
		`head -c 8192 `+q+` | tr -d -c '\000' | wc -c; else echo missing; fi`, 15*time.Second)
	if err != nil {
		return FileInfo{}, err
	}
	f := strings.Fields(out)
	switch {
	case len(f) == 0 || f[0] == "missing":
		return FileInfo{}, os.ErrNotExist
	case f[0] == "dir":
		return FileInfo{Path: p, Kind: "dir"}, nil
	}
	size, _ := strconv.ParseInt(f[1], 10, 64)
	kind, mime := classify(p, func() bool { return len(f) > 2 && f[2] != "0" })
	return FileInfo{Path: p, Kind: kind, MIME: mime, Size: size}, nil
}

// sniffLocal: binary if the first 8KB hold a NUL.
func sniffLocal(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := io.ReadFull(f, buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}

var skipDirs = map[string]bool{"node_modules": true, ".git": true, "dist": true, "target": true, ".next": true, "vendor": true}

func (mc *Machine) findBySuffix(root, rel string) []string {
	if rel == "" {
		return nil
	}
	var found []string
	if mc.Local {
		budget := 20000
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if budget--; budget <= 0 || len(found) >= maxFind {
				return filepath.SkipAll
			}
			if d.IsDir() {
				if p != root && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, "/"+rel) {
				found = append(found, p)
			}
			return nil
		})
		return found
	}
	var prune []string
	for d := range skipDirs {
		prune = append(prune, "-name "+sshx.Quote(d)+" -prune -o")
	}
	out, err := mc.exec(`find `+sshx.Quote(root)+` `+strings.Join(prune, " ")+
		` -type f -path `+sshx.Quote("*/"+rel)+` -print 2>/dev/null | head -`+strconv.Itoa(maxFind), 20*time.Second)
	if err != nil {
		return nil
	}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			found = append(found, l)
		}
	}
	return found
}

// Open returns the local file for streaming (Range requests included).
func (mc *Machine) Open(p string) (*os.File, error) {
	return os.Open(p)
}

// ReadRemote returns up to max bytes of a remote file and whether it was cut.
func (mc *Machine) ReadRemote(p string, max int64) ([]byte, bool, error) {
	data, err := mc.execBytes(fmt.Sprintf("head -c %d %s", max+1, sshx.Quote(p)), 60*time.Second)
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > max {
		return data[:max], true, nil
	}
	return data, false, nil
}

type Diff struct {
	Repo      string `json:"repo,omitempty"`
	Untracked bool   `json:"untracked,omitempty"`
	Diff      string `json:"diff"`
}

// GitDiff shows the file's uncommitted change (staged + unstaged, against
// HEAD) in its repository, or marks it untracked / outside any repo. The
// same script runs locally and remotely; outside a repo it prints nothing.
func (mc *Machine) GitDiff(p string) (Diff, error) {
	q := sshx.Quote(p)
	script := `cd ` + sshx.Quote(filepath.Dir(p)) + ` 2>/dev/null || exit 0; root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0; ` +
		`printf '%s\n' "$root"; if git ls-files --error-unmatch ` + q + ` >/dev/null 2>&1; then echo tracked; ` +
		`git -c color.ui=never diff HEAD -- ` + q + ` | head -c 400000; else echo untracked; fi`
	var out string
	var err error
	if mc.Local {
		var b []byte
		b, err = exec.Command("sh", "-c", script).Output()
		out = string(b)
	} else {
		out, err = mc.exec(script, 30*time.Second)
	}
	if err != nil {
		return Diff{}, err
	}
	root, rest, _ := strings.Cut(out, "\n")
	state, diff, _ := strings.Cut(rest, "\n")
	return Diff{Repo: strings.TrimSpace(root), Untracked: strings.TrimSpace(state) == "untracked", Diff: diff}, nil
}
