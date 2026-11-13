package usage

import "time"

// Year is the heatmap's answer: the last YearDays local days on every machine, each day's spend by folder, and each
// folder's sessions and models over the year, so the page can leave out a folder hidden while the screen is shared.
type Year struct {
	From     string            `json:"from"` // the first day shown, YYYY-MM-DD
	To       string            `json:"to"`   // today
	Zone     string            `json:"zone"`
	Names    map[string]string `json:"names"` // a model's id → its name
	Machines []MachineYear     `json:"machines"`
}

// MachineYear is one machine's year as far as it was read.
type MachineYear struct {
	ID      string       `json:"id"`
	Label   string       `json:"label"`
	Reading int          `json:"reading,omitempty"`  // the ended days never read, being read now
	Busy    bool         `json:"busy,omitempty"`     // a read under way, of those days or of today
	Today   string       `json:"today_at,omitempty"` // when today was last read, RFC 3339
	Away    bool         `json:"away,omitempty"`     // not connected: what was kept, nothing read
	Error   string       `json:"error,omitempty"`    // why the last read failed
	Folders []FolderYear `json:"folders"`
	Days    []DayCells   `json:"days"` // the days with anything in them
}

// FolderYear is one folder over the year: how many sessions it held, the longest one, its models by tokens.
type FolderYear struct {
	Project  string           `json:"project"`
	Dir      string           `json:"dir,omitempty"`
	Sessions int              `json:"sessions"`
	Longest  int64            `json:"longest"` // seconds from its longest session's first response to its last
	Models   map[string]int64 `json:"models,omitempty"`
}

// DayCells is one day's spend, a cell per folder.
type DayCells struct {
	Date  string    `json:"date"`
	Cells []DayCell `json:"cells"`
}

// DayCell is one folder's day, its folder an index into the machine's folders.
type DayCell struct {
	Folder int     `json:"f"`
	Cost   float64 `json:"c"`
	Tokens int64   `json:"t"`
	Turns  int     `json:"n"`
}

// year is the shelf's part of the answer: the kept days and today's reading over the given days.
func (s *shelf) year(host YearHost, days []string, today string, names map[string]string) MachineYear {
	out := MachineYear{ID: host.ID(), Label: host.Label(), Reading: s.pending, Busy: s.busy, Away: host.Away, Error: s.failed, Folders: []FolderYear{}, Days: []DayCells{}}
	if !s.todayAt.IsZero() {
		out.Today = s.todayAt.UTC().Format(time.RFC3339)
	}
	index := map[string]int{}
	spans := []map[string]Span{}
	for _, date := range days {
		day, ok := s.Days[date]
		if !ok && date == today && s.todayDate == today && s.today != nil {
			day, ok = *s.today, true
		}
		if !ok || len(day.Folders) == 0 {
			continue
		}
		cells := DayCells{Date: date, Cells: make([]DayCell, 0, len(day.Folders))}
		for _, folder := range day.Folders {
			i, seen := index[folder.Project]
			if !seen {
				i = len(out.Folders)
				index[folder.Project] = i
				out.Folders = append(out.Folders, FolderYear{Project: folder.Project, Dir: folder.Dir, Models: map[string]int64{}})
				spans = append(spans, map[string]Span{})
			}
			kept := &out.Folders[i]
			if kept.Dir == "" {
				kept.Dir = folder.Dir
			}
			for model, n := range folder.Models {
				kept.Models[model] += n
				if _, named := names[model]; !named {
					names[model] = modelName(model)
				}
			}
			for id, span := range folder.Sessions {
				spans[i][id] = spans[i][id].widen(span)
			}
			cells.Cells = append(cells.Cells, DayCell{Folder: i, Cost: folder.Cost, Tokens: folder.Tokens, Turns: folder.Turns})
		}
		out.Days = append(out.Days, cells)
	}
	for i := range out.Folders {
		out.Folders[i].Sessions = len(spans[i])
		for _, span := range spans[i] {
			out.Folders[i].Longest = max(out.Folders[i].Longest, span.Last-span.First)
		}
	}
	return out
}

// modelName is the catalog's name for a model, else the id as the file gives it.
func modelName(id string) string {
	if m, ok := Lookup(id); ok {
		return m.Name
	}
	return id
}
