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
