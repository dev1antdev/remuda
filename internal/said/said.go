// Package said keeps the first line of every finished agent's last answer, for the roster: read once per finish,
// off the request path, so a wall tile and a sidebar row can say what the agent answered.
package said

import (
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// Max is the longest line kept, in characters; a longer answer is cut at a word with an ellipsis.
const Max = 120

// Inputs are the reads a Tracker makes, both through chat.Service: a remote one runs a script over ssh.
type Inputs struct {
	Version func(machineID, paneID string) (string, bool) // the session file's version; false when the pane has none
	Answer  func(machineID, paneID string) (text, id string)
}

type entry struct {
	version string // the file version the line was read at; "" when the read found no file
	line    string
}

// Tracker holds one line per pane and reads one pane at a time per machine.
type Tracker struct {
	in      Inputs
	mu      sync.Mutex
	lines   map[string]entry
	queue   map[string][]string
	running map[string]bool
}

func New(in Inputs) *Tracker {
	return &Tracker{in: in, lines: map[string]entry{}, queue: map[string][]string{}, running: map[string]bool{}}
}

func key(machineID, paneID string) string { return machineID + "|" + paneID }

// Line is what the pane's agent answered last, "" while it works or when nothing was read.
func (t *Tracker) Line(machineID, paneID string) string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lines[key(machineID, paneID)].line
}

// OnStatus hears every transition: a finish is read, a start of work clears the line, a closed pane is forgotten.
func (t *Tracker) OnStatus(ev events.PaneStatus) {
	if ev.Kind == "" {
		return
	}
	switch ev.To {
	case herdr.StatusDone, herdr.StatusIdle:
		t.ask(ev.Pane.Machine, ev.Pane.Pane)
	case herdr.StatusWorking, events.Gone:
		t.mu.Lock()
		delete(t.lines, key(ev.Pane.Machine, ev.Pane.Pane))
		t.mu.Unlock()
	}
}

// Observe seeds from a snapshot: an agent pane already finished when first seen is read once.
func (t *Tracker) Observe(machineID string, snap *herdr.Snapshot) {
	if snap == nil {
		return
	}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.Agent == "" || (pane.AgentStatus != herdr.StatusDone && pane.AgentStatus != herdr.StatusIdle) {
			continue
		}
		t.mu.Lock()
		_, known := t.lines[key(machineID, pane.PaneID)]
		t.mu.Unlock()
		if !known {
			t.ask(machineID, pane.PaneID)
		}
	}
}

// Forget drops a machine's lines.
func (t *Tracker) Forget(machineID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k := range t.lines {
		if strings.HasPrefix(k, machineID+"|") {
			delete(t.lines, k)
		}
	}
}

// ask queues a read of the pane and starts the machine's reader when none runs.
func (t *Tracker) ask(machineID, paneID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, waiting := range t.queue[machineID] {
		if waiting == paneID {
			return
		}
	}
	t.queue[machineID] = append(t.queue[machineID], paneID)
	if t.running[machineID] {
		return
	}
	t.running[machineID] = true
	go t.drain(machineID)
}

func (t *Tracker) drain(machineID string) {
	for {
		t.mu.Lock()
		waiting := t.queue[machineID]
		if len(waiting) == 0 {
			t.running[machineID] = false
			t.mu.Unlock()
			return
		}
		paneID := waiting[0]
		t.queue[machineID] = waiting[1:]
		t.mu.Unlock()
		t.read(machineID, paneID)
	}
}

// read brings the pane's line up to its file: an unchanged file is not parsed again.
func (t *Tracker) read(machineID, paneID string) {
	k := key(machineID, paneID)
	version, ok := t.in.Version(machineID, paneID)
	if !ok {
		t.mu.Lock()
		if _, known := t.lines[k]; !known {
			t.lines[k] = entry{}
		}
		t.mu.Unlock()
		return
	}
	t.mu.Lock()
	have, known := t.lines[k]
	t.mu.Unlock()
	if known && have.version == version {
		return
	}
	text, _ := t.in.Answer(machineID, paneID)
	t.mu.Lock()
	t.lines[k] = entry{version: version, line: FirstLine(text)}
	t.mu.Unlock()
}

var (
	leadMarks = regexp.MustCompile(`^(?:#{1,6}\s+|>\s*|[-*+]\s+|\d+[.)]\s+|\[[ xX]\]\s+)`)
	rule      = regexp.MustCompile(`^(?:-{3,}|\*{3,}|_{3,})$`)
	image     = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	link      = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	bold      = regexp.MustCompile(`\*\*(.+?)\*\*|__(.+?)__`)
	italic    = regexp.MustCompile(`\*(\S(?:[^*]*\S)?)\*`)
	code      = regexp.MustCompile("`([^`]*)`")
	strike    = regexp.MustCompile(`~~(.+?)~~`)
	tag       = regexp.MustCompile(`<[^>]+>`)
	spaces    = regexp.MustCompile(`\s+`)
)

// FirstLine is the first line of an answer that says something, its Markdown marks stripped, cut to Max characters.
func FirstLine(text string) string {
	inFence := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || rule.MatchString(line) {
			continue
		}
		if line = Plain(line); line != "" {
			return cut(line, Max)
		}
	}
	return ""
}

// Plain is one line with its Markdown marks stripped: list and quote marks, headings, emphasis, code, links, tags.
func Plain(line string) string {
	line = strings.TrimSpace(line)
	for {
		stripped := leadMarks.ReplaceAllString(line, "")
		if stripped == line {
			break
		}
		line = strings.TrimSpace(stripped)
	}
	line = image.ReplaceAllString(line, "$1")
	line = link.ReplaceAllString(line, "$1")
	line = bold.ReplaceAllString(line, "$1$2")
	line = italic.ReplaceAllString(line, "$1")
	line = code.ReplaceAllString(line, "$1")
	line = strike.ReplaceAllString(line, "$1")
	line = tag.ReplaceAllString(line, "")
	return strings.TrimSpace(spaces.ReplaceAllString(line, " "))
}

// cut keeps at most max characters, at a word when one sits past the first two thirds, with an ellipsis.
func cut(line string, max int) string {
	if utf8.RuneCountInString(line) <= max {
		return line
	}
	runes := []rune(line)
	end := max
	if at := strings.LastIndex(string(runes[:max]), " "); at > 0 {
		if words := utf8.RuneCountInString(string(runes[:max])[:at]); words > max*2/3 {
			end = words
		}
	}
	return strings.TrimRight(string(runes[:end]), " ,;:") + "…"
}
