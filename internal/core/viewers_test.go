package core_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// watched is what a viewer was handed: the program's text, and the grids it was told.
type watched struct {
	viewer herdr.Viewer
	text   strings.Builder
	grids  []string
}

// until takes what the viewer is handed until it holds want, as text or as a grid "<cols>x<rows>".
func (w *watched) until(t *testing.T, want string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !strings.Contains(w.text.String(), want) && !slices.Contains(w.grids, want) {
		select {
		case out, ok := <-w.viewer.Out():
			if !ok {
				t.Fatalf("the viewer ended before %q; it had %q and the grids %v", want, w.text.String(), w.grids)
			}
			if out.Grid() {
				w.grids = append(w.grids, fmt.Sprintf("%dx%d", out.Cols, out.Rows))
			} else {
				w.text.Write(out.Data)
			}
		case <-deadline:
			t.Fatalf("no %q within 10 s; the viewer had %q and the grids %v", want, w.text.String(), w.grids)
		}
	}
}

func TestTwoViewersShareATerminalWithoutTakingItsSize(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	cat, err := client.Run(ctx, core.RunParams{Argv: []string{"cat"}, Size: &core.Size{Cols: 100, Rows: 30}})
	if err != nil {
		t.Fatal(err)
	}
	terminals := core.NewTerminals(client)
	deskViewer, err := terminals.Open(cat.Address, herdr.View{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	defer deskViewer.Close()
	phoneViewer, err := terminals.Open(cat.Address, herdr.View{Cols: 40, Rows: 10, Keep: true})
	if err != nil {
		t.Fatal(err)
	}
	desk, phone := &watched{viewer: deskViewer}, &watched{viewer: phoneViewer}
	if n := terminals.Viewers(cat.Address); n != 2 {
		t.Fatalf("viewers = %d, want 2", n)
	}
	phone.until(t, "100x30") // a viewer that keeps the size is told the terminal's grid before its paint
	// a program asking for another grid in its own output is only text: never a grid a viewer follows
	if err := phoneViewer.Write([]byte("\x1b[8;5;20t from the phone\r")); err != nil {
		t.Fatal(err)
	}
	desk.until(t, "from the phone")
	phone.until(t, "from the phone")
	if err := phoneViewer.Resize(40, 10); err != nil {
		t.Fatal(err)
	}
	if got, _ := client.Get(ctx, cat.Address); got.Cols != 100 || got.Rows != 30 {
		t.Fatalf("the phone that keeps the size made it %dx%d", got.Cols, got.Rows)
	}
	if err := deskViewer.Resize(90, 28); err != nil {
		t.Fatal(err)
	}
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		if got, _ := client.Get(ctx, cat.Address); got.Cols == 90 && got.Rows == 28 {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("the desk's resize never reached the terminal")
		}
	}
	phone.until(t, "90x28")
	if !slices.Equal(phone.grids, []string{"100x30", "90x28"}) {
		t.Fatalf("the phone was told the grids %v, want the terminal's own alone", phone.grids)
	}
	phoneViewer.Close()
	if n := terminals.Viewers(cat.Address); n != 1 {
		t.Fatalf("viewers after the phone left = %d, want 1", n)
	}
}

func TestAViewerThatOnlyWatchesCannotType(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	cat, err := client.Run(ctx, core.RunParams{Argv: []string{"cat"}})
	if err != nil {
		t.Fatal(err)
	}
	terminals := core.NewTerminals(client)
	linkViewer, err := terminals.Open(cat.Address, herdr.View{Cols: 80, Rows: 24, Observe: true})
	if err != nil {
		t.Fatal(err)
	}
	defer linkViewer.Close()
	link := &watched{viewer: linkViewer}
	_ = linkViewer.Write([]byte("typed by a link\r"))
	desk, err := terminals.Open(cat.Address, herdr.View{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer desk.Close()
	if n := terminals.Viewers(cat.Address); n != 1 {
		t.Fatalf("viewers = %d: a link that only watches is not the person looking", n)
	}
	if err := desk.Write([]byte("typed at the desk\r")); err != nil {
		t.Fatal(err)
	}
	link.until(t, "typed at the desk")
	if got, _ := client.Read(ctx, cat.Address, 0, ""); strings.Contains(got.Text, "typed by a link") {
		t.Fatalf("a viewer that only watches typed: %q", got.Text)
	}
}

func TestAViewerEndsWithItsProgram(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	brief, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", `read line; echo "bye:$line"`}})
	if err != nil {
		t.Fatal(err)
	}
	terminals := core.NewTerminals(client)
	viewer, err := terminals.Open(brief.Address, herdr.View{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	if err := viewer.Write([]byte("now\r")); err != nil {
		t.Fatal(err)
	}
	(&watched{viewer: viewer}).until(t, "bye:now")
	for deadline := time.After(10 * time.Second); ; {
		select {
		case _, ok := <-viewer.Out():
			if !ok {
				if n := terminals.Viewers(brief.Address); n != 0 {
					t.Fatalf("viewers after the program ended = %d", n)
				}
				return
			}
		case <-deadline:
			t.Fatal("the viewer's bytes never closed after the program ended")
		}
	}
}

// richRows prints 4000 rows whose every cell carries every attribute and three true colours, then END-OF-LOG: past
// 1000 such rows a first paint passes the core's 4 MiB queue, which drops it whole
const richRows = `awk 'BEGIN{for(r=0;r<4000;r++){for(c=0;c<80;c++)printf "\033[0;1;2;3;4;5;7;9;53;38;2;%d;%d;%d;48;2;%d;%d;%d;58;2;%d;%d;%dmx",c%256,r%256,(c*r)%256,255-c%256,255-r%256,(c+r)%256,(c*7)%256,(r*3)%256,(c+r*5)%256; printf "\033[0m\r\n"} print "END-OF-LOG"}'`

// paintedOver runs script in an 80x24 terminal, waits for shown on its screen, then opens a viewer and waits for want
// among what it is handed.
func paintedOver(t *testing.T, script, shown, want string) {
	t.Helper()
	ctx := within(t, 90*time.Second)
	client := coretest.Server(t).Client(t)
	term, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", script}, Size: &core.Size{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatal(err)
	}
	for start := time.Now(); ; time.Sleep(100 * time.Millisecond) {
		if read, _ := client.Read(ctx, term.Address, 0, ""); strings.Contains(read.Text, shown) {
			break
		}
		if time.Since(start) > 60*time.Second {
			t.Fatalf("%q never reached the screen", shown)
		}
	}
	viewer, err := core.NewTerminals(client).Open(term.Address, herdr.View{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	(&watched{viewer: viewer}).until(t, want)
}

func TestAViewerIsPaintedTheRowsAboveTheScreen(t *testing.T) {
	// far above a 24-row screen: only the history carries row-1
	paintedOver(t, "for i in $(seq 1 200); do echo row-$i; done; cat", "row-200", "row-1\r\n")
}

func TestADenselyColouredLogStillPaintsTheScreen(t *testing.T) {
	paintedOver(t, richRows+"; cat", "END-OF-LOG", "END-OF-LOG")
}

func TestAProgramPrintingOverADroppedPaintStillShowsTheScreen(t *testing.T) {
	// a line rewritten every tenth of a second under END-OF-LOG: its output reaches a viewer whose paint the core
	// dropped, and must not pass for that paint
	paintedOver(t, richRows+"; i=0; while :; do i=$((i+1)); printf '\rtick %d' $i; sleep 0.1; done", "tick", "END-OF-LOG")
}
