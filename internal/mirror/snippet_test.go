package mirror

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestTheSnippetNamesRemudasTokensAndHerdrTakesIt runs the snippet through `herdr config check` on a config file of
// its own (HERDR_CONFIG_PATH; no server is asked), and a broken one too, so a pass means herdr read it.
func TestTheSnippetNamesRemudasTokensAndHerdrTakesIt(t *testing.T) {
	names := append(append([]string{}, PaneTokens...), WorkspaceTokens...)
	for _, m := range regexp.MustCompile(`\$([A-Za-z0-9_-]+)`).FindAllStringSubmatch(Snippet, -1) {
		if !slices.Contains(names, m[1]) {
			t.Errorf("the snippet names $%s, which remuda never writes", m[1])
		}
	}
	for _, name := range PaneTokens {
		if !strings.Contains(Snippet, "$"+name) {
			t.Errorf("the snippet leaves $%s out", name)
		}
	}

	bin, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("no herdr on PATH")
	}
	check := func(config string) (string, error) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "config", "check")
		cmd.Env = append(os.Environ(), "HOME="+dir, "HERDR_CONFIG_PATH="+path)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := check(Snippet + "\n" + ToastSnippet); err != nil {
		t.Fatalf("herdr refuses the snippet: %v\n%s", err, out)
	}
	if out, err := check(strings.Replace(Snippet, `fg = "#e78284"`, `fg = "pinkish"`, 1)); err == nil {
		t.Fatalf("herdr took a broken colour, so its check proves nothing: %s", out)
	}
}
