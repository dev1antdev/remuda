package herdr

import (
	"errors"
	"strings"
	"testing"
)

func TestRefusal(t *testing.T) {
	first := []byte("\x1b[0 qherdr: server shut down: terminal attach failed: terminal term_65d5a74a677061 already has an attached client; retry with --takeover\r\n")
	msg, ok := refusal(first)
	if !ok || msg != "terminal term_65d5a74a677061 already has an attached client; retry with --takeover" {
		t.Fatalf("refusal: %v %q", ok, msg)
	}
	if _, ok := refusal([]byte("\x1b[?1000h\x1b[2J\x1b[H$ ")); ok {
		t.Fatal("a screen is not a refusal")
	}
	if msg, ok := refusal([]byte("mux_client_request_session: session request failed: Session open refused by peer\r\n")); !ok || !strings.Contains(msg, "MaxSessions") {
		t.Fatalf("ssh's refusal: %v %q", ok, msg)
	}
	err := &RefusedError{Msg: msg}
	if !Refused(err) || Refused(errors.New("already has an attached client")) {
		t.Fatal("Refused")
	}
}
