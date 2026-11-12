package timelapse

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// The search reads a recording as plain text: every row with its escapes taken out the way the page's own parseAnsi
// takes them out, so a column the server names is the one the player draws, trailing blanks dropped, and the
// lower-cased row beside it. A recording read so is kept by its file's version in a small cache, so a second query —
// the next letter typed — reads no file. A query scans the recordings newest first, so a cache that made room by the
// one used longest ago would throw out, query after query, the very recordings the next one reads first: it makes
// room only out of what earlier queries used, and keeps nothing more once the query's own fill it.

// MaxPlain is the bytes of plain rows the cache holds at most.
const MaxPlain = 32 << 20

// plainRow is one row that moved in a frame: its text as drawn, and lower-cased for the search (the same string when
// lowering changes nothing).
type plainRow struct {
	line int
	text string
	fold string
}

// plainFrame is a frame's rows as plain text: Lines rows, rows the ones that moved.
type plainFrame struct {
	at    int64
	lines int
	rows  []plainRow
}

// plainRecording is a recording's frames as plain text and how long it lasted, in ms.
type plainRecording struct {
	frames   []plainFrame
	duration int64
	bytes    int
}

// version tells one write of a file from the next: a turn's end overwrites its last checkpoint.
type version struct {
	size int64
	mod  time.Time
}

type cached struct {
	version version
	plain   *plainRecording
	used    uint64
}

// plainCache holds recordings as plain text by id, within its bytes; its own lock, never the recorder's.
type plainCache struct {
	mu    sync.Mutex
	most  int
	bytes int
	clock uint64
	held  map[string]*cached
}

func newPlainCache(most int) *plainCache {
	return &plainCache{most: most, held: map[string]*cached{}}
}

// get is the recording as plain text when the cache holds this very version of it
func (c *plainCache) get(id string, v version) *plainRecording {
	c.mu.Lock()
	defer c.mu.Unlock()
	hit := c.held[id]
	if hit == nil || hit.version != v {
		return nil
	}
	c.clock++
	hit.used = c.clock
	return hit.plain
}

// begin is a query's mark: what the cache held before it, the ones the query has not used, may make room
func (c *plainCache) begin() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock++
	return c.clock
}

// put keeps a recording read by the query begun at since, the ones used longest ago before it making room; when the
// query's own recordings fill the cache — or this one is larger than the whole — it is not kept
func (c *plainCache) put(id string, v version, plain *plainRecording, since uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropLocked(id)
	if plain.bytes > c.most {
		return
	}
	for c.bytes+plain.bytes > c.most {
		oldest := ""
		for key, e := range c.held {
			if e.used < since && (oldest == "" || e.used < c.held[oldest].used) {
				oldest = key
			}
		}
		if oldest == "" {
			return
		}
		c.dropLocked(oldest)
	}
	c.clock++
	c.held[id] = &cached{version: v, plain: plain, used: c.clock}
	c.bytes += plain.bytes
}

// drop forgets a recording: deleted, its text must not outlive it
func (c *plainCache) drop(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropLocked(id)
}

func (c *plainCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.held, c.bytes = map[string]*cached{}, 0
}

func (c *plainCache) dropLocked(id string) {
	if e := c.held[id]; e != nil {
		c.bytes -= e.plain.bytes
		delete(c.held, id)
	}
}

// readPlain reads a recording's file as plain text, and the bytes it unpacked: screens compress thirty times over, so
// what a read costs is what comes out of the gzip, not what the file takes on the disk
func readPlain(path string) (*plainRecording, version, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, version{}, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, version{}, 0, err
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, version{}, 0, err
	}
	body := &counted{reader: io.LimitReader(zr, 1<<30)}
	var rec Recording
	if err := json.NewDecoder(body).Decode(&rec); err != nil {
		return nil, version{}, body.read, err
	}
	return plainOf(rec), version{size: info.Size(), mod: info.ModTime()}, body.read, nil
}

// counted is a reader that keeps count of the bytes read through it
type counted struct {
	reader io.Reader
	read   int64
}

func (c *counted) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.read += int64(n)
	return n, err
}

// plainOf is a recording's frames as plain text
func plainOf(rec Recording) *plainRecording {
	out := &plainRecording{frames: make([]plainFrame, 0, len(rec.Frames)), duration: rec.End.Sub(rec.Start).Milliseconds()}
	for _, f := range rec.Frames {
		frame := plainFrame{at: f.At, lines: f.Lines, rows: make([]plainRow, 0, len(f.Rows))}
		for _, row := range f.Rows {
			text := strings.TrimRight(Plain(row.Text), " \t")
			fold := strings.ToLower(text)
			frame.rows = append(frame.rows, plainRow{line: row.Line, text: text, fold: fold})
			out.bytes += len(text) + 40
			if fold != text {
				out.bytes += len(fold)
			}
		}
		out.duration = max(out.duration, f.At)
		out.frames = append(out.frames, frame)
	}
	return out
}

// Plain is a screen row as the player draws it: the SGR colours, cursor moves, erases and OSC titles taken out, a
// charset designation with its byte, every control character but a tab — as web/src/features/terminal/ansi.ts reads it.
func Plain(text string) string {
	if !strings.ContainsFunc(text, func(r rune) bool { return r < ' ' && r != '\t' }) {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); {
		ch := text[i]
		if ch == 0x1b {
			i = pastEscape(text, i)
			continue
		}
		if ch < ' ' && ch != '\t' {
			i++
			continue
		}
		b.WriteByte(ch)
		i++
	}
	return b.String()
}

// pastEscape is where the text goes on after the escape at i: a CSI's parameters and its final character, an OSC up to
// BEL or ESC \, a charset designation's two bytes, else the one character after ESC
func pastEscape(text string, i int) int {
	if i+1 >= len(text) {
		return len(text)
	}
	switch next := text[i+1]; {
	case next == '[':
		j := i + 2
		for j < len(text) && text[j] >= 0x20 && text[j] <= 0x3f {
			j++
		}
		return pastRune(text, j)
	case next == ']':
		rest := text[i+2:]
		bell, st := strings.IndexByte(rest, 0x07), strings.Index(rest, "\x1b\\")
		switch {
		case bell < 0 && st < 0:
			return len(text)
		case st >= 0 && (bell < 0 || st < bell):
			return i + 2 + st + 2
		default:
			return i + 2 + bell + 1
		}
	case strings.IndexByte("()*+#%", next) >= 0:
		return pastRune(text, i+2)
	}
	return pastRune(text, i+1)
}

// pastRune is the index after the character at i, a whole one however many bytes it takes
func pastRune(text string, i int) int {
	if i >= len(text) {
		return len(text)
	}
	_, size := utf8.DecodeRuneInString(text[i:])
	return i + size
}
