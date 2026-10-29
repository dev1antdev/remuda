package herdr

import (
	"bytes"
	"testing"
)

func scanAll(chunks ...[]byte) (forward, replies []byte) {
	var q queryScanner
	for _, c := range chunks {
		f, r := q.scan(c)
		forward = append(forward, f...)
		replies = append(replies, r...)
	}
	return append(forward, q.flush()...), replies
}

func TestQueriesAreAnsweredOnceAndKeptFromTheViewers(t *testing.T) {
	stream := []byte("\x1b[?1016$p\x1b[chello \x1b[31mred\x1b[0m\x1b[6n\x1b[?2026$p\x1b[>c")
	forward, replies := scanAll(stream)
	if want := "hello \x1b[31mred\x1b[0m\x1b[6n"; string(forward) != want {
		t.Fatalf("forward = %q, want %q", forward, want)
	}
	if want := "\x1b[?1016;2$y\x1b[?1;2c\x1b[?2026;0$y\x1b[>0;276;0c"; string(replies) != want {
		t.Fatalf("replies = %q, want %q", replies, want)
	}
}

func TestAQueryCutByTheChunkBoundaryIsStillAnswered(t *testing.T) {
	stream := []byte("a\x1b[?1016$pb\x1b[cc")
	for cut := 1; cut < len(stream); cut++ {
		forward, replies := scanAll(stream[:cut], stream[cut:])
		if string(forward) != "abc" {
			t.Fatalf("cut at %d: forward = %q", cut, forward)
		}
		if string(replies) != "\x1b[?1016;2$y\x1b[?1;2c" {
			t.Fatalf("cut at %d: replies = %q", cut, replies)
		}
	}
}

func TestATailThatIsNoQueryIsForwardedWhole(t *testing.T) {
	forward, replies := scanAll([]byte("x\x1b"), []byte("[?25h"), []byte("\x1b[?"), []byte("12l\x1b[0"), []byte("m"))
	if want := "x\x1b[?25h\x1b[?12l\x1b[0m"; string(forward) != want {
		t.Fatalf("forward = %q, want %q", forward, want)
	}
	if len(replies) != 0 {
		t.Fatalf("replies = %q, want none", replies)
	}
	if f, _ := scanAll([]byte("ends with escape \x1b")); !bytes.HasSuffix(f, []byte{0x1b}) {
		t.Fatalf("the flushed tail should keep the escape: %q", f)
	}
}
