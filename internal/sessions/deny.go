package sessions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Claude Code 2.1.319 and later read permissions.denyInsteadOfPrompting from --settings (a file or a JSON string, merged
// over the person's own settings, true in any source winning): a tool call that would wait on a permission prompt is
// denied at once and the turn carries on. remuda passes it as a JSON string, so nothing is written on the machine.

const settingsFlag = "--settings"

// denySettings is the --settings value remuda adds when the person passes none of their own.
const denySettings = `{"permissions":{"denyInsteadOfPrompting":true}}`

// CanDenyPrompts says whether remuda knows how to start the kind so it denies what would wait on a permission prompt.
func CanDenyPrompts(kind string) bool { return kind == "claude" }

// withDenySetting adds the setting to the person's own arguments: into each --settings they pass as JSON, else as a
// --settings of its own before them. A --settings naming a file is refused, since remuda adds to no file of theirs;
// nothing after a "--" is read as a flag.
func withDenySetting(args []string) ([]string, error) {
	out := append([]string(nil), args...)
	found := false
	for i := 0; i < len(out); i++ {
		word := out[i]
		if word == "--" {
			break
		}
		value, inline := strings.CutPrefix(word, settingsFlag+"=")
		if !inline {
			if word != settingsFlag {
				continue
			}
			if i+1 >= len(out) {
				return nil, errors.New("--settings needs a value")
			}
			i++
			value = out[i]
		}
		merged, err := mergeDenySetting(value)
		if err != nil {
			return nil, err
		}
		if inline {
			out[i] = settingsFlag + "=" + merged
		} else {
			out[i] = merged
		}
		found = true
	}
	if found {
		return out, nil
	}
	return append([]string{settingsFlag, denySettings}, out...), nil
}

// mergeDenySetting is a --settings value the person wrote with permissions.denyInsteadOfPrompting set to true in it.
func mergeDenySetting(value string) (string, error) {
	if !strings.HasPrefix(strings.TrimSpace(value), "{") {
		return "", fmt.Errorf(`the extra arguments pass --settings %s, a file remuda does not add to: put "permissions": {"denyInsteadOfPrompting": true} in it and switch "Don't wait for permissions" off, or pass the settings as JSON`, value)
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var settings map[string]any
	if err := decoder.Decode(&settings); err != nil || decoder.More() {
		return "", errors.New("the --settings in the extra arguments is not a JSON object Claude Code can read")
	}
	permissions := map[string]any{}
	switch p := settings["permissions"].(type) {
	case nil:
	case map[string]any:
		permissions = p
	default:
		return "", errors.New(`the --settings in the extra arguments has "permissions" that is not an object`)
	}
	permissions["denyInsteadOfPrompting"] = true
	settings["permissions"] = permissions
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(settings); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}
