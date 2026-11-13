package prstatus

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/shell"
)

// view is gh pr view's answer with every field the hover card reads, anonymized from GitHub's documented shape.
const view = `{"closingIssuesReferences":[{"id":"I_kwAB","number":198,"repository":{"id":"R_kgAB","name":"storefront","owner":{"id":"O_1","login":"acme"}},"url":"https://github.com/acme/storefront/issues/198"}],` +
	`"isDraft":false,"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","reviewDecision":"APPROVED","state":"OPEN",` +
	`"statusCheckRollup":[{"__typename":"CheckRun","conclusion":"SUCCESS","name":"build","status":"COMPLETED"}],"title":"Add saved carts for signed-in shoppers"}`

func TestParseReadsTheTitleTheMergeStateAndTheIssuesItCloses(t *testing.T) {
	status, err := Parse(view, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if status.Title != "Add saved carts for signed-in shoppers" || status.MergeState != "CLEAN" {
		t.Fatalf("status = %+v", status)
	}
	want := Issue{Number: 198, URL: "https://github.com/acme/storefront/issues/198", Repo: "acme/storefront"}
	if len(status.Closes) != 1 || status.Closes[0] != want {
		t.Fatalf("closes = %+v", status.Closes)
	}
	older, err := Parse(mixed, time.Now())
	if err != nil || older.Title != "" || older.MergeState != "" || older.Closes != nil {
		t.Fatalf("a gh answer without the new fields: %+v %v", older, err)
	}
}

// A gh from before closingIssuesReferences refuses the whole call: the script asks again with the fields every gh knows.
func TestScriptAsksAgainWithoutTheFieldsAnOlderGhRefuses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a POSIX script")
	}
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	standIn := "#!/bin/sh\ncase \"$*\" in\n*closingIssuesReferences*) echo 'Unknown JSON field: \"closingIssuesReferences\"'; echo 'Available fields:'; echo '  state'; exit 1 ;;\n" +
		"*) echo '{\"state\":\"OPEN\",\"title\":\"From an older gh\",\"isDraft\":false,\"mergeable\":\"MERGEABLE\",\"reviewDecision\":\"\",\"statusCheckRollup\":[]}' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(standIn), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", Script("https://github.com/o/r/pull/7"))
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	status, err := Parse(string(out), time.Now())
	if err != nil || status.Title != "From an older gh" || status.State != "OPEN" {
		t.Fatalf("status = %+v %v from %q", status, err, out)
	}
}

func TestFreshReadsAgainOnlyAfterFreshForAndSaysWhatItHolds(t *testing.T) {
	release := make(chan struct{})
	var reads atomic.Int32
	host := &shell.Fake{Name: "box", Answer: func(script string) (string, error) {
		if strings.Contains(script, "gh api -i") {
			return "", nil // the conditional probe answers no headers at once: the read that follows is the one held
		}
		reads.Add(1)
		<-release
		return view, nil
	}}
	clock := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	tr := New(func(string) (shell.Shell, bool) { return host, true })
	tr.now = func() time.Time { return clock }
	var changes atomic.Int32
	tr.OnChange = func() { changes.Add(1) }
	ref := events.PaneRef{Machine: "box", Pane: "w1:p1"}

	if _, ok := tr.Seen("box", "w1:p1"); ok {
		t.Fatal("a pane naming nothing is seen")
	}
	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://github.com/o/r/pull/7", Label: "PR #7"})
	waitFor(t, "the read to begin", func() bool { return reads.Load() == 1 })
	if seen, ok := tr.Seen("box", "w1:p1"); !ok || !seen.Reading || seen.Status != nil {
		t.Fatalf("while reading: %+v %v", seen, ok)
	}
	release <- struct{}{}
	waitFor(t, "the read to end", func() bool { seen, _ := tr.Seen("box", "w1:p1"); return !seen.Reading })
	seen, _ := tr.Seen("box", "w1:p1")
	if seen.Status == nil || seen.Status.Title == "" || seen.Reason != "" {
		t.Fatalf("after the read: %+v", seen)
	}
	if changes.Load() != 2 {
		t.Fatalf("changes told = %d, want the read's start and end", changes.Load())
	}

	clock = clock.Add(FreshFor / 2)
	if tr.Fresh("box", "w1:p1") {
		t.Fatal("a read younger than FreshFor was repeated")
	}
	clock = clock.Add(FreshFor)
	if !tr.Fresh("box", "w1:p1") {
		t.Fatal("a read older than FreshFor was not repeated")
	}
	if tr.Fresh("box", "w1:p1") {
		t.Fatal("a second read began beside the first")
	}
	release <- struct{}{}
	waitFor(t, "the second read", func() bool { seen, _ := tr.Seen("box", "w1:p1"); return !seen.Reading && reads.Load() == 2 })
	if tr.Fresh("box", "w1:p9") {
		t.Fatal("a pane naming nothing was read")
	}
}

func TestSeenKeepsTheLastStatusBesideAFailedRead(t *testing.T) {
	var fail atomic.Bool
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) {
		if fail.Load() {
			return "error connecting to api.github.com\n", &shell.ExitError{Code: 1}
		}
		return view, nil
	}}
	clock := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	tr := New(func(string) (shell.Shell, bool) { return host, true })
	tr.now = func() time.Time { return clock }
	ref := events.PaneRef{Machine: "box", Pane: "w1:p1"}
	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://github.com/o/r/pull/7", Label: "PR #7"})
	waitFor(t, "the first read", func() bool { seen, _ := tr.Seen("box", "w1:p1"); return seen.Status != nil && !seen.Reading })
	fail.Store(true)
	clock = clock.Add(FreshFor)
	tr.Fresh("box", "w1:p1")
	waitFor(t, "the failed read", func() bool { seen, _ := tr.Seen("box", "w1:p1"); return seen.Reason != "" && !seen.Reading })
	seen, _ := tr.Seen("box", "w1:p1")
	if seen.Status == nil || !strings.HasPrefix(seen.Reason, "error connecting") || !seen.Status.At.Before(clock) {
		t.Fatalf("seen = %+v", seen)
	}
}
