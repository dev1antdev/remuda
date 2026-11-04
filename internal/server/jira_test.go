package server

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/jira"
	"github.com/dev1antdev/remuda/internal/tasks"
)

const jiraToken = "pat-0123456789abcdef"

// jiraSite is a Jira server answering a personal access token: one project, one issue.
func jiraSite(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+jiraToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/rest/api/2/myself":
			w.Write([]byte(`{"displayName":"Sergii"}`))
		case "/rest/api/2/project":
			w.Write([]byte(`[{"key":"CRMOPS"}]`))
		case "/rest/api/2/issue/CRMOPS-6279":
			w.Write([]byte(`{"key":"CRMOPS-6279","fields":{"summary":"Conflicts with the base branch","status":{"name":"In Review","statusCategory":{"key":"indeterminate"}},"issuetype":{"name":"Bug"},"labels":["backend"],"description":"Steps:\n* open the job"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The Jira routes: the site and token kept with the token never sent back, a test of them, an issue made a task.
func TestJiraRoutes(t *testing.T) {
	site := jiraSite(t)
	store, err := tasks.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s := &Server{Jira: jira.New(t.TempDir()), Tasks: store}
	mux := http.NewServeMux()
	s.jiraRoutes(mux)

	if code, out := taskCall(mux, "GET", "/api/jira", "", ""); code != 200 || out["site"] != "" || out["has_token"] != false {
		t.Fatalf("nothing set: %d %v", code, out)
	}
	if code, _ := taskCall(mux, "POST", "/api/jira/test", "", ""); code != http.StatusBadGateway {
		t.Fatalf("a test with nothing set: %d", code)
	}
	if code, _ := taskCall(mux, "PUT", "/api/jira", "", `{"site":"ftp://x","token":"t"}`); code != 400 {
		t.Fatalf("a bad site: %d", code)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/jira", strings.NewReader(`{"site":"`+site.URL+`","token":"`+jiraToken+`"}`)))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), jiraToken) || !strings.Contains(rec.Body.String(), `"has_token":true`) {
		t.Fatalf("set: %d %s", rec.Code, rec.Body)
	}
	if code, out := taskCall(mux, "POST", "/api/jira/test", "", ""); code != 200 || out["who"] != "Sergii" || out["projects"] != float64(1) {
		t.Fatalf("test: %d %v", code, out)
	}

	projects, _ := store.Projects()
	project := strconv.FormatInt(projects[0].ID, 10)
	code, out := taskCall(mux, "POST", "/api/jira/import", "", `{"project_id":`+project+`,"key":"`+site.URL+`/browse/crmops-6279"}`)
	if code != 200 || out["title"] != "CRMOPS-6279 Conflicts with the base branch" {
		t.Fatalf("import: %d %v", code, out)
	}
	description, _ := out["description"].(string)
	if !strings.Contains(description, `href="`+site.URL+`/browse/CRMOPS-6279"`) || !strings.Contains(description, "CRMOPS-6279 · Bug · In Review") || !strings.Contains(description, "<li>open the job</li>") {
		t.Fatalf("description %q", description)
	}
	if code, _ := taskCall(mux, "POST", "/api/jira/import", "", `{"project_id":`+project+`,"key":"CRMOPS-6279"}`); code != 400 {
		t.Fatalf("the same issue twice: %d", code)
	}
	if code, _ := taskCall(mux, "POST", "/api/jira/import", "", `{"project_id":`+project+`,"key":"no key"}`); code != 400 {
		t.Fatalf("not a key: %d", code)
	}
	if code, _ := taskCall(mux, "POST", "/api/jira/import", "", `{"project_id":`+project+`,"key":"CRMOPS-1"}`); code != 404 {
		t.Fatalf("an unknown issue: %d", code)
	}

	if code, out := taskCall(mux, "DELETE", "/api/jira", "", ""); code != 200 || out["has_token"] != false {
		t.Fatalf("remove: %d %v", code, out)
	}
}

// A workspace's chips: the keys its branch names, read from the site once a page looked, in the branch's order.
func TestTheRosterNamesTheIssuesOfAWorkspacesBranch(t *testing.T) {
	site := jiraSite(t)
	s := &Server{Jira: jira.New(t.TempDir())}
	session := rosterSession{Branch: "feature/crmops-6279-conflicts"}
	if got := s.issuesOf("local", session); got != nil {
		t.Fatalf("issues with no site set: %v", got)
	}
	if err := s.Jira.Replace(site.URL, "", jiraToken); err != nil {
		t.Fatal(err)
	}
	var got []jira.Issue
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && len(got) == 0; time.Sleep(10 * time.Millisecond) {
		got = s.issuesOf("local", session)
		s.Jira.Touch()
	}
	if len(got) != 1 || got[0].Key != "CRMOPS-6279" || got[0].Status != "In Review" {
		t.Fatalf("issues %+v", got)
	}
	if (&Server{}).issuesOf("local", session) != nil {
		t.Fatal("a server without Jira named issues")
	}
}
