package ghissues

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// fakeHost is a machine whose gh answers through Answer, its file puts noted.
type fakeHost struct {
	*shell.Fake
	mu  sync.Mutex
	put map[string]string
}

func (f *fakeHost) PutFile(path string, data []byte, timeout time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.put == nil {
		f.put = map[string]string{}
	}
	f.put[path] = string(data)
	return nil
}

func newFake(answer func(script string) (string, error)) *fakeHost {
	return &fakeHost{Fake: &shell.Fake{Name: "GOG", Dir: "/home/alice", Answer: answer}}
}

func serviceOn(host *fakeHost) (*Service, *time.Time) {
	clock := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	s := New(func(string) (Host, error) { return host, nil })
	s.now = func() time.Time { return clock }
	return s, &clock
}

const rs = "\x1e\n"

func TestAListIsReadKeptThenConfirmedByAConditionalRequest(t *testing.T) {
	host := newFake(func(script string) (string, error) {
		if strings.Contains(script, "If-None-Match") {
			return rs + headers304 + rs, nil
		}
		return "alice\n" + rs + headers200 + rs + listJSON, nil
	})
	s, clock := serviceOn(host)

	list, err := s.List("gog", "acme/widgets", DefaultList, false)
	if err != nil || list.Source != FromGitHub || len(list.Items) != 3 || list.Me != "alice" {
		t.Fatalf("first read: %+v %v", list, err)
	}
	if got := []int{list.Items[0].Number, list.Items[1].Number, list.Items[2].Number}; got[0] != 42 || got[1] != 41 || got[2] != 40 {
		t.Fatalf("not the most recently updated first: %v", got)
	}
	if first := list.Items[0]; first.State != "closed" || first.Reason != "not_planned" || !first.Author.Bot || first.Author.Login != "renovate" {
		t.Fatalf("a bot's issue closed as not planned: %+v", first)
	}
	if docs := list.Items[2]; docs.Reason != "completed" || docs.Labels[0].Color != "0075ca" || docs.Labels[1].Color != "" {
		t.Fatalf("colours are six hex digits or none: %+v", docs.Labels)
	}

	*clock = clock.Add(30 * time.Second)
	if again, _ := s.List("gog", "acme/widgets", DefaultList, false); again.Source != FromCache || len(host.Scripts()) != 1 {
		t.Fatalf("a list under a minute old asked GitHub again: %s, %d scripts", again.Source, len(host.Scripts()))
	}

	*clock = clock.Add(2 * time.Minute)
	probed, err := s.List("gog", "acme/widgets", DefaultList, false)
	if err != nil || probed.Source != FromProbe || len(probed.Items) != 3 {
		t.Fatalf("a 304 did not keep the list: %+v %v", probed, err)
	}
	last := host.Scripts()[len(host.Scripts())-1]
	if !strings.Contains(last, `If-None-Match: W/"aaa111"`) || strings.Contains(last, "api user") {
		t.Fatalf("the probe did not carry the ETag, or asked the login again:\n%s", last)
	}

	*clock = clock.Add(20 * time.Minute)
	if read, _ := s.List("gog", "acme/widgets", DefaultList, false); read.Source != FromGitHub {
		t.Fatalf("a list past a quarter of an hour was not read whole: %s", read.Source)
	}
	if last := host.Scripts()[len(host.Scripts())-1]; strings.Contains(last, "If-None-Match") {
		t.Fatal("a stale list was only probed")
	}
	if read, _ := s.List("gog", "acme/widgets", DefaultList, true); read.Source != FromGitHub {
		t.Fatalf("Refresh served a kept list: %s", read.Source)
	}
}

func TestTheSearchTravelsQuotedAndAnOrderOfItsOwnIsKept(t *testing.T) {
	host := newFake(func(string) (string, error) { return "alice\n" + rs + headers200 + rs + listJSON, nil })
	s, _ := serviceOn(host)
	query := "is:issue is:open label:\"good first issue\" sort:created-asc it's"
	list, err := s.List("gog", "acme/widgets", query, false)
	if err != nil {
		t.Fatal(err)
	}
	if list.Items[0].Number != 41 {
		t.Fatalf("a search with its own sort was reordered: %d first", list.Items[0].Number)
	}
	script := host.Scripts()[0]
	if !strings.Contains(script, `--search 'is:issue is:open label:"good first issue" sort:created-asc it'\''s'`) || !strings.Contains(script, "-R 'acme/widgets'") {
		t.Fatalf("the search is not one quoted word:\n%s", script)
	}
	for _, bad := range []string{"", "  ", "a\nb", strings.Repeat("x", MaxQuery+1)} {
		if _, err := s.List("gog", "acme/widgets", bad, false); !errors.Is(err, ErrInvalid) {
			t.Errorf("query %q: %v", bad, err)
		}
	}
	for _, bad := range []string{"acme", "acme/widgets/x", "-x/y", "acme/../etc", "acme/wid gets"} {
		if _, err := s.List("gog", bad, DefaultList, false); !errors.Is(err, ErrInvalid) {
			t.Errorf("repo %q: %v", bad, err)
		}
	}
}

func TestGhsTroubleIsSaidWithWhatToRun(t *testing.T) {
	cases := map[string]string{
		"gh: command not found\n": "no-gh",
		"alice\n" + rs + "To get started with GitHub CLI, please run:  gh auth login\n" + rs + "To get started with GitHub CLI, please run:  gh auth login\n": "signed-out",
		rs + "gh: Not Found (HTTP 404)\n" + rs + "GraphQL: Could not resolve to a Repository with the name 'acme/gone'. (repository)\n":                       "not-found",
		rs + headers200 + rs + "something else went wrong\n": "failed",
	}
	for out, kind := range cases {
		host := newFake(func(string) (string, error) { return out, &shell.ExitError{Code: 1} })
		s, _ := serviceOn(host)
		_, err := s.List("gog", "acme/widgets", DefaultList, false)
		var trouble *Trouble
		if !errors.As(err, &trouble) || trouble.Kind != kind || !strings.Contains(trouble.Text, "GOG") {
			t.Errorf("%q: %v, want %s", out, err, kind)
			continue
		}
		if (kind == "no-gh" || kind == "signed-out") && trouble.Run != "gh auth login" {
			t.Errorf("%s says nothing to run: %+v", kind, trouble)
		}
	}
}

func TestAnIssueIsReadWithItsTimelineAndKept(t *testing.T) {
	host := newFake(func(script string) (string, error) {
		if strings.Contains(script, "If-None-Match") {
			return rs + headers304 + rs, nil
		}
		return "alice\n" + rs + headers200 + "\n" + issueJSON + "\n" + rs + timelineJSON + "\n" + rs + closedByJSON, nil
	})
	s, clock := serviceOn(host)
	d, err := s.Issue("gog", "acme/widgets", 41, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Crash on start" || d.State != "open" || d.Role != "CONTRIBUTOR" || d.Milestone == nil || d.Milestone.Title != "v1.2" || d.Me != "alice" {
		t.Fatalf("the issue: %+v", d)
	}
	if len(d.Reactions) != 2 || d.Reactions[0] != (Reaction{"+1", 2}) || d.Reactions[1] != (Reaction{"heart", 1}) {
		t.Fatalf("reactions: %+v", d.Reactions)
	}
	if len(d.Pulls) != 1 || !d.Pulls[0].Draft || d.Pulls[0].Number != 43 {
		t.Fatalf("closing pull requests: %+v", d.Pulls)
	}
	kinds := []string{}
	for _, e := range d.Timeline {
		kinds = append(kinds, e.Kind)
	}
	if got := strings.Join(kinds, " "); got != "labels labels comment renamed assign mention commit closed reopened milestone" {
		t.Fatalf("timeline: %s", got)
	}
	burst := d.Timeline[0]
	if len(burst.Added) != 2 || len(burst.Removed) != 1 || burst.Actor.Login != "alice" || d.Timeline[1].Actor.Login != "bob" {
		t.Fatalf("a burst of one person's labels is not one line, or another's joined it: %+v", d.Timeline[:2])
	}
	comment := d.Timeline[2]
	if comment.Role != "MEMBER" || comment.Body != "I see it too." || len(comment.Reactions) != 1 || comment.Reactions[0].Content != "eyes" {
		t.Fatalf("comment: %+v", comment)
	}
	if r := d.Timeline[3]; r.From != "Crash" || r.To != "Crash on start" {
		t.Fatalf("rename: %+v", r)
	}
	if m := d.Timeline[5]; m.Ref == nil || !m.Ref.Pull || m.Ref.Repo != "acme/widgets" || m.Ref.Number != 43 {
		t.Fatalf("cross-reference: %+v", m)
	}
	if c := d.Timeline[6]; c.SHA == "" || c.URL != "https://github.com/acme/widgets/commit/0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("commit: %+v", c)
	}
	if c := d.Timeline[7]; c.Reason != "not_planned" || !c.Actor.Bot || c.Actor.Login != "dependabot" {
		t.Fatalf("closed: %+v", c)
	}
	names := []string{}
	for _, p := range d.Participants {
		names = append(names, p.Login)
	}
	if got := strings.Join(names, " "); got != "bob alice carol" {
		t.Fatalf("participants, people only, in order: %s", got)
	}

	*clock = clock.Add(5 * time.Minute)
	if again, err := s.Issue("gog", "acme/widgets", 41, false); err != nil || again.Source != FromProbe || len(again.Timeline) != len(d.Timeline) {
		t.Fatalf("a 304 did not keep the issue: %+v %v", again.Source, err)
	}
}

func TestLinkedPullRequestsFallBackToTheMentioningOnes(t *testing.T) {
	host := newFake(func(string) (string, error) {
		return rs + headers200 + "\n" + issueJSON + "\n" + rs + timelineJSON + "\n" + rs + closedByRefused, nil
	})
	s, _ := serviceOn(host)
	d, err := s.Issue("gog", "acme/widgets", 41, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pulls) != 1 || d.Pulls[0].State != "OPEN" || d.Pulls[0].URL != "https://github.com/acme/widgets/pull/43" {
		t.Fatalf("pulls: %+v", d.Pulls)
	}
}

func TestAPullRequestsNumberIsSaidSo(t *testing.T) {
	host := newFake(func(string) (string, error) {
		return rs + headers200 + "\n" + pullJSON + "\n" + rs + "[]" + rs + "{}", nil
	})
	s, _ := serviceOn(host)
	d, err := s.Issue("gog", "acme/widgets", 43, false)
	if err != nil || !d.Pull {
		t.Fatalf("a pull request read as an issue: %+v %v", d, err)
	}
}

func TestAnIssueGitHubDoesNotHaveIsTrouble(t *testing.T) {
	host := newFake(func(string) (string, error) {
		return rs + "HTTP/2.0 404 Not Found\n\n{\"message\": \"Not Found\"}\ngh: Not Found (HTTP 404)\n" + rs, &shell.ExitError{Code: 1}
	})
	s, _ := serviceOn(host)
	_, err := s.Issue("gog", "acme/widgets", 999, false)
	var trouble *Trouble
	if !errors.As(err, &trouble) || trouble.Kind != "not-found" {
		t.Fatalf("%v", err)
	}
}

func TestGitHubRepoReadsEverySpellingOfAnOrigin(t *testing.T) {
	for remote, want := range map[string]string{
		"git@github.com:acme/widgets.git":         "acme/widgets",
		"https://github.com/acme/widgets":         "acme/widgets",
		"https://user:tok@github.com/acme/w.git":  "acme/w",
		"ssh://git@github.com/acme/widgets.git":   "acme/widgets",
		"https://www.github.com/acme/widgets.git": "acme/widgets",
	} {
		if got, _, ok := GitHubRepo(remote); !ok || got != want {
			t.Errorf("%s: %q %v", remote, got, ok)
		}
	}
	if _, host, ok := GitHubRepo("git@gitlab.com:team/app.git"); ok || host != "gitlab.com" {
		t.Errorf("gitlab read as GitHub: %s %v", host, ok)
	}
	if _, _, ok := GitHubRepo(""); ok {
		t.Error("no origin read as GitHub")
	}
}
