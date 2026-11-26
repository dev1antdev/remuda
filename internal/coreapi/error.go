package coreapi

import (
	"errors"
	"fmt"
)

// Error is a structured refusal in the core's words: the machine's remuda-core answered, and said no.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("remuda-core %s: %s", e.Code, e.Message) }

// IsCode reports whether err is a core error with the given code.
func IsCode(err error, code string) bool {
	var he *Error
	return errors.As(err, &he) && he.Code == code
}
