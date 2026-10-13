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

	"github.com/dev1antdev/remuda/internal/transcript"
)

// Scan writes the records under projects that match q, newest file first and up to limit, as grep -r prints them: "./<rel path>:<raw line>".
// A line is sieved by the four spellings (a byte search) before transcript.MatchLine parses it.
func Scan(projects, q, dir string, since time.Time, limit int, w io.Writer) error {
	type file struct {
		rel string
		mod time.Time
	}
	projects = filepath.Clean(projects)
	if real, err := filepath.EvalSymlinks(projects); err == nil {
		projects = real // WalkDir does not follow a symlinked root
	}
	var files []file
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
		files = append(files, file{rel: "./" + filepath.ToSlash(rel), mod: info.ModTime()})
		return nil
	})
	if err != nil {
		return err
	}
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
		fh, err := os.Open(filepath.Join(projects, filepath.FromSlash(f.rel)))
		if err != nil {
			continue
		}
		done := false
		_ = transcript.Lines(fh, lineCap, func(line []byte) { // lines past lineCap are dropped: the parser on the other end reads up to it
			if done || !anyOf(line, pats) {
				return
			}
			m, ok := transcript.MatchLine(line, q)
			if !ok || (!since.IsZero() && m.Ts.Before(since)) {
				return
			}
			out.WriteString(f.rel)
			out.WriteByte(':')
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
