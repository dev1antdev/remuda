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
