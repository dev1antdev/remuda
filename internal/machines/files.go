package machines

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dev1antdev/remuda/internal/sshx"
)

// File access for the viewer: what the agent wrote, read back from the
// machine it lives on. Local reads use the filesystem; remote reads go over
// the SSH master. The one write is SaveUpload, into the pane's own
// .remuda/uploads folder.

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
	if len(f) < 2 {
		return FileInfo{}, fmt.Errorf("stat %s: %q", p, out)
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

// Uploads land in the pane's folder under .remuda/uploads, so the agent can
// read them by a relative path without leaving its project. The folder
// carries its own .gitignore so screenshots never end up in a commit.
const (
	uploadDir    = ".remuda"
	uploadSubdir = "uploads"
)

type Upload struct {
	Name string `json:"name"` // the stored file name
	Path string `json:"path"` // absolute, on this machine
	Rel  string `json:"rel"`  // relative to the pane folder, for the prompt
	Size int    `json:"size"`
	Kind string `json:"kind"` // image | media | pdf | binary | text
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// storedName makes a safe, unique file name: time, a random tag, and the
// original stem and extension sanitized separately — a non-ASCII stem must
// not take the extension with it, and a silly extension must not panic.
func storedName(name string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	stem = strings.Trim(unsafeName.ReplaceAllString(stem, "_"), "._")
	ext = strings.Trim(unsafeName.ReplaceAllString(ext, ""), ".")
	if stem == "" {
		stem = "file"
	}
	if len(stem) > 60 {
		stem = stem[:60]
	}
	if ext != "" {
		if len(ext) > 16 {
			ext = ext[:16]
		}
		ext = "." + strings.ToLower(ext)
	}
	tag := make([]byte, 3)
	rand.Read(tag)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(tag) + "-" + stem + ext
}

// SaveUpload stores one uploaded file for a pane folder.
func (mc *Machine) SaveUpload(cwd, name string, data []byte) (Upload, error) {
	if cwd == "" {
		return Upload{}, fmt.Errorf("the pane has no folder to upload into")
	}
	stored := storedName(name)
	rel := uploadDir + "/" + uploadSubdir + "/" + stored
	abs := cleanPath(cwd + "/" + rel)
	ignore := cleanPath(cwd + "/" + uploadDir + "/.gitignore")

	if mc.Local {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return Upload{}, err
		}
		if _, err := os.Stat(ignore); err != nil {
			os.WriteFile(ignore, []byte("*\n"), 0o644)
		}
		if err := os.WriteFile(abs, data, 0o644); err != nil {
			return Upload{}, err
		}
	} else {
		conn, err := mc.remoteConn()
		if err != nil {
			return Upload{}, err
		}
		if err := conn.Put(abs, data, 2*time.Minute); err != nil {
			return Upload{}, err
		}
		mc.exec("[ -f "+sshx.Quote(ignore)+" ] || printf '*\\n' > "+sshx.Quote(ignore), 10*time.Second)
	}
	kind, _ := classify(abs, func() bool { return bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 })
	return Upload{Name: stored, Path: abs, Rel: rel, Size: len(data), Kind: kind}, nil
}

type Diff struct {
	Repo      string `json:"repo,omitempty"`
	Untracked bool   `json:"untracked,omitempty"`
	Diff      string `json:"diff"`
}

// ReadFrom returns a file's bytes from an offset to its end — a transcript's
// growth since the last look. Remotely it is a tail over ssh.
func (mc *Machine) ReadFrom(p string, offset int64) ([]byte, error) {
	if mc.Local {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, err
		}
		return io.ReadAll(io.LimitReader(f, 256<<20))
	}
	return mc.execBytes(fmt.Sprintf("tail -c +%d %s", offset+1, sshx.Quote(p)), 120*time.Second)
}

// Shell runs a POSIX shell script on the machine — `sh -c` here, `sh -c`
// over the ssh master there (whatever the login shell) — and returns its
// stdout.
func (mc *Machine) Shell(script string, timeout time.Duration) (string, error) {
	if mc.Local {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", script)
		cmd.WaitDelay = 2 * time.Second // a killed sh may leave children on the pipes; do not wait for them
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return out.String(), fmt.Errorf("timed out after %s", timeout)
			}
			if ee, ok := err.(*exec.ExitError); ok {
				return out.String(), &sshx.ExitError{Code: ee.ExitCode(), Stderr: strings.TrimSpace(errb.String())}
			}
			return out.String(), err
		}
		return out.String(), nil
	}
	return mc.exec(script, timeout)
}

// Stream runs a POSIX script on the machine and hands its output (stdout
// and stderr together, as it comes) to sink — for the long jobs a person
// watches: an install, an update. The exit status is the error.
func (mc *Machine) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	if !mc.Local {
		conn, err := mc.remoteConn()
		if err != nil {
			return err
		}
		return conn.Stream(script, timeout, sink)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // its own group: a timeout ends the children too
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 2 * time.Second
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return err
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 8192)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				sink(append([]byte(nil), buf[:n]...))
			}
			if err != nil {
				return
			}
		}
	}()
	err := cmd.Wait()
	pw.Close()
	<-readDone
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("timed out after %s", timeout)
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return &sshx.ExitError{Code: ee.ExitCode()}
		}
		return err
	}
	return nil
}

// GitDiff shows the file's uncommitted change (staged + unstaged, against
// HEAD) in its repository, or marks it untracked / outside any repo. The
// same script runs locally and remotely; outside a repo it prints nothing.
func (mc *Machine) GitDiff(p string) (Diff, error) {
	q := sshx.Quote(p)
	script := `cd ` + sshx.Quote(filepath.Dir(p)) + ` 2>/dev/null || exit 0; root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0; ` +
		`printf '%s\n' "$root"; if git ls-files --error-unmatch ` + q + ` >/dev/null 2>&1; then echo tracked; ` +
		`git -c color.ui=never diff HEAD -- ` + q + ` | head -c 400000; else echo untracked; fi`
	out, err := mc.Shell(script, 30*time.Second)
	if err != nil {
		return Diff{}, err
	}
	root, rest, _ := strings.Cut(out, "\n")
	state, diff, _ := strings.Cut(rest, "\n")
	return Diff{Repo: strings.TrimSpace(root), Untracked: strings.TrimSpace(state) == "untracked", Diff: diff}, nil
}

// ---- browsing folders ----

// DirEntry is one subfolder of a listed folder.
type DirEntry struct {
	Name string `json:"name"`
	Git  bool   `json:"git"` // a repository (a .git inside)
}

// DirListing is a folder and its subfolders, for the folder picker.
type DirListing struct {
	Path   string     `json:"path"`             // resolved, absolute
	Parent string     `json:"parent,omitempty"` // "" at the root
	Home   string     `json:"home,omitempty"`
	Dirs   []DirEntry `json:"dirs"`
}

// ListDirs lists the subfolders of path on the machine ("" or "~" is
// home; "~/x" is under it). Hidden folders only when asked.
func (mc *Machine) ListDirs(path string, hidden bool) (DirListing, error) {
	q := sshx.Quote(path)
	glob := "*/"
	if hidden {
		glob = "*/ .*/"
	}
	script := `p=` + q + `; [ -z "$p" ] && p="$HOME"; case "$p" in "~") p="$HOME";; "~/"*) p="$HOME/${p#"~/"}";; esac; ` +
		`cd "$p" 2>/dev/null || { printf 'ERR\037not a folder: %s\n' "$p"; exit 0; }; printf '%s\n' "$(pwd -P)"; printf '%s\n' "$HOME"; printf '\036\n'; ` +
		`for d in ` + glob + `; do case "$d" in ./|../) continue;; esac; [ -d "$d" ] || continue; n=${d%/}; g=0; [ -e "$d.git" ] && g=1; printf '%s\037%s\n' "$n" "$g"; done`
	out, err := mc.Shell(script, 30*time.Second)
	if err != nil {
		return DirListing{}, err
	}
	if strings.HasPrefix(out, "ERR\x1f") {
		return DirListing{}, fmt.Errorf("%s", strings.TrimSpace(strings.TrimPrefix(out, "ERR\x1f")))
	}
	head, rest, _ := strings.Cut(out, "\x1e\n")
	lines := strings.Split(strings.TrimRight(head, "\n"), "\n")
	l := DirListing{Dirs: []DirEntry{}}
	if len(lines) > 0 {
		l.Path = lines[0]
	}
	if len(lines) > 1 {
		l.Home = lines[1]
	}
	if l.Path != "" && l.Path != "/" {
		l.Parent = filepath.Dir(l.Path)
	}
	for _, line := range strings.Split(strings.TrimRight(rest, "\n"), "\n") {
		if n, g, ok := strings.Cut(line, "\x1f"); ok && n != "" {
			l.Dirs = append(l.Dirs, DirEntry{Name: n, Git: g == "1"})
		}
	}
	sort.Slice(l.Dirs, func(i, j int) bool { return strings.ToLower(l.Dirs[i].Name) < strings.ToLower(l.Dirs[j].Name) })
	return l, nil
}
