package handover

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestTextAtTheCapCutsAtARuneOfAnyWidth(t *testing.T) {
	turns := []transcript.Turn{
		{Role: "user", Parts: []transcript.Part{text("go")}},
		{Role: "assistant", Parts: []transcript.Part{text("done")}},
	}
	summary := recap.Of(turns, time.Time{})
	for _, wide := range []string{"Я", "测", "🚀", "👩🏽‍💻"} {
		from := strings.Repeat(wide, MaxText/len(wide)+7) // the heading alone runs past the cap, mid-rune at every width
		got := Text(from, summary, turns, 1)
		if len(got) > MaxText || !utf8.ValidString(got) || !strings.HasSuffix(got, "can carry)\n") {
			t.Errorf("%s: %d bytes, valid %v, cut %v", wide, len(got), utf8.ValidString(got), strings.HasSuffix(got, "can carry)\n"))
		}
	}
}
