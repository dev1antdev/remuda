package transcript

import (
	"strings"
	"testing"
	"time"
)

func TestRateLimitedReadsTheConversationsLastWord(t *testing.T) {
	prompt := `{"type":"user","message":{"role":"user","content":"carry on"},"timestamp":"2026-11-02T02:59:00Z"}`
	hit := `{"type":"assistant","isApiErrorMessage":true,"error":"rate_limit","message":{"role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"You've reached your Fable limit. Run /usage-credits to continue or switch models with /model."}]},"timestamp":"2026-11-02T03:00:00Z"}`
	meta := `{"type":"system","subtype":"turn_duration","timestamp":"2026-11-02T03:00:01Z"}`
	answer := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]},"timestamp":"2026-11-02T03:10:00Z"}`

	at, text, ok := RateLimited([]byte("art of a cut line\"}\n" + prompt + "\n" + hit + "\n" + meta + "\n"))
	if !ok || !at.Equal(time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)) || !strings.HasPrefix(text, "You've reached your Fable limit.") {
		t.Fatalf("hit: %v %q %v", at, text, ok)
	}
	if _, _, ok := RateLimited([]byte(prompt + "\n" + hit + "\n" + prompt + "\n")); ok {
		t.Fatal("a prompt after the hit still reads as stopped")
	}
	if _, _, ok := RateLimited([]byte(hit + "\n" + answer + "\n")); ok {
		t.Fatal("an answer after the hit still reads as stopped")
	}
	other := strings.Replace(hit, `"rate_limit"`, `"server_error"`, 1)
	if _, _, ok := RateLimited([]byte(other + "\n")); ok {
		t.Fatal("another API error reads as a limit")
	}
	if _, _, ok := RateLimited(nil); ok {
		t.Fatal("nothing reads as a limit")
	}
}
