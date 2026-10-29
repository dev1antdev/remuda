package usage

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

// Report is what a period cost on one machine: by day, by project, by model, and the dearest sessions.
type Report struct {
	Since    string          `json:"since"`
	Total    Totals          `json:"total"`
	Days     []DayTotals     `json:"days"`
	Projects []ProjectTotals `json:"projects"`
	Models   []ModelTotals   `json:"models"`
	Sessions []SessionTotals `json:"sessions"`
	Files    int             `json:"files"` // transcript files read
}

type DayTotals struct {
	Date string `json:"date"` // YYYY-MM-DD in the viewer's zone
	Totals
}

type ProjectTotals struct {
	Project string `json:"project"` // the project directory's name
	Dir     string `json:"dir"`     // the folder the records name
	Totals
	Sessions int `json:"sessions"`
}

type ModelTotals struct {
	Model string `json:"model"`
	Name  string `json:"name"`
	Totals
}

type SessionTotals struct {
	Session string `json:"session"`
	Project string `json:"project"`
	Dir     string `json:"dir"`
	Model   string `json:"model"` // the model of most of its turns
	Totals
	First string `json:"first"`
	Last  string `json:"last"`
}

const topSessions = 40

// ScanHome sums every transcript under a home written since: Claude Code's project files and Codex's rollouts.
func ScanHome(home string, since time.Time, loc *time.Location) (Report, error) {
	b := newBuilder(since, loc)
	roots := transcript.SessionRoots(home)
	if err := b.walkClaude(roots[transcript.Claude]); err != nil {
		return b.report(), err
	}
	b.walkCodex(roots[transcript.Codex])
	return b.report(), nil
}

// Scan sums the Claude Code transcripts under projects written since (an older file holds nothing newer), days cut in loc.
// <session>/subagents/*.jsonl count toward their session.
func Scan(projects string, since time.Time, loc *time.Location) (Report, error) {
	b := newBuilder(since, loc)
	err := b.walkClaude(projects)
	return b.report(), err
}

// builder sums files into one report.
type builder struct {
	since        time.Time
	loc          *time.Location
	rep          Report
	days         map[string]*Totals
	projs        map[string]*ProjectTotals
	projSessions map[string]map[string]bool
	modelsT      map[string]*Totals
	sessions     map[string]*SessionTotals
	sessModels   map[string]map[string]int
}

func newBuilder(since time.Time, loc *time.Location) *builder {
	if loc == nil {
		loc = time.Local
	}
	return &builder{
		since: since, loc: loc,
		rep:          Report{Since: since.UTC().Format(time.RFC3339), Days: []DayTotals{}, Projects: []ProjectTotals{}, Models: []ModelTotals{}, Sessions: []SessionTotals{}},
		days:         map[string]*Totals{},
		projs:        map[string]*ProjectTotals{},
		projSessions: map[string]map[string]bool{},
		modelsT:      map[string]*Totals{},
		sessions:     map[string]*SessionTotals{},
		sessModels:   map[string]map[string]int{},
	}
}

func (b *builder) walkClaude(projects string) error {
	projects = filepath.Clean(projects)
	if real, err := filepath.EvalSymlinks(projects); err == nil {
		projects = real
	}
	return filepath.WalkDir(projects, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || (!b.since.IsZero() && info.ModTime().Before(b.since)) {
			return nil
		}
		rel, _ := filepath.Rel(projects, p)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 2 {
			return nil
		}
		project := parts[0]
		session := strings.TrimSuffix(parts[1], ".jsonl")
		if len(parts) >= 4 && parts[2] == "subagents" {
			session = parts[1] // <project>/<session>/subagents/agent-x.jsonl
		} else if len(parts) != 2 {
			return nil
		}
		b.file(p, project, session)
		return nil
	})
}

// walkCodex sums the rollouts under ~/.codex/sessions: a session is the thread id its name carries, its project the
// folder its first record names, encoded as Claude Code would, so a folder's budget covers both agents.
func (b *builder) walkCodex(sessions string) {
	for _, s := range transcript.CodexFiles(sessions) {
		if !b.since.IsZero() && s.ModTime.Before(b.since) {
			continue
		}
		b.file(s.Path, "", s.ID)
	}
}

// file sums one transcript; a project of "" is taken from the folder its records name.
func (b *builder) file(p, project, session string) {
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	b.rep.Files++
	dir := ""
	var first, last time.Time
	acc := Accumulator{OnCommit: func(r transcript.UsageRecord, t Totals) {
		day := r.At.In(b.loc).Format("2006-01-02")
		if b.days[day] == nil {
			b.days[day] = &Totals{}
		}
		b.days[day].add(t)
		mk := r.Model
		if m, ok := Lookup(r.Model); ok {
			mk = m.ID
		}
		if b.modelsT[mk] == nil {
			b.modelsT[mk] = &Totals{}
		}
		b.modelsT[mk].add(t)
		if b.sessModels[session] == nil {
			b.sessModels[session] = map[string]int{}
		}
		b.sessModels[session][mk]++
		if dir == "" && r.Cwd != "" {
			dir = r.Cwd
		}
		if first.IsZero() || r.At.Before(first) {
			first = r.At
		}
		if r.At.After(last) {
			last = r.At
		}
	}}
	scanner := transcript.NewUsageScanner(p)
	_ = transcript.Lines(f, 64*1024*1024, func(line []byte) {
		r, ok := scanner.Scan(line)
		if !ok || (!b.since.IsZero() && r.At.Before(b.since)) {
			return
		}
		acc.Add(r)
	})
	acc.Flush()
	if acc.Turns == 0 {
		return
	}
	if project == "" {
		project = transcript.EncodePath(dir)
	}
	if b.projs[project] == nil {
		b.projs[project] = &ProjectTotals{Project: project, Dir: dir}
		b.projSessions[project] = map[string]bool{}
	}
	b.projs[project].add(acc.Totals)
	if b.projs[project].Dir == "" {
		b.projs[project].Dir = dir
	}
	b.projSessions[project][session] = true
	st := b.sessions[session]
	if st == nil {
		st = &SessionTotals{Session: session, Project: project, Dir: dir}
		b.sessions[session] = st
	}
	st.add(acc.Totals)
	if st.Dir == "" {
		st.Dir = dir
	}
	if st.First == "" || first.Format(time.RFC3339) < st.First {
		st.First = first.UTC().Format(time.RFC3339)
	}
	if l := last.UTC().Format(time.RFC3339); l > st.Last {
		st.Last = l
	}
	b.rep.Total.add(acc.Totals)
}

func (b *builder) report() Report {
	rep := b.rep
	for day, t := range b.days {
		rep.Days = append(rep.Days, DayTotals{Date: day, Totals: *t})
	}
	sort.Slice(rep.Days, func(i, j int) bool { return rep.Days[i].Date < rep.Days[j].Date })
	for _, p := range b.projs {
		p.Sessions = len(b.projSessions[p.Project])
		rep.Projects = append(rep.Projects, *p)
	}
	sort.Slice(rep.Projects, func(i, j int) bool { return rep.Projects[i].Cost > rep.Projects[j].Cost })
	for id, t := range b.modelsT {
		name := id
		if m, ok := Lookup(id); ok {
			name = m.Name
		}
		rep.Models = append(rep.Models, ModelTotals{Model: id, Name: name, Totals: *t})
	}
	sort.Slice(rep.Models, func(i, j int) bool { return rep.Models[i].Cost > rep.Models[j].Cost })
	for id, s := range b.sessions {
		best, n := "", 0
		for m, c := range b.sessModels[id] {
			if c > n {
				best, n = m, c
			}
		}
		s.Model = best
		rep.Sessions = append(rep.Sessions, *s)
	}
	sort.Slice(rep.Sessions, func(i, j int) bool { return rep.Sessions[i].Cost > rep.Sessions[j].Cost })
	if len(rep.Sessions) > topSessions {
		rep.Sessions = rep.Sessions[:topSessions]
	}
	return rep
}
