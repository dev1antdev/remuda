// Package timelapse records what an agent's screen did while it worked, so forty minutes can be watched in thirty
// seconds. While herdr reports an agent pane working, its screen is read every few seconds the way the Wall's screen
// view reads it (pane.read with ANSI: no attach, so no slot is taken and nothing is resized), a frame kept only when
// the screen changed, in memory; when the turn ends — the pane at rest, asking, gone — the recording is written once,
// gzipped, 0600, under the state dir's timelapse/<machine>/. A turn still going is written again every
// CheckpointEvery, so a crash or a power cut loses no more than that. Recording is on unless the person turns it off.
package timelapse

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
)

// The bounds a recording keeps to: a cockpit may live on a Raspberry Pi's SD card, so a turn is written once (and once
// every CheckpointEvery while it lasts), never a frame at a time, and the folder never grows past MaxTotal.
const (
	Interval        = 4 * time.Second  // a working pane's screen is read at most this often
	MaxFrames       = 900              // frames one recording holds; past it every other one goes and the pace halves
	MaxHeld         = 8 << 20          // bytes of screens one turn holds in memory; past it, as past MaxFrames
	KeepPerPane     = 20               // recordings kept per pane, the newest
	MaxTotal        = 200 << 20        // bytes the folder holds at most; the oldest recordings go first
	MinTurn         = time.Minute      // a shorter turn is not worth keeping
	Grace           = 15 * time.Second // a pane at work again this soon after it stopped is still the same turn
	Away            = 2 * time.Minute  // a pane unseen this long — its machine out of reach — ends its turn
	CheckpointEvery = 10 * time.Minute // a turn still going is written this often
	MaxScreen       = 256 << 10        // a screen read larger than this is no frame
)

// Folder is the folder of the state dir the recordings live in; the nightly backups leave it out.
const Folder = "timelapse"

// settingsFile keeps the person's switch, beside the other settings, so a backup carries it.
const settingsFile = "timelapse.json"

// Why a recording ended: the status the pane rested in, or what stopped it.
const (
	EndedGone    = "gone"    // the pane closed, or its agent left it
	EndedStopped = "stopped" // remuda stopped while the agent worked
	EndedCut     = "cut"     // the last checkpoint of a turn remuda never saw end (a crash, a power cut)
	EndedAway    = "away"    // its machine went out of reach while it worked
)

// Meta is what a recording is of; it travels in the file's gzip header, so a listing reads no frame.
type Meta struct {
	ID        string    `json:"id"`
	Machine   string    `json:"machine"`
	Pane      string    `json:"pane"`
	Workspace string    `json:"workspace"`
	Dir       string    `json:"dir"`    // the workspace's folder
	Folder    string    `json:"folder"` // the sidebar's folder it sits in: the repository's main checkout, else Dir
	Agent     string    `json:"agent"`  // herdr's kind, "claude"
	Title     string    `json:"title"`
	Cols      int       `json:"cols"` // the pane's grid, the widest seen while it worked
	Rows      int       `json:"rows"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Count     int       `json:"count"` // frames
	Ended     string    `json:"ended"` // the status it rested in (done, idle, blocked), or EndedGone, EndedStopped, EndedCut
	Size      int64     `json:"size"`  // the file's bytes, read off the disk
}

// Recording is a recording whole: its frames, each the rows that changed since the one before.
type Recording struct {
	Meta
	Frames []Frame `json:"frames"`
}

// Frame is the screen at At milliseconds after Start: Lines rows, of which Rows are the ones that differ from the frame
// before (every row in the first). Joined with newlines they are the screen's text as herdr printed it, its SGR
// colours carried from row to row as they were.
type Frame struct {
	At    int64 `json:"at"`
	Lines int   `json:"lines"`
	Rows  []Row `json:"rows,omitempty"`
}

// Row is one row of a frame by its place on the screen.
type Row struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Inputs are what the recorder reads; the app wires them over the fleet.
type Inputs struct {
	Screen func(machineID, paneID string) (string, error)                     // the pane's visible screen with its colours
	Folder func(machineID, dir string, workspace *herdr.WorkspaceInfo) string // the sidebar's folder of a workspace; nil reads dir
}

// Stats is what the folder holds, for Settings.
type Stats struct {
	On      bool  `json:"on"`
	Scratch bool  `json:"scratch"` // a second remuda beside the real one, which records nothing
	Count   int   `json:"count"`
	Bytes   int64 `json:"bytes"`
	Max     int64 `json:"max"`
}

// Recorder records every machine's working agents and keeps what it recorded.
type Recorder struct {
	dir   string // <state>/timelapse
	state string
	in    Inputs
	now   func() time.Time
	run   func(func()) // how a machine's reader is started: a goroutine, inline in a test

	keepPerPane int // KeepPerPane and MaxTotal, smaller in a test
	maxTotal    int64

	plain        *plainCache // the recordings as plain text, for the search; its own lock
	searchBudget int64       // SearchBudget and SearchFor, smaller in a test
	searchFor    time.Duration

	mu      sync.Mutex
	off     bool
	scratch bool // a scratch instance: it plays and deletes what the folder holds, and records nothing
	closed  bool
	live    map[string]*take // machine|pane → the turn being recorded
	queue   map[string][]job // machine → the reads and writes waiting for its reader
	running map[string]bool
	busy    map[string]job    // machine → what its reader is doing now
	index   map[string]*entry // id → a recording on the disk
	failed  map[string]time.Time
	seen    map[string]seenSnap // machine → its last snapshot, which Tick observes again between snapshots
}

type settings struct {
	Off bool `json:"off"`
}

// Open reads the switch and lists the recordings already on the disk (their headers only); a file left half written
// by a crash is removed.
func Open(stateDir string, in Inputs) *Recorder {
	r := &Recorder{
		dir: filepath.Join(stateDir, Folder), state: stateDir, in: in, now: time.Now, run: func(fn func()) { go fn() },
		keepPerPane: KeepPerPane, maxTotal: MaxTotal,
		plain: newPlainCache(MaxPlain), searchBudget: SearchBudget, searchFor: SearchFor,
		live: map[string]*take{}, queue: map[string][]job{}, running: map[string]bool{}, busy: map[string]job{}, index: map[string]*entry{}, failed: map[string]time.Time{},
	}
	var saved settings
	if err := state.LoadJSON(stateDir, settingsFile, &saved); err != nil {
		log.Printf("timelapse: %v; recording stays on", err)
	}
	r.off = saved.Off
	r.load()
	return r
}

// On says whether working agents are recorded.
func (r *Recorder) On() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.off && !r.scratch
}

// Scratch makes this a second remuda's recorder beside the real one: the real one records, so this one records nothing
// of the person's screens, whatever the switch says.
func (r *Recorder) Scratch() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scratch = true
}

// SetOn turns recording on or off; off drops the turns being recorded, their checkpoints with them.
func (r *Recorder) SetOn(on bool) error {
	r.mu.Lock()
	r.off = !on
	var dropped []*take
	if !on {
		dropped = r.dropLocked()
	}
	r.mu.Unlock()
	for _, tk := range dropped {
		tk.writing.Lock() // a checkpoint being written lands first, then goes
		r.remove(tk.meta.ID)
		tk.writing.Unlock()
	}
	return state.SaveJSON(r.state, settingsFile, settings{Off: !on})
}

// Stats is the switch and what the folder holds.
func (r *Recorder) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	stats := Stats{On: !r.off, Scratch: r.scratch, Max: r.maxTotal}
	for id, e := range r.index {
		if r.isLive(id) {
			continue
		}
		stats.Count++
		stats.Bytes += e.meta.Size
	}
	return stats
}

// mkdir makes the machine's folder, private.
func mkdir(path string) error {
	return os.MkdirAll(path, 0o700)
}
