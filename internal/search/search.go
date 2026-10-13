// Package search finds what was said across every Claude transcript on every machine.
// There is no index: each machine scans its own ~/.claude/projects on demand, in-process
// here and through the remuda deployed on a remote (`remuda search`), else a plain grep.
package search

import (
	"bufio"
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/version"
)

type Hit struct {
	Machine string    `json:"machine"`
	Project string    `json:"project"` // the project directory's name: EncodePath of the folder Claude was started in
	Dir     string    `json:"dir"`     // the folder at that moment (a cd moves it)
	Session string    `json:"session"`
	UUID    string    `json:"uuid"`
	Ts      time.Time `json:"ts"`
	Role    string    `json:"role"` // user | assistant
	Snippet string    `json:"snippet"`
}

type Result struct {
	Hits   []Hit             `json:"hits"`
	Errors map[string]string `json:"errors,omitempty"` // per machine that could not be searched
	More   bool              `json:"more"`             // cut at the limit
}

const (
	perMachine = 400
	limit      = 200
	lineCap    = 300000   // a record longer than this is a tool result, cut
	outputCap  = 20 << 20 // bytes per machine
	timeout    = 45 * time.Second
)

// Search runs on the given machines at once; dir narrows to one project folder, since to records after a time.
func Search(mcs []*machines.Machine, q, dir string, since time.Time) Result {
	res := Result{Hits: []Hit{}, Errors: map[string]string{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, mc := range mcs {
		wg.Add(1)
		go func(mc *machines.Machine) {
			defer wg.Done()
			hits, err := one(mc, q, dir, since)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				res.Errors[mc.Cfg.ID] = err.Error()
			}
			res.Hits = append(res.Hits, hits...)
		}(mc)
	}
	wg.Wait()
	sort.Slice(res.Hits, func(i, j int) bool { return res.Hits[i].Ts.After(res.Hits[j].Ts) })
	if len(res.Hits) > limit {
		res.Hits = res.Hits[:limit]
		res.More = true
	}
	if len(res.Errors) == 0 {
		res.Errors = nil
	}
	return res
}

func one(mc *machines.Machine, q, dir string, since time.Time) ([]Hit, error) {
	var out string
	if mc.Local {
		var b strings.Builder
		if err := Scan(mc.Home()+"/.claude/projects", q, dir, since, perMachine, &b); err != nil {
			return nil, err
		}
		out = b.String()
	} else {
		var err error
		if out, err = mc.Shell(remoteScript(mc.Home(), q, dir, since), timeout); err != nil {
			return nil, err
		}
	}
	var hits []Hit
	seen := map[string]bool{} // a scanner that failed half-way is followed by grep: no doubles
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 2*lineCap)
	for sc.Scan() {
		line := sc.Bytes()
		// ./<project>/<session>.jsonl:<record>
		i := bytes.IndexByte(line, ':')
		if i <= 0 {
			continue
		}
		path, rec := string(line[:i]), line[i+1:]
		project := projectOf(path)
		if project == "" || strings.Contains(path, "/subagents/") || (dir != "" && project != transcript.EncodePath(dir)) {
			continue
		}
		m, ok := transcript.MatchLine(rec, q)
		if !ok || (!since.IsZero() && m.Ts.Before(since)) || seen[path+m.UUID] {
			continue
		}
		seen[path+m.UUID] = true
		hits = append(hits, Hit{
			Machine: mc.Cfg.ID, Project: project, Dir: m.Cwd, Session: transcript.SessionID(path),
			UUID: m.UUID, Ts: m.Ts, Role: m.Role, Snippet: m.Snippet,
		})
	}
	// grep prints in directory order; the newest are the ones to keep
	sort.Slice(hits, func(i, j int) bool { return hits[i].Ts.After(hits[j].Ts) })
	if len(hits) > perMachine {
		hits = hits[:perMachine]
	}
	return hits, sc.Err()
}

// remoteScript runs the scan on a remote machine: remuda's own scanner where `make deploy` put it, else grep.
// The grep takes the four spellings as they sit in the JSON (grep -i folds ASCII only); BSD grep is seconds per hundred MB.
func remoteScript(home, q, dir string, since time.Time) string {
	seen := map[string]bool{}
	var pats []string
	for _, v := range spellings(q) {
		if !seen[v] {
			seen[v] = true
			pats = append(pats, "-e "+script.Quote(v))
		}
	}
	bin, guard := script.Remuda(home, version.Build)
	args := "search -q " + script.Quote(q) + " -limit " + strconv.Itoa(perMachine)
	if dir != "" {
		args += " -dir " + script.Quote(dir)
	}
	if !since.IsZero() {
		args += " -since " + script.Quote(since.UTC().Format(time.RFC3339))
	}
	return "if " + guard + " && " + bin + " " + args + "; then exit 0; fi; " +
		"cd " + script.Quote(home+"/.claude/projects") + " 2>/dev/null || exit 0; " +
		"grep -raF --include='*.jsonl' " + strings.Join(pats, " ") + " . 2>/dev/null | " +
		"cut -b1-" + strconv.Itoa(lineCap) + " | head -c " + strconv.Itoa(outputCap) + "; exit 0"
}

// jsonText is s as it appears inside a JSON string, without the quotes and without HTML escaping.
func jsonText(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	out := strings.TrimSpace(b.String())
	return out[1 : len(out)-1]
}

// projectOf is the first directory of "./<project>/<session>.jsonl".
func projectOf(path string) string {
	rest := strings.TrimPrefix(path, "./")
	project, _, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	return project
}

func capitalized(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + strings.ToLower(s[n:])
}
