package timelapse

import (
	"testing"
	"time"
)

func TestPlainTakesTheEscapesOutAsThePlayerDraws(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"colours", "\x1b[31mmigration\x1b[0m \x1b[1;38;2;215;119;87mfailed\x1b[m", "migration failed"},
		{"cursor moves and erases", "\x1b[2K\x1b[1;5Hstep\x1b[?25l 3", "step 3"},
		{"an OSC title to BEL", "\x1b]0;claude: working\x07done", "done"},
		{"an OSC title to ESC \\", "\x1b]2;title\x1b\\ok", "ok"},
		{"an OSC never closed", "before\x1b]0;runs to the end", "before"},
		{"a charset designation and its byte", "\x1b(Bline\x1b)0 two", "line two"},
		{"a two-byte escape", "a\x1b7b\x1b8c", "abc"},
		{"control characters but a tab", "a\rb\x07c\td\x00e", "abc\tde"},
		{"a CSI whose final is a letter beyond ASCII", "\x1b[1éafter", "after"},
		{"an escape at the very end", "tail\x1b", "tail"},
		{"text in any alphabet", "\x1b[32m⏺\x1b[0m Миграция упала", "⏺ Миграция упала"},
		{"nothing to take out", "plain text", "plain text"},
	} {
		if got := Plain(c.in); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestARecordingReadsAsPlainRowsTrimmedAndFolded(t *testing.T) {
	rec := screensAt("local", "w1:p1", 0, "\x1b[1mBuild\x1b[0m OK   \n", "\x1b[1mBuild\x1b[0m OK   \nMigration FAILED")
	plain := plainOf(rec)
	if len(plain.frames) != 2 || plain.duration != 2*time.Minute.Milliseconds() {
		t.Fatalf("%+v", plain)
	}
	first := plain.frames[0].rows[0]
	if first.text != "Build OK" || first.fold != "build ok" {
		t.Fatalf("first row %+v", first)
	}
	if moved := plain.frames[1].rows; len(moved) != 1 || moved[0].line != 1 || moved[0].fold != "migration failed" {
		t.Fatalf("the second frame's rows: %+v", moved)
	}
}

func TestTheCacheMakesRoomOnlyOutOfWhatEarlierQueriesUsed(t *testing.T) {
	cache := newPlainCache(250)
	v := version{size: 1}
	recording := func(n int) *plainRecording { return &plainRecording{bytes: n} }
	one := cache.begin()
	cache.put("a", v, recording(100), one)
	cache.put("b", v, recording(100), one)
	cache.put("c", v, recording(100), one) // the query's own fill the cache: c is not kept, a and b stay for the next
	if cache.get("a", v) == nil || cache.get("b", v) == nil || cache.get("c", v) != nil || cache.bytes != 200 {
		t.Fatalf("within one query: %d bytes", cache.bytes)
	}
	two := cache.begin()
	cache.get("b", v)                      // read again by this query
	cache.put("c", v, recording(100), two) // a, used by the first query alone, makes room
	if cache.get("a", v) != nil || cache.get("b", v) == nil || cache.get("c", v) == nil || cache.bytes != 200 {
		t.Fatalf("the next query: %d bytes", cache.bytes)
	}
	if cache.get("b", version{size: 2}) != nil {
		t.Fatal("another version of the file read as the one cached")
	}
	cache.put("huge", v, recording(300), cache.begin())
	if cache.get("huge", v) != nil || cache.bytes != 200 {
		t.Fatalf("one larger than the cache was kept, or emptied it: %d bytes", cache.bytes)
	}
	cache.put("b", version{size: 2}, recording(50), cache.begin()) // a new version replaces the old one
	if cache.get("b", v) != nil || cache.get("b", version{size: 2}) == nil || cache.bytes != 150 {
		t.Fatalf("b's new version: %d bytes", cache.bytes)
	}
	cache.drop("b")
	if cache.get("b", version{size: 2}) != nil || cache.bytes != 100 {
		t.Fatalf("a dropped recording is still held: %d bytes", cache.bytes)
	}
	cache.clear()
	if cache.bytes != 0 || len(cache.held) != 0 {
		t.Fatalf("cleared: %d bytes, %d held", cache.bytes, len(cache.held))
	}
}
