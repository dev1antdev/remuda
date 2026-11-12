package timelapse

import (
	"context"
	"errors"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/scrollback"
)

// The search over the recordings: "when did it print `migration failed`?" A query is a literal, case-folded, found in
// the rows as the player draws them (plain.go); a match spanning two rows — a wrapped line — is not one. For each
// recording, newest first, it answers where the text first stood on the screen, the row with its neighbours as they
// read then, how long it stayed, and when it came back. One query unpacks at most SearchBudget bytes of recordings off
// the disk — past it only what the cache holds is searched — and runs at most SearchFor, the recordings it did not
// read counted; it holds the recorder's lock only to list the recordings, so a turn being recorded or written never
// waits on it.

// The bounds one query keeps to.
const (
	MinQuery     = 2               // runes a query needs
	MaxQuery     = 200             // runes a query keeps
	SearchBudget = 64 << 20        // bytes of recordings one query unpacks off the disk, the one that crosses it read whole; the cache's cost nothing
	SearchFor    = 3 * time.Second // how long one query reads
	MaxHits      = 100             // recordings with the text one query answers
	MaxAgain     = 20              // later appearances one hit lists; Times counts them all
	Context      = 2               // rows either side of the matching one
	ExcerptWidth = 120             // characters an excerpt's rows keep, cut around the match
	checkEvery   = 256             // frames between two looks at the clock
)

// Why a search stopped before the end of its scope.
const (
	CutBudget = "budget"
	CutTime   = "time"
)

// SearchQuery is what to find and where: a machine ("" for every one), a pane and a folder as List takes them (neither
// for every recording), recordings that ended after Since.
type SearchQuery struct {
	Text    string
	Machine string
	Pane    string
	Dir     string
	Since   time.Time
}

// Appearance is one stretch of time the text stood on the screen.
type Appearance struct {
	Frame int   `json:"frame"` // its first frame, by index among the recording's frames
	At    int64 `json:"at"`    // ms after the recording's start
	For   int64 `json:"for"`   // ms it stayed on screen
	Line  int   `json:"line"`  // the row it stood in, the topmost when it stood in several
}

// ScreenHit is one recording that showed the text: where it first appeared, the excerpt of the screen then, and the
// times it came back.
type ScreenHit struct {
	Recording string       `json:"recording"`
	Machine   string       `json:"machine"`
	Pane      string       `json:"pane"`
	Workspace string       `json:"workspace"`
	Dir       string       `json:"dir"`
	Folder    string       `json:"folder"`
	Agent     string       `json:"agent"`
	Title     string       `json:"title"`
	Start     time.Time    `json:"start"` // the recording's; an appearance's moment is Start + At
	First     Appearance   `json:"first"`
	Again     []Appearance `json:"again,omitempty"`
	Times     int          `json:"times"`
	Before    []string     `json:"before"` // the rows above the matching one, blank ones at the top left out
	Row       string       `json:"row"`
	After     []string     `json:"after"` // the rows below it, blank ones at the foot left out
	Col       int          `json:"col"`   // the column the excerpt's rows start at: 0 unless the row was cut on its left
}

// ScreenResult is a query's answer: the hits, newest recording first, and how much of the scope was read.
type ScreenResult struct {
	Hits     []ScreenHit `json:"hits"`
	Searched int         `json:"searched"`      // recordings read
	Unread   int         `json:"unread"`        // recordings of the scope left unread
	Cut      string      `json:"cut,omitempty"` // CutBudget or CutTime when some were left unread
	More     bool        `json:"more"`          // MaxHits reached: older recordings may show it too
}

// Search finds the text in the recordings of the scope, newest first, within the query's bounds; ctx ends it early.
func (r *Recorder) Search(ctx context.Context, q SearchQuery) ScreenResult {
	out := ScreenResult{Hits: []ScreenHit{}}
	needle := strings.ToLower(q.Text)
	if needle == "" {
		return out
	}
	scope := r.scope(q)
	since := r.plain.begin()
	deadline := r.now().Add(r.searchFor)
	late := func() bool { return ctx.Err() != nil || !r.now().Before(deadline) }
	var spent int64
	for i, e := range scope {
		if late() {
			out.Cut, out.Unread = CutTime, out.Unread+len(scope)-i
			break
		}
		plain, read, err := r.plainFor(e, spent, since)
		if errors.Is(err, errOverBudget) {
			out.Cut, out.Unread = CutBudget, out.Unread+1 // an older one the cache holds may still be searched
			continue
		}
		spent += read
		if err != nil {
			continue // gone since it was listed, or unreadable: nothing to find there
		}
		seen, stopped := sight(plain, needle, q.Text, late)
		if stopped {
			out.Cut, out.Unread = CutTime, out.Unread+len(scope)-i
			break
		}
		out.Searched++
		if seen == nil {
			continue
		}
		out.Hits = append(out.Hits, seen.hit(e.meta))
		if len(out.Hits) >= MaxHits {
			out.More = i < len(scope)-1
			out.Unread += len(scope) - i - 1
			break
		}
	}
	return out
}

// scope is the finished recordings the query names, the newest first; the recorder's lock only for the listing
func (r *Recorder) scope(q SearchQuery) []entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []entry
	for id, e := range r.index {
		m := e.meta
		if r.isLive(id) || (q.Machine != "" && m.Machine != q.Machine) || (!q.Since.IsZero() && m.End.Before(q.Since)) {
			continue
		}
		if (q.Pane != "" || q.Dir != "") && !((q.Pane != "" && m.Pane == q.Pane) || (q.Dir != "" && m.Dir == q.Dir)) {
			continue
		}
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].meta.Start.After(out[j].meta.Start) })
	return out
}

// errOverBudget is a recording left unread: the query has unpacked its budget already.
var errOverBudget = errors.New("past the search's budget")

// plainFor is a recording as plain text — from the cache when it holds this version of the file, else read off the
// disk while the query has budget left — and the bytes unpacked for it
func (r *Recorder) plainFor(e entry, spent int64, since uint64) (*plainRecording, int64, error) {
	info, err := os.Stat(e.path)
	if err != nil {
		return nil, 0, err
	}
	if plain := r.plain.get(e.meta.ID, version{size: info.Size(), mod: info.ModTime()}); plain != nil {
		return plain, 0, nil
	}
	if spent >= r.searchBudget {
		return nil, 0, errOverBudget
	}
	plain, v, read, err := readPlain(e.path)
	if err != nil {
		return nil, read, err
	}
	r.plain.put(e.meta.ID, v, plain, since)
	return plain, read, nil
}

// sighting is what one recording showed of the text.
type sighting struct {
	first  Appearance
	again  []Appearance
	times  int
	before []string
	row    string
	after  []string
	col    int
}

// sight walks a recording's frames, keeping the screen as it stands and which of its rows hold the needle: the text
// appears when a frame shows it and the one before did not, and stays until a frame shows it nowhere; stopped when
// late said so between frames
func sight(plain *plainRecording, needle, query string, late func() bool) (*sighting, bool) {
	var (
		screen []string
		has    []bool
		count  int
		open   *Appearance
		seen   *sighting
	)
	closeAt := func(at int64) {
		open.For = max(0, at-open.At)
		seen.times++
		if seen.times == 1 {
			seen.first = *open
		} else if len(seen.again) < MaxAgain {
			seen.again = append(seen.again, *open)
		}
		open = nil
	}
	for i, f := range plain.frames {
		if i > 0 && i%checkEvery == 0 && late() {
			return nil, true
		}
		for line := f.lines; line < len(screen); line++ {
			if has[line] {
				count--
			}
		}
		screen, has = screen[:min(len(screen), max(0, f.lines))], has[:min(len(has), max(0, f.lines))]
		for len(screen) < f.lines {
			screen, has = append(screen, ""), append(has, false)
		}
		for _, row := range f.rows {
			if row.line < 0 || row.line >= f.lines {
				continue
			}
			screen[row.line] = row.text
			now := strings.Contains(row.fold, needle)
			if now != has[row.line] {
				has[row.line] = now
				if now {
					count++
				} else {
					count--
				}
			}
		}
		switch {
		case count > 0 && open == nil:
			open = &Appearance{Frame: i, At: f.at, Line: slices.Index(has, true)}
			if seen == nil {
				seen = &sighting{}
				seen.before, seen.row, seen.after, seen.col = excerpt(screen, open.Line, query)
			}
		case count == 0 && open != nil:
			closeAt(f.at)
		}
	}
	if open != nil {
		closeAt(plain.duration)
	}
	return seen, false
}

// excerpt is the matching row and its neighbours, every one cut to the same ExcerptWidth columns around the match
func excerpt(screen []string, line int, query string) (before []string, row string, after []string, col int) {
	runes := []rune(screen[line])
	if len(runes) > ExcerptWidth {
		at, n := 0, len([]rune(query))
		if found := scrollback.Find([]string{screen[line]}, query); len(found) > 0 {
			at, n = found[0].Col, found[0].Len
		}
		col = min(max(0, at-(ExcerptWidth-n)/2), len(runes)-ExcerptWidth)
	}
	cut := func(text string) string {
		rs := []rune(text)
		if col >= len(rs) {
			return ""
		}
		return strings.TrimRight(string(rs[col:min(len(rs), col+ExcerptWidth)]), " \t")
	}
	for k := max(0, line-Context); k < line; k++ {
		if text := cut(screen[k]); text != "" || len(before) > 0 {
			before = append(before, text)
		}
	}
	for k := line + 1; k < min(len(screen), line+1+Context); k++ {
		after = append(after, cut(screen[k]))
	}
	for len(after) > 0 && after[len(after)-1] == "" {
		after = after[:len(after)-1]
	}
	return before, cut(screen[line]), after, col
}

// hit is the sighting as the answer names it
func (s *sighting) hit(m Meta) ScreenHit {
	return ScreenHit{
		Recording: m.ID, Machine: m.Machine, Pane: m.Pane, Workspace: m.Workspace, Dir: m.Dir, Folder: m.Folder, Agent: m.Agent, Title: m.Title,
		Start: m.Start, First: s.first, Again: s.again, Times: s.times, Before: orEmpty(s.before), Row: s.row, After: orEmpty(s.after), Col: s.col,
	}
}

func orEmpty(rows []string) []string {
	if rows == nil {
		return []string{}
	}
	return rows
}
