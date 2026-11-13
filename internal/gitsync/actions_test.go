package gitsync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/secrets"
	"github.com/dev1antdev/remuda/internal/shell"
)

func idle(string, string) (string, string) { return "", "" }

func TestFetchBringsTheRemotesCommitsAndLeavesTheCheckout(t *testing.T) {
	home := sandbox(t)
	_, clones := remoteAndClones(t, home, 2)
	mine, theirs := clones[0], clones[1]
	commit(t, theirs, "theirs.txt", "from the other clone\n")
	gitIn(t, theirs, "push", "-q")
	head := gitIn(t, mine, "rev-parse", "HEAD")

	outcome := service(t, idle, nil).Fetch("local", mine)
	if outcome.State != Done || outcome.Remote != "origin" || outcome.Repo == nil {
		t.Fatalf("fetch: %+v", outcome)
	}
	if outcome.Repo.Behind != 1 || outcome.Repo.Ahead != 0 || outcome.Repo.Fetched.IsZero() || outcome.Repo.Upstream != "origin/main" {
		t.Fatalf("the checkout read after the fetch: %+v", outcome.Repo)
	}
	if gitIn(t, mine, "rev-parse", "HEAD") != head {
		t.Fatal("a fetch moved the branch")
	}
}

func TestPullFastForwardsAndSaysWhenThereIsNothing(t *testing.T) {
	home := sandbox(t)
	_, clones := remoteAndClones(t, home, 2)
	mine, theirs := clones[0], clones[1]
	commit(t, theirs, "a.txt", "one\n")
	commit(t, theirs, "b.txt", "two\n")
	gitIn(t, theirs, "push", "-q")
	place := service(t, idle, nil)

	pulled := place.Pull("local", filepath.Join(mine))
	if pulled.State != Done || pulled.Count != 2 || pulled.Reason != "" || pulled.Repo == nil || pulled.Repo.Behind != 0 {
		t.Fatalf("pull: %+v", pulled)
	}
	if _, err := os.Stat(filepath.Join(mine, "b.txt")); err != nil {
		t.Fatal("the pull did not bring the file")
	}
	again := place.Pull("local", mine)
	if again.State != Done || again.Reason != ReasonUpToDate {
		t.Fatalf("a second pull: %+v", again)
	}
}

func TestPullLeavesADivergedBranchAlone(t *testing.T) {
	home := sandbox(t)
	_, clones := remoteAndClones(t, home, 2)
	mine, theirs := clones[0], clones[1]
	commit(t, theirs, "theirs.txt", "theirs\n")
	gitIn(t, theirs, "push", "-q")
	commit(t, mine, "mine.txt", "mine\n")
	head := gitIn(t, mine, "rev-parse", "HEAD")

	outcome := service(t, idle, nil).Pull("local", mine)
	if outcome.State != Refused || outcome.Reason != ReasonDiverged || outcome.Ahead != 1 || outcome.Behind != 1 {
		t.Fatalf("a diverged pull: %+v", outcome)
	}
	if gitIn(t, mine, "rev-parse", "HEAD") != head || gitIn(t, mine, "log", "--merges", "--oneline") != "" {
		t.Fatal("a diverged pull changed the branch")
	}
}

func TestPullRefusesWhileAnAgentIsInTheCheckoutAndKeepsGitsRefusal(t *testing.T) {
	home := sandbox(t)
	_, clones := remoteAndClones(t, home, 2)
	mine, theirs := clones[0], clones[1]
	commit(t, theirs, "README.md", "# changed there\n")
	gitIn(t, theirs, "push", "-q")

	var asked string
	busy := service(t, func(machine, checkout string) (string, string) { asked = checkout; return "claude", DoingWork }, nil)
	outcome := busy.Pull("local", filepath.Join(mine))
	if outcome.State != Refused || outcome.Reason != ReasonBusy || outcome.Agent != "claude" || outcome.Doing != DoingWork || asked != mine {
		t.Fatalf("a pull under an agent: %+v (asked for %q)", outcome, asked)
	}
	if _, err := os.Stat(filepath.Join(mine, ".git", "FETCH_HEAD")); err == nil {
		t.Fatal("a refused pull fetched")
	}

	if err := os.WriteFile(filepath.Join(mine, "README.md"), []byte("# mine, not committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inTheWay := service(t, idle, nil).Pull("local", mine)
	if inTheWay.State != Failed || !strings.Contains(inTheWay.Output, "README.md") {
		t.Fatalf("a pull over local changes: %+v", inTheWay)
	}
	if text, _ := os.ReadFile(filepath.Join(mine, "README.md")); string(text) != "# mine, not committed\n" {
		t.Fatal("the local change was touched")
	}
}

func TestPushSendsTheBranchAndSetsAnUpstreamForANewOne(t *testing.T) {
	home := sandbox(t)
	bare, clones := remoteAndClones(t, home, 1)
	mine := clones[0]
	commit(t, mine, "one.txt", "one\n")
	place := service(t, idle, func(shell.Host, string) (review.SecretScan, error) { return review.SecretScan{}, nil })

	pushed := place.Push("local", mine, false)
	if pushed.State != Done || pushed.Count != 1 || pushed.Reason != "" || pushed.Repo == nil || pushed.Repo.Ahead != 0 {
		t.Fatalf("push: %+v", pushed)
	}
	if gitIn(t, bare, "rev-parse", "main") != gitIn(t, mine, "rev-parse", "HEAD") {
		t.Fatal("the remote does not hold the commit")
	}

	gitIn(t, mine, "checkout", "-q", "-b", "feature/x")
	commit(t, mine, "two.txt", "two\n")
	fresh := place.Push("local", mine, false)
	if fresh.State != Done || fresh.Reason != ReasonNewBranch || fresh.Remote != "origin" || fresh.Repo == nil || fresh.Repo.Upstream != "origin/feature/x" {
		t.Fatalf("a new branch's push: %+v", fresh)
	}
	if gitIn(t, mine, "config", "branch.feature/x.merge") != "refs/heads/feature/x" {
		t.Fatal("the new branch's upstream is not set")
	}
}

func TestPushNeverForcesNorSendsTags(t *testing.T) {
	home := sandbox(t)
	bare, clones := remoteAndClones(t, home, 2)
	mine, theirs := clones[0], clones[1]
	commit(t, theirs, "theirs.txt", "theirs\n")
	gitIn(t, theirs, "push", "-q")
	theirHead := gitIn(t, bare, "rev-parse", "main")
	commit(t, mine, "mine.txt", "mine\n")
	gitIn(t, mine, "config", "push.followTags", "true")
	gitIn(t, mine, "tag", "-a", "-m", "a tag", "v1")
	place := service(t, idle, nil)

	rejected := place.Push("local", mine, false)
	if rejected.State != Failed || !strings.Contains(rejected.Output, "rejected") {
		t.Fatalf("a push the remote must refuse: %+v", rejected)
	}
	if gitIn(t, bare, "rev-parse", "main") != theirHead {
		t.Fatal("the push forced its way over the remote's commit")
	}

	gitIn(t, mine, "fetch", "-q")
	gitIn(t, mine, "rebase", "-q", "origin/main")
	if outcome := place.Push("local", mine, false); outcome.State != Done {
		t.Fatalf("the push after a rebase: %+v", outcome)
	}
	if tags := gitIn(t, bare, "tag"); tags != "" {
		t.Fatalf("the push carried tags: %q", tags)
	}
}

func TestTheSecretCheckHoldsAPushUntilTheyPushAnyway(t *testing.T) {
	home := sandbox(t)
	bare, clones := remoteAndClones(t, home, 1)
	mine := clones[0]
	commit(t, mine, "config.go", "package config\n")
	before := gitIn(t, bare, "rev-parse", "main")
	found := review.SecretScan{Findings: []secrets.Finding{{File: "config.go", Line: 3, Rule: secrets.RuleAWS, Excerpt: "AKIA••••••••", Commit: "abc1234"}}, Commits: 1, Push: true}
	var checked string
	place := service(t, idle, func(_ shell.Host, dir string) (review.SecretScan, error) { checked = dir; return found, nil })

	held := place.Push("local", mine, false)
	if held.State != Held || held.Reason != ReasonSecretsFound || held.Scan == nil || len(held.Scan.Findings) != 1 || checked != mine {
		t.Fatalf("a push with a secret: %+v (checked %q)", held, checked)
	}
	if gitIn(t, bare, "rev-parse", "main") != before {
		t.Fatal("a held push reached the remote")
	}
	failing := service(t, idle, func(shell.Host, string) (review.SecretScan, error) {
		return review.SecretScan{}, errors.New("it took longer than 45s")
	})
	if outcome := failing.Push("local", mine, false); outcome.State != Held || outcome.Reason != ReasonCheckFailed || outcome.Output == "" {
		t.Fatalf("a push whose check failed: %+v", outcome)
	}
	if outcome := place.Push("local", mine, true); outcome.State != Done {
		t.Fatalf("push anyway: %+v", outcome)
	}
	if gitIn(t, bare, "rev-parse", "main") == before {
		t.Fatal("push anyway did not push")
	}
}

func TestActionsSayWhyTheyDoNothing(t *testing.T) {
	home := sandbox(t)
	_, clones := remoteAndClones(t, home, 1)
	mine := clones[0]
	alone := filepath.Join(home, "alone")
	gitIn(t, home, "init", "-q", alone)
	commit(t, alone, "a.txt", "a\n")
	plain := filepath.Join(home, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	place := service(t, idle, nil)

	for _, check := range []struct {
		name    string
		outcome Outcome
		reason  string
	}{
		{"a fetch with no remote", place.Fetch("local", alone), ReasonNoRemote},
		{"a push with no remote", place.Push("local", alone, true), ReasonNoRemote},
		{"a pull with no remote", place.Pull("local", alone), ReasonNoRemote},
		{"a folder outside git", place.Fetch("local", plain), ReasonNotRepo},
		{"a folder that is not there", place.Pull("local", filepath.Join(home, "gone")), ReasonMissing},
	} {
		if check.outcome.State != Refused || check.outcome.Reason != check.reason {
			t.Errorf("%s: %+v", check.name, check.outcome)
		}
	}

	gitIn(t, mine, "checkout", "-q", "--detach")
	if outcome := place.Pull("local", mine); outcome.Reason != ReasonDetached {
		t.Errorf("a detached pull: %+v", outcome)
	}
	if outcome := place.Push("local", mine, true); outcome.Reason != ReasonDetached {
		t.Errorf("a detached push: %+v", outcome)
	}
	gitIn(t, mine, "checkout", "-q", "-b", "local-only")
	if outcome := place.Pull("local", mine); outcome.Reason != ReasonNoUpstream || outcome.Branch != "local-only" {
		t.Errorf("a pull of a branch with no upstream: %+v", outcome)
	}

	away := &Service{Host: func(string) (shell.Host, error) { return nil, errors.New("GOG is not connected") }, Scans: OpenScans(t.TempDir())}
	if outcome := away.Fetch("gog", mine); outcome.Reason != ReasonAway || outcome.Output != "GOG is not connected" {
		t.Errorf("a machine away: %+v", outcome)
	}
}

func TestFetchRunsWithoutAPrompt(t *testing.T) {
	home := sandbox(t)
	_, clones := remoteAndClones(t, home, 1)
	mine := clones[0]
	gitIn(t, mine, "remote", "set-url", "origin", "https://127.0.0.1:9/nobody.git")
	outcome := service(t, idle, nil).Fetch("local", mine)
	if outcome.State != Failed || outcome.Output == "" {
		t.Fatalf("a fetch that cannot reach its remote: %+v", outcome)
	}
}
