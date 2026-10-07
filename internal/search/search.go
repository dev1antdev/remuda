// Package search finds what was said across every Claude transcript on
// every machine. There is no index: a search runs one grep per machine over
// ~/.claude/projects (the same shell path the review lens uses, so a
// remote machine needs nothing but its own grep), and the matching records
// come back raw to be parsed here by the transcript package — only the
// user and assistant texts the chat shows count, not tool output. 114 MB
// of transcripts grep in well under a second.
package search

import (
	"bufio"
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/sshx"
	"github.com/dev1antdev/herdr-ui/internal/transcript"
)

type Hit struct {
	Machine string    `json:"machine"`
	Project string    `json:"project"` // the project directory's name, EncodePath of the folder Claude was started in
	Dir     string    `json:"dir"`     // the folder at that moment (a Bash cd moves it), for the label
	Session string    `json:"session"` // the transcript file's id
	UUID    string    `json:"uuid"`    // the record, for the jump
	Ts      time.Time `json:"ts"`
	Role    string    `json:"role"` // user | assistant
	Snippet string    `json:"snippet"`
}

type Result struct {
	Hits   []Hit             `json:"hits"`
	Errors map[string]string `json:"errors,omitempty"` // per machine that could not be searched
	More   bool              `json:"more"`             // cut at the limit; narrow it down
}

// Version is this build's; a remote binary is used only when it prints the
// same, so an older herdr-ui there (one without the subcommand, which
// would start a server instead) is never run.
var Version = "dev"

const (
	perMachine = 400
	limit      = 200
	lineCap    = 300000   // a record longer than this is a tool result, cut
	outputCap  = 20 << 20 // bytes per machine
	timeout    = 45 * time.Second
)

// Search runs on the given machines at once. dir narrows to one project
// folder (the one an agent was started in; records made after a cd are in
// the same file and count), since to records after a time.
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
		if out, err = mc.Shell(script(mc.Home(), q, dir, since), timeout); err != nil {
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

// script runs the scan on a remote machine: herdr-ui's own scanner where
// `make deploy` put the binary (fast — a 400 MB history in well under a
// second), else grep over the project transcripts for the query as it
// sits in the JSON (quotes and newlines escaped), in the four spellings
// that fold case where grep's own -i would not (non-ASCII letters). The
// BSD grep is slow with several patterns, seconds per hundred megabytes,
// which is what the scanner is for. Either way the parser here folds
// properly afterwards.
func script(home, q, dir string, since time.Time) string {
	seen := map[string]bool{}
	var pats []string
	for _, v := range spellings(q) {
		if !seen[v] {
			seen[v] = true
			pats = append(pats, "-e "+sshx.Quote(v))
		}
	}
	bin := sshx.Quote(home + "/.local/bin/herdr-ui")
	args := "search -q " + sshx.Quote(q) + " -limit " + itoa(perMachine)
	if dir != "" {
		args += " -dir " + sshx.Quote(dir)
	}
	if !since.IsZero() {
		args += " -since " + sshx.Quote(since.UTC().Format(time.RFC3339))
	}
	return "if [ -x " + bin + " ] && [ \"$(" + bin + " -version 2>/dev/null)\" = " + sshx.Quote("herdr-ui "+Version) + " ] && " +
		bin + " " + args + "; then exit 0; fi; " +
		"cd " + sshx.Quote(home+"/.claude/projects") + " 2>/dev/null || exit 0; " +
		"grep -raF --include='*.jsonl' " + strings.Join(pats, " ") + " . 2>/dev/null | " +
		"cut -b1-" + itoa(lineCap) + " | head -c " + itoa(outputCap) + "; exit 0"
}

// jsonText is s as it appears inside a JSON string, without the quotes
// and without HTML escaping (transcripts keep < and & as they are).
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

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
