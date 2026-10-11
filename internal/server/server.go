// Package server is herdr-ui's HTTP + WebSocket bridge to herdr — the local
// one and any remote machine's — roster and chat over REST, live terminal
// bytes over WS. herdr owns every process, pane and agent status; Claude Code
// owns its transcript files; this server only connects them to the browser.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/dev1antdev/herdr-ui/internal/auth"
	"github.com/dev1antdev/herdr-ui/internal/code"
	"github.com/dev1antdev/herdr-ui/internal/health"
	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/jobs"
	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/push"
	"github.com/dev1antdev/herdr-ui/internal/quick"
	"github.com/dev1antdev/herdr-ui/internal/radar"
	"github.com/dev1antdev/herdr-ui/internal/review"
	"github.com/dev1antdev/herdr-ui/internal/schedule"
	"github.com/dev1antdev/herdr-ui/internal/tasks"
	"github.com/dev1antdev/herdr-ui/internal/transcript"
	"github.com/dev1antdev/herdr-ui/internal/tsnetx"
	"github.com/dev1antdev/herdr-ui/internal/usage"
	buildversion "github.com/dev1antdev/herdr-ui/internal/version"
	"github.com/dev1antdev/herdr-ui/internal/watch"
)

type Server struct {
	Machines  *machines.Manager
	Watch     *watch.Watcher
	Push      *push.Service
	Tasks     *tasks.Store
	Code      *code.Service
	Review    *review.Store
	Schedules *schedule.Store
	Quick     *quick.Store
	Usage     *usage.Tracker
	// Health and Tailscale are set after New, like Schedules: the machines'
	// vitals, and the tailnet node's status when -tailscale is on
	Health    *health.Collector
	Tailscale func() tsnetx.Status
	Jobs      *jobs.Runner // the updates a person runs from the page
	Radar     *radar.Tracker
	Guard     *auth.Guard
	UI        fs.FS // built SPA

	mu      sync.Mutex
	paneMu  map[push.PaneRef]*sync.Mutex // per-pane submit serialization
	streams termStreams                  // HTTP-transport terminal sessions
}

// New wires the server. Schedules is the one dependency set afterwards: it
// delivers through the server's own SubmitToFolder.
func New(m *machines.Manager, w *watch.Watcher, p *push.Service, t *tasks.Store, c *code.Service, rv *review.Store, r *radar.Tracker, q *quick.Store, u *usage.Tracker, g *auth.Guard, ui fs.FS) *Server {
	return &Server{Machines: m, Watch: w, Push: p, Tasks: t, Code: c, Review: rv, Radar: r, Quick: q, Usage: u, Guard: g, UI: ui, paneMu: map[push.PaneRef]*sync.Mutex{}}
}

// agents herdr-ui offers to start, by herdr manifest kind. The list is also
// served to the UI so there is one copy.
var agents = []agentKind{
	{"claude", "Claude Code"}, {"codex", "Codex"}, {"gemini", "Gemini"}, {"pi", "pi"},
	{"opencode", "OpenCode"}, {"cursor", "Cursor"}, {"amp", "Amp"}, {"grok", "Grok"}, {"qwen", "Qwen"},
}

type agentKind struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// agentOK accepts a known agent kind, or "shell" / "" for no agent.
func agentOK(kind string) bool {
	if kind == "" || kind == "shell" {
		return true
	}
	for _, a := range agents {
		if a.ID == kind {
			return true
		}
	}
	return false
}

func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/roster", s.handleRoster)
	api.HandleFunc("POST /api/machines", s.handleAddMachine)
	api.HandleFunc("DELETE /api/machines/{id}", s.handleRemoveMachine)
	api.HandleFunc("PATCH /api/machines/{id}", s.handleRenameMachine)
	api.HandleFunc("POST /api/machines/{id}/health", s.handleMachineHealth)
	api.HandleFunc("GET /api/machines/{id}/updates", s.handleMachineUpdates)
	api.HandleFunc("POST /api/machines/{id}/update", s.handleMachineUpdate)
	api.HandleFunc("GET /api/jobs/{id}", s.handleJob)
	api.HandleFunc("POST /api/sessions", s.handleNewWorkspace)
	api.HandleFunc("DELETE /api/sessions/{id}", s.closeHandler((*herdr.Client).WorkspaceClose))
	api.HandleFunc("POST /api/tabs", s.handleNewTab)
	api.HandleFunc("DELETE /api/tabs/{id}", s.closeHandler((*herdr.Client).TabClose))
	api.HandleFunc("PATCH /api/tabs/{id}", s.renameHandler((*herdr.Client).TabRename))
	api.HandleFunc("PATCH /api/sessions/{id}", s.renameHandler((*herdr.Client).WorkspaceRename))
	api.HandleFunc("POST /api/worktrees", s.handleNewWorktree)
	api.HandleFunc("GET /api/chat", s.handleChat)
	api.HandleFunc("GET /api/fs/stat", s.handleFsStat)
	api.HandleFunc("GET /api/fs/dirs", s.handleFsDirs)
	api.HandleFunc("GET /api/fs/file", s.handleFsFile)
	api.HandleFunc("GET /api/fs/diff", s.handleFsDiff)
	api.HandleFunc("POST /api/fs/upload", s.handleFsUpload)
	api.HandleFunc("GET /api/push", s.handlePushInfo)
	api.HandleFunc("POST /api/push/subscribe", s.handlePushSubscribe)
	api.HandleFunc("DELETE /api/push/subscribe", s.handlePushUnsubscribe)
	api.HandleFunc("POST /api/push/test", s.handlePushTest)
	api.HandleFunc("POST /api/seen", s.handleSeen)
	api.HandleFunc("GET /api/search", s.handleSearch)
	api.HandleFunc("GET /api/usage", s.handleUsage)
	api.HandleFunc("GET /api/quick", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Quick.List()) })
	api.HandleFunc("PUT /api/quick", func(w http.ResponseWriter, r *http.Request) {
		var list []quick.Prompt
		if !decode(w, r, &list) {
			return
		}
		out, err := s.Quick.Replace(list)
		if err != nil {
			httpErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, out)
	})
	api.HandleFunc("GET /api/schedules", s.handleSchedules)
	api.HandleFunc("PUT /api/schedules", s.handleSchedulePut)
	api.HandleFunc("DELETE /api/schedules/{id}", s.handleScheduleDelete)
	api.HandleFunc("POST /api/schedules/{id}/run", s.handleScheduleRun)
	api.HandleFunc("GET /api/review", s.handleReview)
	api.HandleFunc("GET /api/review/diff", s.handleReviewDiff)
	api.HandleFunc("POST /api/review/seen", s.handleReviewSeen)
	api.HandleFunc("GET /api/code/{machine}", s.handleCodeStatus)
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		api.HandleFunc(m+" /code/{machine}", s.handleCodeProxy)
		api.HandleFunc(m+" /code/{machine}/", s.handleCodeProxy)
	}
	s.tasksRoutes(api)
	api.HandleFunc("POST /api/submit", s.handleSubmit)
	api.HandleFunc("POST /api/keys", s.handleKeys)
	api.HandleFunc("GET /ws/pane", s.handlePaneWS)
	api.HandleFunc("POST /api/term/open", s.handleTermOpen)
	api.HandleFunc("GET /api/term/read", s.handleTermRead)
	api.HandleFunc("POST /api/term/input", s.handleTermInput)
	api.HandleFunc("POST /api/term/resize", s.handleTermResize)
	api.HandleFunc("DELETE /api/term/close", s.handleTermClose)
	api.Handle("GET /", s.staticHandler())

	// the only two routes outside the token guard
	root := http.NewServeMux()
	root.HandleFunc("GET /api/health", s.handleHealth)
	root.HandleFunc("POST /api/login", s.Guard.HandleLogin)
	root.Handle("/", s.Guard.Middleware(api))
	return csrfGuard(root)
}

// csrfGuard rejects cross-origin mutations and WebSocket upgrades. The token
// cookie is SameSite=Lax, which is per site, not per origin: another port on
// the same host would get it — so an Origin that is not ours is refused
// outright, and for a browser that sends no Sec-Fetch-* at all the Origin
// check is the one that holds.
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutation := r.Method != http.MethodGet && r.Method != http.MethodHead || strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		if mutation {
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if u, err := url.Parse(origin); err != nil || !strings.EqualFold(u.Host, r.Host) {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

// rpcErr reports a failed herdr call: herdr's own refusals are the caller's
// problem (4xx), anything else is the bridge's (502).
func rpcErr(w http.ResponseWriter, err error) {
	var he *herdr.Error
	if errors.As(err, &he) {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	httpErr(w, http.StatusBadGateway, err)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpErr(w, 400, fmt.Errorf("bad request body: %v", err))
		return false
	}
	return true
}

// machineFor finds the request's machine: ?machine=, the X-Herdr-UI-Machine
// header, or the id a POST body carried. Empty means local.
func (s *Server) machineFor(r *http.Request, bodyID string) (*machines.Machine, error) {
	id := r.URL.Query().Get("machine")
	if id == "" {
		id = r.Header.Get("X-Herdr-UI-Machine")
	}
	if id == "" {
		id = bodyID
	}
	return s.Machines.Get(id)
}

// ready resolves the machine and its live client, answering 404/502 itself.
func (s *Server) ready(w http.ResponseWriter, r *http.Request, bodyID string) (*machines.Machine, *herdr.Client, *herdr.AttachManager, bool) {
	mc, err := s.machineFor(r, bodyID)
	if err != nil {
		httpErr(w, 404, err)
		return nil, nil, nil, false
	}
	client, attach, err := mc.Ready()
	if err != nil {
		httpErr(w, 502, err)
		return nil, nil, nil, false
	}
	return mc, client, attach, true
}

// handleHealth answers from the watcher's cache: unauthenticated, so it
// never asks herdr anything.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	snap, err := s.Watch.Snapshot(machines.LocalID)
	if snap == nil || err != nil {
		writeJSON(w, map[string]any{"ok": true, "herdr": false})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "herdr": true, "herdr_version": snap.Version, "version": buildversion.Build})
}

// POST /api/machines/{id}/health — read the machine's vitals now.
func (s *Server) handleMachineHealth(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if s.Health == nil {
		httpErr(w, 503, fmt.Errorf("health checks are off"))
		return
	}
	s.Health.Refresh(mc)
	writeJSON(w, map[string]any{"health": s.Health.Report(mc.Cfg.ID), "latest": s.Health.Latest()})
}

// ---- roster ----

type rosterPane struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Agent   string `json:"agent"` // "" for a plain shell
	CWD     string `json:"cwd"`
	Focused bool   `json:"focused"` // the tab's focused pane
	Status  string `json:"status"`  // run | input | idle | done | shell
	Cols    int    `json:"cols"`    // the pane's own grid, so a phone can keep it
	Rows    int    `json:"rows"`
	// an agent's lifecycle as a person reads it (held done, held question,
	// last activity) — nil for a shell
	Radar *radar.Display `json:"radar,omitempty"`
	// the model, how full its context is, what the session cost — once read
	Usage *usage.Display `json:"usage,omitempty"`
}

type rosterTab struct {
	ID     string       `json:"id"`
	Label  string       `json:"label"`
	Number int          `json:"number"`
	Named  bool         `json:"named"` // the user gave it a name (herdr labels unnamed tabs by number)
	Status string       `json:"status"`
	Panes  []rosterPane `json:"panes"`
}

type rosterReview struct {
	Changed int `json:"changed"` // files changed vs the reviewer's baseline (HEAD without one)
	Fresh   int `json:"fresh"`   // of those, moved since the reviewer looked
}

type rosterWorktree struct {
	RepoKey      string `json:"repo_key"`
	RepoName     string `json:"repo_name"`
	Linked       bool   `json:"linked"` // a `git worktree add` checkout, nested under its repo
	CheckoutPath string `json:"checkout_path"`
}

type rosterSession struct {
	ID          string          `json:"id"` // workspace id
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	ActiveTabID string          `json:"active_tab_id"`
	Worktree    *rosterWorktree `json:"worktree,omitempty"`
	Dir         string          `json:"dir,omitempty"`    // the workspace's folder: its checkout, else its first pane's cwd
	Review      *rosterReview   `json:"review,omitempty"` // what the reviewer has not looked at yet
	Tabs        []rosterTab     `json:"tabs"`
}

type rosterMachine struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	Host     string          `json:"host,omitempty"`
	Source   string          `json:"source,omitempty"` // machines.SourceHerdr when mirrored
	State    string          `json:"state"`            // connected | connecting | error
	Error    string          `json:"error,omitempty"`
	Sessions []rosterSession `json:"sessions"`
	Health   *health.Report  `json:"health,omitempty"` // versions, disk, load — see internal/health
}

// sessionsFromSnapshot keeps herdr's own hierarchy: workspace → tabs → panes,
// in herdr's order, so the UI can draw the same tree herdr's sidebar does.
func (s *Server) sessionsFromSnapshot(machineID string, snap *herdr.Snapshot) []rosterSession {
	panesByTab := map[string][]rosterPane{}
	agentInTab := map[string]bool{}
	for i := range snap.Panes {
		p := &snap.Panes[i]
		cols, rows := snap.PaneSize(p.PaneID)
		rp := rosterPane{
			ID:      p.PaneID,
			Title:   p.DisplayTitle(),
			Agent:   p.AgentName(),
			CWD:     p.Dir(),
			Focused: p.Focused || p.PaneID == snap.FocusedPaneID,
			Status:  herdr.UIStatus(p.AgentStatus, p.Agent != ""),
			Cols:    cols,
			Rows:    rows,
		}
		if p.Agent != "" {
			d := s.Radar.Display(machineID, p)
			rp.Radar = &d
			if s.Usage != nil {
				if u, ok := s.Usage.Display(machineID, p.PaneID); ok {
					rp.Usage = &u
				}
			}
		}
		panesByTab[p.TabID] = append(panesByTab[p.TabID], rp)
		if p.Agent != "" {
			agentInTab[p.TabID] = true
		}
	}
	tabsByWS := map[string][]rosterTab{}
	agentInWS := map[string]bool{}
	for _, t := range snap.Tabs {
		panes := panesByTab[t.TabID]
		if panes == nil {
			panes = []rosterPane{}
		}
		tabsByWS[t.WorkspaceID] = append(tabsByWS[t.WorkspaceID], rosterTab{
			ID: t.TabID, Label: t.Label, Number: t.Number,
			Named:  t.Label != strconv.Itoa(t.Number),
			Status: herdr.UIStatus(t.AgentStatus, agentInTab[t.TabID]), Panes: panes,
		})
		if agentInTab[t.TabID] {
			agentInWS[t.WorkspaceID] = true
		}
	}
	out := make([]rosterSession, 0, len(snap.Workspaces))
	for _, ws := range snap.Workspaces {
		tabs := tabsByWS[ws.WorkspaceID]
		if tabs == nil {
			tabs = []rosterTab{}
		}
		rs := rosterSession{
			ID:          ws.WorkspaceID,
			Name:        ws.Label,
			Status:      herdr.UIStatus(ws.AgentStatus, agentInWS[ws.WorkspaceID]),
			ActiveTabID: ws.ActiveTabID,
			Tabs:        tabs,
		}
		if ws.Worktree != nil {
			rs.Worktree = &rosterWorktree{
				RepoKey: ws.Worktree.RepoKey, RepoName: ws.Worktree.RepoName,
				Linked: ws.Worktree.IsLinkedWorktree, CheckoutPath: ws.Worktree.CheckoutPath,
			}
		}
		rs.Dir = workspaceDir(snap, &ws)
		if b, ok := s.Review.Badge(machineID, rs.Dir); rs.Dir != "" && ok && b.Changed > 0 {
			rs.Review = &rosterReview{Changed: b.Changed, Fresh: b.Fresh}
		}
		out = append(out, rs)
	}
	return out
}

// workspaceDir is the folder a workspace lives in: its git checkout when
// herdr knows one, else the cwd of its first pane.
func workspaceDir(snap *herdr.Snapshot, ws *herdr.WorkspaceInfo) string {
	if ws.Worktree != nil && ws.Worktree.CheckoutPath != "" {
		return ws.Worktree.CheckoutPath
	}
	for i := range snap.Panes {
		if snap.Panes[i].WorkspaceID == ws.WorkspaceID {
			if d := snap.Panes[i].Dir(); d != "" {
				return d
			}
		}
	}
	return ""
}

// WorkspaceDirs lists the folders of a snapshot's workspaces (for the
// review badges).
func WorkspaceDirs(snap *herdr.Snapshot) []string {
	var out []string
	seen := map[string]bool{}
	for i := range snap.Workspaces {
		if d := workspaceDir(snap, &snap.Workspaces[i]); d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// handleRoster serves every machine from the watcher's cache — never a herdr
// call on the request path — with an ETag so an unchanged roster is a 304.
func (s *Server) handleRoster(w http.ResponseWriter, r *http.Request) {
	list := s.Machines.List()
	out := make([]rosterMachine, 0, len(list))
	for _, mc := range list {
		st, errMsg := mc.State()
		rm := rosterMachine{
			ID: mc.Cfg.ID, Label: mc.Label(), Host: mc.Cfg.Host, Source: mc.Cfg.Source,
			State: st, Error: errMsg, Sessions: []rosterSession{},
		}
		if st == machines.StateConnected {
			snap, err := s.Watch.Snapshot(mc.Cfg.ID)
			if snap != nil {
				rm.Sessions = s.sessionsFromSnapshot(mc.Cfg.ID, snap)
			}
			if err != nil {
				rm.State, rm.Error = machines.StateError, err.Error()
			}
		}
		if s.Health != nil {
			rm.Health = s.Health.Report(mc.Cfg.ID)
		}
		out = append(out, rm)
	}
	roster := map[string]any{"machines": out, "agents": agents, "version": buildversion.Build}
	if s.Health != nil {
		roster["latest"] = s.Health.Latest()
	}
	if s.Tailscale != nil {
		roster["tailscale"] = s.Tailscale()
	}
	body, _ := json.Marshal(roster)
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:12]) + `"`
	w.Header().Set("X-Herdr-UI-Version", buildversion.Build) // a page older than the server offers a reload
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (s *Server) findPane(mc *machines.Machine, paneID string) (*herdr.PaneInfo, error) {
	snap, err := s.Watch.Snapshot(mc.Cfg.ID)
	if snap == nil {
		return nil, err
	}
	for i := range snap.Panes {
		if snap.Panes[i].PaneID == paneID {
			return &snap.Panes[i], nil
		}
	}
	return nil, fmt.Errorf("pane %s not found", paneID)
}

// ---- machines CRUD ----

func (s *Server) handleAddMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
		Host  string `json:"host"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, err := s.Machines.Add(req.Label, req.Host)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"id": mc.Cfg.ID})
}

func (s *Server) handleRemoveMachine(w http.ResponseWriter, r *http.Request) {
	if err := s.Machines.Remove(r.PathValue("id")); err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// PATCH /api/machines/{id} {label} — a name of the user's own for the
// machine (the local one too); empty goes back to the registry's.
func (s *Server) handleRenameMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		httpErr(w, 400, err)
		return
	}
	mc, err := s.Machines.Rename(r.PathValue("id"), req.Label)
	if err != nil {
		if mc == nil {
			httpErr(w, 404, err)
		} else {
			httpErr(w, 500, err)
		}
		return
	}
	writeJSON(w, map[string]any{"ok": true, "label": mc.Label()})
}

// ---- workspace / tab / worktree create and close ----

type createReq struct {
	Machine   string `json:"machine"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Agent     string `json:"agent"` // herdr agent kind, or "shell"
}

// create is the one path for every "make a container, start an agent in it"
// request: the agent kind is validated before any RPC (an unknown one must
// not leave an orphaned container), the container is created, the agent is
// started, the watcher is refreshed so the UI sees the new pane at once.
func (s *Server) create(w http.ResponseWriter, r *http.Request, build func(*herdr.Client, *createReq) (id string, pane *herdr.PaneInfo, extra map[string]any, err error)) {
	var req createReq
	if !decode(w, r, &req) {
		return
	}
	if !agentOK(req.Agent) {
		httpErr(w, 400, fmt.Errorf("unknown agent %q", req.Agent))
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
	// the container exists either way; an agent that failed to start is
	// reported, not rolled back
	if req.Agent != "" && req.Agent != "shell" {
		if err := startAgent(client, pane.PaneID, req.Agent); err != nil {
			resp["agent_error"] = err.Error()
		}
	}
	s.Watch.Refresh(mc)
	writeJSON(w, resp)
}

// startAgent starts an agent in a pane whose shell may still be coming up
// (a new worktree's pane is "not an available shell" for a moment): the
// busy answer is retried for a few seconds.
func startAgent(client *herdr.Client, paneID, kind string) error {
	deadline := time.Now().Add(12 * time.Second)
	for {
		err := client.AgentStart(paneID, kind)
		if err == nil || !(herdr.IsCode(err, "agent_pane_busy") || herdr.IsCode(err, "agent_not_ready")) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
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
		branch := strings.TrimSpace(req.Branch)
		if req.Workspace == "" || branch == "" {
			return "", nil, nil, &herdr.Error{Code: "invalid_request", Message: "workspace and branch required"}
		}
		res, err := c.WorktreeCreate(req.Workspace, branch, strings.TrimSpace(req.Base), req.Name)
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

// renameHandler serves the two renames: a tab's or a workspace's label,
// from a JSON body {"label": …}.
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

// ---- chat ----

var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,80}$`)

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	paneID := r.URL.Query().Get("pane")
	want := r.URL.Query().Get("session")
	if want != "" && !sessionIDRe.MatchString(want) {
		httpErr(w, 400, fmt.Errorf("bad session id"))
		return
	}

	// Either a pane (its folder, its agent's session) or — a search jump,
	// read-only — a folder and a session id.
	var path, cwd string
	var sessions []transcript.SessionInfo
	if paneID == "" {
		cwd = r.URL.Query().Get("dir")
		if cwd == "" || want == "" {
			httpErr(w, 400, fmt.Errorf("pane, or dir and session, required"))
			return
		}
		p, err := mc.ResolveSession(cwd, "id", want)
		if err != nil {
			httpErr(w, 404, fmt.Errorf("session %s: %w", want, err))
			return
		}
		path = p
	} else {
		pane, err := s.findPane(mc, paneID)
		if err != nil {
			httpErr(w, 404, err)
			return
		}
		s.Radar.Seen(mc.Cfg.ID, pane.PaneID)
		cwd = pane.Dir()

		// herdr names the exact session when it knows it; the folder's session
		// list is the fallback and the picker (local machines only, for now).
		if want != "" { // only a session of this folder: another folder's is the viewer's business
			if mc.Local {
				sessions, _ = transcript.Sessions(cwd)
				for _, c := range sessions {
					if c.ID == want {
						path = c.Path
					}
				}
			} else if p, err := mc.ResolveSession(cwd, "id", want); err == nil && strings.Contains(p, "/"+transcript.EncodePath(cwd)+"/") {
				path = p
			}
		}
		if path == "" && pane.AgentSession != nil && pane.AgentSession.Agent == "claude" {
			path, _ = mc.ResolveSession(cwd, pane.AgentSession.Kind, pane.AgentSession.Value)
		}
		if path == "" && mc.Local {
			if sessions == nil {
				sessions, _ = transcript.Sessions(cwd)
			}
			if len(sessions) > 0 {
				path = sessions[0].Path
			}
		}
	}
	if path == "" {
		writeJSON(w, map[string]any{"sessions": []any{}, "turns": []any{}})
		return
	}

	// a compacted conversation went on in a newer file — unless this very
	// file was asked for (the picker, a search hit in it)
	if want == "" {
		path = mc.FollowContinuation(path)
	}
	version, err := mc.TranscriptVersion(path)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	id := transcript.SessionID(path)
	etag := fmt.Sprintf(`"%s-%s"`, id, version)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	turns, err := mc.Transcript(path, version)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	if sessions == nil && mc.Local && paneID != "" { // the picker, only on a full response; a transcript on its own has none
		sessions, _ = transcript.Sessions(cwd)
	}
	if sessions == nil {
		sessions = []transcript.SessionInfo{}
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"session_id": id, "sessions": sessions, "turns": turns})
}

// ---- files the agent wrote ----

// handleFsStat resolves a path the agent printed (absolute, ~/, or relative
// to the pane's folder) and says what kind of file it is.
func (s *Server) handleFsStat(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	cwd := ""
	if paneID := r.URL.Query().Get("pane"); paneID != "" {
		if pane, err := s.findPane(mc, paneID); err == nil {
			cwd = pane.Dir()
		}
	}
	info, err := mc.ResolvePath(cwd, r.URL.Query().Get("path"))
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, info)
}

// GET /api/fs/dirs?machine&path&hidden — a folder's subfolders on the
// machine, for picking a project folder instead of typing it.
func (s *Server) handleFsDirs(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	l, err := mc.ListDirs(r.URL.Query().Get("path"), r.URL.Query().Get("hidden") == "1")
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, l)
}

// handleFsFile serves the bytes. Text is always text/plain whatever the
// extension claims, everything is sandboxed and never cached. Local files
// stream with Range support (video seeks); remote ones are read whole.
func (s *Server) handleFsFile(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	p := r.URL.Query().Get("path")
	if !strings.HasPrefix(p, "/") {
		httpErr(w, 400, fmt.Errorf("absolute path required (resolve it with /api/fs/stat first)"))
		return
	}
	info, err := mc.ResolvePath("", p)
	if err != nil || info.Kind == "missing" || info.Kind == "dir" {
		httpErr(w, 404, fmt.Errorf("not a readable file: %s", p))
		return
	}
	h := w.Header()
	h.Set("Content-Type", info.MIME)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	h.Set("X-Herdr-UI-Kind", info.Kind)
	h.Set("X-Herdr-UI-Size", strconv.FormatInt(info.Size, 10))
	h.Set("X-Herdr-UI-Max", strconv.FormatInt(info.MaxBytes(), 10))
	if info.Kind != "pdf" { // the browser's PDF viewer refuses a sandboxed document
		h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src data:; media-src 'self'; style-src 'unsafe-inline'")
	}
	max := info.MaxBytes()
	if mc.Local {
		f, err := mc.Open(info.Path)
		if err != nil {
			httpErr(w, 500, err)
			return
		}
		defer f.Close()
		fi, _ := f.Stat()
		var mod time.Time
		if fi != nil {
			mod = fi.ModTime()
		}
		h.Set("X-Herdr-UI-Truncated", strconv.FormatBool(info.Size > max))
		http.ServeContent(w, r, filepath.Base(info.Path), mod, &cappedFile{f, min(info.Size, max)})
		return
	}
	data, truncated, err := mc.ReadRemote(info.Path, max)
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	h.Set("X-Herdr-UI-Truncated", strconv.FormatBool(truncated))
	w.Write(data)
}

// cappedFile lets ServeContent stream at most size bytes of a file.
type cappedFile struct {
	f    *os.File
	size int64
}

func (c *cappedFile) Read(b []byte) (int, error) {
	pos, _ := c.f.Seek(0, 1)
	if pos >= c.size {
		return 0, io.EOF
	}
	if rem := c.size - pos; int64(len(b)) > rem {
		b = b[:rem]
	}
	return c.f.Read(b)
}

func (c *cappedFile) Seek(off int64, whence int) (int64, error) {
	if whence == 2 { // from the end of the *capped* file
		return c.f.Seek(c.size+off, 0)
	}
	return c.f.Seek(off, whence)
}

func (s *Server) handleFsDiff(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	p := r.URL.Query().Get("path")
	if !strings.HasPrefix(p, "/") {
		httpErr(w, 400, fmt.Errorf("absolute path required"))
		return
	}
	d, err := mc.GitDiff(p)
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, d)
}

const (
	maxUploadFiles = 4
	maxUploadBytes = 25 << 20
)

// handleFsUpload stores up to four files from the phone or PC in the pane's
// folder (multipart field "file", repeated) and returns their paths for the
// prompt. Nothing is auto-sent: the files are only mentioned once the user
// sends the message.
func (s *Server) handleFsUpload(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	pane, err := s.findPane(mc, r.URL.Query().Get("pane"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadFiles*maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpErr(w, 400, fmt.Errorf("upload too large or malformed: %v", err))
		return
	}
	defer r.MultipartForm.RemoveAll()
	parts := r.MultipartForm.File["file"]
	if len(parts) == 0 || len(parts) > maxUploadFiles {
		httpErr(w, 400, fmt.Errorf("send 1–%d files as \"file\"", maxUploadFiles))
		return
	}
	var out []machines.Upload
	for _, fh := range parts {
		if fh.Size > maxUploadBytes {
			httpErr(w, 413, fmt.Errorf("%s is over %d MB", fh.Filename, maxUploadBytes>>20))
			return
		}
		f, err := fh.Open()
		if err != nil {
			httpErr(w, 400, err)
			return
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			httpErr(w, 400, err)
			return
		}
		up, err := mc.SaveUpload(pane.Dir(), fh.Filename, data)
		if err != nil {
			httpErr(w, 500, err)
			return
		}
		out = append(out, up)
	}
	writeJSON(w, map[string]any{"files": out})
}

// ---- web push ----

func (s *Server) handlePushInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"public_key": s.Push.PublicKey()})
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subscription push.Subscription `json:"subscription"`
		Prefs        *push.Prefs       `json:"prefs"`
		Label        string            `json:"label"`
		OldEndpoint  string            `json:"old_endpoint"` // the browser rotated the subscription
	}
	if !decode(w, r, &req) {
		return
	}
	sub := req.Subscription
	if !strings.HasPrefix(sub.Endpoint, "https://") || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		httpErr(w, 400, fmt.Errorf("a push subscription with an https endpoint and keys is required"))
		return
	}
	if req.Prefs != nil {
		sub.Prefs = *req.Prefs
	} else {
		sub.Prefs = push.Prefs{Input: true}
	}
	sub.Label = req.Label
	s.Push.Subscribe(sub, req.OldEndpoint)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Endpoint == "" {
		httpErr(w, 400, fmt.Errorf("endpoint required"))
		return
	}
	s.Push.Unsubscribe(req.Endpoint)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.Push.SendTest(req.Endpoint); err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ---- input ----

func (s *Server) paneLock(ref push.PaneRef) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paneMu[ref] == nil {
		s.paneMu[ref] = &sync.Mutex{}
	}
	return s.paneMu[ref]
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
	if err := s.submitTo(mc, client, req.Pane, req.Text); err != nil {
		var busy errAgentBusy
		if errors.As(err, &busy) {
			httpErr(w, http.StatusConflict, err)
			return
		}
		rpcErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// errAgentBusy: the agent shows a menu; the message must not land in it.
type errAgentBusy struct{}

func (errAgentBusy) Error() string {
	return "the agent is waiting for an answer — answer its prompt first"
}

// submitTo delivers text to a pane the way the chat does: herdr's own
// submit (bracketed paste, Enter 300ms later) for an agent, typing like a
// keyboard for a shell. Serialized per pane.
func (s *Server) submitTo(mc *machines.Machine, client *herdr.Client, paneID, text string) error {
	lock := s.paneLock(push.PaneRef{Machine: mc.Cfg.ID, Pane: paneID})
	lock.Lock()
	defer lock.Unlock()
	err := client.AgentPrompt(paneID, text)
	switch {
	case err == nil:
		return nil
	case herdr.IsCode(err, "agent_blocked"):
		return errAgentBusy{}
	case herdr.IsCode(err, "agent_not_found") || herdr.IsCode(err, "agent_not_ready"):
		// No agent in front: type like a keyboard. The settle gap lives server-
		// side so a TUI still redrawing doesn't take the Enter as a newline.
		if err := client.PaneSendText(paneID, text); err != nil {
			return err
		}
		time.Sleep(150 * time.Millisecond)
		return client.PaneSendKeys(paneID, []string{"enter"})
	default:
		return err
	}
}

// SubmitToFolder finds the agent running in a machine's folder and gives it
// the text — the scheduler's door. The pane's title names where it went.
// agentPaneIn is the first agent pane of the first workspace in a folder.
func agentPaneIn(snap *herdr.Snapshot, dir string) *herdr.PaneInfo {
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		if workspaceDir(snap, ws) != dir {
			continue
		}
		for j := range snap.Panes {
			if p := &snap.Panes[j]; p.WorkspaceID == ws.WorkspaceID && p.Agent != "" {
				return p
			}
		}
	}
	return nil
}

func (s *Server) SubmitToFolder(machineID, dir, text string) (string, error) {
	mc, err := s.Machines.Get(machineID)
	if err != nil {
		return "", err
	}
	client, _, err := mc.Ready()
	if err != nil {
		return "", err
	}
	snap, _ := s.Watch.Snapshot(machineID)
	if snap == nil {
		return "", fmt.Errorf("%s has no snapshot yet", mc.Label())
	}
	pane := agentPaneIn(snap, dir)
	if pane == nil {
		return "", fmt.Errorf("no agent is running in %s on %s", dir, mc.Label())
	}
	if err := s.submitTo(mc, client, pane.PaneID, text); err != nil {
		return "", err
	}
	return pane.DisplayTitle(), nil
}

// herdr's pane.send_keys grammar: lowercase names, "+" for modifiers
// (observed live: enter, esc, up, down, tab, shift+tab, backspace, ctrl+c).
var allowedKeys = map[string]bool{
	"enter": true, "esc": true, "tab": true, "shift+tab": true, "space": true,
	"up": true, "down": true, "left": true, "right": true,
	"home": true, "end": true, "pageup": true, "pagedown": true,
	"backspace": true, "delete": true,
	"ctrl+c": true, "ctrl+d": true, "ctrl+u": true, "ctrl+z": true,
	"ctrl+r": true, "ctrl+l": true, "ctrl+k": true,
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
	for i, k := range req.Keys {
		k = strings.ToLower(k)
		if !allowedKeys[k] {
			httpErr(w, 400, fmt.Errorf("key %q not allowed", k))
			return
		}
		req.Keys[i] = k
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	lock := s.paneLock(push.PaneRef{Machine: mc.Cfg.ID, Pane: req.Pane})
	lock.Lock()
	defer lock.Unlock()
	if err := client.PaneSendKeys(req.Pane, req.Keys); err != nil {
		rpcErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ---- terminal WebSocket ----

type wsClientMsg struct {
	Type string `json:"type"` // input | resize
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

func (s *Server) handlePaneWS(w http.ResponseWriter, r *http.Request) {
	mc, _, attach, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	paneID := r.URL.Query().Get("id")
	pane, err := s.findPane(mc, paneID)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	// the client's probe of this URL (a plain GET: "would a socket be
	// accepted?") must cost no attach, here or on a peer
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}
	s.Radar.Seen(mc.Cfg.ID, pane.PaneID) // a terminal in view counts as looked at
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))

	// the machine's own herdr-ui, when it runs, holds the one attach: relay
	if pc, err := s.peerFor(mc); err != nil {
		httpErr(w, 502, err)
		return
	} else if pc != nil {
		s.relayPaneWS(w, r, pc, pane.PaneID, cols, rows)
		return
	}
	ch, cancel, err := attach.Subscribe(pane.TerminalID, cols, rows)
	if err != nil {
		// refused because another client holds it — a herdr-ui that started
		// since the last probe? one more look, then the relay
		if pc := s.peerAfterRefusal(mc, err); pc != nil {
			s.relayPaneWS(w, r, pc, pane.PaneID, cols, rows)
			return
		}
	}

	// ANSI output compresses 3-10×; no context takeover keeps it cheap per connection
	c, err2 := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionNoContextTakeover})
	if err2 != nil {
		if err == nil {
			cancel()
		}
		return
	}
	defer c.CloseNow()
	ctx := r.Context()
	c.SetReadLimit(1 << 20)
	if err != nil {
		c.Close(websocket.StatusInternalError, attachRefused(mc, err)) // the reason reaches the viewer
		return
	}
	defer cancel()

	// The writer is stopped explicitly when the read loop ends: on a hijacked
	// connection r.Context() stays alive until this handler returns, so
	// without this the goroutine (and the attach subscription) would linger
	// until the pane's next byte.
	wctx, wcancel := context.WithCancel(ctx)
	defer wcancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					c.Close(websocket.StatusNormalClosure, "terminal detached")
					return
				}
				// coalesce whatever else is already queued into one frame
				for more := true; more; {
					select {
					case extra, ok := <-ch:
						if !ok {
							more = false
						} else {
							// the chunk is shared with every viewer: a full-slice
							// expression makes this append copy before it writes
							data = append(data[:len(data):len(data)], extra...)
						}
					default:
						more = false
					}
				}
				if err := c.Write(wctx, websocket.MessageBinary, data); err != nil {
					return
				}
			case <-wctx.Done():
				return
			}
		}
	}()

	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		if typ != websocket.MessageText {
			continue
		}
		var msg wsClientMsg
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "input":
			if msg.Data != "" {
				if err := attach.Write(pane.TerminalID, []byte(msg.Data)); err != nil {
					log.Printf("input %s/%s: %v", mc.Cfg.ID, paneID, err)
				}
			}
		case "resize":
			if err := attach.Resize(pane.TerminalID, msg.Cols, msg.Rows); err != nil {
				log.Printf("resize %s/%s: %v", mc.Cfg.ID, paneID, err)
			}
		}
	}
	wcancel()
	<-done
}

// ---- static ----

func (s *Server) staticHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.UI))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(s.UI, path); err != nil {
			// SPA fallback
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// ListenAndServe runs the server until ctx is cancelled.
func ListenAndServe(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: h}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
