package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/events"
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

// olderSpelling is the folder as a request named it, when that spelling is a second key of the same folder and a
// clear must stand under it too: never one with a .. in it, which may name another folder once cleaned.
func olderSpelling(mc *machines.Machine, given, root string) string {
	if given == root || hasUp(mc, given) {
		return ""
	}
	return given
}

// hasUp says the path holds a .. — which, cleaned, may name another folder than the one the machine resolved.
func hasUp(mc *machines.Machine, dir string) bool {
	return slices.Contains(strings.Split(mc.Paths().Slash(dir), "/"), "..")
}

// repoRoot is the folder a repository's settings are read under: the machine's spelling of it, or the caller's own
// when only that spelling has settings — a folder saved before remuda named folders the machine's way keeps what it
// has until it is saved again. False when the machine cannot name the folder at all: its settings are then not read,
// so a record kept under an old spelling never answers for a clear made through another.
func (s *Server) repoRoot(mc *machines.Machine, root string) (record, named string, ok bool) {
	named, ok = mc.Canonical(root)
	if !ok {
		return root, "", false
	}
	if named == root || s.Repos == nil || hasUp(mc, root) { // a .. that cleans elsewhere names no folder of its own
		return named, named, true
	}
	if !s.Repos.Has(mc.ID(), named) && s.Repos.Has(mc.ID(), root) {
		return root, named, true // a record saved before remuda named folders the machine's way
	}
	return named, named, true
}

// handleRepoSettings is GET /api/repos/settings?machine&root: the settings, and what the repository says itself — its
// name, its default base, its team's setup script — read on the machine when it is connected.
func (s *Server) handleRepoSettings(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	root, named, nameable := s.repoRoot(mc, r.URL.Query().Get("root"))
	settings, version := repos.Settings{}, ""
	if nameable {
		settings, version = s.Repos.GetVersion(mc.ID(), root)
		version += "|" + named // the folder the draft is for too: a link pointed elsewhere is a stale draft
	}
	answer := map[string]any{
		"root": root, "name": rpath.For(root).Base(root), "settings": settings, "version": version,
		"defaults": map[string]string{"issue_branch": repos.DefaultIssueBranch, "issue_prompt": repos.DefaultIssuePrompt, "pr_prompt": repos.DefaultPRPrompt},
		"icons":    repos.Icons, "setup": !mc.Windows(),
	}
	if r.URL.Query().Get("quick") == "1" {
		writeJSON(w, answer) // the run button's read: the settings alone, nothing run on the machine
		return
	}
	if !nameable {
		answer["trouble"] = mc.Label() + " cannot name that folder just now: its settings are not read"
	} else if _, _, err := mc.Ready(); err != nil {
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
		Version  *string        `json:"version"` // what the draft was read from; absent for a request that carries none
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
	given := req.Root
	if _, ok := mc.Canonical(given); !ok && unclear(mc, given) {
		httpErr(w, 400, fmt.Errorf("the folder %s is not on %s, and its .. names another one", given, mc.Label()))
		return
	}
	root, named, nameable := s.repoRoot(mc, given) // the very record the read named: one resolution for the request
	if !nameable && s.Repos.Has(mc.ID(), root) {
		// a record is there and the machine cannot name its folder: the dialog's draft was read without it, so saving
		// the whole of it would erase what it could not show (a folder that is not there, a machine that is away)
		httpErr(w, http.StatusBadGateway, fmt.Errorf("%s cannot name %s just now, so its settings are not replaced", mc.Label(), root))
		return
	}
	write := root // the machine's spelling when it can name the folder, the record the read used otherwise
	if nameable {
		write = named
	}
	var version *string
	if req.Version != nil {
		want := "" // a folder the machine cannot name — a checkout not cloned yet — has no record to have read
		if nameable {
			want = s.Repos.VersionAt(mc.ID(), root) + "|" + named
		}
		if *req.Version != want {
			httpErr(w, http.StatusConflict, repos.ErrStale)
			return
		}
		kept := strings.SplitN(*req.Version, "|", 2)[0]
		version = &kept
	}
	saved, err := s.Repos.Commit(mc.ID(), root, write, req.Settings, version, olderSpelling(mc, given, write))
	if errors.Is(err, repos.ErrStale) {
		httpErr(w, http.StatusConflict, err)
		return
	}
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
	root, _, nameable := s.repoRoot(mc, req.Root)
	if !nameable {
		httpErr(w, http.StatusBadGateway, fmt.Errorf("%s cannot name that folder just now: its run actions are not read", mc.Label()))
		return
	}
	action, found := s.Repos.Action(mc.ID(), root, req.Action)
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
func openRunPane(client coreapi.API, snap *coreapi.Snapshot, ws *coreapi.WorkspaceInfo, paneID string, action repos.Action, folder string) (string, error) {
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
		return "", &coreapi.Error{Code: "pane_not_found", Message: "the pane to split beside is not in that workspace"}
	}
	made, err := client.TabCreateIn(ws.WorkspaceID, action.Name, folder, nil)
	if err != nil {
		return "", err
	}
	return made.RootPane.PaneID, nil
}

// typeRun types the action's command into its new shell; a failure is a note in the activity, with the pane to open.
func (s *Server) typeRun(mc *machines.Machine, client coreapi.API, ref events.PaneRef, action repos.Action) {
	if err := s.Input.TypeLine(client, ref, action.Command); err != nil && s.Push != nil {
		s.Push.Notify(push.Payload{
			Machine: mc.ID(), Pane: ref.Pane, Kind: "input",
			Title: "“" + action.Name + "” did not start", Body: err.Error(), Tag: "run|" + ref.Pane,
			Actions: []push.Action{{Action: "open", Title: "Open"}},
		})
	}
}
