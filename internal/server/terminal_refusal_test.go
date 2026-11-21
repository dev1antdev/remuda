package server

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// The refusal travels as a WebSocket close reason, which a browser drops whole when it is not UTF-8: a long reason must
// still be read.
func TestAttachRefusedIsCutBetweenLetters(t *testing.T) {
	msg := attachRefused(errors.New(strings.Repeat("ошибка ", 30)))
	if !utf8.ValidString(msg) || len(msg) > 123 {
		t.Errorf("%q (%d bytes) is no close reason", msg, len(msg))
	}
}
