// Package herdr speaks herdr's newline-JSON RPC over its Unix socket, and holds
// the model remuda asks every machine through.
package herdr

import (
	"errors"
	"fmt"
)

// Error is herdr's own refusal, from the machine's core now: it answered, and said no.

// Error is a structured herdr RPC error: herdr answered, and said no.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("herdr %s: %s", e.Code, e.Message) }

// IsCode reports whether err is a herdr error with the given code.
func IsCode(err error, code string) bool {
	var he *Error
	return errors.As(err, &he) && he.Code == code
}
