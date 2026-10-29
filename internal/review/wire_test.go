package review

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// TestSummaryReadsTheScriptsAnswer pins the wire format summaryScript prints, with a recorded answer and no git.
func TestSummaryReadsTheScriptsAnswer(t *testing.T) {
	const reviewedBlob = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const freshBlob = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	answer := strings.Join([]string{
		"/repo\nmain\n0123456789abcdef0123456789abcdef01234567\n",
		"abc1234" + script.US + "committed as reviewed" + script.US + "1700000000\n" +
			"def5678" + script.US + "something new" + script.US + "1700000100\n",
		"1\t0\ta.txt\n3\t2\tb.txt\n",
		"new.txt\n",
		"a.txt" + script.US + reviewedBlob + "\nb.txt" + script.US + freshBlob + "\nnew.txt" + script.US + freshBlob + "\n",
		"abc1234000000000000000000000000000000000\n\n:100644 100644 0000000 " + reviewedBlob + " M\ta.txt\n" +
			"def5678000000000000000000000000000000000\n\n:100644 100644 0000000 " + freshBlob + " M\tb.txt\n",
		"14\n",
	}, script.RS+"\n")
	host := &shell.Fake{Name: "box", Remote: true, Answer: func(string) (string, error) { return answer, nil }}

	store := New(t.TempDir())
	store.mu.Lock()
	store.baselines[key("box", "/repo")] = Baseline{Head: "oldhead", Blobs: map[string]string{"a.txt": reviewedBlob, "b.txt": reviewedBlob}, At: time.Now()}
	store.mu.Unlock()

	summary := store.Summary(host, "/repo")
	if summary.Error != "" || summary.Repo != "/repo" || summary.Branch != "main" || !strings.HasPrefix(summary.Head, "01234567") {
		t.Fatalf("head: %+v", summary)
	}
	if len(summary.Commits) != 2 || !summary.Commits[0].Seen || summary.Commits[1].Seen || summary.Commits[0].Subject != "committed as reviewed" {
		t.Fatalf("commits: %+v", summary.Commits)
	}
	if summary.CommitsTotal != 14 {
		t.Fatalf("commits total = %d, want 14 (the count past the listed ones)", summary.CommitsTotal)
	}
	if len(summary.Files) != 3 || summary.Fresh != 2 {
		t.Fatalf("files: %+v", summary.Files)
	}
	fresh := map[string]bool{}
	for _, file := range summary.Files {
		fresh[file.Path] = file.Fresh
	}
	if fresh["a.txt"] || !fresh["b.txt"] || !fresh["new.txt"] {
		t.Fatalf("fresh: %v", fresh)
	}
	if badge, ok := store.Badge("box", "/repo"); !ok || badge.Changed != 3 || badge.Fresh != 2 {
		t.Fatalf("badge: %+v %v", badge, ok)
	}
	if scripts := host.Scripts(); len(scripts) != 1 || !strings.Contains(scripts[0], "cd '/repo'") || !strings.Contains(scripts[0], "base='oldhead'") {
		t.Fatalf("script: %q", scripts)
	}
}
