package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOnlyThePersonsOwnBrowserOrTheTokenIsSignedIn(t *testing.T) {
	g, h := viewerGuard(t)
	ask := func(cookie, bearer string) bool {
		req := httptest.NewRequest("GET", "/", nil)
		if cookie != "" {
			req.Header.Set("Cookie", CookieName+"="+cookie)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		return g.SignedIn(req)
	}
	_, _, res := call(h, "GET", "/?token="+testToken, nil, "")
	device := ""
	for _, c := range res.Cookies() {
		if c.Name == CookieName {
			device = c.Value
		}
	}
	if !strings.HasPrefix(device, cookiePrefix) {
		t.Fatalf("no device cookie from the pairing link: %q", device)
	}
	if !ask(device, "") || !ask("", testToken) {
		t.Fatal("the person's own device or the token is not signed in")
	}
	viewerKey, _, _ := g.Keys.MintViewer("the office TV")
	_, _, res = call(h, "GET", "/?viewer="+viewerKey, nil, "")
	viewer := ""
	for _, c := range res.Cookies() {
		if c.Name == CookieName {
			viewer = c.Value
		}
	}
	agentKey, _, _ := g.Keys.Mint("GOG")
	for name, refused := range map[string]bool{
		"a viewer link's screen": ask(viewer, ""),
		"a viewer key":           ask("", viewerKey),
		"an agent's key":         ask("", agentKey),
		"a wrong token":          ask("", strings.Repeat("x", 40)),
		"a made-up device":       ask(cookiePrefix+"nope.secret", ""),
		"nobody":                 ask("", ""),
	} {
		if refused {
			t.Errorf("%s counts as signed in", name)
		}
	}
}
