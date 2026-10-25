// Package code puts a machine's code-server (VS Code in the browser) behind
// remuda's token guard: each machine runs its own on 127.0.0.1:<port> with auth
// off, reached at /code/<machine>/ straight to the port or over the ssh master.
package code

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/machines"
)

const DefaultPort = 7320

type Status struct {
	Available bool   `json:"available"`
	Port      int    `json:"port"`
	Authority string `json:"authority,omitempty"` // the remote authority the workbench names its files by ("remote" since code-server 4.9x, the host before)
	Version   string `json:"version,omitempty"`   // code-server's own, off the same page
	Error     string `json:"error,omitempty"`
}

type Service struct {
	Port   int
	client *http.Client

	mu     sync.Mutex
	checks map[string]check
}

type check struct {
	at time.Time
	st Status
}

func New(port int) *Service {
	return &Service{Port: port, client: &http.Client{Timeout: 3 * time.Second}, checks: map[string]check{}}
}

// Status says whether the machine's code-server answers; a good verdict is cached 15s so a polling page does not probe every time.
func (s *Service) Status(mc *machines.Machine) Status {
	s.mu.Lock()
	if c, ok := s.checks[mc.ID()]; ok && time.Since(c.at) < 15*time.Second && c.st.Available {
		s.mu.Unlock()
		return c.st
	}
	s.mu.Unlock()
	st := s.probe(mc)
	s.mu.Lock()
	s.checks[mc.ID()] = check{at: time.Now(), st: st}
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
	// code-server's healthz says "alive" with a client connected and "expired" without one: both are an answer
	if res.StatusCode != http.StatusOK || body.Status == "" {
		st.Error = fmt.Sprintf("%s:%d answers, but not like a code-server (%s)", mc.Label(), s.Port, res.Status)
		return st
	}
	st.Available = true
	st.Authority, st.Version = s.page(addr)
	return st
}

// page reads the workbench page code-server serves for the remote authority — a file the page asks the editor to open
// must carry it, or the editor cannot resolve the resource — and code-server's version. "" for what the page does not say.
func (s *Service) page(addr string) (authority, version string) {
	res, err := s.client.Get("http://" + addr + "/")
	if err != nil {
		return "", ""
	}
	defer res.Body.Close()
	page, err := io.ReadAll(io.LimitReader(res.Body, 512*1024))
	if err != nil {
		return "", ""
	}
	return settingOf(page, "remoteAuthority"), settingOf(page, "codeServerVersion")
}

// settingOf finds one string setting in the workbench page's data-settings, written with its quotes escaped as &quot;
func settingOf(page []byte, name string) string {
	re := regexp.MustCompile(`(?:&quot;|")` + regexp.QuoteMeta(name) + `(?:&quot;|"):(?:&quot;|")([^&"]+)(?:&quot;|")`)
	m := re.FindSubmatch(page)
	if m == nil {
		return ""
	}
	return html.UnescapeString(string(m[1]))
}

// Forget drops a cached verdict (after an install, say).
func (s *Service) Forget(machineID string) {
	s.mu.Lock()
	delete(s.checks, machineID)
	s.mu.Unlock()
}

// Proxy serves one request to the machine's code-server, with the mount prefix (/code/<machine>) stripped on the way in and put back on absolute redirects.
// WebSocket upgrades pass through: the editor's own connection is one.
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
			// remuda's token never reaches the machine: code-server needs no auth, and whatever listens on that port would read it
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("Cookie")
			for _, c := range pr.In.Cookies() {
				if c.Name != auth.CookieName {
					pr.Out.AddCookie(c)
				}
			}
			path := strings.TrimPrefix(pr.In.URL.Path, prefix)
			if path == "" {
				path = "/"
			}
			pr.Out.URL.Path = path
			pr.Out.URL.RawPath = ""
		},
		ModifyResponse: func(res *http.Response) error {
			// the editor is framed by remuda, and by nothing else
			res.Header.Set("X-Frame-Options", "SAMEORIGIN")
			if loc := res.Header.Get("Location"); strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, prefix) {
				res.Header.Set("Location", prefix+loc)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.Forget(mc.ID())
			if r.Context().Err() == context.Canceled {
				return
			}
			http.Error(w, "code-server: "+err.Error(), http.StatusBadGateway)
		},
		FlushInterval: -1, // the editor streams
	}
	rp.ServeHTTP(w, r)
}
