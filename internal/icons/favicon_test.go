package icons

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// site serves a page with the given head and the pictures at their paths; every request is counted by path.
func site(t *testing.T, head string, pictures map[string][]byte) (*httptest.Server, map[string]int) {
	t.Helper()
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		if r.Header.Get("User-Agent") != "remuda" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		if r.URL.Path == "/" && head != "" {
			_, _ = w.Write([]byte("<!doctype html><html><head>" + head + "</head><body>hi</body></html>"))
			return
		}
		if data, ok := pictures[r.URL.Path]; ok {
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

func fetched(t *testing.T, srv *httptest.Server) (string, string, error) {
	t.Helper()
	return Favicon(context.Background(), Client(), srv.URL)
}

func TestFaviconTakesTheLinkedIcon(t *testing.T) {
	srv, _ := site(t, `<link rel="stylesheet" href="/a.css"><LINK REL="Shortcut Icon" HREF="/static/fav.png">`, map[string][]byte{"/static/fav.png": pngBytes, "/favicon.ico": icoBytes})
	data, from, err := fetched(t, srv)
	if err != nil || from != srv.URL+"/static/fav.png" || !strings.HasPrefix(data, "data:image/png;base64,") {
		t.Fatalf("%q %q %v", from, data, err)
	}
}

func TestFaviconPrefersTheTouchIconThenTheLargest(t *testing.T) {
	head := `<link rel="icon" sizes="16x16" href="/16.png"><link rel="icon" sizes="64x64" href="/64.png"><link rel='apple-touch-icon' href='/touch.png'><link rel="mask-icon" href="/mask.svg">`
	srv, _ := site(t, head, map[string][]byte{"/16.png": pngBytes, "/64.png": pngBytes, "/touch.png": pngBytes, "/mask.svg": []byte(svgText)})
	if _, from, err := fetched(t, srv); err != nil || from != srv.URL+"/touch.png" {
		t.Fatalf("touch first: %q %v", from, err)
	}
	srv, hits := site(t, `<link rel="icon" sizes="16x16" href="/16.png"><link rel="icon" sizes="32x32 64x64" href="/64.png">`, map[string][]byte{"/16.png": pngBytes})
	if _, from, err := fetched(t, srv); err != nil || from != srv.URL+"/16.png" || hits["/64.png"] != 1 {
		t.Fatalf("the largest tried first, a missing one passed over: %q %v %v", from, err, hits)
	}
}

func TestFaviconReadsTheLinksInABigPagesFirstMegabyte(t *testing.T) {
	big := `<link rel="icon" href="/fav.png"><script>` + strings.Repeat("x", MaxPage) + `</script>`
	srv, _ := site(t, big, map[string][]byte{"/fav.png": pngBytes})
	data, from, err := fetched(t, srv)
	if err != nil || from != srv.URL+"/fav.png" || !strings.HasPrefix(data, "data:image/png;base64,") {
		t.Fatalf("a page past a megabyte still names its icon in its head: %q %v", from, err)
	}
}

func TestFaviconResolvesARelativeHrefAgainstThePage(t *testing.T) {
	srv, _ := site(t, `<link rel="icon" href="img/icon.svg">`, map[string][]byte{"/img/icon.svg": []byte(svgText)})
	data, from, err := fetched(t, srv)
	if err != nil || from != srv.URL+"/img/icon.svg" || !strings.HasPrefix(data, "data:image/svg+xml;base64,") {
		t.Fatalf("%q %q %v", from, data, err)
	}
}

func TestFaviconFallsBackToFaviconIco(t *testing.T) {
	srv, _ := site(t, `<title>no icons here</title>`, map[string][]byte{"/favicon.ico": icoBytes})
	data, from, err := fetched(t, srv)
	if err != nil || from != srv.URL+"/favicon.ico" || !strings.HasPrefix(data, "data:image/x-icon;base64,") {
		t.Fatalf("no link: %q %v", from, err)
	}
	down, _ := site(t, "", map[string][]byte{"/favicon.ico": icoBytes})
	if _, from, err := fetched(t, down); err != nil || from != down.URL+"/favicon.ico" {
		t.Fatalf("page down: %q %v", from, err)
	}
}

func TestFaviconRefusesWhatIsNoPicture(t *testing.T) {
	srv, _ := site(t, `<link rel="icon" href="/fake.png">`, map[string][]byte{"/fake.png": []byte("<html>not a picture</html>"), "/favicon.ico": []byte("nope")})
	if _, _, err := fetched(t, srv); err == nil || !strings.Contains(err.Error(), "no icon found on 127.0.0.1") || !strings.Contains(err.Error(), "not a picture") {
		t.Fatalf("a page for a picture: %v", err)
	}
	srv, _ = site(t, `<title>none</title>`, nil)
	if _, _, err := fetched(t, srv); err == nil || !strings.Contains(err.Error(), "names no icon") {
		t.Fatalf("nothing at all: %v", err)
	}
	huge := append(append([]byte{}, pngBytes...), make([]byte, MaxImage)...)
	srv, _ = site(t, `<link rel="icon" href="/huge.png">`, map[string][]byte{"/huge.png": huge})
	if _, _, err := fetched(t, srv); err == nil || !strings.Contains(err.Error(), "over") {
		t.Fatalf("an oversized picture: %v", err)
	}
}

func TestFaviconTakesADataHref(t *testing.T) {
	inline := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
	srv, hits := site(t, `<link rel="icon" href="`+inline+`">`, nil)
	data, from, err := fetched(t, srv)
	if err != nil || data != inline || from != srv.URL || hits["/favicon.ico"] != 0 {
		t.Fatalf("%q %q %v %v", from, data, err, hits)
	}
}

func TestFaviconWantsAWebAddress(t *testing.T) {
	for _, raw := range []string{"", "ftp://example.com", "file:///etc/passwd", "javascript:alert(1)", "https://"} {
		if _, _, err := Favicon(context.Background(), Client(), raw); err != ErrNotWeb {
			t.Errorf("%q: %v", raw, err)
		}
	}
	if site, err := SiteURL("gog.com"); err != nil || site.String() != "https://gog.com" {
		t.Fatalf("a bare host: %v %v", site, err)
	}
}

func TestFaviconTriesAFewOfThePagesLinksOnly(t *testing.T) {
	var head strings.Builder
	for i := 0; i < 200; i++ {
		head.WriteString(`<link rel="icon" href="/icon-` + strconv.Itoa(i) + `.png">`)
	}
	srv, hits := site(t, head.String(), nil)
	if _, _, err := fetched(t, srv); err == nil {
		t.Fatal("a page with no picture gave an icon")
	}
	tried := 0
	for path, n := range hits {
		if strings.HasPrefix(path, "/icon-") {
			tried += n
		}
	}
	if tried != MaxIconLinks || hits["/favicon.ico"] != 1 {
		t.Fatalf("fetched %d of the page's links and /favicon.ico %d times", tried, hits["/favicon.ico"])
	}
}

func TestFaviconGivesUpAtItsDeadline(t *testing.T) {
	was := FaviconTimeout
	FaviconTimeout = 300 * time.Millisecond
	t.Cleanup(func() { FaviconTimeout = was })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/a.png"><link rel="icon" href="/b.png"></head></html>`))
			return
		}
		select { // every icon hangs, as a slow site's would
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	start := time.Now()
	if _, _, err := Favicon(context.Background(), Client(), srv.URL); err == nil {
		t.Fatal("an icon from a site that never answered")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the search ran %v past its deadline", took)
	}
}
