package timelapse

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

// frame is one screen: its rows, a row the frame before had shared with it rather than kept twice.
type frame struct {
	at   time.Time
	rows []string
}

// add keeps the screen when it differs from the last frame, its unchanged rows shared with that frame's and the changed
// ones copied out of the read (a row cut from it would keep the whole read alive); past MaxFrames every other frame
// goes, the first and the last staying, and the pace halves
func (tk *take) add(at time.Time, text string) {
	rows := strings.Split(text, "\n")
	var prev []string
	if n := len(tk.frames); n > 0 {
		prev = tk.frames[n-1].rows
	}
	same := len(rows) == len(prev)
	for i, row := range rows {
		if i < len(prev) && prev[i] == row {
			rows[i] = prev[i]
			continue
		}
		same = false
		rows[i] = strings.Clone(row)
	}
	if same {
		return
	}
	tk.frames = append(tk.frames, frame{at: at, rows: rows})
	if len(tk.frames) > MaxFrames {
		tk.frames = thin(tk.frames)
		tk.every *= 2
	}
}

// thin keeps every other frame, the first and the last always
func thin(frames []frame) []frame {
	kept := make([]frame, 0, len(frames)/2+2)
	for i := 0; i < len(frames); i += 2 {
		kept = append(kept, frames[i])
	}
	if last := frames[len(frames)-1]; kept[len(kept)-1].at != last.at {
		kept = append(kept, last)
	}
	return kept
}

// encode turns the frames into the file's: each the rows that differ from the frame before
func encode(frames []frame, start time.Time) []Frame {
	out := make([]Frame, 0, len(frames))
	var prev []string
	for _, f := range frames {
		next := Frame{At: f.at.Sub(start).Milliseconds(), Lines: len(f.rows)}
		for i, row := range f.rows {
			if i < len(prev) && prev[i] == row {
				continue
			}
			next.Rows = append(next.Rows, Row{Line: i, Text: row})
		}
		out = append(out, next)
		prev = f.rows
	}
	return out
}

// newID names a recording by its start and a random tail: 20261112T140203Z-a1b2c3
func newID(at time.Time) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return at.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

// clip keeps a name the header carries to a few hundred bytes, cut between letters
func clip(s string) string {
	const most = 512
	if len(s) <= most {
		return s
	}
	cut := most
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}
