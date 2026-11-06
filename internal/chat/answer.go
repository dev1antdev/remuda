package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Answer is the keys that pick the choice at index of the dialog on the screen now, after checking that it is the
// dialog the choices were offered on (dialog, its DialogKey) and that the choice still reads label: a later dialog that
// offers the same choices — Claude Code asks every permission with "Yes" first — is not answered by a stale
// notification, nor one that changed or closed, nor a multi-select, where one press only ticks a box.
func Answer(screen string, index int, label, dialog string) ([]string, error) {
	q, ok := ParseQuestion(screen)
	if !ok {
		return nil, fmt.Errorf("no question on the screen now")
	}
	if q.Multi {
		return nil, fmt.Errorf("the dialog takes several choices: tick them and submit at the terminal")
	}
	if DialogKey(q.Text) != dialog {
		return nil, fmt.Errorf("the dialog changed: it asks something else now")
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

// DialogKey names a dialog by its words, every space left out so a pane resized in between, which wraps them anew,
// still reads the same; a question's notification carries it beside its choices.
func DialogKey(text string) string {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(text), "")))
	return hex.EncodeToString(sum[:8])
}

func sameChoice(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}
