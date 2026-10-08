package herdr

import (
	"errors"
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
	err := &RefusedError{Msg: msg}
	if !Refused(err) || Refused(errors.New("already has an attached client")) {
		t.Fatal("Refused")
	}
}
