package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/repos"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// The New worktree dialog's routes, after Orca's: what a repository offers a new worktree (its branches and
// checkouts, origin's issues and pull requests through gh on its machine, a number looked up), a fetch from origin at
// the person's press, and the create itself — herdr's worktree.create around git's fetch and upstream, the setup script,
// the agent. The person's only: neither a viewer link nor an agent's key reaches them.

func (s *Server) worktreeRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/worktrees/source", s.handleWorktreeSource)
	api.HandleFunc("GET /api/worktrees/github", s.handleWorktreeGitHub)
	api.HandleFunc("GET /api/worktrees/lookup", s.handleWorktreeLookup)
	api.HandleFunc("POST /api/worktrees/fetch", s.handleWorktreeFetch)
	api.HandleFunc("GET /api/worktrees/{id}/removal", s.handleWorktreeRemoval)
	api.HandleFunc("POST /api/worktrees/{id}/remove", s.handleWorktreeRemoveAsShown)
}

// handleWorktreeSource is GET /api/worktrees/source?machine&dir: the repository the folder is in, read on its machine,
// with the project's settings and the machine's home for the dialog's ~ paths.
func (s *Server) handleWorktreeSource(w http.ResponseWriter, r *http.Request) {
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	src, err := repos.Read(mc, r.URL.Query().Get("dir"))
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{
		"source":   src,
		"settings": s.repoSettings(mc.ID(), r.URL.Query().Get("dir")), // by the folder the sidebar names the repository by, as the run button and the settings read it,
		"home":     mc.Home(),
		"root":     worktrees.Root(mc.Home()),
		"setup":    !mc.Windows(), // a Windows PC's panes run PowerShell: no setup script is typed there
	})
}

func (s *Server) repoSettings(machineID, root string) repos.Settings {
	if s.Repos == nil {
		return repos.Settings{}
	}
	return s.Repos.Get(machineID, root)
}

// handleWorktreeGitHub is GET /api/worktrees/github?machine&repo: the open issues and pull requests, or the sentence
// that says why there are none to show.
func (s *Server) handleWorktreeGitHub(w http.ResponseWriter, r *http.Request) {
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	lists, err := repos.ListOpen(mc, r.URL.Query().Get("repo"))
	if err != nil {
		writeJSON(w, map[string]any{"issues": []repos.Issue{}, "prs": []repos.PR{}, "trouble": ghTrouble(mc, err)})
		return
	}
	writeJSON(w, lists)
}

// ghTrouble is why gh could not read GitHub on the machine, as a sentence.
func ghTrouble(mc *machines.Machine, err error) string {
	if errors.Is(err, repos.ErrNoGH) {
		return fmt.Sprintf("gh is not installed on %s: install it and sign in (gh auth login) to start from an issue or a pull request.", mc.Label())
	}
	return fmt.Sprintf("gh on %s answered: %s", mc.Label(), err)
}

// handleWorktreeLookup is GET /api/worktrees/lookup?machine&repo&number: a number neither list holds, asked of GitHub.
func (s *Server) handleWorktreeLookup(w http.ResponseWriter, r *http.Request) {
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	number, _ := strconv.Atoi(r.URL.Query().Get("number"))
	found, err := repos.Lookup(mc, r.URL.Query().Get("repo"), number)
	if err != nil {
		httpErr(w, http.StatusBadGateway, errors.New(ghTrouble(mc, err)))
		return
	}
	writeJSON(w, found)
}

// handleWorktreeFetch is POST /api/worktrees/fetch {machine, root}: git fetch --prune origin, at the person's press,
// never while an agent works or asks in that checkout.
func (s *Server) handleWorktreeFetch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Root    string `json:"root"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	if busy := s.busyIn(mc.ID(), req.Root); busy != "" {
		httpErr(w, http.StatusConflict, fmt.Errorf("%s: fetch once it rests", busy))
		return
	}
	if err := repos.Fetch(mc, req.Root); err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// busyIn says, as the start of a sentence, why git must not write in the folder now: an agent there works or asks, or
// its turn is still ending; "" when nothing stands in the way.
func (s *Server) busyIn(machineID, folder string) string {
	if s.Undo == nil || folder == "" {
		return ""
	}
	if s.Undo.Working != nil {
		if agent := s.Undo.Working(machineID, folder); agent != "" {
			return agent + " works or asks there"
		}
	}
	if s.Undo.Tracker != nil && s.Undo.Tracker.Going(machineID, folder) {
		return "an agent's turn is still ending there"
	}
	return ""
}

// newWorktreeReq is the New worktree dialog's Create: the branch and what it was picked from, the repository, whether
// to run its setup script, and the agent as every create route takes it.
type newWorktreeReq struct {
	createReq
	Root  string         `json:"root"`
	Pick  worktrees.Pick `json:"pick"`
	Setup string         `json:"setup"` // run | skip; "" leaves it to the project's settings
}

// handleNewWorktree is POST /api/worktrees: with a pick, the dialog's create (newWorktree); without one, the plain
// worktree.create of a branch on a base, as the API has always taken it.
func (s *Server) handleNewWorktree(w http.ResponseWriter, r *http.Request) {
	var req newWorktreeReq
	if !decode(w, r, &req) {
		return
	}
	if req.Pick.Kind == "" {
		s.createWith(w, r, &req.createReq, plainWorktree)
		return
	}
	s.newWorktree(w, r, &req)
}

func (s *Server) newWorktree(w http.ResponseWriter, r *http.Request, req *newWorktreeReq) {
	if !sessions.KnownAgent(req.Agent) {
		httpErr(w, 400, fmt.Errorf("unknown agent %q", req.Agent))
		return
	}
	start, err := req.start()
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	if req.Workspace == "" {
		httpErr(w, 400, errors.New("a workspace of the repository is needed to branch off"))
		return
	}
	src, err := repos.Read(mc, req.Root)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	settings := s.repoSettings(mc.ID(), req.Root)
	base := strings.TrimSpace(req.Base)
	if base == "" {
		base = settings.Base
	}
	plan, err := worktrees.MakePlan(src, req.Branch, base, req.Pick)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	setup := worktrees.SetupScript(teamScript(src), settings.Setup)
	when := req.Setup
	if when == "" {
		when = settings.SetupWhen
	}
	if setup != "" && when == "ask" {
		httpErr(w, 400, errors.New("choose whether to run the setup script"))
		return
	}
	if plan.Action == "open" {
		if ws := standingOn(s, mc, plan.Path); ws != nil {
			if name := strings.TrimSpace(req.Name); name != "" && name != ws.Label {
				_ = client.WorkspaceRename(ws.WorkspaceID, name)
			}
			s.Watch.Refresh(mc)
			writeJSON(w, map[string]any{"id": ws.WorkspaceID, "pane": firstPaneOf(s, mc, ws.WorkspaceID), "opened": true, "path": plan.Path, "branch": plan.Branch})
			return
		}
	}
	made, err := worktrees.Create(mc, client, src.Root, req.Workspace, plan, strings.TrimSpace(req.Name), worktrees.FolderFor(settings.Location, plan.Branch))
	if err != nil {
		rpcErr(w, err)
		return
	}
	path := made.Worktree.Path
	if path == "" && made.Workspace.Worktree != nil {
		path = made.Workspace.Worktree.CheckoutPath
	}
	pane := made.RootPane.PaneID
	resp := map[string]any{"id": made.Workspace.WorkspaceID, "pane": pane, "path": path, "branch": plan.Branch, "action": plan.Action}
	if len(made.Notes) > 0 {
		resp["notes"] = made.Notes
	}
	if plan.Action != "open" {
		if included, err := worktrees.Include(mc, path); err != nil {
			resp["include_error"] = err.Error()
		} else if included.Listed {
			resp["included"] = included
		}
	}
	runSetup := setup != "" && when != "skip" && plan.Action != "open"
	if runSetup && mc.Windows() {
		resp["setup_error"] = "setup scripts do not run on a Windows PC yet; run it there yourself"
		runSetup = false
	}
	agent := req.Agent != "" && req.Agent != "shell"
	prompt := strings.TrimSpace(req.Prompt)
	ref := events.PaneRef{Machine: mc.ID(), Pane: pane}
	firstSetup := runSetup && (settings.SetupWait || !agent)
	switch {
	case firstSetup:
		resp["setup"] = "first"
		go s.setupThenAgent(mc, client, ref, setup, src.Root, path, plan.Branch, req.Agent, start, prompt)
	case agent:
		if err := sessions.StartAgentWith(client, pane, req.Agent, start); err != nil {
			resp["agent_error"] = err.Error()
		} else if prompt != "" {
			go s.firstPrompt(client, ref, prompt, plan.Branch)
		}
	}
	if runSetup && !firstSetup {
		resp["setup"] = "tab"
		go s.setupInTab(mc, client, made.Workspace.WorkspaceID, setup, src.Root, path, plan.Branch)
	}
	s.Watch.Refresh(mc)
	writeJSON(w, resp)
}

func teamScript(src repos.Source) string {
	if src.Team == nil {
		return ""
	}
	return src.Team.Script
}

// setupThenAgent types the setup into the worktree's own terminal and, once it has ended, starts the agent there.
func (s *Server) setupThenAgent(mc *machines.Machine, client *herdr.Client, ref events.PaneRef, setup, root, path, branch, agent string, start sessions.Start, prompt string) {
	line, err := worktrees.WriteSetup(mc, setup, root, path)
	if err == nil {
		err = s.Input.TypeLine(client, ref, line)
	}
	if err != nil {
		s.setupNote(ref, branch, "The setup of "+branch+" did not start", err)
		return
	}
	if agent == "" || agent == "shell" {
		return
	}
	if _, err := worktrees.AwaitSetup(client, ref.Pane); err != nil {
		s.setupNote(ref, branch, "The agent in "+branch+" was not started", err)
		return
	}
	if err := sessions.StartAgentWith(client, ref.Pane, agent, start); err != nil {
		s.setupNote(ref, branch, "The agent in "+branch+" did not start after its setup", err)
		return
	}
	if prompt != "" {
		_ = s.firstPrompt(client, ref, prompt, branch)
	}
}

// setupInTab runs the setup in a Setup tab of the new workspace, the agent's tab staying in front.
func (s *Server) setupInTab(mc *machines.Machine, client *herdr.Client, workspace, setup, root, path, branch string) {
	tab, err := client.TabCreateIn(workspace, "Setup", path, nil)
	if err != nil {
		s.setupNote(events.PaneRef{Machine: mc.ID()}, branch, "The setup of "+branch+" did not start", err)
		return
	}
	ref := events.PaneRef{Machine: mc.ID(), Pane: tab.RootPane.PaneID}
	line, err := worktrees.WriteSetup(mc, setup, root, path)
	if err == nil {
		err = s.Input.TypeLine(client, ref, line)
	}
	if err != nil {
		s.setupNote(ref, branch, "The setup of "+branch+" did not start", err)
	}
	s.Watch.Refresh(mc)
}

// setupNote tells the person, in the activity and a push, what of a new worktree's setup did not happen.
func (s *Server) setupNote(ref events.PaneRef, branch, title string, err error) {
	if s.Push == nil {
		return
	}
	s.Push.Notify(push.Payload{
		Machine: ref.Machine, Pane: ref.Pane, Kind: "input",
		Title: title, Body: err.Error(), Tag: "setup|" + branch,
		Actions: []push.Action{{Action: "open", Title: "Open"}},
	})
}

// standingOn is the workspace whose checkout is the folder, nil when none has it open.
func standingOn(s *Server, mc *machines.Machine, folder string) *herdr.WorkspaceInfo {
	snap, _ := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return nil
	}
	style := rpath.For(folder)
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		if dir := snap.WorkspaceDir(ws); dir != "" && style.Equal(dir, folder) {
			return ws
		}
	}
	return nil
}

// firstPaneOf is the pane a workspace shows first: its active tab's focused one, else its first.
func firstPaneOf(s *Server, mc *machines.Machine, workspaceID string) string {
	snap, _ := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return ""
	}
	ws := snap.Workspace(workspaceID)
	first := ""
	for i := range snap.Panes {
		p := &snap.Panes[i]
		if p.WorkspaceID != workspaceID {
			continue
		}
		if ws != nil && p.TabID == ws.ActiveTabID && p.Focused {
			return p.PaneID
		}
		if first == "" {
			first = p.PaneID
		}
	}
	return first
}
