package sessions

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/events"
)

// The keys people press in a pane and the screen terminal's typing: herdr's key-combo grammar as remuda allows it, and
// the few keys herdr has no word for sent as the bytes a terminal sends for them.

// namedKeys are the keys herdr's grammar names, which take a modifier.
var namedKeys = map[string]bool{
	"enter": true, "esc": true, "tab": true, "space": true, "backspace": true,
	"up": true, "down": true, "left": true, "right": true,
}

// terminalKeys are the keys herdr 0.9.5 refuses as invalid_key, with the sequence a terminal sends for each; they reach
// the pane as text, unmodified.
var terminalKeys = map[string]string{
	"home": "\x1bOH", "end": "\x1bOF", "pageup": "\x1b[5~", "pagedown": "\x1b[6~", "delete": "\x1b[3~",
}

// keyAllowed is one key in herdr's grammar: optional ctrl, alt and shift, each once, before a named key, or ctrl or alt
// before a letter or digit (a printable key alone is text, not a key); a terminal key stands alone.
func keyAllowed(name string) bool {
	if _, ok := terminalKeys[name]; ok {
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

// Keys lowercases the names and refuses one herdr's grammar has no word for.
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

// keyRun is a stretch of keys sent in one call: herdr's own names, or the bytes of one key it has no word for.
type keyRun struct {
	named []string
	bytes string
}

// keyRuns cuts the keys, in order, into runs of herdr's own names and the terminal keys between them.
func keyRuns(keys []string) []keyRun {
	var runs []keyRun
	for _, key := range keys {
		if bytes, ok := terminalKeys[key]; ok {
			runs = append(runs, keyRun{bytes: bytes})
			continue
		}
		if n := len(runs); n > 0 && runs[n-1].bytes == "" {
			runs[n-1].named = append(runs[n-1].named, key)
			continue
		}
		runs = append(runs, keyRun{named: []string{key}})
	}
	return runs
}

// SendKeys presses named keys in a pane; the names went through Keys.
func (in *Input) SendKeys(client Prompter, ref events.PaneRef, keys []string) error {
	lock := in.lock(ref)
	lock.Lock()
	defer lock.Unlock()
	for _, run := range keyRuns(keys) {
		var err error
		if run.bytes != "" {
			err = client.PaneSendText(ref.Pane, run.bytes)
		} else {
			err = client.PaneSendKeys(ref.Pane, run.named)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// MaxTypedText bounds one request's text: a paste of a page or two.
const MaxTypedText = 64 << 10

// Typed is one request of the screen terminal: text, then keys. A paste's text reaches the pane as a paste (herdr
// brackets it while the app asked for bracketed paste); typed text goes as the keystrokes it was.
type Typed struct {
	Text  string
	Paste bool
	Keys  []string
}

// NewTyped checks a request of the screen terminal: some text or keys, the text within MaxTypedText, the keys in the grammar.
func NewTyped(text string, paste bool, keys []string) (Typed, error) {
	if text == "" && len(keys) == 0 {
		return Typed{}, errors.New("text or keys required")
	}
	if len(text) > MaxTypedText {
		return Typed{}, fmt.Errorf("at most %d KB of text at once", MaxTypedText>>10)
	}
	named, err := Keys(keys)
	if err != nil {
		return Typed{}, err
	}
	return Typed{Text: text, Paste: paste, Keys: named}, nil
}

// Typist is what the screen terminal's typing needs of herdr's client.
type Typist interface {
	PaneSendText(paneID, text string) error
	PaneSendInput(paneID, text string, keys []string) error
}

// SendInput types into a pane with no attach (a Windows machine's), under the pane's lock: typed text as raw keystrokes
// (pane.send_text), a paste through pane.send_input with the keys that follow it, then the keys, those herdr has no
// word for as their bytes.
func (in *Input) SendInput(client Typist, ref events.PaneRef, typed Typed) error {
	lock := in.lock(ref)
	lock.Lock()
	defer lock.Unlock()
	paste := ""
	if typed.Paste {
		paste = typed.Text
	} else if typed.Text != "" {
		if err := client.PaneSendText(ref.Pane, typed.Text); err != nil {
			return err
		}
	}
	for _, run := range keyRuns(typed.Keys) {
		if run.bytes == "" {
			if err := client.PaneSendInput(ref.Pane, paste, run.named); err != nil {
				return err
			}
			paste = ""
			continue
		}
		if paste != "" {
			if err := client.PaneSendInput(ref.Pane, paste, nil); err != nil {
				return err
			}
			paste = ""
		}
		if err := client.PaneSendText(ref.Pane, run.bytes); err != nil {
			return err
		}
	}
	if paste != "" {
		return client.PaneSendInput(ref.Pane, paste, nil)
	}
	return nil
}
