package script

import (
	"os/exec"
	"strings"
	"testing"
)

func TestQuoteSurvivesTheShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	for _, word := range []string{"plain", "it's", `say "hi"`, "$HOME `id` $(id)", "a b\tc", "back\\slash", ""} {
		out, err := exec.Command("sh", "-c", "printf %s "+Quote(word)).Output()
		if err != nil || string(out) != word {
			t.Errorf("Quote(%q) came back as %q (%v)", word, out, err)
		}
	}
}

func TestPreludesAndStat(t *testing.T) {
	if got := InDir("/a b"); got != "cd '/a b' 2>/dev/null || exit 0; " {
		t.Fatalf("InDir: %q", got)
	}
	if got := InRepo("/r"); !strings.HasPrefix(got, InDir("/r")) || !strings.Contains(got, "rev-parse --show-toplevel") {
		t.Fatalf("InRepo: %q", got)
	}
	if got := Stat(Quote("/f"), "%s %Y", "%z %m"); got != `stat -c '%s %Y' '/f' 2>/dev/null || stat -f '%z %m' '/f'` {
		t.Fatalf("Stat: %q", got)
	}
	bin, guard := Remuda("/home/x", "v1.2.3")
	if bin != "'/home/x/.local/bin/remuda'" || guard != `[ -x '/home/x/.local/bin/remuda' ] && [ "$('/home/x/.local/bin/remuda' -version 2>/dev/null)" = 'remuda v1.2.3' ]` {
		t.Fatalf("Remuda: %s / %s", bin, guard)
	}
}

func TestSectionsLinesFields(t *testing.T) {
	out := "head\n" + RS + "\n" + "a" + US + "1\n\nb" + US + "2\n" + RS + "\n"
	sections := Sections(out)
	if len(sections) != 3 || sections[0] != "head\n" || sections[2] != "" {
		t.Fatalf("sections: %q", sections)
	}
	lines := Lines(sections[1])
	if len(lines) != 2 || Fields(lines[1])[1] != "2" {
		t.Fatalf("lines: %q", lines)
	}
}
