package ghissues

import (
	"errors"
	"strings"
	"testing"
)

// A read cut short — the script's timeout on a long timeline or a big list, a page gh could not fetch half way — is said
// as that, never read off the issues' own words: an issue about gh's sign-in or a page "not found" is not gh's trouble.

var timedOut = errors.New("timed out after 45s")

func TestAnIssueCutShortIsSaidAsCutShortNotAsItsOwnWords(t *testing.T) {
	aboutSignIn := strings.Replace(issueJSON, `"It crashes. See #40."`, `"It crashes right after gh auth login, page not found (HTTP 404)."`, 1)
	host := newFake(func(string) (string, error) {
		return "alice\n" + rs + headers200 + "\n" + aboutSignIn + "\n" + rs + `[{"event": "commented", "body": "same here`, timedOut
	})
	s, _ := serviceOn(host)
	_, err := s.Issue("gog", "acme/widgets", 41, false)
	var trouble *Trouble
	if !errors.As(err, &trouble) || trouble.Kind != "failed" || !strings.Contains(trouble.Text, "timed out after 45s") {
		t.Fatalf("an issue whose read ran out of time: %+v", err)
	}
}

func TestAListCutShortIsSaidAsCutShortNotAsItsTitles(t *testing.T) {
	host := newFake(func(string) (string, error) {
		return "alice\n" + rs + headers200 + rs + `[{"number": 7, "title": "gh auth login fails behind a proxy", "author": {"login": "bob"}, "state": "OPEN"}, {"number": 6, "title": "Not found`, timedOut
	})
	s, _ := serviceOn(host)
	_, err := s.List("gog", "acme/widgets", DefaultList, false)
	var trouble *Trouble
	if !errors.As(err, &trouble) || trouble.Kind != "failed" || !strings.Contains(trouble.Text, "timed out after 45s") {
		t.Fatalf("a list whose read ran out of time: %+v", err)
	}
}

func TestATimelineCutHalfWayKeepsItsPagesAndSaysSo(t *testing.T) {
	firstPage := timelineJSON[:strings.Index(timelineJSON, "][")+1]
	host := newFake(func(string) (string, error) {
		return "alice\n" + rs + headers200 + "\n" + issueJSON + "\n" + rs + firstPage + "\ngh: HTTP 502: Server Error (https://api.github.com/repositories/1/issues/41/timeline?page=2)\n" + rs + closedByJSON, nil
	})
	s, _ := serviceOn(host)
	d, err := s.Issue("gog", "acme/widgets", 41, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Timeline) == 0 || d.Timeline[2].Kind != "comment" || !d.Cut {
		t.Fatalf("a timeline whose second page failed showed %d events, cut %v: an issue with comments drawn with none", len(d.Timeline), d.Cut)
	}
}
