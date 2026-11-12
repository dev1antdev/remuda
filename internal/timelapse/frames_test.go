package timelapse

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

// heldBytes is what the frames' rows really keep alive: every row's bytes once per copy, however many frames share it
func heldBytes(frames []frame) int {
	copies := map[*byte]int{}
	for _, f := range frames {
		for _, row := range f.rows {
			if row != "" {
				copies[unsafe.StringData(row)] = len(row)
			}
		}
	}
	total := 0
	for _, n := range copies {
		total += n
	}
	return total
}

// scrolled is a screen of 60 rows of 600 bytes each, scrolled n rows: every row moves on every frame
func scrolled(n int) string {
	rows := make([]string, 60)
	for i := range rows {
		rows[i] = fmt.Sprintf("\x1b[38;2;1;2;3m%06d %s", n+i, strings.Repeat("x", 580))
	}
	return strings.Join(rows, "\n")
}

func TestAScreenThatScrollsHoldsNoMoreThanMaxHeldInMemory(t *testing.T) {
	tk := &take{every: Interval}
	for i := 0; i < MaxFrames; i++ {
		tk.add(t0.Add(time.Duration(i)*Interval), scrolled(i))
	}
	if held := heldBytes(tk.frames); held > MaxHeld+len(scrolled(0)) {
		t.Fatalf("%d frames hold %d MB", len(tk.frames), held>>20)
	}
	if tk.every <= Interval || len(tk.frames) >= MaxFrames {
		t.Fatalf("%d frames at %v: never thinned", len(tk.frames), tk.every)
	}
	if tk.held != heldBytes(tk.frames) {
		t.Fatalf("counted %d, held %d", tk.held, heldBytes(tk.frames))
	}
}

func TestAScreenFlippingBetweenTwoLooksKeepsALookOnceAThinningBroughtItsCopiesTogether(t *testing.T) {
	looks := [2]string{scrolled(0), scrolled(1000)}
	tk := &take{every: Interval}
	for i := 0; i < 3*MaxFrames; i++ {
		tk.add(t0.Add(time.Duration(i)*Interval), looks[i%2])
		if held := heldBytes(tk.frames); held != tk.held || held > MaxHeld+len(looks[0]) {
			t.Fatalf("frame %d: %d frames keep %d KB, counted as %d KB", i, len(tk.frames), held>>10, tk.held>>10)
		}
	}
}

func TestThinningNeverTouchesTheFramesACheckpointIsWriting(t *testing.T) {
	tk := &take{every: Interval}
	for i := 0; i < MaxFrames; i++ {
		tk.add(t0.Add(time.Duration(i)*Interval), fmt.Sprintf("header\nframe %d\nfooter %d", i, i%2))
	}
	writing := tk.frames
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		encode(writing, t0)
	}()
	tk.add(t0.Add(time.Hour), "header\nthe last\nfooter 0")
	wg.Wait()
	if len(tk.frames) > MaxFrames/2+2 {
		t.Fatalf("%d frames: not thinned", len(tk.frames))
	}
}
