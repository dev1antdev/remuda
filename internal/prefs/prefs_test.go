package prefs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestEmptyIsAnObjectAndPersists(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	if b, _ := json.Marshal(st.All()); string(b) != "{}" {
		t.Fatalf("got %s", b)
	}
	if _, err := st.Put(map[string]Entry{"theme": {Value: json.RawMessage(`"light"`), At: at("2026-10-25T10:00:00Z")}}); err != nil {
		t.Fatal(err)
	}
	again := New(dir).All()
	if string(again["theme"].Value) != `"light"` || !again["theme"].At.Equal(at("2026-10-25T10:00:00Z")) {
		t.Fatalf("reloaded %+v", again)
	}
}

func TestNewerStampWinsEqualTakesIncoming(t *testing.T) {
	st := New(t.TempDir())
	st.Put(map[string]Entry{"theme": {Value: json.RawMessage(`"light"`), At: at("2026-10-25T10:00:00Z")}})
	out, _ := st.Put(map[string]Entry{"theme": {Value: json.RawMessage(`"dark"`), At: at("2026-10-25T09:00:00Z")}})
	if string(out["theme"].Value) != `"light"` {
		t.Fatalf("an older stamp replaced the newer value: %s", out["theme"].Value)
	}
	out, _ = st.Put(map[string]Entry{"theme": {Value: json.RawMessage(`"dark"`), At: at("2026-10-25T10:00:00Z")}})
	if string(out["theme"].Value) != `"dark"` {
		t.Fatalf("an equal stamp did not take the incoming value: %s", out["theme"].Value)
	}
	out, _ = st.Put(map[string]Entry{"theme": {Value: json.RawMessage(`"system"`), At: at("2026-10-25T11:00:00Z")}, "wall": {Value: json.RawMessage(`{"size":"l"}`), At: at("2026-10-25T11:00:00Z")}})
	if string(out["theme"].Value) != `"system"` || string(out["wall"].Value) != `{"size":"l"}` || len(out) != 2 {
		t.Fatalf("merged %+v", out)
	}
}

func TestNullDeletesAndMissingStampIsNow(t *testing.T) {
	st := New(t.TempDir())
	st.now = func() time.Time { return at("2026-10-25T12:00:00Z") }
	out, _ := st.Put(map[string]Entry{"radar": {Value: json.RawMessage(`{"order":"recent"}`)}})
	if !out["radar"].At.Equal(at("2026-10-25T12:00:00Z")) {
		t.Fatalf("stamp %v", out["radar"].At)
	}
	out, _ = st.Put(map[string]Entry{"radar": {Value: json.RawMessage(`null`), At: at("2026-10-25T13:00:00Z")}})
	if _, ok := out["radar"]; ok {
		t.Fatalf("null did not delete: %+v", out)
	}
	st.Put(map[string]Entry{"radar": {Value: json.RawMessage(`1`), At: at("2026-10-25T14:00:00Z")}})
	out, _ = st.Put(map[string]Entry{"radar": {Value: json.RawMessage(`null`), At: at("2026-10-25T13:30:00Z")}})
	if string(out["radar"].Value) != `1` {
		t.Fatalf("an older null deleted a newer value: %+v", out)
	}
}

func TestLeavesOutBadKeysAndBigValuesAndKeepsTheRest(t *testing.T) {
	st := New(t.TempDir())
	// one key the server cannot take is left out, and the good keys of the batch go: a folder's bookmarks past the bound
	// must not stop every shared setting on every device from syncing
	out, err := st.Put(map[string]Entry{
		"":                                       {Value: json.RawMessage(`1`)},
		"tab\tkey":                               {Value: json.RawMessage(`1`)},
		"line\nbreak":                            {Value: json.RawMessage(`1`)},
		"bell\x07":                               {Value: json.RawMessage(`1`)},
		strings.Repeat("k", MaxKey+1):            {Value: json.RawMessage(`1`)},
		"panel.sidebar":                          {Value: json.RawMessage(`268`)},
		"pins:local|/Users/me/My Project (2026)": {Value: json.RawMessage(`[]`)},
		"wall":                                   {Value: json.RawMessage(`"` + strings.Repeat("x", MaxValue) + `"`)},
		"junk":                                   {Value: json.RawMessage(`{not json`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("the good keys did not go through: %v", out)
	}
	if _, ok := out["pins:local|/Users/me/My Project (2026)"]; !ok {
		t.Fatalf("a pins key with a folder was left out: %v", out)
	}
	if _, err := st.Put(map[string]Entry{"theme.dark": {Value: json.RawMessage(`"mocha"`)}}); err != nil {
		t.Fatalf("good keys refused: %v", err)
	}
	if len(st.All()) != 3 {
		t.Fatalf("the store: %+v", st.All())
	}
}
