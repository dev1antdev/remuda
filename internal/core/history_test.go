package core

import "testing"

func TestTheHistoryOfAFirstPaintFitsTheCoresQueue(t *testing.T) {
	for _, grid := range [][2]int{{80, 24}, {120, 40}, {250, 70}, {1000, 300}} {
		cols, rows := grid[0], grid[1]
		history := historyFor(cols, rows)
		if history < 0 || history > attachHistory {
			t.Fatalf("%dx%d: history %d outside 0..%d", cols, rows, history, attachHistory)
		}
		if weight := (history + rows) * cols * cellBytes; history > 0 && weight > paintBudget { // a screen alone that large is the core's to send
			t.Fatalf("%dx%d: %d rows of history weigh %d bytes at worst, past %d", cols, rows, history, weight, paintBudget)
		}
	}
	if got := historyFor(80, 24); got < 300 {
		t.Fatalf("an 80x24 terminal gets %d rows, want a useful history", got)
	}
	if got := historyFor(0, 24); got != 0 {
		t.Fatalf("no grid: %d rows, want 0", got)
	}
}
