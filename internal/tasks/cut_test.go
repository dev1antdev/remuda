package tasks

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestLongTextsAreCutAtARune: a source or a Markdown description past its bound is cut between characters, never
// through one — a machine named in Cyrillic would otherwise end in a broken byte the page draws as �.
func TestLongTextsAreCutAtARune(t *testing.T) {
	source := Source("x"+strings.Repeat("я", 70), "w3:p1")
	if !utf8.ValidString(source) || len(source) > MaxSource {
		t.Fatalf("Source cut through a character: %q (%d bytes)", source, len(source))
	}
	if !strings.HasPrefix(source, "x"+strings.Repeat("я", 59)) {
		t.Fatalf("Source lost more than the cut asks: %q", source)
	}
	html := MarkdownHTML("a" + strings.Repeat("я", MaxMarkdown/2+10))
	if !utf8.ValidString(html) {
		t.Fatalf("MarkdownHTML cut through a character: …%q", html[len(html)-12:])
	}
}
