package preview

// What a preview's listener answers: the person's own browser only, its request handed to the dev server as if it
// were asked on the machine itself, and nothing of remuda's going along or coming back.

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
)

// Guarded lets through only what allow says is the person's own — a signed-in device or the token, never a viewer
// link or an agent's key — and tells anyone else where to sign in, with no form of its own: a token is never typed
// into a page that serves an app.
func Guarded(allow func(*http.Request) bool, signIn string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><title>remuda preview</title><p>This preview is for the browser signed in to <a href="%s">remuda</a>. Sign in there, then reload this page.</p>`, html.EscapeString(signIn))
	})
}

// Proxy hands each request to the dev server the dial reaches: the Host it would see on the machine (localhost and its
// port, which a dev server allows by default), the Origin and Referer of the preview's own pages written as that
// address too — another origin's left as it is, so the app's own CORS and CSRF checks still see a stranger, and a
// stranger's Referer dropped — remuda's cookie and any Authorization taken off on
// the way in, and on the way out a Set-Cookie of remuda's name or a Clear-Site-Data — which would sign the person out
// of remuda, since cookies are kept by host and not by port — dropped. WebSocket upgrades (a dev server's live reload)
// pass. A redirect to the server's own address comes back as the preview's. seen hears every request, and holds is
// how many requests are still being answered, an upgraded one for as long as its connection stays open.
func Proxy(dial func() (string, error), port int, base string, seen func(), holds *atomic.Int64) http.Handler {
	own := "localhost:" + strconv.Itoa(port)
	self := originOf(base)
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			addr, _ := pr.In.Context().Value(dialKey{}).(string)
			pr.SetURL(&url.URL{Scheme: "http", Host: addr})
			pr.SetXForwarded()
			pr.Out.Host = own
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("Cookie")
			for _, c := range pr.In.Cookies() {
				if c.Name != auth.CookieName {
					pr.Out.AddCookie(c)
				}
			}
			if origin := pr.Out.Header.Get("Origin"); origin != "" && strings.EqualFold(origin, self) {
				pr.Out.Header.Set("Origin", "http://"+own)
			}
			referer := pr.Out.Header.Get("Referer")
			pr.Out.Header.Del("Referer")
			if len(referer) >= len(self) && strings.EqualFold(referer[:len(self)], self) {
				if rest := referer[len(self):]; rest == "" || rest[0] == '/' {
					pr.Out.Header.Set("Referer", "http://"+own+rest)
				}
			}
		},
		ModifyResponse: func(res *http.Response) error {
			res.Header.Del("Clear-Site-Data")
			keep := res.Header.Values("Set-Cookie")
			res.Header.Del("Set-Cookie")
			for _, line := range keep {
				if name, _, _ := strings.Cut(line, "="); strings.TrimSpace(name) != auth.CookieName {
					res.Header.Add("Set-Cookie", line)
				}
			}
			if loc := res.Header.Get("Location"); loc != "" {
				res.Header.Set("Location", ownLocation(loc, own, base))
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() == context.Canceled {
				return
			}
			http.Error(w, fmt.Sprintf("nothing answers on port %d of the machine: %v", port, err), http.StatusBadGateway)
		},
		FlushInterval: -1,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen()
		holds.Add(1)
		defer holds.Add(-1)
		addr, err := dial()
		if err != nil {
			http.Error(w, "the machine cannot be reached: "+err.Error(), http.StatusBadGateway)
			return
		}
		rp.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), dialKey{}, addr)))
	})
}

type dialKey struct{}

// originOf is an address's scheme and host, as a browser writes an Origin
func originOf(address string) string {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return address
	}
	return u.Scheme + "://" + u.Host
}

// ownLocation is a redirect to the dev server's own address (http://localhost:3000/x) written as the preview's
func ownLocation(loc, own, base string) string {
	u, err := url.Parse(loc)
	if err != nil || u.Host == "" {
		return loc
	}
	host := u.Hostname()
	if (host == "localhost" || host == "127.0.0.1" || host == "::1") && u.Port() == strings.TrimPrefix(own, "localhost:") {
		b, err := url.Parse(base)
		if err != nil {
			return loc
		}
		u.Scheme, u.Host = b.Scheme, b.Host
		return u.String()
	}
	return loc
}

// idle is how long a preview has gone unasked, for the closer
func idle(last *atomic.Int64, now time.Time) time.Duration {
	return now.Sub(time.Unix(0, last.Load()))
}
