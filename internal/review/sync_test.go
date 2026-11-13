package review

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestBadgesReadTheBranchAgainstItsUpstream(t *testing.T) {
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	machine := shell.Local{Name: "local"}
	store := New(t.TempDir())

	git("checkout", "-qb", "feature")
	store.Badges(machine, []string{dir})
	if sync, ok := store.Sync("local", dir); ok {
		t.Fatalf("a branch with no upstream has a sync %+v", sync)
	}

	git("branch", "-q", "--set-upstream-to=main")
	write("b.txt", "b")
	git("add", "-A")
	git("commit", "-qm", "one")
	write("c.txt", "c")
	git("add", "-A")
	git("commit", "-qm", "two")
	git("checkout", "-q", "main")
	write("d.txt", "d")
	git("add", "-A")
	git("commit", "-qm", "on main")
	git("checkout", "-q", "feature")
	store.Badges(machine, []string{dir})
	if sync, ok := store.Sync("local", dir); !ok || sync != (Sync{Ahead: 2, Behind: 1}) {
		t.Fatalf("two ahead, one behind: %+v %v", sync, ok)
	}
	if branch := store.Branch("local", dir); branch != "feature" {
		t.Fatalf("the branch is still read beside its upstream: %q", branch)
	}

	git("checkout", "-q", "--detach")
	store.Badges(machine, []string{dir})
	if sync, ok := store.Sync("local", dir); ok {
		t.Fatalf("a detached HEAD keeps a sync %+v", sync)
	}
}

func TestParseSyncReadsGitsTwoCounts(t *testing.T) {
	for line, want := range map[string]Sync{"2\t1\n": {Ahead: 2, Behind: 1}, "0\t0": {}} {
		if got, ok := parseSync(line); !ok || got != want {
			t.Fatalf("%q: %+v %v", line, got, ok)
		}
	}
	for _, line := range []string{"", "\n", "x\t1", "1", "-1\t0"} {
		if got, ok := parseSync(line); ok {
			t.Fatalf("%q read as %+v", line, got)
		}
	}
}
