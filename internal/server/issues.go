package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/ghissues"
	"github.com/dev1antdev/remuda/internal/rpath"
)

// A GitHub repository's issues: the machine's repositories by their checkouts' origins, the list a search finds, one
// issue with its timeline, and the writes the person presses — all through gh on the machine, as the person. Every route
// is the person's: a viewer link and an agent key are refused before anything runs.

func (s *Server) issueRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/issues/repos", s.handleIssueRepos)
	api.HandleFunc("GET /api/issues", s.handleIssues)
	api.HandleFunc("GET /api/issues/{number}", s.handleIssue)
	api.HandleFunc("POST /api/issues/{number}/comment", s.handleIssueWrite("comment"))
	api.HandleFunc("POST /api/issues/{number}/close", s.handleIssueWrite("close"))
	api.HandleFunc("POST /api/issues/{number}/reopen", s.handleIssueWrite("reopen"))
	api.HandleFunc("POST /api/issues/{number}/assign", s.handleIssueWrite("assign"))
}

// thePersons refuses a viewer link and an agent key: gh runs as the person on their machine.
func thePersons(w http.ResponseWriter, r *http.Request) bool {
	if auth.IsViewer(r.Context()) || auth.CallerName(r.Context()) != "" {
		httpErr(w, http.StatusForbidden, errors.New("the issues are the person's: gh runs as them on their machine"))
		return false
	}
	return true
}

// handleIssueRepos is GET /api/issues/repos?machine=&dir=: the machine's GitHub repositories among its workspaces'
// folders, and what the folder asked about is.
func (s *Server) handleIssueRepos(w http.ResponseWriter, r *http.Request) {
	if !thePersons(w, r) {
		return
	}
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	dir := r.URL.Query().Get("dir")
	if dir != "" && !rpath.For(mc.Home()).IsAbs(dir) {
		httpErr(w, 400, fmt.Errorf("the folder must be a whole path"))
		return
	}
	var dirs []string
	if snap, _ := s.Watch.Snapshot(mc.ID()); snap != nil {
		for i := range snap.Workspaces {
			ws := &snap.Workspaces[i]
			if (s.Scratch != nil && s.Scratch.Is(mc.ID(), ws.WorkspaceID)) || (s.Concierge != nil && s.Concierge.Is(mc.ID(), ws.WorkspaceID)) {
				continue
			}
			if d := snap.WorkspaceDir(ws); d != "" && !containsString(dirs, d) {
				dirs = append(dirs, d)
			}
		}
	}
	repos, err := s.Issues.Repos(mc.ID(), dirs, dir)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, repos)
}

// handleIssues is GET /api/issues?machine=&repo=&q=&fresh=1: the issues the search finds, kept a minute, confirmed by a
// conditional request for a quarter of an hour, read whole after that or when fresh is asked for.
func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	if !thePersons(w, r) {
		return
	}
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		query = ghissues.DefaultList
	}
	list, err := s.Issues.List(mc.ID(), q.Get("repo"), query, q.Get("fresh") == "1")
	issueAnswer(w, list, err)
}

// handleIssue is GET /api/issues/{number}?machine=&repo=&fresh=1: one issue, its timeline and its linked pull requests.
func (s *Server) handleIssue(w http.ResponseWriter, r *http.Request) {
	if !thePersons(w, r) {
		return
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || number <= 0 {
		httpErr(w, 400, fmt.Errorf("no issue %q", r.PathValue("number")))
		return
	}
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	q := r.URL.Query()
	detail, err := s.Issues.Issue(mc.ID(), q.Get("repo"), number, q.Get("fresh") == "1")
	issueAnswer(w, detail, err)
}

// issueAnswer writes a read: why gh cannot answer is an answer of its own, {trouble}, for the panel to say with what to run.
func issueAnswer(w http.ResponseWriter, v any, err error) {
	var trouble *ghissues.Trouble
	switch {
	case errors.Is(err, ghissues.ErrInvalid):
		httpErr(w, 400, err)
	case errors.As(err, &trouble):
		writeJSON(w, map[string]any{"trouble": trouble})
	case err != nil:
		httpErr(w, http.StatusBadGateway, err)
	default:
		writeJSON(w, v)
	}
}

// handleIssueWrite is POST /api/issues/{number}/{comment|close|reopen|assign} {machine, repo, body, reason, comment, me}:
// one write, at the person's press, through gh on the machine. gh's own words come back when it refuses.
func (s *Server) handleIssueWrite(what string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !thePersons(w, r) {
			return
		}
		var req struct {
			Machine string `json:"machine"`
			Repo    string `json:"repo"`
			Body    string `json:"body"`
			Reason  string `json:"reason"`
			Comment string `json:"comment"`
			Me      bool   `json:"me"`
		}
		if !decode(w, r, &req) {
			return
		}
		number, err := strconv.Atoi(r.PathValue("number"))
		if err != nil || number <= 0 {
			httpErr(w, 400, fmt.Errorf("no issue %q", r.PathValue("number")))
			return
		}
		mc, _, _, ok := s.ready(w, r, req.Machine)
		if !ok {
			return
		}
		switch what {
		case "comment":
			err = s.Issues.Comment(mc.ID(), req.Repo, number, req.Body)
		case "close":
			err = s.Issues.Close(mc.ID(), req.Repo, number, req.Reason, req.Comment)
		case "reopen":
			err = s.Issues.Reopen(mc.ID(), req.Repo, number, req.Comment)
		case "assign":
			err = s.Issues.Assign(mc.ID(), req.Repo, number, req.Me)
		}
		switch {
		case errors.Is(err, ghissues.ErrInvalid), errors.Is(err, ghissues.ErrEmpty):
			httpErr(w, 400, err)
		case err != nil:
			httpErr(w, http.StatusBadGateway, err)
		default:
			writeJSON(w, map[string]any{"ok": true})
		}
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
