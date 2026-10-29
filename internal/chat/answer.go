package chat

import (
	"fmt"
	"strings"
)

// Answer is the keys that pick the choice at index of the dialog on the screen now, after checking that it still
// reads label: a dialog that changed or closed since the choices were offered is not answered.
func Answer(screen string, index int, label string) ([]string, error) {
	q, ok := ParseQuestion(screen)
	if !ok {
		return nil, fmt.Errorf("no question on the screen now")
	}
	if index < 0 || index >= len(q.Options) {
		return nil, fmt.Errorf("the dialog changed: it has %d choices now", len(q.Options))
	}
	if got := q.Options[index].Label; !sameChoice(got, label) {
		return nil, fmt.Errorf("the dialog changed: choice %d now reads %q", index+1, got)
	}
	return KeysTo(q.Options, index), nil
}

// KeysTo walks the cursor from the picked choice to the one at index and confirms it, as the question card does.
func KeysTo(options []Option, index int) []string {
	picked := 0
	for i, o := range options {
		if o.Picked {
			picked = i
			break
		}
	}
	steps := index - picked
	key := "down"
	if steps < 0 {
		key, steps = "up", -steps
	}
	keys := make([]string, 0, steps+1)
	for range steps {
		keys = append(keys, key)
	}
	return append(keys, "enter")
}

func sameChoice(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}
