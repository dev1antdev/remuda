package code

import (
	"strings"
	"testing"
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
	if ThemeName("light") != "Catppuccin Latte" || ThemeName("dark") != "Catppuccin Frappé" || ThemeName("") != "Catppuccin Frappé" {
		t.Fatal("names")
	}
}
