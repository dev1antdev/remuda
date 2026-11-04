package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const token = "s3cr3t-token-value"

func TestTheConfigurationKeepsItsTokenAndNeverShowsIt(t *testing.T) {
	dir := t.TempDir()
	s := Load(dir)
	if s.Configured() {
		t.Fatal("configured from nothing")
	}
	if err := s.Replace("acme.atlassian.net/", "ann@acme.io", token); err != nil {
		t.Fatal(err)
	}
	view := s.View()
	if view.Site != "https://acme.atlassian.net" || !view.HasToken || !view.Cloud || view.Account != "ann@acme.io" {
		t.Fatalf("view %+v", view)
	}
	shown, _ := json.Marshal(view)
	if strings.Contains(string(shown), token) {
		t.Fatalf("the token is in the view: %s", shown)
	}
	if err := s.Replace("https://acme.atlassian.net", "ann@acme.io", ""); err != nil || s.Config().Token != token {
		t.Fatalf("an empty token did not keep the one kept: %v %q", err, s.Config().Token)
	}
	for _, other := range []string{"https://evil.example", "https://acme.atlassian.net.evil.example", "acme2.atlassian.net"} {
		if err := s.Replace(other, "ann@acme.io", ""); err == nil || s.Config().Site != "https://acme.atlassian.net" {
			t.Fatalf("the token kept for acme.atlassian.net went with %s: %v %+v", other, err, s.View())
		}
	}
	info, err := os.Stat(filepath.Join(dir, configFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file %v %v", info, err)
	}
	if again := Load(dir); again.Config() != s.Config() {
		t.Fatalf("read back %+v", again.Config())
	}
	for _, bad := range []struct{ site, account string }{
		{"ftp://acme", "a"}, {"", "a"}, {"https://acme.atlassian.net/?x=1", "a"}, {"http://acme.atlassian.net", "a"}, {"https://acme.atlassian.net", ""},
	} {
		if err := s.Replace(bad.site, bad.account, "t"); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if err := s.Replace("https://jira.acme.local/jira/", "", "pat"); err != nil || s.View().Site != "https://jira.acme.local/jira" || s.View().Cloud {
		t.Fatalf("a server under a path: %v %+v", err, s.View())
	}
	if err := s.Replace("http://jira.acme.local/jira", "", ""); err == nil || s.View().Site != "https://jira.acme.local/jira" {
		t.Fatalf("the token kept for https went over plain http: %v", err)
	}
	if err := s.Replace("https://JIRA.acme.local/", "", ""); err != nil || s.Config().Token != "pat" {
		t.Fatalf("the same host under another path lost its token: %v", err)
	}
	if err := s.Clear(); err != nil || s.Configured() {
		t.Fatalf("clear: %v", err)
	}
	if err := Load(t.TempDir()).Replace("https://x.atlassian.net", "a@b", ""); err == nil {
		t.Fatal("a first configuration without a token was taken")
	}
}

func TestKeysAreReadFromWordsAndBranches(t *testing.T) {
	got := KeysIn([]string{"Fixed CRMOPS-6279 and the utf-8 path, see MSN-12 and ISO-8601.", "CRMOPS-6279 again, ABC-0 is none"})
	if want := []string{"CRMOPS-6279", "MSN-12", "ISO-8601"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("keys %v", got)
	}
	if got := BranchKeys("feature/crmops-6279-conflicts"); !reflect.DeepEqual(got, []string{"CRMOPS-6279"}) {
		t.Fatalf("branch %v", got)
	}
	for typed, want := range map[string]string{
		"CRMOPS-6279": "CRMOPS-6279", " crmops-6279 ": "CRMOPS-6279",
		"https://roofr.atlassian.net/browse/CRMOPS-6279":                                   "CRMOPS-6279",
		"https://roofr.atlassian.net/jira/software/projects/X/boards/1?selectedIssue=XY-9": "XY-9",
	} {
		if key, ok := KeyOf(typed); !ok || key != want {
			t.Errorf("KeyOf(%q) = %q %v", typed, key, ok)
		}
	}
	for _, bad := range []string{"", "6279", "CRMOPS", "CRMOPS-0", "https://example.com"} {
		if _, ok := KeyOf(bad); ok {
			t.Errorf("KeyOf(%q) read a key", bad)
		}
	}
	if got := Merge([]string{"A-1"}, []string{"B-2", "A-1", "C-3", "D-4"}); !reflect.DeepEqual(got, []string{"A-1", "B-2", "C-3"}) {
		t.Fatalf("merge %v", got)
	}
	if Project("CRMOPS-6279") != "CRMOPS" {
		t.Fatal("project")
	}
}

const serverIssue = `{"key":"CRMOPS-6279","fields":{"summary":"Conflicts with the base branch","status":{"name":"In Review","statusCategory":{"key":"indeterminate"}},"assignee":{"displayName":"Sergii"},"reporter":{"displayName":"Ann"},"issuetype":{"name":"Bug"},"labels":["backend"],"description":"h2. Steps\n# open the job\n# *save* it\n{code:go}\nfmt.Println(1)\n{code}\nSee [the doc|https://x.io/d] and {{flag}}."}}`

func TestAServerIsAskedOverV2WithItsPersonalAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/jira/rest/api/2/issue/CRMOPS-6279":
			w.Write([]byte(serverIssue))
		case "/jira/rest/api/2/project":
			w.Write([]byte(`[{"key":"CRMOPS"},{"key":"MSN"}]`))
		case "/jira/rest/api/2/myself":
			w.Write([]byte(`{"name":"sergii","displayName":"Sergii S"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewClient(Config{Site: srv.URL + "/jira", Token: token}, nil)
	ctx := context.Background()
	issue, err := c.Issue(ctx, "CRMOPS-6279")
	if err != nil {
		t.Fatal(err)
	}
	want := Issue{Key: "CRMOPS-6279", Summary: "Conflicts with the base branch", Status: "In Review", Category: "indeterminate", Assignee: "Sergii", URL: srv.URL + "/jira/browse/CRMOPS-6279"}
	if issue != want {
		t.Fatalf("issue %+v", issue)
	}
	full, err := c.Full(ctx, "CRMOPS-6279")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"## Steps", "1. open the job", "1. **save** it", "```go\nfmt.Println(1)\n```", "[the doc](https://x.io/d)", "`flag`"} {
		if !strings.Contains(full.Description, part) {
			t.Errorf("the description lacks %q:\n%s", part, full.Description)
		}
	}
	if full.Type != "Bug" || full.Reporter != "Ann" || !reflect.DeepEqual(full.Labels, []string{"backend"}) {
		t.Fatalf("full %+v", full)
	}
	projects, err := c.Projects(ctx)
	if err != nil || !reflect.DeepEqual(projects, []string{"CRMOPS", "MSN"}) {
		t.Fatalf("projects %v %v", projects, err)
	}
	if who, err := c.Myself(ctx); err != nil || who != "Sergii S" {
		t.Fatalf("myself %q %v", who, err)
	}
	if _, err := c.Issue(ctx, "CRMOPS-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing issue: %v", err)
	}
	wrong := NewClient(Config{Site: srv.URL + "/jira", Token: "nope"}, nil)
	if _, err := wrong.Issue(ctx, "CRMOPS-6279"); !errors.Is(err, ErrRefused) {
		t.Fatalf("a refused token: %v", err)
	}
}

// cloudSite sends a *.atlassian.net site's requests to a test server.
func cloudSite(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: rewrite{target: target}}
	return srv, client
}

type rewrite struct{ target *url.URL }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.URL.Scheme, out.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(out)
}

const cloudIssue = `{"key":"MSN-2459","fields":{"summary":"Moderation history","status":{"name":"Done","statusCategory":{"key":"done"}},"assignee":null,"reporter":{"displayName":"Bo"},"issuetype":{"name":"Story"},"labels":[],
"description":{"type":"doc","version":1,"content":[
 {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Goal"}]},
 {"type":"paragraph","content":[{"type":"text","text":"Show "},{"type":"text","text":"every","marks":[{"type":"strong"}]},{"type":"text","text":" change, see "},{"type":"text","text":"spec","marks":[{"type":"link","attrs":{"href":"https://x.io/s"}}]},{"type":"hardBreak"},{"type":"mention","attrs":{"text":"@Ann"}}]},
 {"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}]},
 {"type":"codeBlock","attrs":{"language":"ts"},"content":[{"type":"text","text":"const a = 1"}]},
 {"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"quoted"}]}]},
 {"type":"rule"},
 {"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]}
]}}}`

func TestCloudIsAskedOverV3WithTheAccountsEmailAndAPageAtATime(t *testing.T) {
	var pages int32
	_, client := cloudSite(t, func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "ann@acme.io" || pass != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/rest/api/3/project/search":
			atomic.AddInt32(&pages, 1)
			if r.URL.Query().Get("startAt") == "0" {
				w.Write([]byte(`{"values":[{"key":"MSN"},{"key":"CRMOPS"}],"isLast":false}`))
			} else {
				w.Write([]byte(`{"values":[{"key":"GOG"}],"isLast":true}`))
			}
		case "/rest/api/3/issue/MSN-2459":
			w.Write([]byte(cloudIssue))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	c := NewClient(Config{Site: "https://acme.atlassian.net", Account: "ann@acme.io", Token: token}, client)
	ctx := context.Background()
	projects, err := c.Projects(ctx)
	if err != nil || !reflect.DeepEqual(projects, []string{"MSN", "CRMOPS", "GOG"}) || pages != 2 {
		t.Fatalf("projects %v %v after %d pages", projects, err, pages)
	}
	full, err := c.Full(ctx, "MSN-2459")
	if err != nil {
		t.Fatal(err)
	}
	if full.Status != "Done" || full.Category != "done" || full.Assignee != "" || full.URL != "https://acme.atlassian.net/browse/MSN-2459" {
		t.Fatalf("issue %+v", full.Issue)
	}
	for _, part := range []string{"## Goal", "Show **every** change, see [spec](https://x.io/s)\n@Ann", "- one\n- two", "```ts\nconst a = 1\n```", "> quoted", "---", "a · b"} {
		if !strings.Contains(full.Description, part) {
			t.Errorf("the description lacks %q:\n%s", part, full.Description)
		}
	}
}

func TestAnErrorNamesNeitherTheTokenNorTheAddress(t *testing.T) {
	c := NewClient(Config{Site: "http://127.0.0.1:1/secret-path", Token: token}, &http.Client{Timeout: time.Second})
	_, err := c.Issue(context.Background(), "A-1")
	if err == nil || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "secret-path") {
		t.Fatalf("error %v", err)
	}
	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer refused.Close()
	_, err = NewClient(Config{Site: refused.URL, Token: token}, nil).Issue(context.Background(), "A-1")
	if !errors.Is(err, ErrRefused) || strings.Contains(err.Error(), token) {
		t.Fatalf("refused %v", err)
	}
}

func TestWikiAndDocumentsBecomeMarkdown(t *testing.T) {
	got := WikiMarkdown("h1. Title\n* one\n** two\nbq. quoted\n----\n{noformat}raw *x*{noformat}\n{color:red}red{color} _it_ and *b*")
	for _, part := range []string{"# Title", "- one", "  - two", "> quoted", "---", "```\nraw *x*\n```", "red *it* and **b**"} {
		if !strings.Contains(got, part) {
			t.Errorf("wiki lacks %q:\n%s", part, got)
		}
	}
	if descriptionMarkdown(json.RawMessage(`null`)) != "" || descriptionMarkdown(nil) != "" || descriptionMarkdown(json.RawMessage(`42`)) != "" {
		t.Fatal("no description read as one")
	}
}

// fakeSite is a server site that counts what it was asked.
type fakeSite struct {
	mu     sync.Mutex
	asked  []string
	status string
	broken bool
}

func (f *fakeSite) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.asked = append(f.asked, r.URL.Path)
	status, broken := f.status, f.broken
	f.mu.Unlock()
	if broken {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch r.URL.Path {
	case "/rest/api/2/project":
		w.Write([]byte(`[{"key":"CRMOPS"}]`))
	case "/rest/api/2/issue/CRMOPS-6279":
		w.Write([]byte(`{"key":"CRMOPS-6279","fields":{"summary":"S","status":{"name":"` + status + `","statusCategory":{"key":"indeterminate"}}}}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeSite) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.asked)
}

func settle(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		reading := s.reading
		s.mu.Unlock()
		if !reading {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the sweep never ended")
}

func TestTheServiceReadsOnlyWhatALookedAtRosterNamesAndOnlyWhenDue(t *testing.T) {
	site := &fakeSite{status: "In Review"}
	srv := httptest.NewServer(http.HandlerFunc(site.handler))
	defer srv.Close()
	clock := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	s := New(t.TempDir())
	s.now = func() time.Time { return clock }
	changes := int32(0)
	s.OnChange = func() { atomic.AddInt32(&changes, 1) }

	s.Touch()
	if s.Issues([]string{"CRMOPS-6279"}) != nil || site.count() != 0 {
		t.Fatal("asked a site nobody set")
	}
	if err := s.Replace(srv.URL, "", token); err != nil {
		t.Fatal(err)
	}
	if got := s.Issues([]string{"CRMOPS-6279", "UTF-8"}); got != nil {
		t.Fatalf("issues before any read: %v", got)
	}
	s.Touch()
	settle(t, s)
	got := s.Issues([]string{"CRMOPS-6279", "UTF-8"})
	if len(got) != 1 || got[0].Status != "In Review" || got[0].URL != srv.URL+"/browse/CRMOPS-6279" {
		t.Fatalf("issues %+v", got)
	}
	if atomic.LoadInt32(&changes) == 0 {
		t.Fatal("OnChange was not told")
	}
	asked := site.count()
	for _, path := range site.asked {
		if strings.Contains(path, "UTF-8") {
			t.Fatal("a key of a project the site does not know was read")
		}
	}

	s.Touch()
	settle(t, s)
	if site.count() != asked {
		t.Fatal("read again before Every")
	}
	site.mu.Lock()
	site.status = "Done"
	site.mu.Unlock()
	clock = clock.Add(Every)
	s.Issues([]string{"CRMOPS-6279"})
	s.Touch()
	settle(t, s)
	if got := s.Issues([]string{"CRMOPS-6279"}); got[0].Status != "Done" {
		t.Fatalf("not read again after Every: %+v", got)
	}

	site.mu.Lock()
	site.broken = true
	site.mu.Unlock()
	clock = clock.Add(Every)
	s.Touch()
	settle(t, s)
	failed := site.count()
	s.Touch()
	settle(t, s)
	if site.count() != failed {
		t.Fatal("asked a failing site again at once")
	}

	clock = clock.Add(WantedFor + Every)
	site.mu.Lock()
	site.broken = false
	site.mu.Unlock()
	before := site.count()
	s.Touch()
	settle(t, s)
	for _, path := range site.asked[before:] {
		if strings.Contains(path, "/issue/") {
			t.Fatal("read a key no roster named for WantedFor")
		}
	}
	s.mu.Lock()
	kept := len(s.issues)
	s.mu.Unlock()
	if kept != 0 {
		t.Fatalf("%d issues no roster named for WantedFor are still kept", kept)
	}

	if err := s.Clear(); err != nil || s.Issues([]string{"CRMOPS-6279"}) != nil || s.Site() != "" {
		t.Fatal("the issues outlived the configuration")
	}
}
