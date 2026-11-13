package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dev1antdev/remuda/internal/ghpr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/rpath"
)

// The pull request panel: the checkout's and the repository's pull requests, one in full, its jobs, logs and files —
// read with gh on the machine — and what the person writes on GitHub at a press. Every route is the person's alone: a
// viewer link and an agent's key reach none (internal/auth). A gh that is not installed or not signed in answers a read
// with {problem}, which the panel shows in place of everything.

func (s *Server) ghprRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/pr/branch", s.handlePRBranch)
	api.HandleFunc("GET /api/pr/search", s.handlePRSearch)
	api.HandleFunc("GET /api/pr/detail", s.handlePRDetail)
	api.HandleFunc("GET /api/pr/job", s.handlePRJob)
	api.HandleFunc("GET /api/pr/log", s.handlePRLog)
	api.HandleFunc("GET /api/pr/files", s.handlePRFiles)
	api.HandleFunc("POST /api/pr/comment", s.handlePRComment)
	api.HandleFunc("POST /api/pr/reply", s.handlePRReply)
	api.HandleFunc("POST /api/pr/resolve", s.handlePRResolve)
	api.HandleFunc("POST /api/pr/ask-again", s.handlePRAskAgain)
	api.HandleFunc("POST /api/pr/state", s.handlePRState)
}

// prRead writes a read's answer: a Problem as {problem}, any other failure as gh's words.
func prRead(w http.ResponseWriter, v any, err error) {
	if p, ok := ghpr.AsProblem(err); ok {
		writeJSON(w, map[string]any{"problem": p})
		return
	}
	if errors.Is(err, ghpr.ErrNotFound) {
		httpErr(w, 404, err)
		return
	}
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, v)
}

// prWrite answers a write: ok, or why not — a newer head a 409, gh's refusal a 502.
func prWrite(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		writeJSON(w, map[string]any{"ok": true})
	case errors.Is(err, ghpr.ErrNewerHead):
		httpErr(w, http.StatusConflict, err)
	default:
		var failed *ghpr.Failed
		if _, ok := ghpr.AsProblem(err); ok || errors.As(err, &failed) {
			httpErr(w, http.StatusBadGateway, err)
			return
		}
		httpErr(w, 400, err)
	}
}

// prMachine is the request's machine once it answers, or why not.
func (s *Server) prMachine(w http.ResponseWriter, r *http.Request, bodyID string) (*machines.Machine, bool) {
	mc, _, _, ok := s.ready(w, r, bodyID)
	return mc, ok
}

// prTarget reads ?repo= and ?number= (number when asked).
func prTarget(w http.ResponseWriter, r *http.Request, number bool) (ghpr.Repo, int, bool) {
	repo, err := ghpr.ParseRepo(r.URL.Query().Get("repo"))
	if err != nil {
		httpErr(w, 400, err)
		return ghpr.Repo{}, 0, false
	}
	if !number {
		return repo, 0, true
	}
	n, err := strconv.Atoi(r.URL.Query().Get("number"))
	if err != nil || n < 1 {
		httpErr(w, 400, fmt.Errorf("a pull request's number is needed"))
		return ghpr.Repo{}, 0, false
	}
	return repo, n, true
}

// handlePRBranch is GET ?machine&dir: the checkout's branch and remotes, and the branch's pull requests.
func (s *Server) handlePRBranch(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.prMachine(w, r, "")
	if !ok {
		return
	}
	dir := r.URL.Query().Get("dir")
	if !rpath.For(mc.Home()).IsAbs(dir) {
		httpErr(w, 400, fmt.Errorf("an absolute folder is needed"))
		return
	}
	checkout, err := ghpr.ReadCheckout(mc, dir)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	cards, err := ghpr.BranchPRs(mc, checkout)
	prRead(w, map[string]any{"checkout": checkout, "items": cards}, err)
}

// handlePRSearch is GET ?machine&repo&q&scope&state[&more=1&cursors={…}]: a page of the repository's pull requests.
func (s *Server) handlePRSearch(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.prMachine(w, r, "")
	if !ok {
		return
	}
	repo, _, ok := prTarget(w, r, false)
	if !ok {
		return
	}
	q := r.URL.Query()
	search := ghpr.Search{Text: q.Get("q"), Scope: q.Get("scope"), State: q.Get("state"), More: q.Get("more") == "1"}
	if raw := q.Get("cursors"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &search.Cursors); err != nil {
			httpErr(w, 400, fmt.Errorf("bad cursors: %v", err))
			return
		}
	}
	found, err := ghpr.SearchPRs(mc, repo, search)
	prRead(w, found, err)
}

// handlePRDetail is GET ?machine&repo&number: one pull request in full.
func (s *Server) handlePRDetail(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.prMachine(w, r, "")
	if !ok {
		return
	}
	repo, n, ok := prTarget(w, r, true)
	if !ok {
		return
	}
	d, err := ghpr.ReadDetail(mc, repo, n)
	prRead(w, d, err)
}

func jobOf(w http.ResponseWriter, r *http.Request) (int64, bool) {
	job, err := strconv.ParseInt(r.URL.Query().Get("job"), 10, 64)
	if err != nil || job < 1 {
		httpErr(w, 400, fmt.Errorf("a job's id is needed"))
		return 0, false
	}
	return job, true
}

// handlePRJob is GET ?machine&repo&job: an Actions job and its steps.
func (s *Server) handlePRJob(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.prMachine(w, r, "")
	if !ok {
		return
	}
	repo, _, ok := prTarget(w, r, false)
	if !ok {
		return
	}
	job, ok := jobOf(w, r)
	if !ok {
		return
	}
	got, err := ghpr.ReadJob(mc, repo, job)
	prRead(w, got, err)
}

// handlePRLog is GET ?machine&repo&job: the end of a job's failed steps' output.
func (s *Server) handlePRLog(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.prMachine(w, r, "")
	if !ok {
		return
	}
	repo, _, ok := prTarget(w, r, false)
	if !ok {
		return
	}
	job, ok := jobOf(w, r)
	if !ok {
		return
	}
	got, err := ghpr.ReadLog(mc, repo, job)
	prRead(w, got, err)
}

// handlePRFiles is GET ?machine&repo&number: the files the pull request changes, with their patches.
func (s *Server) handlePRFiles(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.prMachine(w, r, "")
	if !ok {
		return
	}
	repo, n, ok := prTarget(w, r, true)
	if !ok {
		return
	}
	got, err := ghpr.ReadFiles(mc, repo, n)
	prRead(w, got, err)
}

// prBody is what the writes take.
type prBody struct {
	Machine  string `json:"machine"`
	Repo     string `json:"repo"`
	Number   int    `json:"number"`
	Thread   string `json:"thread"`
	Body     string `json:"body"`
	Resolved bool   `json:"resolved"`
	Reviewer string `json:"reviewer"`
	Action   string `json:"action"`
	Head     string `json:"head"`
	ID       string `json:"id"`
	Method   string `json:"method"`
	Subject  string `json:"subject"`
}

// prWriteBody reads a write's body and its machine; repo, when asked, must be a repository and the number a number.
func (s *Server) prWriteBody(w http.ResponseWriter, r *http.Request, target bool) (*machines.Machine, prBody, ghpr.Repo, bool) {
	var req prBody
	if !decode(w, r, &req) {
		return nil, req, ghpr.Repo{}, false
	}
	mc, ok := s.prMachine(w, r, req.Machine)
	if !ok {
		return nil, req, ghpr.Repo{}, false
	}
	if !target {
		return mc, req, ghpr.Repo{}, true
	}
	repo, err := ghpr.ParseRepo(req.Repo)
	if err == nil && req.Number < 1 {
		err = fmt.Errorf("a pull request's number is needed")
	}
	if err != nil {
		httpErr(w, 400, err)
		return nil, req, ghpr.Repo{}, false
	}
	return mc, req, repo, true
}

// handlePRComment is POST {machine, repo, number, body}: a comment on the conversation.
func (s *Server) handlePRComment(w http.ResponseWriter, r *http.Request) {
	if mc, req, repo, ok := s.prWriteBody(w, r, true); ok {
		prWrite(w, ghpr.PostComment(mc, repo, req.Number, req.Body))
	}
}

// handlePRReply is POST {machine, thread, body}: a reply in a thread on a line.
func (s *Server) handlePRReply(w http.ResponseWriter, r *http.Request) {
	if mc, req, _, ok := s.prWriteBody(w, r, false); ok {
		prWrite(w, ghpr.Reply(mc, req.Thread, req.Body))
	}
}

// handlePRResolve is POST {machine, thread, resolved}: a thread resolved, or opened again.
func (s *Server) handlePRResolve(w http.ResponseWriter, r *http.Request) {
	if mc, req, _, ok := s.prWriteBody(w, r, false); ok {
		prWrite(w, ghpr.Resolve(mc, req.Thread, req.Resolved))
	}
}

// handlePRAskAgain is POST {machine, repo, number, reviewer}: a reviewer asked for another review.
func (s *Server) handlePRAskAgain(w http.ResponseWriter, r *http.Request) {
	if mc, req, repo, ok := s.prWriteBody(w, r, true); ok {
		prWrite(w, ghpr.AskAgain(mc, repo, req.Number, req.Reviewer))
	}
}

// handlePRState is POST {machine, repo, number, action, head, id, method, subject, body}: merged (naming the head the
// person saw), merged when ready, that cancelled, closed or reopened. The page asks first, naming what still stands in
// the way.
func (s *Server) handlePRState(w http.ResponseWriter, r *http.Request) {
	if mc, req, repo, ok := s.prWriteBody(w, r, true); ok {
		change := ghpr.Change{Action: req.Action, Head: req.Head, ID: req.ID, Method: req.Method, Subject: req.Subject, Body: req.Body}
		prWrite(w, ghpr.ChangeState(mc, repo, req.Number, change))
	}
}
