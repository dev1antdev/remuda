package search

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
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

// ScanRoots writes the records under the session folders one machine knows, including those its agents named for
// themselves, that match q, newest file first and up to limit: Claude Code's as grep -r prints them
// ("./<rel path>:<raw line>"), Codex's as "codex:<cwd>␟<thread id>␟<raw line>".
func ScanRoots(roots transcript.Roots, q, dir string, since time.Time, limit int, w io.Writer) error {
	var files []scanFile
	for _, projects := range roots[transcript.Claude] {
		found, err := claudeFiles(projects, dir, since)
		if err != nil {
			return err
		}
		files = append(files, found...)
	}
	for _, folder := range roots[transcript.Codex] {
		files = append(files, codexFiles(folder, dir, since)...)
	}
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
		if !strings.HasSuffix(d.Name(), ".jsonl") || !d.Type().IsRegular() {
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
	// the newest hits are what the page shows, and a transcript's lines come oldest first: a bounded set keeps the
	// newest as it goes instead of stopping at the first limit lines of the oldest session
	kept := make([]scanHit, 0, max(limit, 1))
	for _, f := range files {
		fh, err := transcript.OpenSession(f.path)
		if err != nil {
			continue
		}
		where := f.rel + ":"
		if f.format == transcript.Codex {
			where = "codex:" + f.cwd + script.US + f.id + script.US
		}
		_ = transcript.Lines(fh, lineCap, func(line []byte) { // lines past lineCap are dropped: the parser on the other end reads up to it
			if !anyOf(line, pats) {
				return
			}
			m, ok := matchLine(f.format, line, q)
			if !ok || (!since.IsZero() && m.Ts.Before(since)) {
				return
			}
			kept = append(kept, scanHit{ts: m.Ts, line: where + string(line)})
			if len(kept) > 2*limit {
				sort.Slice(kept, func(i, j int) bool { return kept[i].ts.After(kept[j].ts) })
				kept = kept[:limit]
			}
		})
		fh.Close()
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].ts.After(kept[j].ts) })
	if len(kept) > limit {
		kept = kept[:limit]
	}
	out := bufio.NewWriter(w)
	defer out.Flush()
	for _, h := range kept {
		out.WriteString(h.line)
		out.WriteByte('\n')
	}
	return nil
}

// scanHit is one hit as the scan hands it on: when it was said, and the line it prints.
type scanHit struct {
	ts   time.Time
	line string
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
