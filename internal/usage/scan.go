package usage

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/transcript"
)

// Report is what a period cost on one machine: by day, by project, by
// model, and the sessions that cost the most.
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

// Scan sums the transcripts under projects from since on: a file not
// written since then holds nothing newer. Subagent transcripts
// (<session>/subagents/*.jsonl) count toward their session. Days are cut
// in loc.
func Scan(projects string, since time.Time, loc *time.Location) (Report, error) {
	projects = filepath.Clean(projects)
	if real, err := filepath.EvalSymlinks(projects); err == nil {
		projects = real
	}
	if loc == nil {
		loc = time.Local
	}
	rep := Report{Since: since.UTC().Format(time.RFC3339), Days: []DayTotals{}, Projects: []ProjectTotals{}, Models: []ModelTotals{}, Sessions: []SessionTotals{}}
	days := map[string]*Totals{}
	projs := map[string]*ProjectTotals{}
	projSessions := map[string]map[string]bool{}
	modelsT := map[string]*Totals{}
	sessions := map[string]*SessionTotals{}
	sessModels := map[string]map[string]int{}

	err := filepath.WalkDir(projects, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || (!since.IsZero() && info.ModTime().Before(since)) {
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
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		rep.Files++
		// every response once (the accumulator's rule), attributed as it
		// commits: to its day, its model, its session
		dir := ""
		var first, last time.Time
		acc := Accumulator{OnCommit: func(r transcript.UsageRecord, t Totals) {
			day := r.At.In(loc).Format("2006-01-02")
			if days[day] == nil {
				days[day] = &Totals{}
			}
			days[day].add(t)
			mk := r.Model
			if m, ok := Lookup(r.Model); ok {
				mk = m.ID
			}
			if modelsT[mk] == nil {
				modelsT[mk] = &Totals{}
			}
			modelsT[mk].add(t)
			if sessModels[session] == nil {
				sessModels[session] = map[string]int{}
			}
			sessModels[session][mk]++
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
		_ = transcript.Lines(f, 64*1024*1024, func(line []byte) {
			if !bytes.Contains(line, []byte(`"usage"`)) {
				return
			}
			r, ok := transcript.UsageOf(line)
			if !ok || (!since.IsZero() && r.At.Before(since)) {
				return
			}
			acc.Add(r)
		})
		acc.Flush()
		if acc.Turns == 0 {
			return nil
		}
		if projs[project] == nil {
			projs[project] = &ProjectTotals{Project: project, Dir: dir}
			projSessions[project] = map[string]bool{}
		}
		projs[project].add(acc.Totals)
		if projs[project].Dir == "" {
			projs[project].Dir = dir
		}
		projSessions[project][session] = true
		st := sessions[session]
		if st == nil {
			st = &SessionTotals{Session: session, Project: project, Dir: dir}
			sessions[session] = st
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
		rep.Total.add(acc.Totals)
		return nil
	})
	if err != nil {
		return rep, err
	}
	for day, t := range days {
		rep.Days = append(rep.Days, DayTotals{Date: day, Totals: *t})
	}
	sort.Slice(rep.Days, func(i, j int) bool { return rep.Days[i].Date < rep.Days[j].Date })
	for _, p := range projs {
		p.Sessions = len(projSessions[p.Project])
		rep.Projects = append(rep.Projects, *p)
	}
	sort.Slice(rep.Projects, func(i, j int) bool { return rep.Projects[i].Cost > rep.Projects[j].Cost })
	for id, t := range modelsT {
		name := id
		if m, ok := Lookup(id); ok {
			name = m.Name
		}
		rep.Models = append(rep.Models, ModelTotals{Model: id, Name: name, Totals: *t})
	}
	sort.Slice(rep.Models, func(i, j int) bool { return rep.Models[i].Cost > rep.Models[j].Cost })
	for id, s := range sessions {
		best, n := "", 0
		for m, c := range sessModels[id] {
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
	return rep, nil
}
