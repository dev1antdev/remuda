// Package tsnetx puts remuda on the tailnet by itself: with -tailscale
// the binary joins as its own node (tsnet, userspace networking, no
// tailscaled needed), gets a MagicDNS name and a real certificate, and
// serves HTTPS there — a phone opens https://remuda.<tailnet>.ts.net
// with no `tailscale serve` or SSH tunnel beside it. The loopback
// listener stays for the desk.
//
// The first run needs a login: the status carries the URL to visit (it
// is logged, and the UI shows it as a toast on the loopback side) unless
// TS_AUTHKEY is set. State lives under the remuda state dir.
package tsnetx

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

// Status is where the node stands, for /api/roster and the log.
type Status struct {
	State    string `json:"state"`               // starting | needs_login | running | error
	LoginURL string `json:"login_url,omitempty"` // visit it to join the tailnet
	URL      string `json:"url,omitempty"`       // https://<name>.<tailnet>.ts.net once running
	Hostname string `json:"hostname,omitempty"`
	Error    string `json:"error,omitempty"`
}

// quietLogf keeps the lines a person needs — a certificate that could not
// be fetched, an error — and drops the rest of tsnet's chatter.
func quietLogf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	l := strings.ToLower(line)
	if strings.Contains(l, "cert") || strings.Contains(l, "acme") || strings.Contains(l, "error") || strings.Contains(l, "failed") {
		log.Printf("tsnet: %s", line)
	}
}

// Node is the running tsnet server.
type Node struct {
	srv *tsnet.Server
	mu  sync.Mutex
	st  Status
}

// Start joins the tailnet in the background and serves h over HTTPS on
// :443 (and redirects :80) once the node is up. It returns at once; the
// status tells how far the node got.
func Start(stateDir, hostname, authKey string, h http.Handler) *Node {
	n := &Node{
		srv: &tsnet.Server{
			Dir:      filepath.Join(stateDir, "tailscale"),
			Hostname: hostname,
			AuthKey:  authKey,
			Logf:     quietLogf, // tsnet is chatty; certificate and error lines still get through
			UserLogf: func(string, ...any) {},
		},
		st: Status{State: "starting", Hostname: hostname},
	}
	go n.run(h)
	return n
}

func (n *Node) set(st Status) {
	n.mu.Lock()
	n.st = st
	n.mu.Unlock()
}

// Status is the node's current state.
func (n *Node) Status() Status {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.st
}

// Close leaves the tailnet.
func (n *Node) Close() error { return n.srv.Close() }

func (n *Node) run(h http.Handler) {
	if err := n.srv.Start(); err != nil {
		n.set(Status{State: "error", Error: err.Error(), Hostname: n.srv.Hostname})
		log.Printf("tailscale: %v", err)
		return
	}
	lc, err := n.srv.LocalClient()
	if err != nil {
		n.set(Status{State: "error", Error: err.Error(), Hostname: n.srv.Hostname})
		return
	}
	// until the node is up: the login URL, when the control plane gives one
	logged := ""
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		st, err := lc.Status(ctx)
		cancel()
		if err == nil && st.BackendState == "Running" {
			break
		}
		if err == nil && st.AuthURL != "" {
			n.set(Status{State: "needs_login", LoginURL: st.AuthURL, Hostname: n.srv.Hostname})
			if logged != st.AuthURL {
				logged = st.AuthURL
				log.Printf("tailscale: not logged in yet — visit %s", st.AuthURL)
			}
		}
		time.Sleep(2 * time.Second)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	st, err := n.srv.Up(ctx)
	cancel()
	if err != nil {
		n.set(Status{State: "error", Error: err.Error(), Hostname: n.srv.Hostname})
		log.Printf("tailscale: up: %v", err)
		return
	}
	dns := strings.TrimSuffix(st.Self.DNSName, ".")
	url := "https://" + dns
	n.set(Status{State: "running", URL: url, Hostname: n.srv.Hostname})
	log.Printf("tailscale: serving %s", url)

	// :80 → https; :443 with the tailnet's certificate (needs HTTPS and
	// MagicDNS enabled in the tailnet's DNS settings)
	go func() {
		ln, err := n.srv.Listen("tcp", ":80")
		if err != nil {
			return
		}
		_ = http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, url+r.URL.RequestURI(), http.StatusTemporaryRedirect)
		}))
	}()
	ln, err := n.srv.ListenTLS("tcp", ":443")
	if err != nil {
		n.set(Status{State: "error", Error: "https: " + err.Error() + " (enable HTTPS certificates in the tailnet's DNS settings)", URL: url, Hostname: n.srv.Hostname})
		log.Printf("tailscale: https: %v", err)
		return
	}
	n.serve(ln, h)
}

func (n *Node) serve(ln net.Listener, h http.Handler) {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		n.set(Status{State: "error", Error: err.Error(), Hostname: n.srv.Hostname})
		log.Printf("tailscale: serve: %v", err)
	}
}
