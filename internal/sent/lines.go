package sent

import (
	"strings"
	"sync"
	"time"
)

// What the screen terminal types arrives a few keystrokes at a time; the activity gets a row per line sent instead —
// the text typed since the last row when Enter ends it, else the keys pressed before it ("↓ ↓ Enter", a menu answered)
// — and one for an interrupt. Per device and pane, so two devices typing into one pane keep their lines apart.

// lineRunes is how much of a line is kept while it is typed; a row quotes MaxText of it.
const lineRunes = 512

// lineKeys is how many keys before an Enter a row names.
const lineKeys = 16

// keptLines bounds the lines kept at once; past it the one typed into longest ago goes.
const keptLines = 256

// Row is one activity row the typing came to: its words, and whether they name keys rather than quote a text.
type Row struct {
	What string
	Keys bool
}

type line struct {
	text []rune
	keys []string
	at   time.Time
}

// Lines keeps each device's line in each pane until Enter sends it; its zero value is ready.
type Lines struct {
	mu    sync.Mutex
	lines map[string]*line
}

// Typed hears one request — its text, then its keys — and answers the rows it completed, in order.
func (l *Lines) Typed(device, machineID, paneID, text string, keys []string, now time.Time) []Row {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := device + "\x00" + machineID + "\x00" + paneID
	cur := l.open(key, now)
	cur.text = keepTail(append(cur.text, []rune(text)...))
	var rows []Row
	for _, name := range keys {
		switch name {
		case "enter":
			if words := strings.TrimSpace(string(cur.text)); words != "" {
				rows = append(rows, Row{What: words})
			} else {
				rows = append(rows, Row{What: KeyWords(append(cur.keys, name)), Keys: true})
			}
			cur.text, cur.keys = nil, nil
		case "ctrl+c":
			rows = append(rows, Row{What: KeyWords([]string{name}), Keys: true})
			cur.text, cur.keys = nil, nil
		case "backspace":
			if n := len(cur.text); n > 0 {
				cur.text = cur.text[:n-1]
			}
		case "ctrl+u":
			cur.text = nil
		default:
			if len(cur.keys) < lineKeys {
				cur.keys = append(cur.keys, name)
			}
		}
	}
	if len(cur.text) == 0 && len(cur.keys) == 0 {
		delete(l.lines, key)
	}
	return rows
}

// open is the line under the key, made when there is none; the oldest goes once keptLines are kept.
func (l *Lines) open(key string, now time.Time) *line {
	if l.lines == nil {
		l.lines = map[string]*line{}
	}
	cur := l.lines[key]
	if cur == nil {
		if len(l.lines) >= keptLines {
			l.dropOldest()
		}
		cur = &line{}
		l.lines[key] = cur
	}
	cur.at = now
	return cur
}

func (l *Lines) dropOldest() {
	oldest := ""
	for key, cur := range l.lines {
		if oldest == "" || cur.at.Before(l.lines[oldest].at) {
			oldest = key
		}
	}
	delete(l.lines, oldest)
}

func keepTail(text []rune) []rune {
	if len(text) <= lineRunes {
		return text
	}
	return append([]rune(nil), text[len(text)-lineRunes:]...)
}
