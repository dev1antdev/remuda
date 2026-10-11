// Package auth is deliberately small: one bearer token, required on every
// request behind the guard. No accounts, no third parties. The token lives in
// a 0600 file in the state dir and in an HttpOnly cookie on signed-in devices.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dev1antdev/remuda/internal/state"
)

const CookieName = "remuda_token"

// LoadOrCreateToken returns REMUDA_TOKEN if set, otherwise the token stored
// at <state dir>/token, creating it on first run.
func LoadOrCreateToken(dir string) (string, error) {
	if t := strings.TrimSpace(os.Getenv("REMUDA_TOKEN")); t != "" {
		return t, nil
	}
	path := filepath.Join(dir, "token")
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	t := hex.EncodeToString(buf)
	if err := state.WriteFile(path, []byte(t+"\n")); err != nil {
		return "", err
	}
	return t, nil
}

type Guard struct {
	token string
	host  string // shown on the sign-in page: which machine's remuda this is
}

func NewGuard(token string) *Guard {
	h, _ := os.Hostname()
	return &Guard{token: token, host: strings.TrimSuffix(h, ".local")}
}

func (g *Guard) ok(presented string) bool {
	return presented != "" &&
		subtle.ConstantTimeCompare([]byte(presented), []byte(g.token)) == 1
}

func (g *Guard) fromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return c.Value
	}
	return ""
}

// Middleware rejects unauthenticated requests. A valid ?token= query (the QR /
// pairing link) is exchanged for the cookie and stripped from the URL.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if qt := r.URL.Query().Get("token"); qt != "" && g.ok(qt) {
			g.setCookie(w, r, qt)
			q := r.URL.Query()
			q.Del("token")
			u := *r.URL
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), http.StatusSeeOther)
			return
		}
		if g.ok(g.fromRequest(r)) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		g.serveLogin(w)
	})
}

// HandleLogin exchanges a posted token for the session cookie.
func (g *Guard) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	t := strings.TrimSpace(r.PostFormValue("token"))
	if !g.ok(t) {
		http.Error(w, "wrong token", http.StatusForbidden)
		return
	}
	g.setCookie(w, r, t)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (g *Guard) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   365 * 24 * 3600,
	})
}

// serveLogin names the machine: with several remudas reachable on
// 127.0.0.1 through port forwards, "wrong token" usually means "wrong host".
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
