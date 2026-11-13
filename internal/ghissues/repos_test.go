package ghissues

import (
	"strings"
	"testing"
	"time"
)

func originLine(fields ...string) string {
	return strings.Join(fields, "\x1f") + "\n"
}

func TestReposGroupTheCheckoutsByTheirGitHubOrigin(t *testing.T) {
	out := originLine("D", "/home/alice/widgets", "/home/alice/widgets", "/home/alice/widgets/.git", "git@github.com:acme/widgets.git") +
		originLine("L", "/home/alice/widgets", "branch.dev/feat/41.remuda-issue acme/widgets#41") +
		originLine("L", "/home/alice/widgets", "branch.fix.v2.remuda-issue #7, other/repo#9, 12") +
		originLine("D", "/home/alice/.herdr/worktrees/widgets/dev-feat-41", "/home/alice/.herdr/worktrees/widgets/dev-feat-41", "/home/alice/widgets/.git", "https://github.com/acme/widgets") +
		originLine("L", "/home/alice/.herdr/worktrees/widgets/dev-feat-41", "branch.dev/feat/41.remuda-issue acme/widgets#41") +
		originLine("D", "/home/alice/app", "/home/alice/app", "/home/alice/app/.git", "git@gitlab.com:team/app.git") +
		originLine("D", "/home/alice/notes", "") +
		originLine("D", "/home/alice/gone", "-") +
		originLine("D", "/home/alice/scratch", "/home/alice/scratch", "/home/alice/scratch/.git", "")
	host := newFake(func(string) (string, error) { return out, nil })
	s, clock := serviceOn(host)
	dirs := []string{"/home/alice/widgets", "/home/alice/.herdr/worktrees/widgets/dev-feat-41", "/home/alice/app", "/home/alice/notes", "/home/alice/gone"}
	answer, err := s.Repos("gog", dirs, "/home/alice/scratch")
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Repos) != 1 {
		t.Fatalf("repos: %+v", answer.Repos)
	}
	r := answer.Repos[0]
	if r.Repo != "acme/widgets" || r.Folder != "/home/alice/widgets" || len(r.Dirs) != 2 {
		t.Fatalf("the repository and its two checkouts: %+v", r)
	}
	want := []Link{{"dev/feat/41", 41}, {"fix.v2", 7}, {"fix.v2", 12}}
	if len(r.Links) != len(want) {
		t.Fatalf("links: %+v", r.Links)
	}
	for i := range want {
		if r.Links[i] != want[i] {
			t.Fatalf("links: %+v, want %+v", r.Links, want)
		}
	}
	if answer.Here == nil || answer.Here.Why != "no-origin" || answer.Here.Folder != "/home/alice/scratch" {
		t.Fatalf("a checkout with no origin: %+v", answer.Here)
	}
	script := host.Scripts()[0]
	for _, d := range append(dirs, "/home/alice/scratch") {
		if !strings.Contains(script, "'"+d+"'") {
			t.Fatalf("%s not read:\n%s", d, script)
		}
	}
	if !strings.Contains(script, "--no-optional-locks -c core.fsmonitor=false") {
		t.Fatal("a read could touch the index")
	}

	for asked, why := range map[string]string{"/home/alice/app": "not-github", "/home/alice/notes": "no-git", "/home/alice/gone": "no-folder"} {
		answer, _ := s.Repos("gog", dirs, asked)
		if answer.Here == nil || answer.Here.Why != why {
			t.Errorf("%s: %+v, want %s", asked, answer.Here, why)
		}
	}
	if answer, _ := s.Repos("gog", dirs, "/home/alice/app"); answer.Here.Host != "gitlab.com" {
		t.Errorf("an origin elsewhere names its host: %+v", answer.Here)
	}
	if here, _ := s.Repos("gog", dirs, "/home/alice/.herdr/worktrees/widgets/dev-feat-41"); here.Here.Repo != "acme/widgets" || here.Here.Folder != "/home/alice/widgets" {
		t.Errorf("a linked worktree's repository and main folder: %+v", here.Here)
	}
	reads := len(host.Scripts())
	*clock = clock.Add(2 * time.Minute)
	_, _ = s.Repos("gog", dirs, "")
	if len(host.Scripts()) == reads {
		t.Error("origins older than a minute were not read again")
	}
}

func TestAWindowsCheckoutsFoldersReadInItsOwnSpelling(t *testing.T) {
	out := originLine("D", `C:\Users\me\widgets`, "C:/Users/me/widgets", "/c/Users/me/widgets/.git", "https://github.com/acme/widgets.git")
	host := newFake(func(string) (string, error) { return out, nil })
	host.Dir = `C:\Users\me`
	s, _ := serviceOn(host)
	answer, err := s.Repos("win", []string{`C:\Users\me\widgets`}, `C:\Users\me\widgets`)
	if err != nil || len(answer.Repos) != 1 {
		t.Fatalf("%+v %v", answer, err)
	}
	if got := answer.Repos[0].Folder; got != `C:\Users\me\widgets` {
		t.Fatalf("main folder %q", got)
	}
	if answer.Repos[0].Dirs[0] != `C:\Users\me\widgets` {
		t.Fatalf("the folder is not given back as it was asked: %+v", answer.Repos[0].Dirs)
	}
}
