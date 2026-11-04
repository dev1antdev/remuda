package limits

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/state"
)

// StatuslineTimeout is how long the person's own status line may take behind the wrapper.
var StatuslineTimeout = 10 * time.Second

// wrapFile keeps, in remuda's state dir, the statusLine setting the wrapper took the place of; backupFile the whole
// settings file as it was before the install.
const (
	wrapFile   = "statusline.json"
	backupFile = "claude-settings.backup.json"
)

type wrapped struct {
	Original json.RawMessage `json:"original"` // the statusLine object the person had, null when they had none
}

// RunStatusline is `remuda statusline` as Claude Code's status line: it keeps the rate limits Claude Code hands it in
// remuda's state dir (only when they moved), then runs the person's own status line on the same input and prints what
// it prints. Nothing it does can fail the status line: every trouble is swallowed.
func RunStatusline(stdin io.Reader, stdout io.Writer, stateDir string, now time.Time) {
	input, _ := io.ReadAll(io.LimitReader(stdin, 1<<20))
	keepSnapshot(stateDir, input, now)
	command := originalCommand(stateDir)
	if command == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), StatuslineTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	_ = cmd.Run()
}

// keepSnapshot writes the input's rate_limits to the state dir when they differ from the ones kept
func keepSnapshot(stateDir string, input []byte, now time.Time) {
	var status struct {
		RateLimits json.RawMessage `json:"rate_limits"`
	}
	if json.Unmarshal(input, &status) != nil || len(status.RateLimits) == 0 || string(status.RateLimits) == "null" {
		return
	}
	var kept Snapshot
	if data, err := os.ReadFile(filepath.Join(stateDir, SnapshotFile)); err == nil && json.Unmarshal(data, &kept) == nil {
		if compact(kept.RateLimits) == compact(status.RateLimits) && now.Sub(kept.At) < time.Minute {
			return
		}
	}
	_ = state.SaveJSON(stateDir, SnapshotFile, Snapshot{At: now.UTC(), RateLimits: status.RateLimits})
}

func compact(raw json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return string(raw)
	}
	return b.String()
}

// originalCommand is the command of the status line the wrapper stands in front of, "" when there was none
func originalCommand(stateDir string) string {
	var w wrapped
	if state.LoadJSON(stateDir, wrapFile, &w) != nil || len(w.Original) == 0 {
		return ""
	}
	var line struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(w.Original, &line) != nil {
		return ""
	}
	return strings.TrimSpace(line.Command)
}

// WrapperCommand is the status line command that runs this remuda first.
func WrapperCommand(self string) string { return script.Quote(self) + " statusline" }

// isWrapper says whether a statusLine setting is remuda's wrapper
func isWrapper(raw json.RawMessage) bool {
	var line struct {
		Command string `json:"command"`
	}
	return json.Unmarshal(raw, &line) == nil && strings.Contains(line.Command, "remuda") && strings.HasSuffix(strings.TrimSpace(line.Command), " statusline")
}

// SettingsPath is Claude Code's user settings file under home.
func SettingsPath(home string) string { return filepath.Join(home, ".claude", "settings.json") }

// InstallStatusline puts the wrapper in front of the status line in Claude Code's settings: the setting it replaces and
// the whole file as it was are kept in remuda's state dir first; every other setting stays as it is, in its order.
func InstallStatusline(home, stateDir, self string) (string, error) {
	path := SettingsPath(home)
	settings, mode, err := readSettings(path)
	if err != nil {
		return "", err
	}
	current, _ := settings.get("statusLine")
	if isWrapper(current) {
		return "Claude Code's status line already goes through remuda.", nil
	}
	if raw, err := os.ReadFile(path); err == nil {
		if err := writeFile(filepath.Join(stateDir, backupFile), raw, 0o600); err != nil {
			return "", fmt.Errorf("the settings could not be backed up: %w", err)
		}
	}
	original := json.RawMessage("null")
	if len(current) > 0 {
		original = current
	}
	if err := state.SaveJSON(stateDir, wrapFile, wrapped{Original: original}); err != nil {
		return "", err
	}
	line := map[string]any{"type": "command", "command": WrapperCommand(self), "padding": 0}
	if len(current) > 0 {
		var kept map[string]any
		if json.Unmarshal(current, &kept) == nil && kept != nil {
			kept["command"] = WrapperCommand(self)
			line = kept
		}
	}
	value, err := json.Marshal(line)
	if err != nil {
		return "", err
	}
	settings.set("statusLine", value)
	if err := writeSettings(path, settings, mode); err != nil {
		return "", err
	}
	if command := originalCommand(stateDir); command != "" {
		return "Claude Code's status line now goes through remuda first, which keeps its rate limits, then runs " + command + " as before.", nil
	}
	return "Claude Code's status line now goes through remuda, which keeps its rate limits; there was no status line of your own, so it prints nothing.", nil
}

// UninstallStatusline puts the person's own status line back, or none when they had none; a status line changed since
// the install is left alone.
func UninstallStatusline(home, stateDir string) (string, error) {
	path := SettingsPath(home)
	settings, mode, err := readSettings(path)
	if err != nil {
		return "", err
	}
	current, has := settings.get("statusLine")
	if !has || !isWrapper(current) {
		return "Claude Code's status line does not go through remuda; nothing changed.", nil
	}
	var w wrapped
	_ = state.LoadJSON(stateDir, wrapFile, &w)
	if len(w.Original) == 0 || string(w.Original) == "null" {
		settings.remove("statusLine")
	} else {
		settings.set("statusLine", w.Original)
	}
	if err := writeSettings(path, settings, mode); err != nil {
		return "", err
	}
	_ = os.Remove(filepath.Join(stateDir, wrapFile))
	return "Claude Code's status line is yours again, as it was before the install.", nil
}

// StatuslineStatus says whether the wrapper is installed, what it runs after, and when the limits were last kept.
func StatuslineStatus(home, stateDir string) (string, error) {
	settings, _, err := readSettings(SettingsPath(home))
	if err != nil {
		return "", err
	}
	current, _ := settings.get("statusLine")
	if !isWrapper(current) {
		return "not installed: Claude Code's status line does not go through remuda", nil
	}
	words := "installed"
	if command := originalCommand(stateDir); command != "" {
		words += ", then runs " + command
	}
	var snap Snapshot
	if data, err := os.ReadFile(filepath.Join(stateDir, SnapshotFile)); err == nil && json.Unmarshal(data, &snap) == nil && !snap.At.IsZero() {
		words += "; limits last kept " + snap.At.Local().Format("2006-01-02 15:04")
	} else {
		words += "; no limits kept yet (they come after a session's first answer, on a Pro, Max or Ultra plan)"
	}
	return words, nil
}

// object is a JSON object whose keys keep their order, each value as it was written
type object []field

type field struct {
	key   string
	value json.RawMessage
}

func (o object) get(key string) (json.RawMessage, bool) {
	for _, f := range o {
		if f.key == key {
			return f.value, true
		}
	}
	return nil, false
}

func (o *object) set(key string, value json.RawMessage) {
	for i := range *o {
		if (*o)[i].key == key {
			(*o)[i].value = value
			return
		}
	}
	*o = append(*o, field{key, value})
}

func (o *object) remove(key string) {
	for i := range *o {
		if (*o)[i].key == key {
			*o = append((*o)[:i], (*o)[i+1:]...)
			return
		}
	}
}

// readSettings reads the settings file's top level in order; a missing file is an empty object
func readSettings(path string) (object, os.FileMode, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return object{}, 0o644, nil
	}
	if err != nil {
		return nil, 0, err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return object{}, mode, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, 0, fmt.Errorf("%s is not a JSON object; nothing changed", path)
	}
	var out object
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, 0, fmt.Errorf("%s does not read as JSON (%v); nothing changed", path, err)
		}
		key, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, 0, fmt.Errorf("%s does not read as JSON (%v); nothing changed", path, err)
		}
		out = append(out, field{key, value})
	}
	return out, mode, nil
}

// writeSettings writes the object back, two-space indented as Claude Code writes it, in one rename
func writeSettings(path string, o object, mode os.FileMode) error {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(f.key)
		b.Write(key)
		b.WriteByte(':')
		b.Write(f.value)
	}
	b.WriteByte('}')
	var out bytes.Buffer
	if err := json.Indent(&out, b.Bytes(), "", "  "); err != nil {
		return err
	}
	out.WriteByte('\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFile(path, out.Bytes(), mode)
}

// writeFile replaces a file in one rename, with the mode given
func writeFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
