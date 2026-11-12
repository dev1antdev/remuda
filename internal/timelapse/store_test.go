package timelapse

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recordingAt is a finished recording of a pane, started at the given minute after t0
func recordingAt(machine, pane string, minute int) Recording {
	start := t0.Add(time.Duration(minute) * time.Minute)
	meta := Meta{ID: newID(start), Machine: machine, Pane: pane, Dir: "/repo/app", Agent: "claude", Start: start, End: start.Add(2 * time.Minute), Count: 2, Ended: "done"}
	return Recording{Meta: meta, Frames: []Frame{{At: 0, Lines: 1, Rows: []Row{{Line: 0, Text: strings.Repeat("x", 4000)}}}, {At: 120000, Lines: 1, Rows: []Row{{Line: 0, Text: "done"}}}}}
}

func TestTheFileIsTheRecordingGzippedWithItsMetaInTheHeader(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	rec := recordingAt("local", "w1:p1", 0)
	rec.Workspace = "приложение" // a header field holds any text, as JSON
	if err := r.write(rec); err != nil {
		t.Fatal(err)
	}
	meta, path, err := r.File(rec.ID)
	if err != nil || filepath.Base(path) != rec.ID+suffix {
		t.Fatalf("%v %s", err, path)
	}
	raw, _ := os.ReadFile(path)
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	if !strings.Contains(string(body), `"frames":[{"at":0,"lines":1,"rows":[{"line":0,"text":"xxx`) || meta.Size != int64(len(raw)) {
		t.Fatalf("body %.120s, size %d of %d", body, meta.Size, len(raw))
	}
	head, err := readMeta(path)
	if err != nil || head.Workspace != "приложение" || head.Count != 2 || head.Size != int64(len(raw)) {
		t.Fatalf("header %+v %v", head, err)
	}
	back, err := r.Read(rec.ID)
	if err != nil || len(back.Frames) != 2 || back.Frames[1].Rows[0].Text != "done" {
		t.Fatalf("read back %+v %v", back.Meta, err)
	}
}

func TestTheFolderKeepsToItsBoundsTheOldestGoingFirst(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	r.keepPerPane = 3
	for minute := 0; minute < 5; minute++ {
		if err := r.write(recordingAt("local", "w1:p1", minute)); err != nil {
			t.Fatal(err)
		}
	}
	list := r.List("local", "w1:p1", "")
	if len(list) != 3 || !list[0].Start.Equal(t0.Add(4*time.Minute)) || !list[2].Start.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("kept %+v", list)
	}
	if entries, _ := os.ReadDir(filepath.Join(r.dir, "local")); len(entries) != 3 {
		t.Fatalf("%d files on the disk", len(entries))
	}
	r.maxTotal = list[0].Size*2 + list[0].Size/2
	if err := r.write(recordingAt("gog", "w2:p1", 10)); err != nil {
		t.Fatal(err)
	}
	if len(r.List("local", "w1:p1", "")) != 1 || len(r.List("gog", "w2:p1", "")) != 1 {
		t.Fatalf("past the total: %d and %d kept", len(r.List("local", "w1:p1", "")), len(r.List("gog", "w2:p1", "")))
	}
	if stats := r.Stats(); stats.Count != 2 || stats.Bytes > r.maxTotal {
		t.Fatalf("stats %+v", stats)
	}
}

func TestOneOrEveryRecordingIsDeleted(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	first, second := recordingAt("local", "w1:p1", 0), recordingAt("local", "w1:p2", 1)
	r.write(first)
	r.write(second)
	if err := r.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(first.ID); err != ErrNotFound {
		t.Fatalf("deleted twice: %v", err)
	}
	if _, _, err := r.File(first.ID); err != ErrNotFound {
		t.Fatal("still there")
	}
	if n, err := r.DeleteAll(); err != nil || n != 1 {
		t.Fatalf("deleted %d: %v", n, err)
	}
	if _, err := os.Stat(r.dir); !os.IsNotExist(err) || r.Stats().Count != 0 {
		t.Fatalf("the folder stayed: %v", err)
	}
	if err := r.write(recordingAt("local", "w1:p1", 5)); err != nil || r.Stats().Count != 1 {
		t.Fatalf("recording after a delete of everything: %v", err)
	}
}

func TestOpeningRemovesWhatACrashLeftHalfWrittenAndLeavesStrangersAlone(t *testing.T) {
	dir := t.TempDir()
	r := Open(dir, Inputs{})
	rec := recordingAt("local", "w1:p1", 0)
	r.write(rec)
	folder := filepath.Join(dir, Folder, "local")
	half := filepath.Join(folder, rec.ID+suffix+".123456789")
	stranger := filepath.Join(folder, "notes.txt")
	renamed := filepath.Join(folder, "20261112T150000Z-abcdef"+suffix)
	os.WriteFile(half, []byte("half"), 0o600)
	os.WriteFile(stranger, []byte("mine"), 0o600)
	raw, _ := os.ReadFile(filepath.Join(folder, rec.ID+suffix))
	os.WriteFile(renamed, raw, 0o600) // a copy under another name: its header names another recording
	again := Open(dir, Inputs{})
	if _, err := os.Stat(half); !os.IsNotExist(err) {
		t.Fatal("the half-written file stayed")
	}
	if _, err := os.Stat(stranger); err != nil {
		t.Fatal("a file that is no recording was removed")
	}
	if list := again.List("local", "w1:p1", ""); len(list) != 1 || list[0].ID != rec.ID {
		t.Fatalf("listed %+v", list)
	}
}

func TestAMachineWhoseIdIsNoPlainNameGetsAHashedFolder(t *testing.T) {
	if machineFolder("local") != "local" || machineFolder("gog.lan") != "gog.lan" {
		t.Fatal("a plain name changed")
	}
	for _, id := range []string{"../etc", "a/b", "", ".hidden", strings.Repeat("x", 80)} {
		if got := machineFolder(id); !strings.HasPrefix(got, "m-") || strings.ContainsAny(got, "/.") {
			t.Fatalf("%q → %q", id, got)
		}
	}
}
