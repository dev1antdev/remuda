package herdr

import (
	"errors"
	"fmt"
)

// Error is a structured refusal in herdr's words: the machine's core answered, and said no.
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
