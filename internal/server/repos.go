package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/repos"
	"github.com/dev1antdev/remuda/internal/rpath"
)

// Project settings and run actions, per repository: GET and PUT /api/repos/settings, and POST /api/run, the run
// button's press — a shell herdr makes in a new tab or beside the pane, the action's command typed into it.

func (s *Server) repoRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/repos/settings", s.handleRepoSettings)
	api.HandleFunc("PUT /api/repos/settings", s.handleRepoSettingsPut)
	api.HandleFunc("POST /api/run", s.handleRunAction)
}

// handleRepoSettings is GET /api/repos/settings?machine&root: the settings, and what the repository says itself — its
// name, its default base, its team's setup script — read on the machine when it is connected.
func (s *Server) handleRepoSettings(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	root := r.URL.Query().Get("root")
	answer := map[string]any{
		"root": root, "name": rpath.For(root).Base(root), "settings": s.repoSettings(mc.ID(), root),
		"defaults": map[string]string{"issue_branch": repos.DefaultIssueBranch, "issue_prompt": repos.DefaultIssuePrompt, "pr_prompt": repos.DefaultPRPrompt},
		"icons":    repos.Icons, "setup": !mc.Windows(),
	}
	if r.URL.Query().Get("quick") == "1" {
		writeJSON(w, answer) // the run button's read: the settings alone, nothing run on the machine
		return
	}
	if _, _, err := mc.Ready(); err != nil {
		answer["trouble"] = err.Error()
	} else if src, err := repos.Read(mc, root); err != nil {
		answer["trouble"] = err.Error()
	} else {
		answer["name"], answer["base"], answer["team"] = src.Name, src.Base, src.Team
	}
	writeJSON(w, answer)
}

// handleRepoSettingsPut is PUT /api/repos/settings {machine, root, settings}: kept whole.
func (s *Server) handleRepoSettingsPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine  string         `json:"machine"`
		Root     string         `json:"root"`
		Settings repos.Settings `json:"settings"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, err := s.machineFor(r, req.Machine)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if s.Repos == nil || strings.TrimSpace(req.Root) == "" || !rpath.For(req.Root).IsAbs(req.Root) {
		httpErr(w, 400, errors.New("which repository: its main folder, as a whole path"))
		return
	}
	saved, err := s.Repos.Put(mc.ID(), req.Root, req.Settings)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"settings": saved})
}

// handleRunAction is POST /api/run {machine, workspace, pane, root, action}: the action saved for the repository by its
// id — never a command the request carries — run in a shell herdr makes for it, in a new tab of the workspace or split
// beside the pane, in the action's folder of the workspace's checkout; the answer is the new pane, the command typed
// into its shell once it has come up.
func (s *Server) handleRunAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine   string `json:"machine"`
		Workspace string `json:"workspace"`
		Pane      string `json:"pane"`
		Root      string `json:"root"`
		Action    string `json:"action"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	if s.Repos == nil {
		httpErr(w, 404, errors.New("no run actions here"))
		return
	}
	action, found := s.Repos.Action(mc.ID(), req.Root, req.Action)
	if !found {
		httpErr(w, 404, errors.New("that run action is not saved for the repository any more"))
		return
	}
	snap, err := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	ws := snap.Workspace(req.Workspace)
	if ws == nil {
		httpErr(w, 404, fmt.Errorf("no workspace %s", req.Workspace))
		return
	}
	folder := snap.WorkspaceDir(ws)
	if action.Folder != "" && folder != "" {
		folder = rpath.For(folder).Join(folder, action.Folder)
	}
	pane, err := openRunPane(client, snap, ws, req.Pane, action, folder)
	if err != nil {
		rpcErr(w, err)
		return
	}
	ref := events.PaneRef{Machine: mc.ID(), Pane: pane}
	go s.typeRun(mc, client, ref, action)
	s.noteSent(r.Context(), mc.ID(), pane, action.Command, false)
	s.Watch.Refresh(mc)
	writeJSON(w, map[string]any{"pane": pane, "place": action.Place})
}

// openRunPane is the shell an action runs in: a tab named after it, or a split of the pane (one of the workspace's).
func openRunPane(client *herdr.Client, snap *herdr.Snapshot, ws *herdr.WorkspaceInfo, paneID string, action repos.Action, folder string) (string, error) {
	if action.Place == "right" || action.Place == "down" {
		for i := range snap.Panes {
			if p := &snap.Panes[i]; p.PaneID == paneID && p.WorkspaceID == ws.WorkspaceID {
				made, err := client.PaneSplit(paneID, action.Place, folder)
				if err != nil {
					return "", err
				}
				return made.PaneID, nil
			}
		}
		return "", &herdr.Error{Code: "pane_not_found", Message: "the pane to split beside is not in that workspace"}
	}
	made, err := client.TabCreateIn(ws.WorkspaceID, action.Name, folder, nil)
	if err != nil {
		return "", err
	}
	return made.RootPane.PaneID, nil
}

// typeRun types the action's command into its new shell; a failure is a note in the activity, with the pane to open.
func (s *Server) typeRun(mc *machines.Machine, client *herdr.Client, ref events.PaneRef, action repos.Action) {
	if err := s.Input.TypeLine(client, ref, action.Command); err != nil && s.Push != nil {
		s.Push.Notify(push.Payload{
			Machine: mc.ID(), Pane: ref.Pane, Kind: "input",
			Title: "“" + action.Name + "” did not start", Body: err.Error(), Tag: "run|" + ref.Pane,
			Actions: []push.Action{{Action: "open", Title: "Open"}},
		})
	}
}
