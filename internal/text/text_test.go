package text

import "testing"

func TestFirstLine(t *testing.T) {
	if got := FirstLine("  Прогони тесты и коротко скажи, что упало\nи ещё", 10); got != "Прогони те…" {
		t.Fatalf("got %q", got)
	}
	if got := FirstLine("short", 60); got != "short" {
		t.Fatalf("got %q", got)
	}
}

func TestSessionID(t *testing.T) {
	for _, ok := range []string{"9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d", "abc", "s_1.2", "A"} {
		if !SessionID(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "-x", ".x", "a/b", "a b", "a;b", "a\nb", "ä", "x$(y)", "0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789x"} {
		if SessionID(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
