package server

import (
	"net/http/httptest"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
)

// A read of a pane's conversation is the person's look at it — which takes its finish's notification back, or keeps it
// from going out at all — only from the chat lens of a page they can see: never a tile's tail, a viewer's screen, or a
// page whose own document is hidden (the Mac app's window closed, which goes on polling for its menu bar; a tab behind
// a pop-out), which says so with look=0.
func TestAChatReadIsALookOnlyFromAPageThePersonSees(t *testing.T) {
	read := func(query string, viewer bool) bool {
		r := httptest.NewRequest("GET", "/api/chat?machine=local&pane=w1:p1"+query, nil)
		if viewer {
			r = r.WithContext(auth.WithViewer(r.Context()))
		}
		return looksAtPane(r, r.URL.Query().Get("pane"), 0)
	}
	if !read("", false) {
		t.Fatal("the chat lens on screen is a look")
	}
	if read("&look=0", false) {
		t.Fatal("a page whose document is hidden is no look")
	}
	if read("", true) {
		t.Fatal("a viewer's screen is no look")
	}
	if looksAtPane(httptest.NewRequest("GET", "/api/chat?machine=local&pane=w1:p1&last=12", nil), "w1:p1", 12) {
		t.Fatal("a tile's tail is no look")
	}
	if looksAtPane(httptest.NewRequest("GET", "/api/chat?machine=local&dir=/work&session=abc", nil), "", 0) {
		t.Fatal("a transcript on its own is no pane's")
	}
}
