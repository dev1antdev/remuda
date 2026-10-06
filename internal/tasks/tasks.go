// Package tasks connects herdr-ui to the user's own Vikunja. herdr-ui keeps
// no task data: the Vikunja URL and an API token live in the state dir, and
// the browser talks to Vikunja's REST API through one proxy route that adds
// the token — so the token never reaches a browser, and one port (herdr-ui's)
// is all a phone or a port forward needs.
package tasks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/state"
)

const configFile = "vikunja.json"

// Config is what the user gave us: where Vikunja is and how to speak as them.
type Config struct {
	URL   string `json:"url"`   // https://todo.example.com — no /api/v1
	Token string `json:"token"` // an API token from Vikunja's settings
}

// Status is what the UI may know: everything but the token.
type Status struct {
	Configured bool   `json:"configured"`
	URL        string `json:"url,omitempty"`
	Version    string `json:"version,omitempty"` // Vikunja's, from /info
	Error      string `json:"error,omitempty"`   // the last connection problem
}

type Service struct {
	dir    string
	client *http.Client

	mu      sync.Mutex
	cfg     Config
	version string
	lastErr string

	patchMu sync.Mutex // one PATCH at a time: its GET+POST must not interleave
}

func New(dir string) *Service {
	s := &Service{dir: dir, client: &http.Client{Timeout: 30 * time.Second}}
	if b, err := os.ReadFile(filepath.Join(dir, configFile)); err == nil {
		_ = json.Unmarshal(b, &s.cfg)
	}
	return s
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.URL == "" {
		return Status{}
	}
	return Status{Configured: true, URL: s.cfg.URL, Version: s.version, Error: s.lastErr}
}

// Configure checks the URL and token against Vikunja before keeping them:
// /info says it is a Vikunja and which one, /projects says the token works.
func (s *Service) Configure(rawURL, token string) (Status, error) {
	u, err := normalizeURL(rawURL)
	if err != nil {
		return Status{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Status{}, errors.New("an API token is required")
	}
	cfg := Config{URL: u, Token: token}
	version, err := s.probe(cfg)
	if err != nil {
		return Status{}, err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := state.WriteFile(filepath.Join(s.dir, configFile), append(b, '\n')); err != nil {
		return Status{}, err
	}
	s.mu.Lock()
	s.cfg, s.version, s.lastErr = cfg, version, ""
	s.mu.Unlock()
	return s.Status(), nil
}

// Disconnect forgets the URL and token.
func (s *Service) Disconnect() error {
	s.mu.Lock()
	s.cfg, s.version, s.lastErr = Config{}, "", ""
	s.mu.Unlock()
	err := os.Remove(filepath.Join(s.dir, configFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Check re-probes a stored configuration (e.g. on startup) so the UI can
// show a stale token or an unreachable host before the first click.
func (s *Service) Check() Status {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if cfg.URL == "" {
		return Status{}
	}
	version, err := s.probe(cfg)
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.version, s.lastErr = version, ""
	}
	s.mu.Unlock()
	return s.Status()
}

func (s *Service) probe(cfg Config) (string, error) {
	res, err := s.client.Get(cfg.URL + "/api/v1/info")
	if err != nil {
		return "", fmt.Errorf("cannot reach %s: %w", cfg.URL, err)
	}
	defer res.Body.Close()
	var info struct {
		Version string `json:"version"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&info) != nil || info.Version == "" {
		return "", fmt.Errorf("%s does not answer like a Vikunja (/api/v1/info → %s)", cfg.URL, res.Status)
	}
	req, _ := http.NewRequest(http.MethodGet, cfg.URL+"/api/v1/projects?per_page=1", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	res2, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach %s: %w", cfg.URL, err)
	}
	defer res2.Body.Close()
	switch res2.StatusCode {
	case http.StatusOK:
		return info.Version, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", errors.New("Vikunja rejected the token (it needs at least projects: read_all)")
	default:
		return "", fmt.Errorf("Vikunja answered %s to /projects", res2.Status)
	}
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("the Vikunja URL is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("not a URL: %q", raw)
	}
	u.Path = strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v1"), "/")
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

// Proxy forwards one API call to Vikunja as the user: `path` is the part
// after /api/v1/, the method, query and body pass through, the token is
// added here. PATCH is ours: Vikunja's task update replaces the whole task
// (a body of {done:true} would wipe the due date), so a PATCH fetches the
// task, merges the fields and sends the result.
func (s *Service) Proxy(w http.ResponseWriter, r *http.Request, path string) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if cfg.URL == "" {
		http.Error(w, `{"error":"Vikunja is not connected"}`, http.StatusConflict)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete, http.MethodPatch:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	method := r.Method
	if method == http.MethodPatch {
		// two merges of one task would each carry the other's stale fields
		s.patchMu.Lock()
		defer s.patchMu.Unlock()
		if body, err = s.merged(cfg, path, body); err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		method = http.MethodPost
	}
	target := cfg.URL + "/api/v1/" + strings.TrimPrefix(path, "/")
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), method, target, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.client.Do(req)
	if err != nil {
		s.setErr("Vikunja unreachable: " + err.Error())
		jsonError(w, http.StatusBadGateway, "Vikunja unreachable: "+err.Error())
		return
	}
	defer res.Body.Close()
	// Vikunja's failures become herdr-ui's {"error"}: the page treats a 401
	// as its own sign-in gone (and reloads), so a rejected token must not
	// arrive as one; and its {"code","message"} body is not ours to show raw
	if res.StatusCode >= 400 {
		msg := vikunjaMessage(res)
		switch res.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			msg = "Vikunja rejected the token (expired, revoked, or missing a scope): " + msg
			s.setErr(msg)
			jsonError(w, http.StatusConflict, msg)
		default:
			jsonError(w, res.StatusCode, msg)
		}
		return
	}
	for _, h := range []string{"Content-Type", "X-Pagination-Total-Pages", "X-Pagination-Result-Count"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

func (s *Service) setErr(msg string) {
	s.mu.Lock()
	s.lastErr = msg
	s.mu.Unlock()
}

// vikunjaMessage reads the message out of a Vikunja error body.
func vikunjaMessage(res *http.Response) string {
	var body struct {
		Message string `json:"message"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&body) == nil && body.Message != "" {
		return body.Message
	}
	return res.Status
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// merged returns the current object at path with the patch's fields applied.
func (s *Service) merged(cfg Config, path string, patch []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(patch, &fields); err != nil {
		return nil, errors.New("a PATCH body must be a JSON object")
	}
	req, _ := http.NewRequest(http.MethodGet, cfg.URL+"/api/v1/"+strings.TrimPrefix(path, "/"), nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Vikunja answered %s", vikunjaMessage(res))
	}
	var current map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&current); err != nil {
		return nil, err
	}
	for k, v := range fields {
		current[k] = v
	}
	return json.Marshal(current)
}
