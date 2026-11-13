package usage

import (
	"sort"

	"github.com/dev1antdev/remuda/internal/transcript"
)

// Day is what one local day held on a machine, folder by folder: the year's heatmap is made of them.
type Day struct {
	Date    string      `json:"date"` // YYYY-MM-DD in the viewer's zone
	Folders []DayFolder `json:"folders"`
}

// DayFolder is one folder's day: its responses, their tokens and price, the models by tokens, and each session's first
// and last response that day.
type DayFolder struct {
	Project  string           `json:"project"`
	Dir      string           `json:"dir,omitempty"`
	Cost     float64          `json:"cost"`
	Tokens   int64            `json:"tokens"`
	Turns    int              `json:"turns"`
	Models   map[string]int64 `json:"models,omitempty"`   // the catalog's id, else the name the file gives → tokens
	Sessions map[string]Span  `json:"sessions,omitempty"` // session id → its first and last response that day
}

// Span is a session's first and last response, unix seconds.
type Span struct {
	First int64 `json:"first"`
	Last  int64 `json:"last"`
}

func (s Span) widen(o Span) Span {
	if s.First == 0 || (o.First != 0 && o.First < s.First) {
		s.First = o.First
	}
	if o.Last > s.Last {
		s.Last = o.Last
	}
	return s
}

// tokenSum is every token a response read and wrote.
func tokenSum(t Tokens) int64 { return t.Context() + t.Output }

// note adds one priced response of a session to the folder's day.
func (f *DayFolder) note(r transcript.UsageRecord, t Totals, model, session string) {
	f.Cost += t.Cost
	f.Turns += t.Turns
	n := tokenSum(t.Tokens)
	f.Tokens += n
	if f.Models == nil {
		f.Models = map[string]int64{}
	}
	f.Models[model] += n
	if f.Sessions == nil {
		f.Sessions = map[string]Span{}
	}
	at := r.At.Unix()
	f.Sessions[session] = f.Sessions[session].widen(Span{First: at, Last: at})
}

// merge adds another reading of the same folder and day.
func (f *DayFolder) merge(o *DayFolder) {
	f.Cost += o.Cost
	f.Turns += o.Turns
	f.Tokens += o.Tokens
	if f.Dir == "" {
		f.Dir = o.Dir
	}
	for model, n := range o.Models {
		if f.Models == nil {
			f.Models = map[string]int64{}
		}
		f.Models[model] += n
	}
	for id, span := range o.Sessions {
		if f.Sessions == nil {
			f.Sessions = map[string]Span{}
		}
		f.Sessions[id] = f.Sessions[id].widen(span)
	}
}

// addDays files one transcript's days under its folder, once the folder is known.
func (b *builder) addDays(days map[string]*DayFolder, project, dir string) {
	for date, folder := range days {
		if b.daily[date] == nil {
			b.daily[date] = map[string]*DayFolder{}
		}
		kept := b.daily[date][project]
		if kept == nil {
			kept = &DayFolder{Project: project, Dir: dir}
			b.daily[date][project] = kept
		}
		kept.merge(folder)
	}
}

// dailyReport is the days in order, each day's folders by project.
func (b *builder) dailyReport() []Day {
	out := make([]Day, 0, len(b.daily))
	for date, folders := range b.daily {
		day := Day{Date: date, Folders: make([]DayFolder, 0, len(folders))}
		for _, folder := range folders {
			day.Folders = append(day.Folders, *folder)
		}
		sort.Slice(day.Folders, func(i, j int) bool { return day.Folders[i].Project < day.Folders[j].Project })
		out = append(out, day)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}
