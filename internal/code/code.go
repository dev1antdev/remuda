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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
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
	themed map[string]string // the theme name each machine's editor was last dressed in
}

type check struct {
	at time.Time
	st Status
}

func New(port int) *Service {
	return &Service{Port: port, client: &http.Client{Timeout: 3 * time.Second}, checks: map[string]check{}, themed: map[string]string{}}
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

// Open asks the machine's code-server to open a file in the editor already on screen — its own CLI over its IPC socket,
// so the frame never reloads and the editor keeps its state. The CLI exits 0 even when no editor listens, so its words
// are read for an error.
func (s *Service) Open(mc *machines.Machine, path string, line int) error {
	out, err := shell.Text(mc, OpenScript(path, line), 15*time.Second)
	if err != nil {
		return fmt.Errorf("code-server on %s: %w", mc.Label(), err)
	}
	if said := lastLine(out); strings.Contains(strings.ToLower(said), "error") {
		return fmt.Errorf("code-server on %s: %s", mc.Label(), said)
	}
	return nil
}

// OpenScript is the command behind Open: code-server's CLI with --reuse-window, from PATH or where the installer puts it;
// a line rides on the path as file:line, which the CLI reads the way VS Code's does.
func OpenScript(path string, line int) string {
	target := path
	if line > 0 {
		target = fmt.Sprintf("%s:%d", path, line)
	}
	return script.PATH + `; B=$(command -v code-server 2>/dev/null || echo "$HOME/.local/opt/code-server/bin/code-server"); "$B" --reuse-window ` + script.Quote(target) + ` 2>&1`
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// WorkspaceFile is where a machine's one workspace file for the editor lives, under its home.
const WorkspaceFile = ".config/code-server/remuda.code-workspace"

// Workspace writes every folder herdr has a workspace in on the machine into one .code-workspace file and names its path:
// the editor opened on it shows them all, and since it watches the file, a later write changes its folders in place.
func (s *Service) Workspace(mc *machines.Machine, folders []string) (string, error) {
	home := mc.Home()
	if home == "" {
		return "", fmt.Errorf("the home folder of %s is not known yet", mc.Label())
	}
	path := home + "/" + WorkspaceFile
	data := WorkspaceJSON(folders)
	cmd := "mkdir -p " + script.Quote(path[:strings.LastIndex(path, "/")]) + " && printf %s " + script.Quote(string(data)) + " > " + script.Quote(path)
	if _, err := mc.Run(cmd, 10*time.Second); err != nil {
		return "", fmt.Errorf("writing the workspace file on %s: %w", mc.Label(), err)
	}
	return path, nil
}

// WorkspaceJSON is the file's content: the folders by their last name, each named so the explorer reads well.
func WorkspaceJSON(folders []string) []byte {
	type folder struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	list := make([]folder, 0, len(folders))
	for _, dir := range folders {
		dir = strings.TrimRight(dir, "/")
		if dir == "" {
			continue
		}
		list = append(list, folder{Name: dir[strings.LastIndex(dir, "/")+1:], Path: dir})
	}
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) || (list[i].Name == list[j].Name && list[i].Path < list[j].Path)
	})
	out, _ := json.MarshalIndent(map[string]any{"folders": list, "settings": map[string]any{}}, "", "  ")
	return append(out, '\n')
}

// ThemeName is the Catppuccin theme of the page's theme, as the extension names it.
func ThemeName(theme string) string {
	if theme == "light" {
		return "Catppuccin Latte"
	}
	return "Catppuccin Frappé"
}

// Theme dresses the machine's editor in the page's theme: Catppuccin's extension installed once from Open VSX, then
// workbench.colorTheme in code-server's settings.json, which the editor applies as the file changes — but an editor
// already open knows nothing of an extension installed under it, so installed says the frame must load again. A
// machine is dressed again only when the theme differs from the one last written while remuda runs.
func (s *Service) Theme(mc *machines.Machine, theme string) (installed bool, err error) {
	name := ThemeName(theme)
	s.mu.Lock()
	worn := s.themed[mc.ID()]
	s.mu.Unlock()
	if worn == name {
		return false, nil
	}
	read := script.PATH + `; B=$(command -v code-server 2>/dev/null || echo "$HOME/.local/opt/code-server/bin/code-server"); if ! "$B" --list-extensions 2>/dev/null | grep -qi '^catppuccin.catppuccin-vsc$'; then "$B" --install-extension catppuccin.catppuccin-vsc >/dev/null 2>&1 && echo ` + installedMark + `; fi; f="$HOME/.local/share/code-server/User/settings.json"; mkdir -p "$(dirname "$f")"; if [ -f "$f" ]; then cat "$f"; else echo '{}'; fi`
	current, err := shell.Text(mc, read, 120*time.Second)
	if err != nil {
		return false, fmt.Errorf("code-server's settings on %s: %w", mc.Label(), err)
	}
	if strings.HasPrefix(current, installedMark+"\n") {
		installed = true
		current = strings.TrimPrefix(current, installedMark+"\n")
	}
	next := WithTheme([]byte(current), name)
	write := `f="$HOME/.local/share/code-server/User/settings.json"; printf %s ` + script.Quote(string(next)) + ` > "$f"`
	if _, err := mc.Run(write, 10*time.Second); err != nil {
		return installed, fmt.Errorf("writing code-server's settings on %s: %w", mc.Label(), err)
	}
	s.mu.Lock()
	s.themed[mc.ID()] = name
	s.mu.Unlock()
	return installed, nil
}

// installedMark is the first line of the theme script's answer when it had to install the extension.
const installedMark = "REMUDA-INSTALLED-THEME"

var themeLineRe = regexp.MustCompile(`"workbench\.colorTheme"\s*:\s*"[^"]*"`)

// WithTheme is settings.json with workbench.colorTheme set to name: merged as JSON when the file parses, else the one
// line replaced or added — VS Code allows comments and trailing commas there, which a person may have written.
func WithTheme(settings []byte, name string) []byte {
	var m map[string]any
	if err := json.Unmarshal(settings, &m); err == nil && m != nil {
		m["workbench.colorTheme"] = name
		out, _ := json.MarshalIndent(m, "", "  ")
		return append(out, '\n')
	}
	text := string(settings)
	if themeLineRe.MatchString(text) {
		return []byte(themeLineRe.ReplaceAllString(text, `"workbench.colorTheme": "`+name+`"`))
	}
	if i := strings.Index(text, "{"); i >= 0 {
		return []byte(text[:i+1] + "\n  \"workbench.colorTheme\": \"" + name + "\",\n" + text[i+1:])
	}
	return []byte("{\n  \"workbench.colorTheme\": \"" + name + "\"\n}\n")
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
