package server

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/sshx"
)

// The refusal travels as a WebSocket close reason, which a browser drops whole when it is not UTF-8: a long reason must
// still be read.
func TestAttachRefusedIsCutBetweenLetters(t *testing.T) {
	msg := attachRefused(errors.New(strings.Repeat("ошибка ", 30)))
	if !utf8.ValidString(msg) || len(msg) > 123 {
		t.Errorf("%q (%d bytes) is no close reason", msg, len(msg))
	}
}

// A close reason holds 123 bytes: a refusal that reached the attach keeps its cause there, the machine's name the first
// thing to go.
func TestARefusedSessionKeepsItsCauseInTheReason(t *testing.T) {
	host := strings.Repeat("build-worker-", 6) + ".platform-engineering.prod.example.com"
	words, refused := sshx.SessionRefused(host, "mux_client_request_session: session request failed: Session open refused by peer")
	if !refused || !strings.HasPrefix(words, "too many ssh sessions") {
		t.Fatalf("the cause does not come first: %q", words)
	}
	msg := attachRefused(errors.New(words))
	if !strings.Contains(msg, "MaxSessions") || !utf8.ValidString(msg) || len(msg) > 123 {
		t.Errorf("%q (%d bytes) loses the cause", msg, len(msg))
	}
}
