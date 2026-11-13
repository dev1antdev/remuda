package preview

import (
	"strconv"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestRecallPutsBackWhatAnEarlierLookLearnt(t *testing.T) {
	seen := map[int]Seen{}
	first := Recall(macAnswer, seen)
	if got := ports(Parse(first, nil)); got != ports(Parse(macAnswer, nil)) {
		t.Fatalf("a first look reads as the script answered: %s", got)
	}
	if seen[4100].Dir != "/Users/me/.herdr/worktrees/app/task-1" || seen[4300].ParentDir != "/Users/me/code/notes" {
		t.Fatalf("seen %+v", seen)
	}
	// the next look knows 4100 and 4300; 4400 stopped listening, 4200 is a new program under an old pid
	again := line("5173", "[::1]", "4100", "node", Known, Known, Known) +
		line("8000", "*", "4300", "python3", Known, Known, Known) +
		line("3000", "127.0.0.1", "4200", "ruby", Known, Known, Known)
	list := Parse(Recall(again, seen), nil)
	if got := ports(list); got != "5173@::1=vite 8000@127.0.0.1=http.server" {
		t.Fatalf("listeners %s", got)
	}
	if _, ok := seen[4200]; ok {
		t.Error("a pid another program took is asked about again")
	}
	if _, ok := seen[4400]; ok {
		t.Error("a pid that listens no more is forgotten")
	}
}

func TestScriptKnowingLooksUpNothingOfTheKnownPids(t *testing.T) {
	text := ScriptKnowing([]int{4100, 4300})
	if !strings.Contains(text, `known=" 4100 4300 "`) || !strings.Contains(text, `\037=\037=\037=\n`) {
		t.Fatalf("the script does not skip the known pids:\n%s", text)
	}
	if !strings.Contains(Script(), `known="  "`) {
		t.Fatal("Script knows nothing")
	}
}

func TestAKnownServerIsNotLookedUpAgain(t *testing.T) {
	port, pid, dir := serveHere(t)
	seen := map[int]Seen{}
	first, err := shell.Text(shell.Local{Name: "local"}, Script(), FindTimeout)
	if err != nil && strings.TrimSpace(first) == "" {
		t.Fatal(err)
	}
	Recall(first, seen)
	if seen[pid].Dir != dir {
		t.Fatalf("the first look learnt %+v of %d, want the folder %s", seen[pid], pid, dir)
	}
	again, err := shell.Text(shell.Local{Name: "local"}, ScriptKnowing([]int{pid}), FindTimeout)
	if err != nil && strings.TrimSpace(again) == "" {
		t.Fatal(err)
	}
	if !strings.Contains(again, "\x1f"+strconv.Itoa(pid)+"\x1f"+seen[pid].Command+"\x1f=\x1f=\x1f=\n") {
		t.Fatalf("the known pid was looked up again:\n%q", again)
	}
	for _, l := range Parse(Recall(again, seen), nil) {
		if l.Port == port {
			if l.Dir != dir || l.Label != "http.server" {
				t.Fatalf("recalled %+v", l)
			}
			return
		}
	}
	t.Fatalf("port %d lost on the second look", port)
}
