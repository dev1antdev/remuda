package server

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// The refusal travels as a WebSocket close reason, which a browser drops whole when it is not UTF-8: a machine named at
// length in Cyrillic must still have its reason read.
func TestAttachRefusedIsCutBetweenLetters(t *testing.T) {
	dir := t.TempDir()
	fleet := machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
	mc, err := fleet.Rename(machines.LocalID, strings.Repeat("Ж", 60))
	if err != nil {
		t.Fatal(err)
	}
	for _, refusal := range []error{&herdr.RefusedError{Msg: "already has an attached client"}, &herdr.Error{Code: "x", Message: strings.Repeat("ошибка ", 30)}} {
		msg := attachRefused(mc, refusal)
		if !utf8.ValidString(msg) || len(msg) > 123 {
			t.Errorf("%q (%d bytes) is no close reason", msg, len(msg))
		}
	}
}
