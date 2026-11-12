package undo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestUndoPutsEveryKindOfFileBack(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\ntwo\nthree\n", "gone.txt": "keep me\n", "old.txt": "moved\n", "tool.sh": "echo hi\n", "pic.bin": "\x00\x01\x02bin"})
	write(t, dir, "a.txt", "one\nmine\nthree\n") // the person's own work before the turn
	write(t, dir, "notes.md", "my notes\n")      // and a new file of theirs
	before := status(t, dir)
	f := newFixture(t)
	turn := f.turn(t, dir, func() {
		write(t, dir, "a.txt", "one\nmine\nagent\nthree\n")
		write(t, dir, "new.go", "package main\n")
		write(t, dir, "staged.go", "package staged\n")
		git(t, dir, "add", "staged.go")
		if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "mv", "old.txt", "renamed.txt")
		if err := os.Chmod(filepath.Join(dir, "tool.sh"), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, dir, "pic.bin", "\x00\xff\xfe\x00 another picture")
	})
	was := map[string]string{}
	for _, file := range turn.Files {
		was[file.Path] = file.Was
	}
	want := map[string]string{"a.txt": WasChanged, "new.go": WasAdded, "staged.go": WasAdded, "gone.txt": WasDeleted, "old.txt": WasDeleted, "renamed.txt": WasAdded, "tool.sh": WasMode, "pic.bin": WasChanged}
	for path, kind := range want {
		if was[path] != kind {
			t.Errorf("%s: %q, want %q (all: %v)", path, was[path], kind, was)
		}
	}
	if len(turn.Files) != len(want) {
		t.Errorf("files %v, want %d", was, len(want))
	}
	plan, err := f.service.Plan("local", turn.ID)
	if err != nil || plan.Refused != nil {
		t.Fatalf("plan: %+v %v", plan.Refused, err)
	}
	result, err := f.service.Undo("local", turn.ID, false)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if result.Files != len(want) {
		t.Errorf("put back %d files, want %d", result.Files, len(want))
	}
	if got := readFile(t, dir, "a.txt"); got != "one\nmine\nthree\n" {
		t.Errorf("a.txt is %q: the person's own change must stay, the agent's go", got)
	}
	if got := readFile(t, dir, "pic.bin"); got != "\x00\x01\x02bin" {
		t.Errorf("the binary file is %q", got)
	}
	if got := readFile(t, dir, "gone.txt"); got != "keep me\n" {
		t.Errorf("the deleted file is %q", got)
	}
	if info, err := os.Stat(filepath.Join(dir, "tool.sh")); err != nil || info.Mode()&0o111 != 0 {
		t.Errorf("tool.sh's mode was not put back: %v %v", info.Mode(), err)
	}
	if got := status(t, dir); got != before {
		t.Errorf("the folder is not as the turn found it:\n%s\nwant\n%s", got, before)
	}
	if turned, _ := f.store.Get(turn.ID); turned.Undone == nil {
		t.Error("the turn is not marked undone")
	}
	if plan, _ := f.service.Plan("local", turn.ID); plan.Refused == nil || !strings.Contains(plan.Refused.Why, "undone already") {
		t.Errorf("a second undo is not refused: %+v", plan.Refused)
	}
}

func TestCheckpointsWriteNothingIntoTheRepository(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	write(t, dir, "a.txt", "changed\n")
	write(t, dir, "fresh.txt", "a blob git has never seen\n")
	write(t, dir, "big.bin", strings.Repeat("\x01\x02", 4000))
	git(t, dir, "add", "big.bin")
	objects := func() string { return git(t, dir, "count-objects", "-v") }
	beforeStatus := status(t, dir) // git status refreshes the index's stat cache: read it before the index is
	index, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	beforeObjects := objects()
	f := newFixture(t)
	turn := f.turn(t, dir, func() { write(t, dir, "b.txt", "written in the turn\n") })
	if got := objects(); got != beforeObjects {
		t.Errorf("objects were written:\n%s\nwant\n%s", got, beforeObjects)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, ".git", "index")); string(after) != string(index) {
		t.Error("the person's index changed")
	}
	if got := git(t, dir, "stash", "list"); got != "" {
		t.Errorf("a stash was made: %s", got)
	}
	if got := status(t, dir); beforeStatus != "M a.txt\nA  big.bin\n?? fresh.txt" || got != "M a.txt\nA  big.bin\n?? b.txt\n?? fresh.txt" {
		t.Errorf("status %q", got)
	}
	if len(turn.Files) != 1 || turn.Files[0].Path != "b.txt" {
		t.Errorf("the turn's files %+v: the work there before is not the turn's", turn.Files)
	}
}

func TestUndoRefusesFilesChangedAfterTheTurn(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n", "b.txt": "bee\n"})
	f := newFixture(t)
	turn := f.turn(t, dir, func() {
		write(t, dir, "a.txt", "agent\n")
		write(t, dir, "b.txt", "agent\n")
		write(t, dir, "new.txt", "agent\n")
	})
	write(t, dir, "b.txt", "the person after the turn\n")
	if err := os.Remove(filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	plan, err := f.service.Plan("local", turn.ID)
	if err != nil || plan.Refused == nil {
		t.Fatalf("not refused: %+v %v", plan, err)
	}
	if strings.Join(plan.Refused.Files, ",") != "b.txt,new.txt" {
		t.Errorf("named %v", plan.Refused.Files)
	}
	_, err = f.service.Undo("local", turn.ID, false)
	var refusal *Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("undo: %v", err)
	}
	if got := readFile(t, dir, "a.txt"); got != "agent\n" {
		t.Errorf("a refused undo changed a.txt: %q", got)
	}
}

func TestUndoRefusesACommitMadeAfterTheTurn(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n", "other.txt": "x\n"})
	f := newFixture(t)
	turn := f.turn(t, dir, func() { write(t, dir, "a.txt", "agent\n") })
	git(t, dir, "commit", "-qam", "the person commits the agent's work")
	plan, _ := f.service.Plan("local", turn.ID)
	if plan.Refused == nil || strings.Join(plan.Refused.Files, ",") != "a.txt" {
		t.Fatalf("a file committed after the turn is not named: %+v", plan.Refused)
	}
	dir2 := repo(t, map[string]string{"a.txt": "one\n", "other.txt": "x\n"})
	turn2 := f.turn(t, dir2, func() { write(t, dir2, "a.txt", "agent\n") })
	write(t, dir2, "other.txt", "y\n")
	git(t, dir2, "commit", "-qm", "someone else's commit", "other.txt")
	if plan, _ := f.service.Plan("local", turn2.ID); plan.Refused != nil || !plan.HeadMoved {
		t.Fatalf("a commit of other files after the turn refuses: %+v", plan.Refused)
	}
	if _, err := f.service.Undo("local", turn2.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dir2, "a.txt"); got != "one\n" {
		t.Errorf("a.txt is %q", got)
	}
}

func TestUndoRefusesWhileAnAgentWorks(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	f := newFixture(t)
	turn := f.turn(t, dir, func() { write(t, dir, "a.txt", "agent\n") })
	f.working = "Claude Code"
	plan, _ := f.service.Plan("local", turn.ID)
	if plan.Refused == nil || !strings.Contains(plan.Refused.Why, "Claude Code is working") {
		t.Fatalf("not refused: %+v", plan.Refused)
	}
	if _, err := f.service.Undo("local", turn.ID, false); err == nil {
		t.Fatal("an undo went through while the agent works")
	}
	if got := readFile(t, dir, "a.txt"); got != "agent\n" {
		t.Errorf("a.txt is %q", got)
	}
}

func TestTheCapsKeepATurnThatCannotBeUndone(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	host := local{shell.Local{Name: "local"}}
	start, err := read(host, dir, readOptions{patch: forwardPatch, limit: 64})
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "a.txt", strings.Repeat("a long line of the agent's\n", 40))
	end, err := read(host, dir, readOptions{base: start.Head, patch: reversePatch, limit: 64})
	if err != nil {
		t.Fatal(err)
	}
	if !end.Cut {
		t.Fatal("a patch past the limit is not cut")
	}
	turn, startPatch, endPatch := Delta(start, end)
	if !strings.Contains(turn.Why, "too big to keep whole") || startPatch != "" || endPatch != "" || turn.Size != 0 || len(turn.Files) != 1 {
		t.Fatalf("a cut checkpoint: %+v", turn)
	}
	end, _ = read(host, dir, readOptions{base: start.Head, patch: reversePatch})
	saved := MaxDelta
	t.Cleanup(func() { MaxDelta = saved })
	MaxDelta = 100
	turn, _, _ = Delta(start, end)
	if !strings.Contains(turn.Why, "past the 100 B an undo keeps") {
		t.Fatalf("a big delta: %q", turn.Why)
	}
}
