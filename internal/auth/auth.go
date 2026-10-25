// Package auth is deliberately small: one bearer token, required on every
// request behind the guard. No accounts, no third parties. The token lives in
// a 0600 file in the state dir; a signed-in browser keeps a device cookie with a
// secret of its own (devices.go), never the token itself.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

const CookieName = "remuda_token"

// LoadOrCreateToken returns REMUDA_TOKEN when set (32 characters or more), else the token stored at <state dir>/token, creating it on first run.
func LoadOrCreateToken(dir string) (token string, created bool, err error) {
	if t := strings.TrimSpace(os.Getenv("REMUDA_TOKEN")); t != "" {
		if len(t) < 32 {
			return "", false, fmt.Errorf("REMUDA_TOKEN is %d characters; 32 or more, please", len(t))
		}
		return t, false, nil
	}
	path := filepath.Join(dir, "token")
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, false, nil
		}
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", false, err
	}
	t := hex.EncodeToString(buf)
	if err := state.WriteFile(path, []byte(t+"\n")); err != nil {
		return "", false, err
	}
	return t, true, nil
}

type Guard struct {
	token   string
	host    string
	Devices *Devices // the signed-in browsers; nil keeps the token itself in the cookie, as before devices
	stale   sync.Map // ids of revoked devices whose cookie was already logged
}

// NewGuard guards with one token; with a Devices store every signed-in browser gets a secret of its own instead.
func NewGuard(token string, devices *Devices) *Guard {
	h, _ := os.Hostname()
	return &Guard{token: token, host: strings.TrimSuffix(h, ".local"), Devices: devices}
}

func (g *Guard) ok(presented string) bool {
	return presented != "" &&
		subtle.ConstantTimeCompare([]byte(presented), []byte(g.token)) == 1
}

// caller is who a request comes from: a device, the bearer token (no device), a legacy cookie holding the token (to upgrade),
// a device no longer signed in (stale), or nobody.
type caller struct {
	device  string
	ok      bool
	upgrade bool
	stale   bool
}

func (g *Guard) authenticate(r *http.Request) caller {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return caller{ok: g.ok(strings.TrimPrefix(h, "Bearer "))}
	}
	c, err := r.Cookie(CookieName)
	if err != nil {
		return caller{}
	}
	if IsDeviceCookie(c.Value) {
		if g.Devices == nil {
			return caller{}
		}
		device, ok := g.Devices.Check(c.Value)
		if !ok {
			return caller{stale: true}
		}
		return caller{device: device.ID, ok: true}
	}
	if !g.ok(c.Value) {
		return caller{}
	}
	return caller{ok: true, upgrade: g.Devices != nil}
}

type deviceKey struct{}

// DeviceID is the signed-in device a request came from, "" for a bearer caller.
func DeviceID(ctx context.Context) string {
	id, _ := ctx.Value(deviceKey{}).(string)
	return id
}

// Middleware rejects unauthenticated requests; a valid ?token= (the pairing link) signs the browser in and is stripped from the URL.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if qt := r.URL.Query().Get("token"); qt != "" {
			if !g.ok(qt) {
				g.refused(r)
			} else {
				if who := g.authenticate(r); who.device == "" { // a browser already signed in keeps its device
					if !g.signIn(w, r) {
						return
					}
				}
				q := r.URL.Query()
				q.Del("token")
				u := *r.URL
				u.RawQuery = q.Encode()
				http.Redirect(w, r, u.String(), http.StatusSeeOther)
				return
			}
		}
		who := g.authenticate(r)
		if who.ok {
			if who.upgrade && !g.signIn(w, r) {
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), deviceKey{}, who.device)))
			return
		}
		if who.stale {
			g.staleCookie(w, r)
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		g.serveLogin(w)
	})
}

// HandleLogin signs the browser in with a posted token.
func (g *Guard) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	t := strings.TrimSpace(r.PostFormValue("token"))
	if !g.ok(t) {
		g.refused(r)
		http.Error(w, "wrong token", http.StatusForbidden)
		return
	}
	if !g.signIn(w, r) {
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// signIn sets the cookie a browser keeps: a device of its own, or the token itself without a store; false when the device could not be kept (answered).
func (g *Guard) signIn(w http.ResponseWriter, r *http.Request) bool {
	if g.Devices == nil {
		g.setCookie(w, r, g.token)
		return true
	}
	cookie, _, err := g.Devices.Mint(ipOf(r), r.UserAgent())
	if err != nil {
		log.Printf("auth: could not keep the device: %v", err)
		http.Error(w, "could not sign in", http.StatusInternalServerError)
		return false
	}
	g.setCookie(w, r, cookie)
	return true
}

// SignOut drops the cookie of the device a request came from, after the device was revoked.
func (g *Guard) SignOut(w http.ResponseWriter, r *http.Request) {
	g.clearCookie(w, r)
}

// staleCookie is a device no longer signed in: its cookie is dropped and the fact logged once, with no wait (a page of the person's own must not be slowed).
func (g *Guard) staleCookie(w http.ResponseWriter, r *http.Request) {
	g.clearCookie(w, r)
	c, _ := r.Cookie(CookieName)
	id := c.Value
	if rest, ok := strings.CutPrefix(id, cookiePrefix); ok {
		id, _, _ = strings.Cut(rest, ".")
	}
	if _, seen := g.stale.LoadOrStore(id, true); !seen {
		log.Printf("auth: device %s is no longer signed in (from %s)", id, r.RemoteAddr)
	}
}

// refused logs a wrong token with its address and makes the caller wait, so guessing is slow.
func (g *Guard) refused(r *http.Request) {
	log.Printf("auth: wrong token from %s", r.RemoteAddr)
	time.Sleep(500 * time.Millisecond)
}

func (g *Guard) setCookie(w http.ResponseWriter, r *http.Request, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   365 * 24 * 3600,
	})
}

func (g *Guard) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// ipOf is the address a request came from, without the port.
func ipOf(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// serveLogin is the sign-in page, naming the machine: through port forwards, "wrong token" usually means "wrong host".
func (g *Guard) serveLogin(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	host := template.HTMLEscapeString(g.host)
	fmt.Fprint(w, `<!doctype html><meta name=viewport content="width=device-width,initial-scale=1">
<title>remuda on `+host+` — sign in</title>
<body style="font-family:system-ui;background:#111;color:#ddd;display:grid;place-items:center;min-height:100vh;margin:0">
<form method=post action=/api/login style="display:flex;flex-direction:column;gap:12px;width:min(90vw,360px)">
<h1 style="font-size:20px;margin:0">remuda <span style="color:#999;font-weight:400">on `+host+`</span></h1>
<p style="margin:0;color:#999">Paste the access token of <b style="color:#ddd">`+host+`</b> (<code>remuda -print-token</code> there).</p>
<input name=token type=password autofocus placeholder="token"
 style="padding:10px;border-radius:8px;border:1px solid #444;background:#1a1a1a;color:#eee">
<button style="padding:10px;border-radius:8px;border:0;background:#3b82f6;color:#fff;font-weight:600">Sign in</button>
</form>`)
}
