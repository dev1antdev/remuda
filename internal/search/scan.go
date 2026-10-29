package search

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// scanFile is one session file to sieve: Claude Code's printed as grep prints them, Codex's with the folder and
// thread id its name does not carry.
type scanFile struct {
	path   string
	mod    time.Time
	format transcript.Format
	rel    string // Claude: "./<project>/<session>.jsonl"
	cwd    string // Codex: the folder the rollout was recorded in
	id     string // Codex: the thread id
}

// ScanHome writes the records under a home that match q, newest file first and up to limit: Claude Code's as
// grep -r prints them ("./<rel path>:<raw line>"), Codex's as "codex:<cwd>␟<thread id>␟<raw line>".
func ScanHome(home, q, dir string, since time.Time, limit int, w io.Writer) error {
	roots := transcript.SessionRoots(home)
	files, err := claudeFiles(roots[transcript.Claude], dir, since)
	if err != nil {
		return err
	}
	files = append(files, codexFiles(roots[transcript.Codex], dir, since)...)
	return scan(files, q, since, limit, w)
}

// Scan is ScanHome over Claude Code's project files alone.
func Scan(projects, q, dir string, since time.Time, limit int, w io.Writer) error {
	files, err := claudeFiles(projects, dir, since)
	if err != nil {
		return err
	}
	return scan(files, q, since, limit, w)
}

func claudeFiles(projects, dir string, since time.Time) ([]scanFile, error) {
	projects = filepath.Clean(projects)
	if real, err := filepath.EvalSymlinks(projects); err == nil {
		projects = real // WalkDir does not follow a symlinked root
	}
	var files []scanFile
	project := ""
	if dir != "" {
		project = transcript.EncodePath(dir)
	}
	err := filepath.WalkDir(projects, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "subagents" || (project != "" && filepath.Dir(p) == projects && d.Name() != project) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || (!since.IsZero() && info.ModTime().Before(since)) {
			return nil
		}
		rel, _ := filepath.Rel(projects, p)
		files = append(files, scanFile{path: p, mod: info.ModTime(), format: transcript.Claude, rel: "./" + filepath.ToSlash(rel)})
		return nil
	})
	return files, err
}

func codexFiles(sessions, dir string, since time.Time) []scanFile {
	var files []scanFile
	for _, s := range transcript.CodexFiles(sessions) {
		if !since.IsZero() && s.ModTime.Before(since) {
			continue
		}
		cwd := transcript.CodexCwd(s.Path)
		if dir != "" && cwd != dir {
			continue
		}
		files = append(files, scanFile{path: s.Path, mod: s.ModTime, format: transcript.Codex, cwd: cwd, id: s.ID})
	}
	return files
}

// scan sieves each file's lines by the four spellings (a byte search) before the format's MatchLine parses them.
func scan(files []scanFile, q string, since time.Time, limit int, w io.Writer) error {
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	var pats [][]byte
	seen := map[string]bool{}
	for _, v := range spellings(q) {
		if !seen[v] {
			seen[v] = true
			pats = append(pats, []byte(v))
		}
	}
	out := bufio.NewWriter(w)
	defer out.Flush()
	n := 0
	for _, f := range files {
		fh, err := os.Open(f.path)
		if err != nil {
			continue
		}
		done := false
		_ = transcript.Lines(fh, lineCap, func(line []byte) { // lines past lineCap are dropped: the parser on the other end reads up to it
			if done || !anyOf(line, pats) {
				return
			}
			m, ok := matchLine(f.format, line, q)
			if !ok || (!since.IsZero() && m.Ts.Before(since)) {
				return
			}
			if f.format == transcript.Codex {
				out.WriteString("codex:" + f.cwd + script.US + f.id + script.US)
			} else {
				out.WriteString(f.rel + ":")
			}
			out.Write(line)
			out.WriteByte('\n')
			if n++; n >= limit {
				done = true
			}
		})
		fh.Close()
		if done {
			return nil
		}
	}
	return nil
}

func matchLine(format transcript.Format, line []byte, q string) (transcript.Match, bool) {
	if format == transcript.Codex {
		return transcript.MatchCodexLine(line, q)
	}
	return transcript.MatchLine(line, q)
}

func anyOf(line []byte, pats [][]byte) bool {
	for _, p := range pats {
		if bytes.Contains(line, p) {
			return true
		}
	}
	return false
}

// spellings are the forms a case-folded match can take in the raw JSON: as typed, lower, UPPER and Capitalized.
// grep -i folds ASCII only and a byte sieve folds nothing, so the forms are spelled out.
func spellings(q string) []string {
	out := make([]string, 0, 4)
	for _, v := range []string{q, strings.ToLower(q), strings.ToUpper(q), capitalized(q)} {
		out = append(out, jsonText(v))
	}
	return out
}
