package code

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/herdr/herdrfake"
	"github.com/dev1antdev/remuda/internal/machines"
)

func TestSettingOfReadsTheWorkbenchPage(t *testing.T) {
	page := []byte(`<meta id="vscode-workbench-web-configuration" data-settings="{&quot;remoteAuthority&quot;:&quot;remote&quot;,&quot;serverBasePath&quot;:&quot;.&quot;,&quot;productConfiguration&quot;:{&quot;codeServerVersion&quot;:&quot;4.140.0&quot;}}">`)
	if got := settingOf(page, "remoteAuthority"); got != "remote" {
		t.Fatalf("authority = %q, want remote", got)
	}
	if got := settingOf(page, "codeServerVersion"); got != "4.140.0" {
		t.Fatalf("version = %q, want 4.140.0", got)
	}
	older := []byte(`<meta data-settings="{&quot;remoteAuthority&quot;:&quot;127.0.0.1:7320&quot;}">`)
	if got := settingOf(older, "remoteAuthority"); got != "127.0.0.1:7320" {
		t.Fatalf("authority = %q, want the host", got)
	}
	if got := settingOf([]byte(`{"remoteAuthority":"remote"}`), "remoteAuthority"); got != "remote" {
		t.Fatalf("authority = %q from plain json", got)
	}
	if got := settingOf([]byte(`<html>nothing here</html>`), "remoteAuthority"); got != "" {
		t.Fatalf("authority = %q, want none", got)
	}
}

func TestOpenScriptQuotesThePathAndCarriesTheLine(t *testing.T) {
	got := OpenScript("/Users/me/Projects/a b/it's.go", 0)
	if !strings.Contains(got, `--reuse-window '/Users/me/Projects/a b/it'\''s.go'`) || !strings.Contains(got, "code-server") {
		t.Fatalf("script = %s", got)
	}
	if got := OpenScript("/w/main.go", 42); !strings.Contains(got, `'/w/main.go:42'`) {
		t.Fatalf("line: %s", got)
	}
	if lastLine("a\n[12:00] error got error from Code {\"errno\":-2}\n") != `[12:00] error got error from Code {"errno":-2}` {
		t.Fatal("lastLine")
	}
}

func TestWorkspaceJSONNamesTheFoldersSorted(t *testing.T) {
	got := string(WorkspaceJSON([]string{"/Users/me/Projects/zeta/", "/Users/me/Projects/alpha", "", "/srv/Alpha"}))
	want := "{\n  \"folders\": [\n    {\n      \"name\": \"alpha\",\n      \"path\": \"/Users/me/Projects/alpha\"\n    },\n    {\n      \"name\": \"Alpha\",\n      \"path\": \"/srv/Alpha\"\n    },\n    {\n      \"name\": \"zeta\",\n      \"path\": \"/Users/me/Projects/zeta\"\n    }\n  ],\n  \"settings\": {}\n}\n"
	if got != want {
		t.Fatalf("workspace json:\n%s", got)
	}
}

func TestWithThemeSetsTheColourTheme(t *testing.T) {
	got := string(WithTheme([]byte(`{"workbench.colorTheme": "Default Dark Modern", "telemetry.telemetryLevel": "off"}`), "Catppuccin Latte"))
	if !strings.Contains(got, `"workbench.colorTheme": "Catppuccin Latte"`) || !strings.Contains(got, `"telemetry.telemetryLevel": "off"`) {
		t.Fatalf("merged: %s", got)
	}
	commented := "{\n  // mine\n  \"workbench.colorTheme\": \"Dark\",\n  \"editor.fontSize\": 14,\n}\n"
	got = string(WithTheme([]byte(commented), "Catppuccin Frappé"))
	if !strings.Contains(got, `"workbench.colorTheme": "Catppuccin Frappé"`) || !strings.Contains(got, "// mine") || !strings.Contains(got, `"editor.fontSize": 14`) {
		t.Fatalf("replaced in place: %s", got)
	}
	got = string(WithTheme([]byte("{\n  // only a comment\n}\n"), "Catppuccin Frappé"))
	if !strings.HasPrefix(got, "{\n  \"workbench.colorTheme\": \"Catppuccin Frappé\",\n") {
		t.Fatalf("added: %s", got)
	}
}

func TestEveryThemeOfThePageHasItsEditorTheme(t *testing.T) {
	want := map[string]EditorTheme{
		"latte":            {"Catppuccin.catppuccin-vsc", "Catppuccin Latte"},
		"frappe":           {"Catppuccin.catppuccin-vsc", "Catppuccin Frappé"},
		"macchiato":        {"Catppuccin.catppuccin-vsc", "Catppuccin Macchiato"},
		"mocha":            {"Catppuccin.catppuccin-vsc", "Catppuccin Mocha"},
		"dracula":          {"dracula-theme.theme-dracula", "Dracula Theme"},
		"nord":             {"arcticicestudio.nord-visual-studio-code", "Nord"},
		"gruvbox-dark":     {"jdinhlife.gruvbox", "Gruvbox Dark Medium"},
		"tokyo-night":      {"enkia.tokyo-night", "Tokyo Night"},
		"rose-pine":        {"mvllow.rose-pine", "Rosé Pine"},
		"one-dark":         {"zhuangtongfa.material-theme", "One Dark Pro"},
		"kanagawa":         {"metaphore.kanagawa-vscode-color-theme", "Kanagawa Wave"},
		"solarized-light":  {"", "Solarized Light"},
		"gruvbox-light":    {"jdinhlife.gruvbox", "Gruvbox Light Medium"},
		"rose-pine-dawn":   {"mvllow.rose-pine", "Rosé Pine Dawn"},
		"tokyo-night-day":  {"enkia.tokyo-night", "Tokyo Night Light"},
		"github-light":     {"GitHub.github-vscode-theme", "GitHub Light Default"},
		"one-light":        {"akamud.vscode-theme-onelight", "Atom One Light"},
		"everforest-light": {"sainnhe.everforest", "Everforest Light"},
		"ayu-light":        {"teabyii.ayu", "Ayu Light Bordered"},
		"flexoki-light":    {"", "Default Light Modern"},
	}
	for theme, editor := range want {
		if got := EditorThemeOf(theme); got != editor || !KnownTheme(theme) || ThemeName(theme) != editor.Name {
			t.Errorf("%s: %+v, want %+v", theme, got, editor)
		}
	}
	if len(editorThemes) != len(want) {
		t.Errorf("%d editor themes, want the page's %d", len(editorThemes), len(want))
	}
	if ThemeName("light") != "Catppuccin Latte" || ThemeName("dark") != "Catppuccin Frappé" || !KnownTheme("dark") || !KnownTheme("light") {
		t.Error("the bare dark and light a page from before sends")
	}
	if KnownTheme("monokai") || KnownTheme("") || ThemeName("monokai") != "Default Dark Modern" {
		t.Error("an unknown word")
	}
}

func TestThemeReadScriptInstallsOnlyAnExtension(t *testing.T) {
	got := ThemeReadScript("teabyii.ayu")
	if !strings.Contains(got, `grep -qixF 'teabyii.ayu'`) || !strings.Contains(got, `--install-extension 'teabyii.ayu'`) || !strings.Contains(got, installedMark) || !strings.Contains(got, `cat "$f"`) {
		t.Fatalf("script: %s", got)
	}
	if got := ThemeReadScript(""); strings.Contains(got, "--install-extension") || !strings.Contains(got, `cat "$f"`) {
		t.Fatalf("a built-in theme installs nothing: %s", got)
	}
}

// proxied is a code-server stand-in behind Service.Proxy on this machine, and the paths it was asked for.
func proxied(t *testing.T, answer http.HandlerFunc) (http.Handler, *[]string) {
	t.Helper()
	var asked []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		answer(w, r)
	}))
	t.Cleanup(backend.Close)
	port := backend.Listener.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	mc, err := herdrfake.Manager(dir).Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Port: port}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.Proxy(w, r, mc, "/code/local") }), &asked
}

// TestThePortRoutesOfCodeServerStayShut: a page of any port served through them would run on remuda's origin.
func TestThePortRoutesOfCodeServerStayShut(t *testing.T) {
	h, asked := proxied(t, func(w http.ResponseWriter, r *http.Request) {})
	for _, p := range []string{
		"/proxy/3000/", "/proxy", "/absproxy/3000/x.js", "/absproxy", "/PROXY/3000/", "/AbsProxy/3000/",
		"//proxy/3000/", "/./proxy/3000/", "/x/../proxy/3000/", "/proxy//3000",
		"/proxy/3000/../../x", "/absproxy/3000/../../x", "/../proxy/3000/",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, &http.Request{Method: "GET", URL: &url.URL{Path: "/code/local" + p}, Header: http.Header{}})
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", p, rec.Code)
		}
	}
	for _, target := range []string{"/code/local/proxy/3000/..%2f..%2fx", "/code/local/x%2f..%2f..%2fproxy/3000/", "/code/local/%2e%2e/proxy/3000/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Previews") {
			t.Errorf("%s answered %d %q", target, rec.Code, rec.Body)
		}
	}
	if len(*asked) != 0 {
		t.Errorf("code-server was asked for %v", *asked)
	}
	for _, p := range []string{"/", "/proxyish/x", "/stable/proxy/x", "/_static/absproxy.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/code/local"+p, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s answered %d", p, rec.Code)
		}
	}
}

// TestAnEditorsAnswerLeavesTheSessionAlone: no cookie of remuda's, no clearing of the site, and a worker kept to the mount.
func TestAnEditorsAnswerLeavesTheSessionAlone(t *testing.T) {
	h, _ := proxied(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", auth.CookieName+"=x; Path=/")
		w.Header().Add("Set-Cookie", "code-server-session=y; Path=/")
		w.Header().Set("Clear-Site-Data", `"cookies"`)
		w.Header().Set("Service-Worker-Allowed", "/")
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/code/local/", nil))
	got := rec.Header()
	if cookies := got.Values("Set-Cookie"); len(cookies) != 1 || !strings.HasPrefix(cookies[0], "code-server-session=") {
		t.Errorf("cookies %v", cookies)
	}
	if got.Get("Clear-Site-Data") != "" {
		t.Error("the site's data would be cleared")
	}
	if allowed := got.Get("Service-Worker-Allowed"); allowed != "/code/local/" {
		t.Errorf("a worker is allowed at %q", allowed)
	}
}
