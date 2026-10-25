package server

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/text"
)

// Sessions: workspaces, tabs and worktrees made and closed through herdr, and what people type into panes.

func (s *Server) sessionRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/sessions", s.handleNewWorkspace)
	api.HandleFunc("DELETE /api/sessions/{id}", s.closeHandler((*herdr.Client).WorkspaceClose))
	api.HandleFunc("PATCH /api/sessions/{id}", s.renameHandler((*herdr.Client).WorkspaceRename))
	api.HandleFunc("POST /api/tabs", s.handleNewTab)
	api.HandleFunc("DELETE /api/tabs/{id}", s.closeHandler((*herdr.Client).TabClose))
	api.HandleFunc("PATCH /api/tabs/{id}", s.renameHandler((*herdr.Client).TabRename))
	api.HandleFunc("POST /api/worktrees", s.handleNewWorktree)
	api.HandleFunc("POST /api/submit", s.handleSubmit)
	api.HandleFunc("POST /api/keys", s.handleKeys)
	api.HandleFunc("POST /api/seen", s.handleSeen)
}

type createReq struct {
	Machine   string `json:"machine"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Agent     string `json:"agent"`
	Resume    string `json:"resume"` // a session id of the folder's: the agent comes back to that conversation
	Model     string `json:"model"`  // the model as the agent's CLI names it, "" for its default
	Args      string `json:"args"`   // extra arguments as the person typed them, split like a shell would
	Prompt    string `json:"prompt"` // a first prompt, handed over once the agent is up
}

// start is what the request asks of the agent beyond its kind, or the reason it cannot be asked.
func (req *createReq) start() (sessions.Start, error) {
	start := sessions.Start{Resume: req.Resume, Model: strings.TrimSpace(req.Model)}
	if req.Agent == "" || req.Agent == "shell" {
		if start.Resume != "" || start.Model != "" || strings.TrimSpace(req.Args) != "" || strings.TrimSpace(req.Prompt) != "" {
			return start, errors.New("a session to resume, a model, arguments or a first prompt need an agent")
		}
		return start, nil
	}
	if start.Resume != "" && !text.SessionID(start.Resume) {
		return start, errors.New("resume needs a session id")
	}
	args, err := sessions.SplitArgs(req.Args)
	if err != nil {
		return start, err
	}
	start.Args = args
	if _, err := sessions.StartArgs(req.Agent, start); err != nil {
		return start, err
	}
	return start, nil
}

// create is the one path for every "make a container, start an agent in it" request.
// The agent kind is checked before any RPC: an unknown one must not leave an orphaned container.
func (s *Server) create(w http.ResponseWriter, r *http.Request, build func(*herdr.Client, *createReq) (id string, pane *herdr.PaneInfo, extra map[string]any, err error)) {
	var req createReq
	if !decode(w, r, &req) {
		return
	}
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
	id, pane, extra, err := build(client, &req)
	if err != nil {
		rpcErr(w, err)
		return
	}
	resp := map[string]any{"id": id, "pane": pane.PaneID}
	for k, v := range extra {
		resp[k] = v
	}
	prompt := strings.TrimSpace(req.Prompt)
	if req.Agent != "" && req.Agent != "shell" {
		if err := sessions.StartAgentWith(client, pane.PaneID, req.Agent, start); err != nil {
			resp["agent_error"] = err.Error()
			if prompt != "" {
				resp["agent_error"] = err.Error() + " (the first prompt was not sent)"
			}
		} else if prompt != "" {
			go s.firstPrompt(client, events.PaneRef{Machine: mc.ID(), Pane: pane.PaneID}, prompt, firstPromptPlace(&req, extra))
		}
	}
	s.Watch.Refresh(mc)
	writeJSON(w, resp)
}

// firstPrompt delivers the prompt a person wrote for the agent they just started; a failure is a note in the activity, with the pane to open.
func (s *Server) firstPrompt(client *herdr.Client, ref events.PaneRef, prompt, place string) {
	err := s.Input.FirstPrompt(client, ref, prompt, sessions.FirstPromptTimeout)
	if err == nil || s.Push == nil {
		return
	}
	s.Push.Notify(push.Payload{
		Machine: ref.Machine, Pane: ref.Pane, Kind: "input",
		Title:   "The first prompt for " + place + " was not delivered",
		Body:    err.Error(),
		Tag:     "first|" + ref.Pane,
		Actions: []push.Action{{Action: "open", Title: "Open"}},
	})
}

// firstPromptPlace names where the agent was started for the note: the worktree's branch, the workspace's name, else its folder.
func firstPromptPlace(req *createReq, extra map[string]any) string {
	if branch, ok := extra["branch"].(string); ok && branch != "" {
		return branch
	}
	if req.Name != "" {
		return req.Name
	}
	if req.Dir != "" {
		return filepath.Base(req.Dir)
	}
	return "the new tab"
}

func (s *Server) handleNewWorkspace(w http.ResponseWriter, r *http.Request) {
	s.create(w, r, func(c *herdr.Client, req *createReq) (string, *herdr.PaneInfo, map[string]any, error) {
		res, err := c.WorkspaceCreate(req.Dir, req.Name)
		if err != nil {
			return "", nil, nil, err
		}
		return res.Workspace.WorkspaceID, &res.RootPane, nil, nil
	})
}

func (s *Server) handleNewTab(w http.ResponseWriter, r *http.Request) {
	s.create(w, r, func(c *herdr.Client, req *createReq) (string, *herdr.PaneInfo, map[string]any, error) {
		if req.Workspace == "" {
			return "", nil, nil, &herdr.Error{Code: "invalid_request", Message: "workspace required"}
		}
		res, err := c.TabCreate(req.Workspace, req.Name)
		if err != nil {
			return "", nil, nil, err
		}
		return res.Tab.TabID, &res.RootPane, nil, nil
	})
}

func (s *Server) handleNewWorktree(w http.ResponseWriter, r *http.Request) {
	s.create(w, r, func(c *herdr.Client, req *createReq) (string, *herdr.PaneInfo, map[string]any, error) {
		branch, base := strings.TrimSpace(req.Branch), strings.TrimSpace(req.Base)
		if req.Workspace == "" || branch == "" {
			return "", nil, nil, &herdr.Error{Code: "invalid_request", Message: "workspace and branch required"}
		}
		if !sessions.RefOK(branch) || (base != "" && !sessions.RefOK(base)) {
			return "", nil, nil, &herdr.Error{Code: "invalid_request", Message: "branch names: no leading dash, no spaces, no .."}
		}
		res, err := c.WorktreeCreate(req.Workspace, branch, base, req.Name)
		if err != nil {
			return "", nil, nil, err
		}
		return res.Workspace.WorkspaceID, &res.RootPane,
			map[string]any{"path": res.Worktree.Path, "branch": res.Worktree.Branch}, nil
	})
}

// closeHandler closes a workspace or tab by path id and refreshes the roster.
func (s *Server) closeHandler(close func(*herdr.Client, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mc, client, _, ok := s.ready(w, r, "")
		if !ok {
			return
		}
		if err := close(client, r.PathValue("id")); err != nil {
			rpcErr(w, err)
			return
		}
		s.Watch.Refresh(mc)
		writeJSON(w, map[string]any{"ok": true})
	}
}

// renameHandler serves the two renames, a tab's or a workspace's label, from a JSON body {"label": …}.
func (s *Server) renameHandler(rename func(*herdr.Client, string, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Label   string `json:"label"`
			Machine string `json:"machine"`
		}
		if !decode(w, r, &req) {
			return
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			httpErr(w, 400, errors.New("a label is required"))
			return
		}
		mc, client, _, ok := s.ready(w, r, req.Machine)
		if !ok {
			return
		}
		if err := rename(client, r.PathValue("id"), label); err != nil {
			rpcErr(w, err)
			return
		}
		s.Watch.Refresh(mc)
		writeJSON(w, map[string]any{"ok": true})
	}
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Pane    string `json:"pane"`
		Text    string `json:"text"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" || req.Pane == "" {
		httpErr(w, 400, fmt.Errorf("pane and text required"))
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	if err := s.Input.Submit(client, events.PaneRef{Machine: mc.ID(), Pane: req.Pane}, req.Text); err != nil {
		if errors.Is(err, sessions.ErrAgentBusy) {
			httpErr(w, http.StatusConflict, err)
			return
		}
		rpcErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string   `json:"machine"`
		Pane    string   `json:"pane"`
		Keys    []string `json:"keys"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Pane == "" || len(req.Keys) == 0 {
		httpErr(w, 400, fmt.Errorf("pane and keys required"))
		return
	}
	keys, err := sessions.Keys(req.Keys)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	if err := s.Input.SendKeys(client, events.PaneRef{Machine: mc.ID(), Pane: req.Pane}, keys); err != nil {
		rpcErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleSeen: a client is looking at the pane, so a held "done" comes down.
func (s *Server) handleSeen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Pane    string `json:"pane"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Pane == "" {
		httpErr(w, 400, fmt.Errorf("pane required"))
		return
	}
	if req.Machine == "" {
		req.Machine = machines.LocalID
	}
	s.Radar.Seen(req.Machine, req.Pane)
	w.WriteHeader(http.StatusNoContent)
}
