package usage

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// MachineReport is one machine's report in the fleet's answer, or why it has none.
type MachineReport struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Report *Report `json:"report,omitempty"`
	Error  string  `json:"error,omitempty"`
}

// Collect runs the report on every host at once: in process here, through the machine's own remuda on a remote.
func Collect(hosts []shell.Host, since time.Time, loc *time.Location, tz string) []MachineReport {
	out := []MachineReport{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(host shell.Host) {
			defer wg.Done()
			report := MachineReport{ID: host.ID(), Label: host.Label()}
			if rep, err := ReadHost(host, since, loc, tz, false, 90*time.Second); err != nil {
				report.Error = err.Error()
			} else {
				report.Report = &rep
			}
			mu.Lock()
			out = append(out, report)
			mu.Unlock()
		}(host)
	}
	wg.Wait()
	return out
}

// errNoReport is a remote remuda that answered something other than a report.
var errNoReport = errors.New("the machine's remuda gave no report")

// ReadHost is one machine's report since a moment, its days by folder when daily: in process here, through the
// machine's own remuda on a remote.
func ReadHost(host shell.Host, since time.Time, loc *time.Location, tz string, daily bool, timeout time.Duration) (Report, error) {
	roots := transcript.RootsFor(host)
	var rep Report
	var err error
	if host.Local() {
		if daily {
			rep, err = ScanRootsDays(roots, since, loc)
		} else {
			rep, err = ScanRoots(roots, since, loc)
		}
	} else {
		run := ScriptRoots
		if daily {
			run = DaysScriptRoots
		}
		var raw string
		raw, err = shell.Text(host, run(roots, host.Home(), since, tz), timeout)
		if err == nil && json.Unmarshal([]byte(raw), &rep) != nil {
			return Report{}, errNoReport
		}
		if err != nil {
			err = errors.New(strings.TrimSpace(err.Error()))
		}
	}
	if err != nil {
		return Report{}, err
	}
	// opencode keeps its sessions in a database, not under a transcript root: a machine without one adds nothing
	if responses, e := transcript.NewReader(host).OpencodeSince(since); e == nil && len(responses) > 0 {
		b := newBuilder(since, loc)
		if daily {
			b.daily = map[string]map[string]*DayFolder{}
		}
		b.addOpencode(responses)
		rep = mergeReports(rep, b.report())
	}
	return rep, nil
}

// addOpencode sums opencode's answers into a report, session by session, as the files' builder does.
func (b *builder) addOpencode(responses []transcript.OpencodeResponse) {
	for _, r := range responses {
		if !b.since.IsZero() && r.At.Before(b.since) {
			continue
		}
		t := Totals{Tokens: Tokens{Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, CacheWrite5m: r.CacheWrite}, Cost: r.Cost, Turns: 1}
		if m, ok := Lookup(r.Model); ok {
			t.Cost = Cost(m, t.Tokens, "", "")
		} else if r.Cost == 0 {
			t.Unknown = 1
		}
		day := r.At.In(b.loc).Format("2006-01-02")
		if b.days[day] == nil {
			b.days[day] = &Totals{}
		}
		b.days[day].add(t)
		model := r.Model
		if m, ok := Lookup(r.Model); ok {
			model = m.ID
		}
		if b.modelsT[model] == nil {
			b.modelsT[model] = &Totals{}
		}
		b.modelsT[model].add(t)
		dir := r.Dir
		project := filepath.Base(dir)
		if project == "" || project == "." || project == string(filepath.Separator) {
			project = r.Session
		}
		if b.projs[project] == nil {
			b.projs[project] = &ProjectTotals{Project: project, Dir: dir}
			b.projSessions[project] = map[string]bool{}
		}
		b.projs[project].add(t)
		b.projSessions[project][r.Session] = true
		st := b.sessions[r.Session]
		if st == nil {
			st = &SessionTotals{Session: r.Session, Project: project, Dir: dir, Model: r.Model}
			b.sessions[r.Session] = st
			b.sessModels[r.Session] = map[string]int{}
		}
		if st.Dir == "" {
			st.Dir = dir
		}
		st.add(t)
		at := r.At.UTC().Format(time.RFC3339)
		if st.First == "" || at < st.First {
			st.First = at
		}
		if at > st.Last {
			st.Last = at
		}
		b.sessModels[r.Session][r.Model]++
		b.rep.Total.add(t)
		if b.daily != nil {
			if b.daily[day] == nil {
				b.daily[day] = map[string]*DayFolder{}
			}
			folder := b.daily[day][project]
			if folder == nil {
				folder = &DayFolder{Project: project, Dir: dir}
				b.daily[day][project] = folder
			}
			folder.note(transcript.UsageRecord{MessageID: r.MessageID, Model: r.Model, At: r.At, Cwd: dir}, t, model, r.Session)
		}
	}
}

// mergeReports adds a second report's figures, days and folders to the first: opencode's to the transcripts'.
func mergeReports(base, extra Report) Report {
	base.Total.add(extra.Total)
	base.Files += extra.Files
	days := map[string]int{}
	for i, day := range base.Days {
		days[day.Date] = i
	}
	for _, day := range extra.Days {
		if i, ok := days[day.Date]; ok {
			base.Days[i].add(day.Totals)
			continue
		}
		days[day.Date] = len(base.Days)
		base.Days = append(base.Days, day)
	}
	sort.Slice(base.Days, func(i, j int) bool { return base.Days[i].Date < base.Days[j].Date })
	projects := map[string]int{}
	for i, project := range base.Projects {
		projects[project.Project] = i
	}
	for _, project := range extra.Projects {
		if i, ok := projects[project.Project]; ok {
			base.Projects[i].add(project.Totals)
			base.Projects[i].Sessions += project.Sessions
			continue
		}
		projects[project.Project] = len(base.Projects)
		base.Projects = append(base.Projects, project)
	}
	sort.Slice(base.Projects, func(i, j int) bool { return base.Projects[i].Cost > base.Projects[j].Cost })
	models := map[string]int{}
	for i, model := range base.Models {
		models[model.Model] = i
	}
	for _, model := range extra.Models {
		if i, ok := models[model.Model]; ok {
			base.Models[i].add(model.Totals)
			continue
		}
		models[model.Model] = len(base.Models)
		base.Models = append(base.Models, model)
	}
	sort.Slice(base.Models, func(i, j int) bool { return base.Models[i].Cost > base.Models[j].Cost })
	base.Sessions = append(base.Sessions, extra.Sessions...)
	sort.Slice(base.Sessions, func(i, j int) bool { return base.Sessions[i].Cost > base.Sessions[j].Cost })
	if len(base.Sessions) > topSessions {
		base.Sessions = base.Sessions[:topSessions]
	}
	if len(extra.Daily) > 0 {
		index := map[string]map[string]*DayFolder{}
		var dates []string
		add := func(day Day) {
			if _, ok := index[day.Date]; !ok {
				index[day.Date] = map[string]*DayFolder{}
				dates = append(dates, day.Date)
			}
			for i := range day.Folders {
				folder := day.Folders[i]
				if kept := index[day.Date][folder.Project]; kept != nil {
					kept.merge(&folder)
				} else {
					index[day.Date][folder.Project] = &folder
				}
			}
		}
		for _, day := range base.Daily {
			add(day)
		}
		for _, day := range extra.Daily {
			add(day)
		}
		sort.Strings(dates)
		base.Daily = nil
		for _, date := range dates {
			day := Day{Date: date}
			for _, folder := range index[date] {
				day.Folders = append(day.Folders, *folder)
			}
			sort.Slice(day.Folders, func(i, j int) bool { return day.Folders[i].Project < day.Folders[j].Project })
			base.Daily = append(base.Daily, day)
		}
	}
	return base
}
