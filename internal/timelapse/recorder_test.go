package timelapse

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/dev1antdev/remuda/internal/herdr"
)

var t0 = time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)

// bench is a recorder on a temp state dir with its readers inline, a clock of its own and screens the test sets
type bench struct {
	*Recorder
	dir    string
	clock  time.Time
	screen map[string]string // pane → what its screen shows now
	reads  int
}

func newBench(t *testing.T) *bench {
	t.Helper()
	b := &bench{dir: t.TempDir(), clock: t0, screen: map[string]string{}}
	b.Recorder = Open(b.dir, Inputs{
		Screen: func(machineID, paneID string) (string, error) {
			b.reads++
			text, ok := b.screen[paneID]
			if !ok {
				return "", fmt.Errorf("no pane %s", paneID)
			}
			return text, nil
		},
		Folder: func(machineID, dir string, ws *herdr.WorkspaceInfo) string { return "/repo" },
	})
	b.reopen()
	return b
}

// reopen makes the recorder's clock and readers the bench's: a fresh Open (a restart) gets them too
func (b *bench) reopen() {
	b.Recorder.now = func() time.Time { return b.clock }
	b.Recorder.run = func(fn func()) { fn() }
}

// snap is one machine's snapshot with one agent pane in the status given, or none
func snap(status string, panes ...string) *herdr.Snapshot {
	s := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "app"}},
		Layouts:    []herdr.Layout{{Panes: []herdr.LayoutPane{{PaneID: "w1:p1", Rect: herdr.Rect{Width: 120, Height: 40}}}}},
	}
	for _, id := range panes {
		s.Panes = append(s.Panes, herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", Agent: "claude", AgentStatus: status, CWD: "/repo/app", Title: "Draw the page"})
	}
	return s
}

// work observes the pane working for d, a snapshot every 2 s, its screen moving on every 4 s
func (b *bench) work(d time.Duration, pane string) {
	for end := b.clock.Add(d); b.clock.Before(end); b.clock = b.clock.Add(2 * time.Second) {
		b.screen[pane] = fmt.Sprintf("\x1b[1mworking\x1b[0m\nstep %d\n> ", b.clock.Sub(t0)/(4*time.Second))
		b.Observe("local", snap(herdr.StatusWorking, pane), b.clock)
	}
}

// rest observes the pane resting in status for d
func (b *bench) rest(status string, d time.Duration, pane string) {
	for end := b.clock.Add(d); !b.clock.After(end); b.clock = b.clock.Add(2 * time.Second) {
		b.Observe("local", snap(status, pane), b.clock)
	}
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.Walk(filepath.Join(dir, Folder), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// screens is every frame's text, the rows put back together as the page does it
func screens(rec Recording) []string {
	var out []string
	var rows []string
	for _, f := range rec.Frames {
		next := make([]string, f.Lines)
		copy(next, rows)
		for _, row := range f.Rows {
			next[row.Line] = row.Text
		}
		rows = next
		out = append(out, strings.Join(rows, "\n"))
	}
	return out
}

func TestATurnIsRecordedWhileItWorksAndWrittenOnceWhenItEnds(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	b.screen["w1:p1"] = "Done: the page draws.\n> "
	b.rest(herdr.StatusDone, 10*time.Second, "w1:p1")
	if got := files(t, b.dir); len(got) != 0 {
		t.Fatalf("written while the turn might still go on: %v", got)
	}
	b.rest(herdr.StatusDone, Grace+2*time.Second, "w1:p1")
	written := files(t, b.dir)
	if len(written) != 1 || !strings.HasSuffix(written[0], suffix) || filepath.Base(filepath.Dir(written[0])) != "local" {
		t.Fatalf("written %v", written)
	}
	if info, _ := os.Stat(written[0]); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	list := b.List("local", "w1:p1", "")
	if len(list) != 1 {
		t.Fatalf("listed %+v", list)
	}
	meta := list[0]
	if meta.Workspace != "app" || meta.Dir != "/repo/app" || meta.Folder != "/repo" || meta.Agent != "claude" || meta.Title != "Draw the page" || meta.Cols != 120 || meta.Rows != 40 || meta.Ended != herdr.StatusDone || meta.Size <= 0 {
		t.Fatalf("meta %+v", meta)
	}
	if !meta.Start.Equal(t0) || meta.End.Sub(meta.Start) != 2*time.Minute {
		t.Fatalf("from %v to %v", meta.Start, meta.End)
	}
	rec, err := b.Read(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	texts := screens(rec)
	// one frame per screen that moved: 30 steps of 4 s, and the answer it stopped on
	if len(texts) != 31 || rec.Count != 31 || texts[0] != "\x1b[1mworking\x1b[0m\nstep 0\n> " || texts[30] != "Done: the page draws.\n> " {
		t.Fatalf("%d frames, first %q, last %q", len(texts), texts[0], texts[len(texts)-1])
	}
	if len(rec.Frames[1].Rows) != 1 || rec.Frames[1].Rows[0].Line != 1 {
		t.Fatalf("a frame carries the rows that did not move: %+v", rec.Frames[1])
	}
	if rec.Frames[0].At != 0 || rec.Frames[1].At != 4000 {
		t.Fatalf("frame times %d, %d", rec.Frames[0].At, rec.Frames[1].At)
	}
	if b.List("local", "", "/repo/app")[0].ID != meta.ID || len(b.List("local", "w9:p9", "")) != 0 || len(b.List("gog", "w1:p1", "/repo/app")) != 0 {
		t.Fatal("the folder's listing or another pane's")
	}
}

func TestABlinkThroughIdleIsStillTheSameTurn(t *testing.T) {
	b := newBench(t)
	b.work(time.Minute, "w1:p1")
	b.rest(herdr.StatusIdle, 6*time.Second, "w1:p1")
	b.work(time.Minute, "w1:p1")
	b.rest(herdr.StatusBlocked, Grace+2*time.Second, "w1:p1")
	list := b.List("local", "w1:p1", "")
	if len(list) != 1 || list[0].Ended != herdr.StatusBlocked || list[0].End.Sub(list[0].Start) < 2*time.Minute {
		t.Fatalf("listed %+v", list)
	}
}

func TestAShortTurnOrAStillScreenIsNotKept(t *testing.T) {
	b := newBench(t)
	b.work(40*time.Second, "w1:p1")
	b.rest(herdr.StatusDone, Grace+2*time.Second, "w1:p1")
	if got := files(t, b.dir); len(got) != 0 {
		t.Fatalf("a 40 s turn written: %v", got)
	}
	b.screen["w1:p1"] = "thinking…"
	for end := b.clock.Add(3 * time.Minute); b.clock.Before(end); b.clock = b.clock.Add(2 * time.Second) {
		b.Observe("local", snap(herdr.StatusWorking, "w1:p1"), b.clock)
	}
	b.rest(herdr.StatusDone, Grace+2*time.Second, "w1:p1")
	if got := files(t, b.dir); len(got) != 0 {
		t.Fatalf("a screen that never moved written: %v", got)
	}
	if b.reads == 0 {
		t.Fatal("the screen was never read")
	}
}

func TestAPaneGoneIsWrittenAtOnceAndAShellIsNeverRead(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	b.Observe("local", snap(herdr.StatusWorking), b.clock)
	list := b.List("local", "w1:p1", "")
	if len(list) != 1 || list[0].Ended != EndedGone {
		t.Fatalf("listed %+v", list)
	}
	shell := snap(herdr.StatusWorking, "w1:p2")
	shell.Panes[0].Agent = ""
	reads := b.reads
	b.Observe("local", shell, b.clock)
	if b.reads != reads {
		t.Fatal("a shell's screen read")
	}
}

func TestPastMaxFramesEveryOtherFrameGoesAndThePaceHalves(t *testing.T) {
	tk := &take{every: Interval}
	for i := 0; i <= MaxFrames; i++ {
		tk.add(t0.Add(time.Duration(i)*Interval), fmt.Sprintf("header\nframe %d", i))
	}
	if len(tk.frames) != MaxFrames/2+1 || tk.every != 2*Interval {
		t.Fatalf("%d frames at %v", len(tk.frames), tk.every)
	}
	if tk.frames[0].rows[1] != "frame 0" || tk.frames[len(tk.frames)-1].rows[1] != fmt.Sprintf("frame %d", MaxFrames) || tk.frames[1].rows[1] != "frame 2" {
		t.Fatalf("kept %v … %v", tk.frames[1].rows, tk.frames[len(tk.frames)-1].rows)
	}
	if unsafe.StringData(tk.frames[0].rows[0]) != unsafe.StringData(tk.frames[1].rows[0]) {
		t.Fatal("a row that did not move is kept twice")
	}
	before := len(tk.frames)
	tk.add(t0.Add(time.Hour), fmt.Sprintf("header\nframe %d", MaxFrames))
	if len(tk.frames) != before {
		t.Fatal("an unchanged screen kept as a frame")
	}
}

func TestATurnStillGoingIsCheckpointedUnlistedAndReadAsCutAfterACrash(t *testing.T) {
	b := newBench(t)
	b.work(CheckpointEvery+time.Minute, "w1:p1")
	written := files(t, b.dir)
	if len(written) != 1 {
		t.Fatalf("no checkpoint after %v: %v", CheckpointEvery, written)
	}
	if len(b.List("local", "w1:p1", "")) != 0 || b.Stats().Count != 0 {
		t.Fatal("a turn still going listed")
	}
	crashed := Open(b.dir, Inputs{})
	list := crashed.List("local", "w1:p1", "")
	if len(list) != 1 || list[0].Ended != EndedCut || list[0].Count < 100 {
		t.Fatalf("after a crash %+v", list)
	}
	b.rest(herdr.StatusDone, Grace+2*time.Second, "w1:p1")
	if got := files(t, b.dir); len(got) != 1 || b.List("local", "w1:p1", "")[0].Ended != herdr.StatusDone {
		t.Fatalf("the turn's end did not replace its checkpoint: %v", got)
	}
}

func TestClosingWritesTheTurnsStillGoingAsStopped(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	b.Close()
	list := b.List("local", "w1:p1", "")
	if len(list) != 1 || list[0].Ended != EndedStopped {
		t.Fatalf("listed %+v", list)
	}
	reads := b.reads
	b.work(time.Minute, "w1:p1")
	if b.reads != reads {
		t.Fatal("recorded after Close")
	}
}

func TestOffRecordsNothingAndDropsTheTurnBeingRecorded(t *testing.T) {
	b := newBench(t)
	b.work(CheckpointEvery+time.Minute, "w1:p1")
	if err := b.SetOn(false); err != nil {
		t.Fatal(err)
	}
	if got := files(t, b.dir); len(got) != 0 {
		t.Fatalf("the dropped turn's checkpoint stayed: %v", got)
	}
	b.work(2*time.Minute, "w1:p1")
	b.rest(herdr.StatusDone, Grace+2*time.Second, "w1:p1")
	if got := files(t, b.dir); len(got) != 0 || b.On() {
		t.Fatalf("recorded while off: %v", got)
	}
	again := Open(b.dir, Inputs{})
	if again.On() || again.Stats().On {
		t.Fatal("the switch did not survive a restart")
	}
}

func TestAScratchInstanceRecordsNothingButPlaysWhatIsThere(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	b.Observe("local", snap(herdr.StatusWorking), b.clock)
	beside := Open(b.dir, Inputs{Screen: b.Recorder.in.Screen})
	beside.Scratch()
	beside.now, beside.run = b.Recorder.now, b.Recorder.run
	reads := b.reads
	beside.Observe("local", snap(herdr.StatusWorking, "w1:p2"), b.clock)
	if b.reads != reads || beside.On() || !beside.Stats().Scratch {
		t.Fatal("a scratch instance read a screen, or says it records")
	}
	if list := beside.List("local", "w1:p1", ""); len(list) != 1 || beside.Delete(list[0].ID) != nil {
		t.Fatalf("a scratch instance does not list or delete the folder's recordings: %+v", list)
	}
}

func TestAMachineRemovedHasItsTurnWrittenAsGone(t *testing.T) {
	b := newBench(t)
	b.work(2*time.Minute, "w1:p1")
	b.Forget("local")
	if list := b.List("local", "w1:p1", ""); len(list) != 1 || list[0].Ended != EndedGone {
		t.Fatalf("listed %+v", list)
	}
}
