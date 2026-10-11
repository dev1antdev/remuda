// Package code puts a machine's code-server (VS Code in the browser) behind
// remuda: each machine runs its own on 127.0.0.1:<port>, auth off, and
// remuda's bearer-token guard is the door — the browser reaches it at
// /code/<machine>/, locally straight to the port, remotely through a port
// forwarded over the same ssh master the roster uses. One origin, one token,
// nothing else to expose.
package code

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
)

const DefaultPort = 7320

type Status struct {
	Available bool   `json:"available"`
	Port      int    `json:"port"`
	Error     string `json:"error,omitempty"`
}

type Service struct {
	Port   int
	client *http.Client

	mu     sync.Mutex
	checks map[string]check // by machine id
}

type check struct {
	at time.Time
	st Status
}

func New(port int) *Service {
	return &Service{Port: port, client: &http.Client{Timeout: 3 * time.Second}, checks: map[string]check{}}
}

// Status says whether the machine's code-server answers; a good verdict is
// cached briefly so a page polling it does not probe every few seconds.
func (s *Service) Status(mc *machines.Machine) Status {
	s.mu.Lock()
	if c, ok := s.checks[mc.Cfg.ID]; ok && time.Since(c.at) < 15*time.Second && c.st.Available {
		s.mu.Unlock()
		return c.st
	}
	s.mu.Unlock()
	st := s.probe(mc)
	s.mu.Lock()
	s.checks[mc.Cfg.ID] = check{at: time.Now(), st: st}
	s.mu.Unlock()
	return st
}

func (s *Service) probe(mc *machines.Machine) Status {
	st := Status{Port: s.Port}
	addr, err := mc.CodeAddr(s.Port)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	res, err := s.client.Get("http://" + addr + "/healthz")
	if err != nil {
		st.Error = fmt.Sprintf("code-server is not answering on %s:%d", mc.Label(), s.Port)
		return st
	}
	defer res.Body.Close()
	var body struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&body)
	// "alive" with a client connected, "expired" without one: both are a
	// code-server answering
	if res.StatusCode != http.StatusOK || body.Status == "" {
		st.Error = fmt.Sprintf("%s:%d answers, but not like a code-server (%s)", mc.Label(), s.Port, res.Status)
		return st
	}
	st.Available = true
	return st
}

// Forget drops a cached verdict (after an install, say).
func (s *Service) Forget(machineID string) {
	s.mu.Lock()
	delete(s.checks, machineID)
	s.mu.Unlock()
}

// Proxy serves one request to the machine's code-server. `prefix` is the
// mount (/code/<machine>) the browser used, stripped on the way in and put
// back on any absolute redirect on the way out; WebSocket upgrades pass
// through (the editor's own connection is one).
func (s *Service) Proxy(w http.ResponseWriter, r *http.Request, mc *machines.Machine, prefix string) {
	addr, err := mc.CodeAddr(s.Port)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	target := &url.URL{Scheme: "http", Host: addr}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = pr.In.Host // code-server reads the Host it was asked for
			pr.SetXForwarded()
			path := strings.TrimPrefix(pr.In.URL.Path, prefix)
			if path == "" {
				path = "/"
			}
			pr.Out.URL.Path = path
			pr.Out.URL.RawPath = ""
		},
		ModifyResponse: func(res *http.Response) error {
			// the editor is framed by remuda
			res.Header.Del("X-Frame-Options")
			if loc := res.Header.Get("Location"); strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, prefix) {
				res.Header.Set("Location", prefix+loc)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.Forget(mc.Cfg.ID)
			if r.Context().Err() == context.Canceled {
				return
			}
			http.Error(w, "code-server: "+err.Error(), http.StatusBadGateway)
		},
		FlushInterval: -1, // the editor streams
	}
	rp.ServeHTTP(w, r)
}
