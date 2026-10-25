// Package server is remuda's HTTP and WebSocket adapter: every route is a
// thin handler that reads the request, asks one service and writes the
// answer. The roster and chat go over REST, live terminal bytes over a
// WebSocket, and /api/health and /api/login are the only two routes
// outside the token guard.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/budget"
	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/code"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/live"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/prefs"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/quick"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/said"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/tsnetx"
	"github.com/dev1antdev/remuda/internal/updates"
	"github.com/dev1antdev/remuda/internal/usage"
	buildversion "github.com/dev1antdev/remuda/internal/version"
	"github.com/dev1antdev/remuda/internal/watch"
)

// Server holds the services the routes answer from; the app fills it in and hands out Handler.
type Server struct {
	Machines  *machines.Manager
	Watch     *watch.Watcher
	Input     *sessions.Input
	Chat      *chat.Service
	Updates   *updates.Service
	Tasks     *tasks.Store
	Launcher  *tasks.Launcher
	Push      *push.Service
	Feed      *feed.Service
	Radar     *radar.Tracker
	Said      *said.Tracker
	PRs       *prstatus.Tracker
	Recap     *recap.Service
	Usage     *usage.Tracker
	Budgets   *budget.Service
	Health    *health.Collector
	Review    *review.Store
	Schedules *schedule.Store
	Quick     *quick.Store
	Prefs     *prefs.Store
	Code      *code.Service
	Tailscale func() tsnetx.Status
	Guard     *auth.Guard
	UI        fs.FS
	Live      *live.Hub // the page's event stream, nil without one

	streams termStreams
}

// Handler mounts every route: each context registers its own, the static page comes last, and the guard wraps all but two.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	s.rosterRoutes(api)
	s.machineRoutes(api)
	s.sessionRoutes(api)
	s.chatRoutes(api)
	s.fileRoutes(api)
	s.pushRoutes(api)
	s.searchRoutes(api)
	s.usageRoutes(api)
	s.budgetRoutes(api)
	s.quickRoutes(api)
	s.prefsRoutes(api)
	s.scheduleRoutes(api)
	s.reviewRoutes(api)
	s.codeRoutes(api)
	s.updateRoutes(api)
	s.tasksRoutes(api)
	s.terminalRoutes(api)
	s.screenRoutes(api)
	s.questionRoutes(api)
	s.feedRoutes(api)
	s.recapRoutes(api)
	s.liveRoutes(api)
	s.deviceRoutes(api)
	api.Handle("GET /", s.staticHandler())

	// the only two routes outside the token guard
	root := http.NewServeMux()
	root.HandleFunc("GET /api/health", s.handleHealth)
	root.HandleFunc("POST /api/login", s.Guard.HandleLogin)
	root.Handle("/", s.Guard.Middleware(api))
	return secureHeaders(csrfGuard(root))
}

// handleHealth answers from the watcher's cache, unauthenticated: it never asks herdr and says only what the peer probe needs.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	snap, err := s.Watch.Snapshot(machines.LocalID)
	if snap == nil || err != nil {
		writeJSON(w, map[string]any{"ok": true, "herdr": false})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "herdr": true, "version": buildversion.Build})
}

// appCSP is what the page may load and do; inline styles because xterm and React set them on
// elements, ws:/wss: spelled out because some browsers do not read 'self' for WebSockets.
const appCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; " +
	"font-src 'self' data:; connect-src 'self' ws: wss:; worker-src 'self'; frame-src 'self'; frame-ancestors 'self'; object-src 'none'; base-uri 'none'; form-action 'self'"

// secureHeaders puts nosniff, Referrer-Policy, X-Frame-Options, HSTS over TLS and the page's CSP on every response but the code-server proxy's and the file viewer's bytes.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if !strings.HasPrefix(r.URL.Path, "/code/") {
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			if !strings.HasPrefix(r.URL.Path, "/api/fs/file") {
				h.Set("Content-Security-Policy", appCSP)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// csrfGuard refuses cross-origin mutations and WebSocket upgrades by Sec-Fetch-Site and Origin.
// The cookie's SameSite=Lax is per site, not per origin: another port on the same host would get it.
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

// ---- the helpers every handler uses ----

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

// rpcErr reports a failed herdr call: herdr's own refusals are the caller's problem (4xx), anything else the bridge's (502).
func rpcErr(w http.ResponseWriter, err error) {
	var he *herdr.Error
	if errors.As(err, &he) {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	httpErr(w, http.StatusBadGateway, err)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpErr(w, 400, fmt.Errorf("bad request body: %v", err))
		return false
	}
	return true
}

// machineFor finds the request's machine: ?machine=, the X-Remuda-Machine header, or the id a POST body carried; empty means local.
func (s *Server) machineFor(r *http.Request, bodyID string) (*machines.Machine, error) {
	id := r.URL.Query().Get("machine")
	if id == "" {
		id = r.Header.Get("X-Remuda-Machine")
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

// findPane is a pane from the watcher's cache, or why not.
func (s *Server) findPane(mc *machines.Machine, paneID string) (*herdr.PaneInfo, error) {
	snap, err := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return nil, err
	}
	if pane := snap.Pane(paneID); pane != nil {
		return pane, nil
	}
	return nil, fmt.Errorf("pane %s not found", paneID)
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
	// header and idle timeouts only: WebSockets and the 25 s long-poll live longer than any read or write deadline
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
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
