package sent

import (
	"strings"
	"testing"
	"time"
)

func TestTheScreenTerminalsTypingIsARowPerLine(t *testing.T) {
	var lines Lines
	now := time.Now()
	typed := func(device, text string, keys ...string) []Row {
		return lines.Typed(device, "win", "w1:p1", text, keys, now)
	}
	if rows := typed("Mac", "dotnet bui"); len(rows) != 0 {
		t.Fatalf("a keystroke is no row: %+v", rows)
	}
	if rows := typed("iPhone", "ls"); len(rows) != 0 {
		t.Fatalf("another device's line: %+v", rows)
	}
	if rows := typed("Mac", "lx", "backspace", "enter"); len(rows) != 1 || rows[0] != (Row{What: "dotnet buil"}) {
		t.Fatalf("Enter sends the line, a backspace takes a character off: %+v", rows)
	}
	if rows := typed("Mac", "", "down", "down", "enter"); len(rows) != 1 || rows[0] != (Row{What: "↓ ↓ Enter", Keys: true}) {
		t.Fatalf("Enter after keys alone names them: %+v", rows)
	}
	typed("Mac", "rm -rf", "ctrl+u")
	if rows := typed("Mac", "echo a", "enter"); len(rows) != 1 || rows[0].What != "echo a" {
		t.Fatalf("Ctrl+U clears the line: %+v", rows)
	}
	if rows := typed("iPhone", "", "ctrl+c"); len(rows) != 1 || rows[0] != (Row{What: "Ctrl+C", Keys: true}) {
		t.Fatalf("an interrupt is a row of its own: %+v", rows)
	}
	if rows := typed("iPhone", "", "enter"); len(rows) != 1 || rows[0].What != "Enter" {
		t.Fatalf("the interrupt let the line go: %+v", rows)
	}
	if rows := typed("Mac", "a", "enter", "enter"); len(rows) != 2 || rows[0].What != "a" || rows[1].What != "Enter" {
		t.Fatalf("two Enters are two rows: %+v", rows)
	}
	if len(lines.lines) != 0 {
		t.Fatalf("a sent line is let go: %d kept", len(lines.lines))
	}
	if rows := typed("Mac", strings.Repeat("я", 2000), "enter"); len(rows) != 1 || len([]rune(rows[0].What)) != lineRunes {
		t.Fatalf("a long line keeps its tail: %d runes", len([]rune(rows[0].What)))
	}
}

func TestLinesKeepABoundedNumber(t *testing.T) {
	var lines Lines
	start := time.Now()
	for i := 0; i < keptLines+10; i++ {
		lines.Typed("Mac", "local", string(rune('a'+i%26))+strings.Repeat("x", i/26), "x", nil, start.Add(time.Duration(i)*time.Second))
	}
	if len(lines.lines) != keptLines {
		t.Fatalf("%d lines kept", len(lines.lines))
	}
	if _, ok := lines.lines["Mac\x00local\x00a"]; ok {
		t.Fatal("the line typed into longest ago is the one that goes")
	}
}
