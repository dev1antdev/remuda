package sessions

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/events"
)

// The keys people press in a pane: the key-combo grammar as remuda allows it, names the core knows.

// namedKeys are the keys the core names, which take a modifier.
var namedKeys = map[string]bool{
	"enter": true, "esc": true, "tab": true, "space": true, "backspace": true,
	"up": true, "down": true, "left": true, "right": true,
}

// aloneKeys are the keys the core names and a modifier cannot go with; they stand alone.
var aloneKeys = map[string]bool{"home": true, "end": true, "pageup": true, "pagedown": true, "delete": true}

// keyAllowed is one key in the core's grammar: optional ctrl, alt and shift, each once, before a named key, or ctrl or
// alt before a letter or digit (a printable key alone is text, not a key); a key with no modifier stands alone.
func keyAllowed(name string) bool {
	if aloneKeys[name] {
		return true
	}
	parts := strings.Split(name, "+")
	base, mods := parts[len(parts)-1], parts[:len(parts)-1]
	seen := map[string]bool{}
	for _, mod := range mods {
		if (mod != "ctrl" && mod != "alt" && mod != "shift") || seen[mod] {
			return false
		}
		seen[mod] = true
	}
	if namedKeys[base] {
		return true
	}
	printable := len(base) == 1 && (base[0] >= 'a' && base[0] <= 'z' || base[0] >= '0' && base[0] <= '9')
	return printable && (seen["ctrl"] || seen["alt"])
}

// Keys lowercases the names and refuses one the core's grammar has no word for.
func Keys(names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.ToLower(name)
		if !keyAllowed(name) {
			return nil, fmt.Errorf("key %q not allowed", name)
		}
		out = append(out, name)
	}
	return out, nil
}

// keysPerCall bounds one keys request: the core paces its keys (30 ms each) and does not stop a batch once sent, so a
// long one is several requests, each within a call's time, and a failure ends the rest.
const keysPerCall = 32

// SendKeys presses named keys in a pane, in batches of keysPerCall; the names went through Keys.
func (in *Input) SendKeys(client Prompter, ref events.PaneRef, keys []string) error {
	lock := in.lock(ref)
	lock.Lock()
	defer lock.Unlock()
	for len(keys) > 0 {
		n := min(len(keys), keysPerCall)
		if err := client.PaneSendKeys(ref.Pane, keys[:n]); err != nil {
			return err
		}
		keys = keys[n:]
	}
	return nil
}
