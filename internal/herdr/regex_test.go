package herdr

import (
	"strings"
	"testing"
)

// TestARegexHerdrReadsOtherwiseIsRefused: what herdr 0.9.5 refused, or read otherwise than Go, when a throwaway server
// was handed each of these as an output watch.
func TestARegexHerdrReadsOtherwiseIsRefused(t *testing.T) {
	for pattern, why := range map[string]string{
		`\101`:        "octal",            // herdr: backreferences are not supported
		`\0`:          "octal",            // the same
		`exit \12`:    "octal",            // the same
		`[[]x`:        "class of its own", // herdr: unclosed character class
		`[[a]] tag`:   "class of its own", // herdr: the class {a}, then " tag"; Go: "[a", "] tag"
		`\<x`:         "word's start",     // herdr: a word's start; Go: "<x"
		`x\>`:         "word's start",
		`[a&&b]`:      "combining", // herdr: a and b at once, nothing; Go: a, & or b
		`[a~~b]`:      "combining",
		`\b{start}x`:  "word's start",
		`a{b}`:        "only as a count", // herdr refuses a brace that counts nothing; Go reads it as it is
		`{"ok":true}`: "only as a count",
		`a{ 2}`:       "only as a count", // herdr reads it as a count, Go as the characters
		`x{,3}`:       "x{0,n}",          // herdr refuses; Go reads "x{,3}" as it is
		`[\d-z]`:      "range",           // herdr refuses; Go reads \d, -, z
		`\Qa.b\E`:     `\Q`,
	} {
		_, err := CompileRegex(pattern)
		if err == nil || !strings.Contains(err.Error(), why) {
			t.Errorf("%q: %v, want a refusal saying %q", pattern, err, why)
		}
	}
	for _, pattern := range []string{`[]a]`, `[^]a]`, `[[:alpha:]]+`, `[[:^digit:]]`, `\pL`, `\p{Cyrillic}+`, `\x41`, `\x{263A}`, `a-b`, `[a-z-]`, `\[x\]`, `(?i)error`, `(?U)a+`, `\@`, `\-`} {
		if _, err := CompileRegex(pattern); err != nil {
			t.Errorf("%q is read alike by both: %v", pattern, err)
		}
	}
	if _, err := CompileRegex(`(`); err == nil || !strings.Contains(err.Error(), "does not read") {
		t.Errorf("a regex Go cannot read says so in Go's words: %v", err)
	}
}

// TestARegexMatchesTheRowsHerdrMatched: herdr's Unicode \d, \w, \s and \b, as the throwaway server matched these rows.
func TestARegexMatchesTheRowsHerdrMatched(t *testing.T) {
	for _, c := range []struct {
		pattern, row string
		herdr        bool
	}{
		{`\bошибка\b`, "ok ошибка готово", true},
		{`ошибка:\s+\w+`, "ошибка:  сборка", true},
		{`^\d failed$`, "٣ failed", true},
		{`^\d+ failed`, "3 failed", true},
		{`\bLocal`, "élan Local", true},
		{`a<b`, "wrong a<b", true},
		{`[^\w]`, "é", false},
		{`^\W`, "é", false},
		{`\s`, "a b", true},
		{`^\S+$`, "a b", false},
		{`[\s\d]`, "x ", true},
		{`\bfoo\b`, "foobar", false},
		{`\bfoo\b`, "a foo b", true},
		{`(?i)K`, "ok", true},
	} {
		re, err := CompileRegex(c.pattern)
		if err != nil {
			t.Fatalf("%q: %v", c.pattern, err)
		}
		if got := re.MatchString(c.row); got != c.herdr {
			t.Errorf("%q on %q: %v, herdr says %v", c.pattern, c.row, got, c.herdr)
		}
	}
}
