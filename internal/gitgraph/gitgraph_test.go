package gitgraph

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	// the person's own git settings (a signing key, hooks, a template) never reach the tests' repositories
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Ada")
	t.Setenv("GIT_AUTHOR_EMAIL", "ada@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Ada")
	t.Setenv("GIT_COMMITTER_EMAIL", "ada@example.com")
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, path, text string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// history is a repository with a branch forked off main and merged back, a tag, a branch left unmerged and a
// "remote" branch: every kind of label a row carries.
func history(t *testing.T) string {
	t.Helper()
	needGit(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "README.md", "# app\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "first")
	run(t, dir, "tag", "v0.1.0")
	run(t, dir, "checkout", "-qb", "feature")
	write(t, dir, "src/a.go", "package a\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "feature: a")
	run(t, dir, "checkout", "-q", "main")
	write(t, dir, "README.md", "# app\n\nmore\n")
	run(t, dir, "commit", "-qam", "readme")
	run(t, dir, "merge", "-q", "--no-ff", "-m", "Merge feature", "feature")
	run(t, dir, "checkout", "-qb", "wip")
	write(t, dir, "notes.txt", "wip\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "wip: notes")
	run(t, dir, "checkout", "-q", "main")
	run(t, dir, "update-ref", "refs/remotes/origin/main", "main~1")
	run(t, dir, "update-ref", "refs/t3/checkpoints/1", "wip") // another tool's private ref: never walked
	return dir
}

var host = shell.Local{Name: "local"}

func TestTheLogReadsEveryBranchTagAndRemoteWithTheirLabels(t *testing.T) {
	dir := history(t)
	log, err := ReadLog(host, dir, Query{All: true, Remotes: true, Limit: Page})
	if err != nil {
		t.Fatal(err)
	}
	if !log.Repo || log.Root != dir || log.Branch != "main" || log.Head == "" || log.Error != "" {
		t.Fatalf("the state is %+v", log.State)
	}
	if len(log.Commits) != 5 || log.More {
		t.Fatalf("%d commits, more %v", len(log.Commits), log.More)
	}
	subjects := []string{}
	for _, c := range log.Commits {
		subjects = append(subjects, c.Subject)
	}
	if subjects[len(subjects)-1] != "first" {
		t.Fatalf("the root is not last: %v", subjects)
	}
	labels := map[string]Ref{}
	for _, c := range log.Commits {
		for _, r := range c.Refs {
			labels[r.Kind+":"+r.Name] = r
		}
	}
	for _, want := range []string{"branch:main", "branch:feature", "branch:wip", "tag:v0.1.0", "remote:origin/main"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("no label %s among %v", want, labels)
		}
	}
	if !labels["branch:main"].Head || labels["branch:wip"].Head {
		t.Errorf("the checked-out branch is not the one marked: %v", labels)
	}
	merge := log.Commits[1]
	if merge.Subject == "Merge feature" && len(merge.Parents) != 2 {
		t.Errorf("the merge has %d parents", len(merge.Parents))
	}
	if strings.Join(log.Branches, ",") != "feature,main,wip" || len(log.Remotes) != 0 {
		t.Errorf("branches %v, remotes %v", log.Branches, log.Remotes)
	}
}

func TestCurrentWalksHEADAloneAndRemotesCanBeLeftOut(t *testing.T) {
	dir := history(t)
	current, err := ReadLog(host, dir, Query{All: false, Limit: Page})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range current.Commits {
		if c.Subject == "wip: notes" {
			t.Fatal("Current walked a branch HEAD does not hold")
		}
	}
	if len(current.Commits) != 4 {
		t.Fatalf("HEAD holds %d commits here", len(current.Commits))
	}
	all, _ := ReadLog(host, dir, Query{All: true, Remotes: false, Limit: Page})
	for _, c := range all.Commits {
		for _, r := range c.Refs {
			if r.Kind == "remote" {
				t.Fatal("a remote branch labelled a row with the remotes left out")
			}
		}
	}
}

func TestPagesSayWhetherMoreFollowAndCarryOn(t *testing.T) {
	dir := history(t)
	first, err := ReadLog(host, dir, Query{All: true, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Commits) != 2 || !first.More {
		t.Fatalf("a page of 2: %d commits, more %v", len(first.Commits), first.More)
	}
	next, _ := ReadLog(host, dir, Query{All: true, Skip: 2, Limit: 2})
	last, _ := ReadLog(host, dir, Query{All: true, Skip: 4, Limit: 2})
	if len(next.Commits) != 2 || !next.More || len(last.Commits) != 1 || last.More {
		t.Fatalf("the pages after: %d (%v), %d (%v)", len(next.Commits), next.More, len(last.Commits), last.More)
	}
	if next.Commits[0].Hash == first.Commits[1].Hash || next.Fingerprint != first.Fingerprint {
		t.Fatal("the second page repeats the first, or the refs moved")
	}
}

func TestTheFingerprintMovesWithTheRefsAndNotWithAnEdit(t *testing.T) {
	dir := history(t)
	before, _ := ReadState(host, dir)
	write(t, dir, "README.md", "edited\n")
	write(t, dir, "new.txt", "fresh\n")
	edited, _ := ReadState(host, dir)
	if edited.Fingerprint != before.Fingerprint {
		t.Fatal("an edit in the working tree moved the refs' fingerprint")
	}
	if edited.Dirty != 2 || len(edited.Changes) != 1 || edited.Changes[0].Path != "README.md" || edited.Changes[0].Status != " M" {
		t.Fatalf("the uncommitted files: %d, %+v", edited.Dirty, edited.Changes)
	}
	run(t, dir, "tag", "v0.2.0")
	tagged, _ := ReadState(host, dir)
	if tagged.Fingerprint == before.Fingerprint {
		t.Fatal("a new tag left the fingerprint as it was")
	}
	run(t, dir, "branch", "-f", "wip", "main")
	moved, _ := ReadState(host, dir)
	if moved.Fingerprint == tagged.Fingerprint {
		t.Fatal("a branch moved left the fingerprint as it was")
	}
}

func TestAReadWritesNothingIntoTheRepository(t *testing.T) {
	dir := history(t)
	write(t, dir, "README.md", "edited\n")
	index := filepath.Join(dir, ".git", "index")
	stat, _ := os.Stat(index)
	if _, err := ReadLog(host, dir, Query{All: true, Remotes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadState(host, dir); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(index)
	if !after.ModTime().Equal(stat.ModTime()) || after.Size() != stat.Size() {
		t.Fatal("a read rewrote the index")
	}
	if _, err := os.Stat(index + ".lock"); err == nil {
		t.Fatal("a read left an index lock")
	}
}

func TestAFolderOutsideGitAndARepositoryWithNoCommits(t *testing.T) {
	needGit(t)
	plain := t.TempDir()
	log, err := ReadLog(host, plain, Query{All: true})
	if err != nil || log.Repo {
		t.Fatalf("a plain folder reads as %+v, %v", log.State, err)
	}
	gone, err := ReadState(host, filepath.Join(plain, "gone"))
	if err != nil || gone.Repo {
		t.Fatalf("a folder gone reads as %+v, %v", gone, err)
	}
	empty := t.TempDir()
	run(t, empty, "init", "-q", "-b", "main")
	write(t, empty, "a.txt", "a\n")
	fresh, err := ReadLog(host, empty, Query{All: true})
	if err != nil || !fresh.Repo || fresh.Head != "" || len(fresh.Commits) != 0 || fresh.Error != "" || fresh.Dirty != 1 {
		t.Fatalf("a repository with no commit reads as %+v, %v", fresh, err)
	}
}

func TestADetachedHEADAndAMergeStoppedOnConflicts(t *testing.T) {
	dir := history(t)
	run(t, dir, "checkout", "-q", "--detach", "HEAD~1")
	detached, _ := ReadLog(host, dir, Query{All: true})
	if detached.Branch != "" {
		t.Fatalf("a detached HEAD reads as on %q", detached.Branch)
	}
	found := false
	for _, c := range detached.Commits {
		for _, r := range c.Refs {
			found = found || r.Kind == "head"
		}
	}
	if !found {
		t.Fatal("no HEAD label on the detached commit")
	}
	run(t, dir, "checkout", "-q", "main")
	run(t, dir, "checkout", "-qb", "clash")
	write(t, dir, "README.md", "theirs\n")
	run(t, dir, "commit", "-qam", "clash")
	run(t, dir, "checkout", "-q", "main")
	write(t, dir, "README.md", "ours\n")
	run(t, dir, "commit", "-qam", "ours")
	cmd := exec.Command("git", "merge", "clash")
	cmd.Dir = dir
	_ = cmd.Run()
	stopped, _ := ReadState(host, dir)
	if stopped.Operation != "merge" || stopped.Conflicts != 1 {
		t.Fatalf("a merge stopped on a conflict reads as %q with %d conflicts", stopped.Operation, stopped.Conflicts)
	}
}

func TestACommitsDetailAndAFilesPatch(t *testing.T) {
	dir := history(t)
	write(t, dir, "src/a.go", "package a\n\nfunc A() {}\n")
	run(t, dir, "mv", "README.md", "notes.md")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "rename and grow\n\nThe body says why.")
	head := run(t, dir, "rev-parse", "HEAD")
	detail, err := ReadCommit(host, dir, head)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Author != "Ada" || detail.Email != "ada@example.com" || !strings.Contains(detail.Message, "The body says why.") || detail.Parent == "" {
		t.Fatalf("the detail is %+v", detail)
	}
	byPath := map[string]File{}
	for _, f := range detail.Files {
		byPath[f.Path] = f
	}
	if f := byPath["src/a.go"]; f.Status != "M" || f.Add != 2 {
		t.Errorf("src/a.go is %+v", f)
	}
	if f := byPath["notes.md"]; f.Status != "R" || f.From != "README.md" {
		t.Errorf("the rename is %+v", f)
	}
	patch, err := ReadPatch(host, dir, head, "src/a.go", "")
	if err != nil || !strings.Contains(patch, "+func A() {}") || !strings.Contains(patch, "@@") {
		t.Fatalf("the patch is %q, %v", patch, err)
	}
	renamed, err := ReadPatch(host, dir, head, "notes.md", "README.md")
	if err != nil || !strings.Contains(renamed, "rename from README.md") {
		t.Fatalf("the rename's patch is %q, %v", renamed, err)
	}
	root := run(t, dir, "rev-list", "--max-parents=0", "HEAD")
	first, _ := ReadCommit(host, dir, root)
	if first.Parent != "" || len(first.Files) != 1 || first.Files[0].Path != "README.md" || first.Files[0].Status != "A" {
		t.Fatalf("the root commit's detail is %+v", first)
	}
	if _, err := ReadCommit(host, dir, strings.Repeat("0", 40)); err != ErrNoCommit {
		t.Fatalf("a hash the repository lacks answered %v", err)
	}
}

func TestInputsThatCouldReachGitAsSomethingElseAreRefused(t *testing.T) {
	for _, hash := range []string{"", "HEAD", "main", "-n1", "abc", strings.Repeat("g", 40)} {
		if HashOK(hash) {
			t.Errorf("%q passes as a hash", hash)
		}
	}
	for _, path := range []string{"", "/etc/passwd", "-p", "../x", "a/../../b", "a\nb"} {
		if PathOK(path) {
			t.Errorf("%q passes as a path", path)
		}
	}
	if _, err := ReadPatch(host, t.TempDir(), "abcdef1", "--output=x", ""); err != ErrInput {
		t.Fatalf("an option as a path answered %v", err)
	}
}
