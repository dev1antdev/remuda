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

func TestRefusesBadKeysAndBigValues(t *testing.T) {
	st := New(t.TempDir())
	for _, key := range []string{"", "has space", "slash/key", strings.Repeat("k", 81), "quote\"s"} {
		if _, err := st.Put(map[string]Entry{key: {Value: json.RawMessage(`1`)}}); err == nil {
			t.Fatalf("key %q was accepted", key)
		}
	}
	if _, err := st.Put(map[string]Entry{"panel.sidebar": {Value: json.RawMessage(`268`)}, "prompt.history:local|/x": {Value: json.RawMessage(`[]`)}}); err == nil {
		t.Fatal("a key with | and / was accepted")
	}
	if _, err := st.Put(map[string]Entry{"panel.sidebar": {Value: json.RawMessage(`268`)}, "theme.dark": {Value: json.RawMessage(`"mocha"`)}}); err != nil {
		t.Fatalf("good keys refused: %v", err)
	}
	big := json.RawMessage(`"` + strings.Repeat("x", MaxValue) + `"`)
	if _, err := st.Put(map[string]Entry{"wall": {Value: big}}); err == nil {
		t.Fatal("a value over the cap was accepted")
	}
	if _, err := st.Put(map[string]Entry{"wall": {Value: json.RawMessage(`{not json`)}}); err == nil {
		t.Fatal("a value that is not JSON was accepted")
	}
	if len(st.All()) != 2 {
		t.Fatalf("a refused put changed the store: %+v", st.All())
	}
}
